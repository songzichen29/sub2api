package qqcommunity

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
)

// RuntimeChatConfig is owned by the DSH runtime. Native model settings are
// retired when this is enabled; account, image and moderation services remain.
type RuntimeChatConfig struct {
	Enabled             bool     `json:"enabled"`
	Names               []string `json:"names"`
	AmbientEnabled      bool     `json:"ambient_enabled"`
	AmbientGroups       []string `json:"ambient_groups"`
	SamplePercent       int      `json:"sample_percent"`
	StartHour           int      `json:"start_hour"`
	EndHour             int      `json:"end_hour"`
	DailyCalls          int      `json:"daily_calls"`
	HourlyReplies       int      `json:"hourly_replies"`
	DailyReplies        int      `json:"daily_replies"`
	ImageInputEnabled   bool     `json:"image_input_enabled"`
	WebEnabled          bool     `json:"web_enabled"`
	MemesEnabled        bool     `json:"memes_enabled"`
	CollectMemes        bool     `json:"collect_memes"`
	MediaSamplePercent  int      `json:"media_sample_percent"`
	MediaDailyCalls     int      `json:"media_daily_calls"`
	ContextWindowTokens int      `json:"context_window_tokens"`
	WelcomeEnabled      bool     `json:"welcome_enabled"`
	WelcomeMessage      string   `json:"welcome_message"`
}

func DefaultRuntimeChatConfig() RuntimeChatConfig {
	return RuntimeChatConfig{Names: []string{"小碗", "大碗"}, AmbientGroups: []string{}, SamplePercent: 100,
		StartHour: 8, EndHour: 2, DailyCalls: 1800, HourlyReplies: 100, DailyReplies: 1000,
		ImageInputEnabled: true, WebEnabled: true, MemesEnabled: true, CollectMemes: true,
		MediaSamplePercent: 10, MediaDailyCalls: 100, ContextWindowTokens: 65536,
		WelcomeEnabled: true, WelcomeMessage: "欢迎 {name} 加入本群！我是小碗，可以陪大家聊天，也能查询余额和今日用量。发送“帮助”查看功能，账户查询需要先绑定。"}
}

func validateRuntimeChat(c Config) error {
	r := c.RuntimeChat
	if !r.Enabled {
		return nil
	}
	if len(r.Names) == 0 || len(r.Names) > 12 || len(r.AmbientGroups) > 100 {
		return errors.New("机器人称呼须为 1～12 个，接话群最多 100 个")
	}
	for _, name := range r.Names {
		if strings.TrimSpace(name) == "" || len([]rune(name)) > 40 || strings.ContainsAny(name, "\r\n") {
			return errors.New("机器人称呼须为 1～40 字")
		}
	}
	for _, group := range r.AmbientGroups {
		if !c.allows(group) {
			return errors.New("自主接话群须属于已开放群")
		}
	}
	if r.SamplePercent < 0 || r.SamplePercent > 100 || r.MediaSamplePercent < 0 || r.MediaSamplePercent > 100 ||
		r.StartHour < 0 || r.StartHour > 23 || r.EndHour < 0 || r.EndHour > 23 ||
		r.DailyCalls < 0 || r.DailyCalls > 100000 || r.HourlyReplies < 0 || r.HourlyReplies > 100000 ||
		r.DailyReplies < 0 || r.DailyReplies > 100000 || r.MediaDailyCalls < 0 || r.MediaDailyCalls > 100000 ||
		r.ContextWindowTokens < 8192 || r.ContextWindowTokens > 262144 {
		return errors.New("群聊时段、比例、额度或模型上下文容量超出范围")
	}
	if len([]rune(r.WelcomeMessage)) > 1000 || (r.WelcomeEnabled && strings.TrimSpace(r.WelcomeMessage) == "") {
		return errors.New("启用新人欢迎时请填写欢迎语，最多 1000 字")
	}
	return nil
}

func (c *Config) retireNativeChat() {
	if c.RuntimeChat.Enabled {
		c.Autonomous = AutonomousConfig{}
		c.HistoryMessages = 0
		c.MemeEnabled = false
	}
}

// Omit obsolete controls from saved JSON and public API responses. Old jobs
// remain inspectable, but cannot re-enable the retired model path.
func (c Config) MarshalJSON() ([]byte, error) {
	type plainConfig Config
	raw, err := json.Marshal(plainConfig(c))
	if err != nil || !c.RuntimeChat.Enabled {
		return raw, err
	}
	var values map[string]json.RawMessage
	if err = json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	for _, name := range []string{"autonomous", "history_messages", "meme_enabled"} {
		delete(values, name)
	}
	return json.Marshal(values)
}

