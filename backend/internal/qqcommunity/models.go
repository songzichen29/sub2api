package qqcommunity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
)

// Models uses the selected connection's key, with no model-name allowlist.
// Empty draft secrets preserve the saved key; changing address requires a key.
func (s *Service) Models(ctx context.Context, kind string, draft Config) ([]string, error) {
	c, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	base, key, err := modelConnection(c, draft, kind)
	if err != nil {
		return nil, err
	}
	if err = validateURL(base); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, errors.New("请先配置 Key")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, errors.New("模型列表请求失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("模型列表获取失败，请检查 Key 权限和地址")
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, errors.New("模型列表格式无效")
	}
	out := []string{}
	seen := map[string]bool{}
	for _, m := range body.Data {
		id := strings.TrimSpace(m.ID)
		if id != "" && len(id) <= 200 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

func modelConnection(saved, draft Config, kind string) (string, string, error) {
	draft.BaseURL = strings.TrimRight(strings.TrimSpace(draft.BaseURL), "/")
	draft.ImageBaseURL = strings.TrimRight(strings.TrimSpace(draft.ImageBaseURL), "/")
	if kind != "chat" && kind != "image" {
		return "", "", errors.New("未知模型连接")
	}
	if draft.ClearAPIKey {
		draft.APIKey = ""
	} else if draft.APIKey == "" {
		if saved.APIKey != "" && draft.BaseURL != saved.BaseURL {
			return "", "", errors.New("更换地址后请填写对应 Key")
		}
		draft.APIKey = saved.APIKey
	}
	if kind == "chat" {
		return draft.BaseURL, draft.APIKey, nil
	}
	if draft.ClearImageAPIKey {
		draft.ImageAPIKey = ""
	} else if draft.ImageAPIKey == "" {
		oldBase, _ := imageConnection(saved)
		newBase, _ := imageConnection(draft)
		if saved.ImageAPIKey != "" && oldBase != newBase {
			return "", "", errors.New("更换生图地址后请填写或清除生图 Key")
		}
		draft.ImageAPIKey = saved.ImageAPIKey
	}
	base, key := imageConnection(draft)
	return base, key, nil
}
