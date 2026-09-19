//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateSettingsPersistsPaymentExtensionFields(t *testing.T) {
	repo := &settingHandlerRepoStub{values: map[string]string{}}
	settingService := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	paymentConfigService := service.NewPaymentConfigService(nil, repo, nil, nil)
	handler := NewSettingHandler(settingService, nil, nil, nil, paymentConfigService, nil, nil)

	rec := doUpdateSettings(t, handler, map[string]any{
		"payment_discount_rules": []map[string]any{{
			"threshold": 10,
			"type":      "rate",
			"value":     0.9,
			"label":     "ten",
			"enabled":   true,
		}},
		"payment_quick_amounts":          []float64{10, 50},
		"payment_paid_user_rate_enabled": true,
		"payment_paid_user_rate_rules":   []any{},
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.JSONEq(t, `[{"threshold":10,"type":"rate","value":0.9,"label":"ten","enabled":true}]`, repo.values[service.SettingDiscountRules])
	require.JSONEq(t, `[10,50]`, repo.values[service.SettingQuickAmounts])
	require.Equal(t, "true", repo.values[service.SettingPaidUserRateEnabled])
	_, persistedRules := repo.values[service.SettingPaidUserRateRules]
	require.True(t, persistedRules)
	require.Contains(t, rec.Body.String(), `"payment_paid_user_rate_enabled":true`)
	require.Contains(t, rec.Body.String(), `"payment_quick_amounts":[10,50]`)
}

func TestUpdateSettingsStandaloneImportRequiresPasswordOnFirstEnable(t *testing.T) {
	handler, repo := newStepUpSwitchTestHandler(t, map[string]string{})

	rec := doUpdateSettings(t, handler, map[string]any{
		"standalone_account_import_enabled": true,
	}, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NotContains(t, repo.values, service.SettingKeyStandaloneAccountImportEnabled)
}

func TestUpdateSettingsStandaloneImportHashesPasswordAndReturnsOnlyConfiguredState(t *testing.T) {
	handler, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	const password = "correct horse battery staple"

	rec := doUpdateSettings(t, handler, map[string]any{
		"standalone_account_import_enabled":  true,
		"standalone_account_import_password": password,
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	storedHash := repo.values[service.SettingKeyStandaloneAccountImportPasswordHash]
	require.NotEmpty(t, storedHash)
	require.NotEqual(t, password, storedHash)
	require.True(t, service.CheckStandaloneAccountImportPassword(storedHash, password))
	require.NotContains(t, rec.Body.String(), password)
	require.NotContains(t, rec.Body.String(), storedHash)
	require.Contains(t, rec.Body.String(), `"standalone_account_import_enabled":true`)
	require.Contains(t, rec.Body.String(), `"standalone_account_import_password_configured":true`)

	var responseBody map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &responseBody))
	data := responseBody["data"].(map[string]any)
	require.NotContains(t, data, "standalone_account_import_password")
	require.NotContains(t, data, "standalone_account_import_password_hash")
}

func TestUpdateSettingsStandaloneImportKeepsExistingPasswordWhenOmitted(t *testing.T) {
	existingHash, err := service.HashStandaloneAccountImportPassword("existing-password")
	require.NoError(t, err)
	handler, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyStandaloneAccountImportEnabled:      "false",
		service.SettingKeyStandaloneAccountImportPasswordHash: existingHash,
	})

	rec := doUpdateSettings(t, handler, map[string]any{
		"standalone_account_import_enabled": true,
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, existingHash, repo.values[service.SettingKeyStandaloneAccountImportPasswordHash])
}