func runtimePolicy(c Config) map[string]any {
	r := c.RuntimeChat
	owners := []string{}
	seen := map[string]bool{}
	for _, admin := range c.GroupAdmins {
		if admin.MemberID != "" && !seen[admin.MemberID] {
			owners = append(owners, admin.MemberID)
			seen[admin.MemberID] = true
		}
	}
	sort.Strings(owners)
	commands := []string{"帮助", "签到", "我的余额", "余额", "今日用量", "用量", "成长任务", "邀请奖励", "邀请活动", "我的邀请", "我的奖励", "抽奖", "抽奖次数", "我的抽奖", "抽奖活动", "渠道状态", "渠道", "监控", "渠道监控", "生图", "画图", "价格", "绑定", "确认", "解绑", "转账"}
	for _, command := range c.CustomCommands {
		if command.Enabled {
			commands = append(commands, strings.TrimPrefix(command.Command, "/"))
		}
	}
	persona := c.SystemPrompt
	if c.SiteKnowledge.Enabled {
		persona += "\n\n站点公开知识：\n" + c.SiteKnowledge.Content
		for _, entry := range c.SiteKnowledge.Entries {
			if entry.Enabled {
				persona += "\n" + entry.Title + "\n" + entry.Content + "\n" + entry.URL
			}
		}
	}
	api := "openai-completions"
	if c.ChatProtocol == "responses" {
		api = "openai-responses"
	}
	return map[string]any{
		"enabled": c.Enabled && r.Enabled, "chat_disabled": c.ChatDisabled,
		"bot_id": c.AppID, "groups": c.AllowedGroups, "group_policies": c.GroupPolicies,
		"owners": owners, "private_users": owners, "business_commands": commands, "business_observation": true,
		"model": c.ChatModel, "model_base_url": c.BaseURL, "model_api": api, "model_api_key": c.APIKey,
		"persona": persona, "max_output_tokens": c.MaxOutputTokens, "context_window_tokens": r.ContextWindowTokens,
		"daily_chat_limit": c.DailyChatLimit, "total_daily_chat_limit": c.TotalDailyChatLimit,
		"cooldown_seconds": c.CooldownSeconds, "user_hourly_limit": 60,
		"names": r.Names, "ambient_enabled": r.AmbientEnabled, "ambient_groups": r.AmbientGroups,
		"ambient_sample_percent": r.SamplePercent, "active_start_hour": r.StartHour, "active_end_hour": r.EndHour,
		"ambient_daily_calls": r.DailyCalls, "ambient_hourly_replies": r.HourlyReplies, "ambient_daily_replies": r.DailyReplies,
		"image_input_enabled": r.ImageInputEnabled, "ambient_media_daily_calls": r.MediaDailyCalls,
		"web_enabled": r.WebEnabled, "memes_enabled": r.MemesEnabled, "collect_memes": r.CollectMemes, "image_generation_enabled": c.ImageEnabled,
		"proactive_enabled": false, "welcome_enabled": r.WelcomeEnabled,
	}
}

func runtimeRevision(policy map[string]any) string {
	raw, _ := json.Marshal(policy)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type ChatRuntimeStatus struct {
	Enabled          bool       `json:"enabled"`
	Connected        bool       `json:"connected"`
	Revision         string     `json:"revision"`
	ExpectedRevision string     `json:"expected_revision"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty"`
	Sessions         int64      `json:"sessions"`
	Messages         int64      `json:"messages"`
	Summaries        int64      `json:"summaries"`
}

func (s *Service) ChatRuntimeStatus(c Config) ChatRuntimeStatus {
	s.chatRuntimeMu.Lock()
	out := s.chatRuntimeStatus
	s.chatRuntimeMu.Unlock()
	out.Enabled = c.RuntimeChat.Enabled
	out.ExpectedRevision = runtimeRevision(runtimePolicy(c))
	out.Connected = out.Enabled && out.LastSeenAt != nil && time.Since(*out.LastSeenAt) < 45*time.Second && out.Revision == out.ExpectedRevision
	return out
}

func (s *Service) ServeRuntime(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	c, err := s.Config(req.Context())
	if err != nil {
		http.Error(w, "runtime configuration unavailable", http.StatusServiceUnavailable)
		return
	}
	provided := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if !runtimeAuthorized(c, req, provided) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if req.Method == http.MethodGet {
		policy := runtimePolicy(c)
		policy["revision"] = runtimeRevision(policy)
		_ = json.NewEncoder(w).Encode(policy)
		return
	}
	if req.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var report ChatRuntimeStatus
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&report) != nil || len(report.Revision) != 64 || report.Sessions < 0 || report.Messages < 0 || report.Summaries < 0 {
		http.Error(w, "invalid runtime report", http.StatusBadRequest)
		return
	}
	now := time.Now()
	report.LastSeenAt = &now
	s.chatRuntimeMu.Lock()
	s.chatRuntimeStatus = report
	s.chatRuntimeMu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func runtimeAuthorized(c Config, req *http.Request, provided string) bool {
	return strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") && c.AppSecret != "" &&
		subtle.ConstantTimeCompare([]byte(provided), []byte(c.AppSecret)) == 1 && req.Header.Get("X-Self-ID") == c.AppID
}
