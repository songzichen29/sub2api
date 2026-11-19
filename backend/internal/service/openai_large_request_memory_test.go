package service

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func largeNativeResponsesBody(imageBytes int) []byte {
	prefix := []byte(`{"model":"gpt-6-astra","store":false,"stream":true,"input":[{"type":"custom_tool_call_output","id":"fc_valid","call_id":"call_valid","output":[{"type":"input_image","image_url":"data:image/png;base64,`)
	suffix := []byte(`"}]}]}`)
	body := make([]byte, 0, len(prefix)+imageBytes+len(suffix))
	body = append(body, prefix...)
	body = append(body, bytes.Repeat([]byte{'A'}, imageBytes)...)
	return append(body, suffix...)
}

func TestLargeNativeResponsesRemainsByteIdentical(t *testing.T) {
	body := largeNativeResponsesBody(2 << 20)
	for _, fn := range []func([]byte) ([]byte, bool, error){
		normalizeOpenAIResponsesLegacyIngress,
		sanitizeOpenAIResponsesInputItemIDs,
		normalizeOpenAIResponsesReasoningContentReplay,
		func(body []byte) ([]byte, bool, error) {
			return normalizeOpenAIAPIKeyStoreFalseReasoningReplay(body, false)
		},
	} {
		out, changed, err := fn(body)
		require.NoError(t, err)
		require.False(t, changed)
		require.True(t, bytes.Equal(body, out))
		require.Same(t, &body[0], &out[0], "no-op 必须复用原始缓冲区")
	}
}

func TestRawInputRewritePreservesImagesAndSurroundingFields(t *testing.T) {
	body := []byte(" \n\t" + string(largeNativeResponsesBody(2<<20)))
	body = bytes.Replace(body, []byte(`"id":"fc_valid"`), []byte(`"id":"bad"`), 1)
	body = append(body[:len(body)-1], []byte(`,"opaque":9007199254740993}`)...)
	image := gjson.GetBytes(body, "input.0.output.0.image_url").String()
	out, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, gjson.ValidBytes(out))
	require.False(t, gjson.GetBytes(out, "input.0.id").Exists())
	require.Equal(t, image, gjson.GetBytes(out, "input.0.output.0.image_url").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(out, "opaque").Raw)
	require.Equal(t, "call_valid", gjson.GetBytes(out, "input.0.call_id").String())
}

func TestStoreFalseRawReplayMatchesDecodedBehavior(t *testing.T) {
	fixtures := []string{
		` {"store":false,"input":[{"type":"reasoning","id":"rs_a","encrypted_content":"opaque","summary":null,"opaque":9007199254740993},{"type":"custom_tool_call_output","id":"fc_a","output":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}],"tail":"keep"} `,
		`{"store":false,"input":[null,42,"keep",{"type":"reasoning","encrypted_content":" "},{"type":"item_reference","id":"rs_drop"},{"type":"item_reference","id":"msg_keep"}]}`,
		`{"store":false,"input":[{"type":"reasoning","id":"keep","encrypted_content":"opaque","summary":[]},{"type":"message","call_id":null,"content":"keep"}]}`,
		`{"store":false,"input":[],"input":[{"type":"reasoning","id":"rs_drop"}]}`,
		`{"store":false,"input":[{"type":"message","type":"reasoning","id":"rs_drop"}]}`,
		`{"store":false,"input":[{"type":"reasoning","id":"rs_drop"}]} trailing`,
		`{"store":false,"input":[{"type":"reasoning","id":"rs_drop"}]`,
		`{"store":false,"input":[]}`,
		`{"store":false,"input":[{"type":"reasoning","encrypted_content":42,"summary":[]}]}`,
		`{"store":false,"input":[{"type":"reasoning","encrypted_content":"opaque","summary":false,"id":42}]}`,
		`{"store":false,"input":[{"ty\u0070e":"reasoning","encrypted_content":"opaque","summ\u0061ry":null}]}`,
		`{"store":false,"input":[{"type":"reasoning","encrypted_content":"opaque","summary":[],"summary":null}]}`,
		`{"store":false,"input":[{"type":"message","call_id":null,"content":"\u4f60\u597d"}]}`,
		`{"store":false,"store":true,"input":[{"type":"reasoning"}]}`,
		`{"store":true,"input":[{"type":"reasoning"}]}`,
	}
	for _, fixture := range fixtures {
		body := []byte(fixture)
		for _, knownStoreFalse := range []bool{false, true} {
			want, wantChanged, wantErr := normalizeOpenAIAPIKeyStoreFalseReasoningReplayDecoded(body, knownStoreFalse)
			got, changed, err := normalizeOpenAIAPIKeyStoreFalseReasoningReplay(body, knownStoreFalse)
			require.Equal(t, wantErr != nil, err != nil)
			if wantErr != nil {
				continue
			}
			require.Equal(t, wantChanged, changed)
			require.JSONEq(t, string(want), string(got))
		}
	}
}

