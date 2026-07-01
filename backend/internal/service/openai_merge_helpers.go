package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type openAIRequestPatch struct {
	path   string
	delete bool
	value  any
}

type openAIRequestView struct {
	body               []byte
	Model              string
	Stream             bool
	PromptCacheKey     string
	PreviousResponseID string
	ServiceTier        string
	ReasoningEffort    string
	patches            []openAIRequestPatch
	patchesDisabled    bool
}

func newOpenAIRequestView(body []byte) openAIRequestView {
	view := openAIRequestView{body: body}
	if len(body) == 0 {
		return view
	}
	view.Model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	view.Stream = gjson.GetBytes(body, "stream").Bool()
	view.PromptCacheKey = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	view.PreviousResponseID = strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String())
	view.ServiceTier = strings.TrimSpace(gjson.GetBytes(body, "service_tier").String())
	view.ReasoningEffort = strings.TrimSpace(gjson.GetBytes(body, "reasoning.effort").String())
	return view
}

func (v *openAIRequestView) Decode(dst map[string]any) (map[string]any, error) {
	if dst == nil {
		dst = make(map[string]any)
	}
	if len(v.body) == 0 {
		return dst, nil
	}
	dec := json.NewDecoder(bytes.NewReader(v.body))
	dec.UseNumber()
	if err := dec.Decode(&dst); err != nil {
		return nil, err
	}
	return dst, nil
}

func (v *openAIRequestView) MarkPatchSet(path string, value any) {
	path = strings.TrimSpace(path)
	if path == "" || strings.ContainsRune(path, '\\') || v.patchesDisabled {
		v.DisablePatches()
		return
	}
	v.patches = append(v.patches, openAIRequestPatch{path: path, value: value})
}

func (v *openAIRequestView) MarkPatchDelete(path string) {
	path = strings.TrimSpace(path)
	if path == "" || strings.ContainsRune(path, '\\') || v.patchesDisabled {
		v.DisablePatches()
		return
	}
	v.patches = append(v.patches, openAIRequestPatch{path: path, delete: true})
}

func (v *openAIRequestView) DisablePatches() {
	v.patchesDisabled = true
	v.patches = nil
}

func (v openAIRequestView) HasPatches() bool { return !v.patchesDisabled && len(v.patches) > 0 }

func (v openAIRequestView) ApplyPatches() ([]byte, error) {
	if v.patchesDisabled || len(v.patches) == 0 {
		return nil, errors.New("openai request view patches disabled")
	}
	body := append([]byte(nil), v.body...)
	for _, patch := range v.patches {
		var err error
		if patch.delete {
			body, err = sjson.DeleteBytes(body, patch.path)
		} else {
			body, err = sjson.SetBytes(body, patch.path, patch.value)
		}
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func openAIRequestBodyMayContainImageInput(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	input := gjson.GetBytes(body, "input")
	messages := gjson.GetBytes(body, "messages.#-1")
	return openAIJSONValueMayContainImageInput(input) || openAIJSONValueMayContainImageInput(messages)
}

func openAIJSONValueMayContainImageInput(value gjson.Result) bool {
	if !value.Exists() {
		return false
	}
	if value.IsArray() {
		found := false
		value.ForEach(func(_, item gjson.Result) bool {
			if openAIJSONValueMayContainImageInput(item) {
				found = true
				return false
			}
			return true
		})
		return found
	}
	if value.IsObject() {
		if strings.TrimSpace(value.Get("type").String()) == "input_image" || value.Get("image_url").Exists() {
			return true
		}
		return openAIJSONValueMayContainImageInput(value.Get("content"))
	}
	return false
}

func openAIRequestBodyMayContainEmptyBase64InputImage(body []byte) bool {
	if len(body) == 0 || !openAIRequestBodyMayContainInputImageToken(body) {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.Exists() {
		return false
	}
	return openAIJSONValueMayContainEmptyBase64InputImage(input)
}

func openAIRequestBodyMayContainInputImageToken(body []byte) bool {
	return bytes.Contains(body, []byte("input_image")) || bytes.Contains(body, []byte("\\u"))
}

func openAIJSONValueMayContainEmptyBase64InputImage(value gjson.Result) bool {
	if !value.Exists() {
		return false
	}
	if value.IsArray() {
		found := false
		value.ForEach(func(_, item gjson.Result) bool {
			if openAIJSONValueMayContainEmptyBase64InputImage(item) {
				found = true
				return false
			}
			return true
		})
		return found
	}
	if value.IsObject() {
		if strings.TrimSpace(value.Get("type").String()) == "input_image" && isEmptyBase64ImageURL(value.Get("image_url").String()) {
			return true
		}
		return openAIJSONValueMayContainEmptyBase64InputImage(value.Get("content"))
	}
	return false
}

func openAIRequestBodyHasImageGenerationTool(body []byte) bool {
	return openAIJSONToolsContainImageGeneration(gjson.GetBytes(body, "tools"))
}

func openAIRequestBodyImageGenerationToolNeedsNormalization(body []byte) bool {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return false
	}
	needs := false
	tools.ForEach(func(_, item gjson.Result) bool {
		if strings.TrimSpace(item.Get("type").String()) != "image_generation" {
			return true
		}
		if item.Get("format").Exists() {
			needs = true
			return false
		}
		return true
	})
	return needs
}

func decodeOpenAIRequestMapUseNumber(body []byte, dst *map[string]any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	return dec.Decode(dst)
}

func openAIMergeHelperString(v any) string {
	s, _ := v.(string)
	return s
}

func isEmptyBase64ImageURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	idx := strings.Index(lower, "base64,")
	if idx < 0 || !strings.HasPrefix(lower, "data:image/") {
		return false
	}
	return strings.TrimSpace(lower[idx+len("base64,"):]) == ""
}

func (s *OpenAIGatewayService) bindHTTPResponseAccount(ctx context.Context, c *gin.Context, account *Account, responseID string) {
	s.bindOpenAIResponseAccount(ctx, c, account, responseID)
}

func stripCodexSparkImageGenerationToolFromRawPayload(payload []byte, model string) ([]byte, bool, error) {
	if !isCodexSparkModel(model) || !strings.Contains(string(payload), "image_generation") {
		return payload, false, nil
	}
	payloadMap := make(map[string]any)
	if err := decodeOpenAIRequestMapUseNumber(payload, &payloadMap); err != nil {
		return payload, false, err
	}
	if !stripCodexSparkImageGenerationTools(payloadMap) {
		return payload, false, nil
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(payloadMap)
	if err != nil {
		return payload, false, err
	}
	return rebuilt, true, nil
}

func removeEmptyPreviousResponseIDFromRawPayload(payload []byte) ([]byte, bool, error) {
	value := gjson.GetBytes(payload, "previous_response_id")
	if !value.Exists() {
		return payload, false, nil
	}
	if value.Type == gjson.Null || strings.TrimSpace(value.String()) == "" {
		updated, err := sjson.DeleteBytes(payload, "previous_response_id")
		return updated, err == nil, err
	}
	return payload, false, nil
}
