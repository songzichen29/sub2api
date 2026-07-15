package service

import (
	"bytes"
	"testing"
)

var passthroughImageIntentBenchmarkSink bool

func buildLargeOpenAIResponsesImageToolBody(size int) []byte {
	prefix := []byte(`{"model":"gpt-5.4","input":"`)
	suffix := []byte(`","tools":[{"type":"image_generation","model":"gpt-image-2"}]}`)
	padding := size - len(prefix) - len(suffix)
	if padding < 0 {
		padding = 0
	}
	body := make([]byte, 0, len(prefix)+padding+len(suffix))
	body = append(body, prefix...)
	body = append(body, bytes.Repeat([]byte{'x'}, padding)...)
	body = append(body, suffix...)
	return body
}

func BenchmarkOpenAIPassthroughImageIntentReuse_LargeBody(b *testing.B) {
	body := buildLargeOpenAIResponsesImageToolBody(32 << 20)

	b.Run("Once", func(b *testing.B) {
		b.SetBytes(int64(len(body)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			passthroughImageIntentBenchmarkSink = IsImageGenerationIntent(openAIResponsesEndpoint, "gpt-5.4", body)
		}
	})

	b.Run("Twice", func(b *testing.B) {
		b.SetBytes(int64(len(body)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			permissionIntent := IsImageGenerationIntent(openAIResponsesEndpoint, "gpt-5.4", body)
			billingIntent := IsImageGenerationIntent(openAIResponsesEndpoint, "gpt-5.4", body)
			passthroughImageIntentBenchmarkSink = permissionIntent && billingIntent
		}
	})
}
