package service

import (
	"fmt"
	"math"
	"strings"
)

const (
	AccountMetricDefaultCooldownSeconds = 10 * 60
	AccountMetricDefaultRecoverySamples = 3
	AccountMetricDefaultMinSamples      = 10
	AccountMetricMaxCooldownSeconds     = 24 * 60 * 60
	AccountMetricMaxRecoverySamples     = 30
	AccountMetricMaxMinSamples          = 5000
	AccountMetricMaxLatencyMs           = 60 * 60 * 1000
)

// AccountMetricThreshold configures a pause bound and an optional recovery
// bound. Recovery defaults to the pause bound when omitted.
type AccountMetricThreshold struct {
	PauseThreshold    float64  `json:"pause_threshold"`
	RecoveryThreshold *float64 `json:"recovery_threshold,omitempty"`
	MinSamples        int      `json:"min_samples,omitempty"`
}

// AccountMetricRecoveryConfig is opt-in for each account. The sampling window
// is fixed to one rolling hour; trial limits are shared and persisted by the
// repository.
type AccountMetricRecoveryConfig struct {
	Enabled         bool                    `json:"enabled"`
	CooldownSeconds int                     `json:"cooldown_seconds,omitempty"`
	RecoverySamples int                     `json:"recovery_samples,omitempty"`
	SuccessRate     *AccountMetricThreshold `json:"success_rate,omitempty"`
	CacheRate       *AccountMetricThreshold `json:"cache_rate,omitempty"`
	TTFTMs          *AccountMetricThreshold `json:"ttft_ms,omitempty"`
	DurationMs      *AccountMetricThreshold `json:"duration_ms,omitempty"`
}

type AccountMetricRuleEvidence struct {
	Metric         string  `json:"metric"`
	Value          float64 `json:"value"`
	Threshold      float64 `json:"threshold"`
	Samples        int64   `json:"samples"`
	Required       int64   `json:"required_samples"`
	Breached       bool    `json:"breached"`
	RecoveryPassed bool    `json:"recovery_passed"`
}

type AccountMetricEvaluation struct {
	Triggered bool                        `json:"triggered"`
	Evidence  []AccountMetricRuleEvidence `json:"evidence"`
}

func NormalizeAccountMetricRecoveryConfig(config AccountMetricRecoveryConfig) (AccountMetricRecoveryConfig, error) {
	if config.CooldownSeconds == 0 {
		config.CooldownSeconds = AccountMetricDefaultCooldownSeconds
	}
	if config.CooldownSeconds < 30 || config.CooldownSeconds > AccountMetricMaxCooldownSeconds {
		return config, fmt.Errorf("%w: cooldown_seconds must be between 30 and %d", ErrInvalidAccountMetricConfig, AccountMetricMaxCooldownSeconds)
	}
	if config.RecoverySamples == 0 {
		config.RecoverySamples = AccountMetricDefaultRecoverySamples
	}
	if config.RecoverySamples < 1 || config.RecoverySamples > AccountMetricMaxRecoverySamples {
		return config, fmt.Errorf("%w: recovery_samples must be between 1 and %d", ErrInvalidAccountMetricConfig, AccountMetricMaxRecoverySamples)
	}

	rules := []struct {
		name       string
		rule       **AccountMetricThreshold
		min, max   float64
		lowerBound bool
	}{
		{name: "success_rate", rule: &config.SuccessRate, min: 0, max: 1, lowerBound: true},
		{name: "cache_rate", rule: &config.CacheRate, min: 0, max: 1, lowerBound: true},
		{name: "ttft_ms", rule: &config.TTFTMs, min: 1, max: AccountMetricMaxLatencyMs},
		{name: "duration_ms", rule: &config.DurationMs, min: 1, max: AccountMetricMaxLatencyMs},
	}
	configured := 0
	for _, entry := range rules {
		if *entry.rule == nil {
			continue
		}
		configured++
		rule := **entry.rule
		if !finite(rule.PauseThreshold) || rule.PauseThreshold < entry.min || rule.PauseThreshold > entry.max {
			return config, fmt.Errorf("%w: %s pause_threshold must be finite and between %g and %g", ErrInvalidAccountMetricConfig, entry.name, entry.min, entry.max)
		}
		if rule.MinSamples == 0 {
			rule.MinSamples = AccountMetricDefaultMinSamples
		}
		if rule.MinSamples < 1 || rule.MinSamples > AccountMetricMaxMinSamples {
			return config, fmt.Errorf("%w: %s min_samples must be between 1 and %d", ErrInvalidAccountMetricConfig, entry.name, AccountMetricMaxMinSamples)
		}
		if rule.RecoveryThreshold == nil {
			recovery := rule.PauseThreshold
			rule.RecoveryThreshold = &recovery
		}
		recovery := *rule.RecoveryThreshold
		if !finite(recovery) || recovery < entry.min || recovery > entry.max {
			return config, fmt.Errorf("%w: %s recovery_threshold must be finite and between %g and %g", ErrInvalidAccountMetricConfig, entry.name, entry.min, entry.max)
		}
		if entry.lowerBound && recovery < rule.PauseThreshold {
			return config, fmt.Errorf("%w: %s recovery_threshold must be greater than or equal to pause_threshold", ErrInvalidAccountMetricConfig, entry.name)
		}
		if !entry.lowerBound && recovery > rule.PauseThreshold {
			return config, fmt.Errorf("%w: %s recovery_threshold must be less than or equal to pause_threshold", ErrInvalidAccountMetricConfig, entry.name)
		}
		**entry.rule = rule
	}
	if config.Enabled && configured == 0 {
		return config, fmt.Errorf("%w: at least one metric threshold is required when enabled", ErrInvalidAccountMetricConfig)
	}
	return config, nil
}

