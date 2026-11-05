package qqcommunity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Identities are supplied by the authenticated bridge, never by model arguments.
type runtimeToolRequest struct {
	Tool      string `json:"tool"`
	GroupID   string `json:"group_id"`
	MemberID  string `json:"member_id"`
	MessageID string `json:"message_id"`
	Prompt    string `json:"prompt,omitempty"`
	Keyword   string `json:"keyword,omitempty"`
	Query     string `json:"query,omitempty"`
	URL       string `json:"url,omitempty"`
}

var runtimeNumericID = regexp.MustCompile(`^[1-9][0-9]{0,23}$`)
var runtimeMessageID = regexp.MustCompile(`^-?[0-9]{1,24}$`)

func runtimeToolScope(c Config, in runtimeToolRequest) bool {
	if !c.Enabled || !c.RuntimeChat.Enabled || c.ChatDisabled || !runtimeNumericID.MatchString(in.MemberID) || !runtimeMessageID.MatchString(in.MessageID) {
		return false
	}
	if in.GroupID != "" {
		return c.allows(in.GroupID) && !c.groupPolicy(in.GroupID).DisableChat
	}
	for _, admin := range c.GroupAdmins {
		if admin.MemberID == in.MemberID {
			return true
		}
	}
	return false
}

func runtimeImageEvent(in runtimeToolRequest) (Event, error) {
	prompt := strings.TrimSpace(in.Prompt)
	if in.GroupID == "" || prompt == "" || len([]rune(prompt)) > 6000 {
		return Event{}, errors.New("请在已开放群描述要画的图片，最多6000字")
	}
	e := Event{ID: in.MessageID, GroupID: in.GroupID, MemberID: in.MemberID, Content: "/生图 " + prompt}
	e.Author.MemberID = in.MemberID
	e.Timestamp, _ = json.Marshal(time.Now().UTC().Format(time.RFC3339))
	return e, nil
}

func (s *Service) ServeRuntimeTools(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	fail := func(code int, text string) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": text})
	}
	c, err := s.Config(req.Context())
	if err != nil {
		fail(503, "运行配置暂不可用")
		return
	}
	if !runtimeAuthorized(c, req, strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")) {
		fail(401, "unauthorized")
		return
	}
	if req.Method != http.MethodPost {
		fail(405, "method_not_allowed")
		return
	}
	var in runtimeToolRequest
	d := json.NewDecoder(http.MaxBytesReader(w, req.Body, 32<<10))
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(&in) != nil || d.Decode(&extra) != io.EOF || !runtimeToolScope(c, in) {
		fail(400, "工具参数或会话范围无效")
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 25*time.Second)
	defer cancel()
	var out any
	switch in.Tool {
	case "generate_image":
		if !c.ImageEnabled || c.groupPolicy(in.GroupID).DisableImages {
			fail(400, "本群未启用生图")
			return
		}
		e, eventErr := runtimeImageEvent(in)
		if eventErr != nil {
			fail(400, eventErr.Error())
			return
		}
		raw, _ := json.Marshal(e)
		// Use the original QQ message id. The normal durable queue deduplicates
		// this request and owns image quotas, generation and the QQ receipt.
		if err = s.Enqueue(ctx, c, Payload{Type: groupAtMessageCreate, Data: raw}); err != nil {
			fail(503, "生图任务提交失败，请稍后再试")
			return
		}
		var jobID, jobStatus string
		if err = s.db.QueryRowContext(ctx, `SELECT id,status FROM qqc_jobs WHERE app_id=$1 AND event_key=$2`, c.AppID, groupAtMessageCreate+":"+in.MessageID).Scan(&jobID, &jobStatus); err != nil || (jobStatus != "queued" && jobStatus != "running" && jobStatus != "completed") {
			fail(400, "生图请求未受理或此前已失败，请检查群策略后重新发送")
			return
		}
		out = map[string]string{"status": "queued", "job_id": jobID, "job_status": jobStatus, "message": "生图请求已交给站点，稍后由站点发送图片或失败说明。当前仅已提交，不能声称画好或发送成功。"}
	case "search_meme":
		if !c.RuntimeChat.MemesEnabled {
			fail(400, "表情包功能已关闭")
			return
		}
		raw, _ := json.Marshal(memeRequest{Keyword: in.Keyword})
		m, parseErr := parseMemeRequest(string(raw))
		if parseErr != nil {
			fail(400, parseErr.Error())
			return
		}
		memeConfig := c
		memeConfig.MemeEnabled = true // Runtime toggle owns this call; no retired setting is restored.
		text, data, mime := s.resolveMemeReply(ctx, memeConfig, *m)
		if len(data) == 0 {
			fail(503, text)
			return
		}
		out = map[string]string{"mimeType": mime, "data": base64.StdEncoding.EncodeToString(data), "description": text}
	case "web_search":
		if !c.RuntimeChat.WebEnabled {
			fail(400, "联网查询已关闭")
			return
		}
		out, err = runtimeWebSearch(ctx, in.Query)
	case "web_fetch":
		if !c.RuntimeChat.WebEnabled {
			fail(400, "联网查询已关闭")
			return
		}
		out, err = runtimeWebFetch(ctx, in.URL)
	default:
		fail(400, "unknown_tool")
		return
	}
	if err != nil {
		fail(503, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(out)
}
