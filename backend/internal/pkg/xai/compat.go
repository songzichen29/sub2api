// Package xai provides neutral compatibility helpers for shared OpenAI code.
// The production branch keeps its existing Grok2API bridge but does not enable
// the upstream xAI OAuth/media implementation.
package xai

import "strings"

type QuotaWindow struct {
	Limit     *float64
	Remaining *float64
	ResetUnix *int64
	ResetAt   string
	Used      *float64
}

type Model struct {
	ID          string
	DisplayName string
}

const DefaultCLIBaseURL = ""

func IncludeIndependentReasoningTokens(input, output, total, reasoning int64) int64 {
	if total > input && total-input > output {
		return total - input
	}
	if reasoning > 0 {
		return output + reasoning
	}
	return output
}

func IsGrokImagineModel(string) bool { return false }
func IsGrokModelID(string) bool { return false }
func ResolveGrokTextResponsesModelID(model string) string { return model }
func StripGrokProviderPrefix(model string) string {
	return strings.TrimPrefix(strings.TrimPrefix(model, "grok/"), "xai/")
}
func DefaultModels() []Model { return nil }
func IsOfficialBaseURLHost(string) bool { return false }