func EvaluateAccountMetricPause(config AccountMetricRecoveryConfig, snapshot *AccountMetricSnapshot) (*AccountMetricEvaluation, error) {
	config, err := NormalizeAccountMetricRecoveryConfig(config)
	if err != nil {
		return nil, err
	}
	evaluation := &AccountMetricEvaluation{Evidence: make([]AccountMetricRuleEvidence, 0, 4)}
	if !config.Enabled || snapshot == nil {
		return evaluation, nil
	}
	for _, measurement := range accountMetricMeasurements(config, snapshot) {
		if measurement.value == nil || !finite(*measurement.value) || measurement.samples < int64(measurement.rule.MinSamples) {
			continue
		}
		breached := measurement.lowerBound && *measurement.value < measurement.rule.PauseThreshold
		if !measurement.lowerBound {
			breached = *measurement.value > measurement.rule.PauseThreshold
		}
		evaluation.Evidence = append(evaluation.Evidence, AccountMetricRuleEvidence{
			Metric:    measurement.name,
			Value:     *measurement.value,
			Threshold: measurement.rule.PauseThreshold,
			Samples:   measurement.samples,
			Required:  int64(measurement.rule.MinSamples),
			Breached:  breached,
		})
		evaluation.Triggered = evaluation.Triggered || breached
	}
	return evaluation, nil
}

// EvaluateAccountMetricTrialFailure applies pause bounds to the smaller,
// explicitly bounded recovery trial sample set.
func EvaluateAccountMetricTrialFailure(config AccountMetricRecoveryConfig, snapshot *AccountMetricSnapshot) (*AccountMetricEvaluation, error) {
	config, err := NormalizeAccountMetricRecoveryConfig(config)
	if err != nil {
		return nil, err
	}
	evaluation := &AccountMetricEvaluation{Evidence: make([]AccountMetricRuleEvidence, 0, 4)}
	if !config.Enabled || snapshot == nil {
		return evaluation, nil
	}
	required := int64(config.RecoverySamples)
	for _, measurement := range accountMetricMeasurements(config, snapshot) {
		if measurement.value == nil || !finite(*measurement.value) || measurement.samples < required {
			continue
		}
		breached := measurement.lowerBound && *measurement.value < measurement.rule.PauseThreshold
		if !measurement.lowerBound {
			breached = *measurement.value > measurement.rule.PauseThreshold
		}
		evaluation.Evidence = append(evaluation.Evidence, AccountMetricRuleEvidence{
			Metric:    measurement.name,
			Value:     *measurement.value,
			Threshold: measurement.rule.PauseThreshold,
			Samples:   measurement.samples,
			Required:  required,
			Breached:  breached,
		})
		evaluation.Triggered = evaluation.Triggered || breached
	}
	return evaluation, nil
}