func TestLegacyIngressNativeFastPathKeepsValidation(t *testing.T) {
	for _, body := range []string{`{"input":[]} trailing`, `{"input":[}`, `[{"input":[]}]`} {
		_, _, err := normalizeOpenAIResponsesLegacyIngress([]byte(body))
		require.Error(t, err)
	}
	body := []byte(`{"model":"gpt-5.4","input":"keep","mess\u0061ges":[]}`)
	out, changed, err := normalizeOpenAIResponsesLegacyIngress(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(out, "messages").Exists())
	require.Equal(t, "keep", gjson.GetBytes(out, "input").String())
}

func TestStoreFalseRawReplayPreservesLargeImage(t *testing.T) {
	body := bytes.Replace(largeNativeResponsesBody(2<<20), []byte(`"input":[`),
		[]byte(`"input":[{"type":"reasoning","id":"rs_history","encrypted_content":"opaque","summary":null,"call_id":"unused"},`), 1)
	original := bytes.Clone(body)
	out, changed, err := normalizeOpenAIAPIKeyStoreFalseReasoningReplay(body, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, original, body)
	require.False(t, gjson.GetBytes(out, "input.0.id").Exists())
	require.False(t, gjson.GetBytes(out, "input.0.call_id").Exists())
	require.Equal(t, "[]", gjson.GetBytes(out, "input.0.summary").Raw)
	require.Equal(t, parseRawJSONView(body).Get("input.1").Raw, parseRawJSONView(out).Get("input.1").Raw)
	decoded, decodedChanged, err := normalizeOpenAIAPIKeyStoreFalseReasoningReplayDecoded(body, false)
	require.NoError(t, err)
	require.True(t, decodedChanged)
	require.JSONEq(t, string(decoded), string(out))
}

func BenchmarkOpenAIStoreFalseRewrite(b *testing.B) {
	body := bytes.Replace(largeNativeResponsesBody(69<<20), []byte(`"input":[`),
		[]byte(`"input":[{"type":"reasoning","id":"rs_history","encrypted_content":"opaque","summary":null},`), 1)
	for _, tc := range []struct {
		name string
		fn   func([]byte, bool) ([]byte, bool, error)
	}{
		{"decoded", normalizeOpenAIAPIKeyStoreFalseReasoningReplayDecoded},
		{"raw", normalizeOpenAIAPIKeyStoreFalseReasoningReplay},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				out, changed, err := tc.fn(body, false)
				if err != nil || !changed {
					b.Fatalf("replay normalization failed: changed=%v err=%v", changed, err)
				}
				runtime.KeepAlive(out)
			}
		})
	}
}

func BenchmarkOpenAILargeNativeRequest(b *testing.B) {
	body := largeNativeResponsesBody(69 << 20)
	for _, tc := range []struct {
		name string
		fn   func([]byte) ([]byte, bool, error)
	}{
		{"legacy_ingress", normalizeOpenAIResponsesLegacyIngress},
		{"input_ids", sanitizeOpenAIResponsesInputItemIDs},
		{"reasoning_replay", normalizeOpenAIResponsesReasoningContentReplay},
		{"api_key_store_false", func(body []byte) ([]byte, bool, error) {
			return normalizeOpenAIAPIKeyStoreFalseReasoningReplay(body, false)
		}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				out, changed, err := tc.fn(body)
				if err != nil || changed || !bytes.Equal(out, body) {
					b.Fatalf("native image request changed: changed=%v err=%v", changed, err)
				}
				runtime.KeepAlive(out)
			}
		})
	}
}
