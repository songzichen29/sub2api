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

func decodeOpenAIRequestMapUseNumber(body []byte, dst *map[string]any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	return dec.Decode(dst)
}

func openAIMergeHelperString(v any) string {
	s, _ := v.(string)
	return s
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
