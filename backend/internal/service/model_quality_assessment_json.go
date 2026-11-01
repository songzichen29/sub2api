package service

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// Accept only lossless presentation differences around one complete object.
// Never extract a convenient JSON fragment from prose or repair missing scores.
func qualityAssessmentJSON(text string) (string, error) {
	if len(text) > 32768 {
		return "", errors.New("评分正文超过大小上限")
	}
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "\uFEFF"))
	if strings.HasPrefix(text, "```") {
		first, last := strings.IndexByte(text, '\n'), strings.LastIndexByte(text, '\n')
		if first < 0 || last <= first || strings.TrimSpace(text[last+1:]) != "```" {
			return "", errors.New("评分 JSON 代码块不完整")
		}
		label := strings.ToLower(strings.TrimSpace(text[:first]))
		if label != "```" && label != "```json" {
			return "", errors.New("评分代码块不是 JSON")
		}
		text = strings.TrimSpace(text[first+1 : last])
	}
	if !strings.HasPrefix(text, "{") || !json.Valid([]byte(text)) {
		return "", errors.New("评分正文必须是单个完整 JSON 对象")
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if err := qualityCheckJSONValue(decoder, 0); err != nil {
		return "", err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return "", errors.New("评分正文包含多个结果或额外文本")
	}
	return text, nil
}

// encoding/json otherwise silently uses the last duplicate property. Reject
// ambiguous objects rather than letting an extra score override an earlier one.
func qualityCheckJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("评分 JSON 嵌套层级过深")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.New("评分 JSON 不完整")
	}
	delim, container := token.(json.Delim)
	if !container {
		return nil
	}
	keys := map[string]bool{}
	for decoder.More() {
		if delim == '{' {
			key, err := decoder.Token()
			if err != nil {
				return errors.New("评分 JSON 不完整")
			}
			name, ok := key.(string)
			// Struct decoding is case-insensitive; Score and score must not
			// silently overwrite each other either.
			name = strings.ToLower(name)
			if !ok || keys[name] {
				return errors.New("评分 JSON 存在重复字段")
			}
			keys[name] = true
		}
		if err := qualityCheckJSONValue(decoder, depth+1); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return errors.New("评分 JSON 不完整")
	}
	return nil
}