func EvaluateAccountMetricRecovery(config AccountMetricRecoveryConfig, snapshot *AccountMetricSnapshot) (*AccountMetricEvaluation, error) {
	config, err := NormalizeAccountMetricRecoveryConfig(config)
	if err != nil {
		return nil, err
	}
	evaluation := &AccountMetricEvaluation{Triggered: true, Evidence: make([]AccountMetricRuleEvidence, 0, 4)}
	if !config.Enabled || snapshot == nil {
		evaluation.Triggered = false
		return evaluation, nil
	}
	measurements := accountMetricMeasurements(config, snapshot)
	if len(measurements) == 0 {
		evaluation.Triggered = false
		return evaluation, nil
	}
	for _, measurement := range measurements {
		required := int64(config.RecoverySamples)
		passed := measurement.value != nil && finiteValue(measurement.value) && measurement.samples >= required
		if passed {
			bound := *measurement.rule.RecoveryThreshold
			if measurement.lowerBound {
				passed = *measurement.value >= bound
			} else {
				passed = *measurement.value <= bound
			}
		}
		evidence := AccountMetricRuleEvidence{
			Metric:         measurement.name,
			Threshold:      *measurement.rule.RecoveryThreshold,
			Samples:        measurement.samples,
			Required:       required,
			RecoveryPassed: passed,
		}
		if measurement.value != nil && finiteValue(measurement.value) {
			evidence.Value = *measurement.value
		}
		evaluation.Evidence = append(evaluation.Evidence, evidence)
		evaluation.Triggered = evaluation.Triggered && passed
	}
	return evaluation, nil
}

type accountMetricMeasurement struct {
	name       string
	rule       *AccountMetricThreshold
	value      *float64
	samples    int64
	lowerBound bool
}

func accountMetricMeasurements(config AccountMetricRecoveryConfig, snapshot *AccountMetricSnapshot) []accountMetricMeasurement {
	measurements := make([]accountMetricMeasurement, 0, 4)
	appendMetric := func(name string, rule *AccountMetricThreshold, value *float64, samples int64, lowerBound bool) {
		if rule != nil {
			measurements = append(measurements, accountMetricMeasurement{name: name, rule: rule, value: value, samples: samples, lowerBound: lowerBound})
		}
	}
	appendMetric("success_rate", config.SuccessRate, snapshot.SuccessRate, snapshot.SuccessSamples+snapshot.ErrorSamples, true)
	appendMetric("cache_rate", config.CacheRate, snapshot.CacheRate, snapshot.CacheSamples, true)
	appendMetric("ttft_ms", config.TTFTMs, snapshot.AverageTTFTMs, snapshot.TTFTSamples, false)
	appendMetric("duration_ms", config.DurationMs, snapshot.AverageDurationMs, snapshot.DurationSamples, false)
	return measurements
}

func finiteValue(value *float64) bool {
	return value != nil && finite(*value)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func AccountMetricPauseReason(evidence []AccountMetricRuleEvidence) string {
	parts := make([]string, 0, len(evidence))
	for _, item := range evidence {
		if !item.Breached {
			continue
		}
		comparison := "above"
		if item.Metric == "success_rate" || item.Metric == "cache_rate" {
			comparison = "below"
		}
		parts = append(parts, fmt.Sprintf("%s %.4g %s %.4g (%d samples)", item.Metric, item.Value, comparison, item.Threshold, item.Samples))
	}
	return strings.Join(parts, "; ")
}
