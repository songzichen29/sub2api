package service

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

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

func stripOpenAIImageGenerationToolFromRawPayload(payload []byte) ([]byte, bool, error) {
	if !strings.Contains(string(payload), "image_generation") {
		return payload, false, nil
	}
	payloadMap := make(map[string]any)
	if err := decodeOpenAIRequestMapUseNumber(payload, &payloadMap); err != nil {
		return payload, false, err
	}
	if !stripOpenAIImageGenerationTools(payloadMap) {
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
