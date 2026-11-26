package qqcommunity

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func replyText(value string) *string { return &value }

func replyTestConfig() Config {
	c := DefaultConfig()
	c.Enabled, c.AppID, c.AppSecret = true, "123", "secret"
	c.AllowedGroups = []string{"456", "457"}
	c.GroupPolicies = []GroupPolicy{{GroupID: "456", Label: "工作群"}}
	c.BaseURL, c.APIKey, c.ChatModel = "https://example.invalid", "private-key", "test-model"
	c.RuntimeChat = DefaultRuntimeChatConfig()
	c.RuntimeChat.Enabled, c.RuntimeChat.PrivateChatEnabled = true, true
	c.RuntimeChat.PrivateUsers = []string{"789", "790"}
	c.SystemPrompt = "基础人设"
	c.retireNativeChat()
	return c
}

func TestReplySettingsSharedGroupPrivateAndPromptInheritance(t *testing.T) {
	c := replyTestConfig()
	settings, err := applyReplySettings(c, "789", []ReplySettingsUpdate{
		{Scope: "group_defaults", Mode: replyText("voice"), Voice: replyText("御姐"), Prompt: replyText("简短回复"), PromptAction: "append"},
		{Scope: "group", GroupIDs: []string{"456", "457"}, Prompt: replyText("先给结论"), PromptAction: "append"},
		{Scope: "group", GroupIDs: []string{"456"}, Mode: replyText("text")},
		{Scope: "private", Mode: replyText("voice"), Voice: replyText("曼波"), Prompt: replyText("")},
	})
	require.NoError(t, err)
	require.Equal(t, "基础人设\n简短回复", *settings.GroupDefaults.Prompt)
	require.Equal(t, "基础人设\n简短回复\n先给结论", *settings.Groups["456"].Prompt)
	require.Equal(t, "text", *settings.Groups["456"].Mode)
	require.Nil(t, settings.Groups["457"].Mode)
	require.Equal(t, "曼波", *settings.PrivateUsers["789"].Voice)
	require.Equal(t, "", *settings.PrivateUsers["789"].Prompt)
	require.Empty(t, c.ReplySettings.Groups, "计算不能修改原配置")

	c.ReplySettings = settings
	reset, err := applyReplySettings(c, "789", []ReplySettingsUpdate{{Scope: "group", GroupIDs: []string{"456"}, Reset: []string{"mode", "voice", "prompt"}}})
	require.NoError(t, err)
	require.Equal(t, ReplyRule{}, reset.Groups["456"])
	require.Equal(t, "text", *c.ReplySettings.Groups["456"].Mode)
	require.Equal(t, settings.GroupDefaults, reset.GroupDefaults)
}

func TestReplySettingsInvalidBatchLeavesOriginalUntouched(t *testing.T) {
	c := replyTestConfig()
	c.ReplySettings.Groups = map[string]ReplyRule{"456": {Prompt: replyText("原规则")}}
	for _, invalid := range []ReplySettingsUpdate{
		{Scope: "group", GroupIDs: []string{"999"}, Mode: replyText("voice")},
		{Scope: "group", GroupIDs: []string{"456", "456"}, Mode: replyText("voice")},
		{Scope: "group", Mode: replyText("voice")},
		{Scope: "private", GroupIDs: []string{"456"}, Mode: replyText("voice")},
		{Scope: "private", Mode: replyText("audio")},
		{Scope: "private", Voice: replyText(" ")},
		{Scope: "private", Reset: []string{"unknown"}},
		{Scope: "private", Reset: []string{"mode", "mode"}},
		{Scope: "private", Reset: []string{"mode"}, Mode: replyText("text")},
		{Scope: "private", PromptAction: "append"},
		{Scope: "private", PromptAction: "invalid", Prompt: replyText("人设")},
		{Scope: "private", Prompt: replyText(strings.Repeat("a", 64001))},
		{Scope: "private"},
		{Scope: "unknown", Mode: replyText("voice")},
	} {
		result, err := applyReplySettings(c, "789", []ReplySettingsUpdate{
			{Scope: "group", GroupIDs: []string{"456"}, Prompt: replyText("新增"), PromptAction: "append"}, invalid,
		})
		require.Error(t, err)
		require.Equal(t, c.ReplySettings, result)
		require.Equal(t, "原规则", *c.ReplySettings.Groups["456"].Prompt)
	}
}

func TestReplySettingsRuntimeToolsPermissionsAndReadback(t *testing.T) {
	for _, tc := range []struct {
		name, tool, group, member string
		code                      int
	}{
		{"private-query", "get_reply_settings", "", "789", 200},
		{"private-groups", "list_reply_groups", "", "789", 200},
		{"group-query", "get_reply_settings", "456", "789", 403},
		{"group-write", "update_reply_settings", "456", "789", 403},
		{"outsider", "get_reply_settings", "", "999", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, mock, _, _ := observationFixture(t)
			s.encryptor = previewEncryptor{}
			c := replyTestConfig()
			c.ReplySettings.PrivateUsers = map[string]ReplyRule{"789": {Prompt: replyText("自己的私聊人设")}, "790": {Prompt: replyText("其他私聊人设")}}
			expectObservationConfig(mock, c)
			body, err := json.Marshal(runtimeToolRequest{Tool: tc.tool, GroupID: tc.group, MemberID: tc.member, MessageID: "42"})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/runtime/tools", strings.NewReader(string(body)))
			req.Header.Set("Authorization", "Bearer secret")
			req.Header.Set("X-Self-ID", c.AppID)
			w := httptest.NewRecorder()
			s.ServeRuntimeTools(w, req)
			require.Equal(t, tc.code, w.Code, w.Body.String())
			require.NotContains(t, w.Body.String(), "private-key")
			require.NotContains(t, w.Body.String(), "secret")
			require.NotContains(t, w.Body.String(), "其他私聊人设")
			if tc.name == "private-query" {
				require.Contains(t, w.Body.String(), "自己的私聊人设")
				require.Contains(t, w.Body.String(), "基础人设")
				require.Contains(t, w.Body.String(), sectionVersion(c, "reply-settings"))
			}
			if tc.name == "private-groups" {
				require.Contains(t, w.Body.String(), "工作群")
				require.Contains(t, w.Body.String(), "457")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

type replySavedConfig struct{ config *Config }

func (capture replySavedConfig) Match(value driver.Value) bool {
	text, ok := value.(string)
	var saved Config
	if !ok || json.Unmarshal([]byte(text), &saved) != nil {
		return false
	}
	*capture.config = saved
	return true
}

func expectReplyConfigLock(mock sqlmock.Sqlmock, c Config) {
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	expectObservationConfig(mock, c)
}

func expectReplyConfigCommit(mock sqlmock.Sqlmock, saved *Config, section bool) {
	mock.ExpectExec("DELETE FROM qqc_group_messages").WithArgs("123").WillReturnResult(sqlmock.NewResult(0, 0))
	if section {
		mock.ExpectQuery("SELECT value FROM settings").WithArgs(configHistoryKey).WillReturnRows(sqlmock.NewRows([]string{"value"}))
		mock.ExpectExec("INSERT INTO settings").WithArgs(configHistoryKey, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec("INSERT INTO settings").WithArgs(configKey, replySavedConfig{saved}).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func TestReplySettingsSaveReplayConflictAndRevocation(t *testing.T) {
	s, mock, _, _ := observationFixture(t)
	s.encryptor, s.encryptionReady = previewEncryptor{}, true
	c := replyTestConfig()
	in := runtimeToolRequest{Tool: "update_reply_settings", MemberID: "789", MessageID: "42", OperationID: "42:reply-settings", Version: sectionVersion(c, "reply-settings"),
		Updates: []ReplySettingsUpdate{{Scope: "group_defaults", Prompt: replyText("追加一次"), PromptAction: "append"}}}
	var saved Config
	expectReplyConfigLock(mock, c)
	expectReplyConfigCommit(mock, &saved, true)
	result, err := s.updateReplySettings(context.Background(), c.AppID, in)
	require.NoError(t, err)
	require.Equal(t, "saved", result.Status)
	require.Equal(t, uint64(1), saved.ReplySettings.Revision)
	require.Equal(t, "789", saved.ReplySettings.LastOperation.Actor)
	require.Equal(t, "基础人设\n追加一次", *saved.ReplySettings.GroupDefaults.Prompt)
	require.NotEqual(t, in.Version, result.Version)

	expectReplyConfigLock(mock, saved)
	mock.ExpectRollback()
	replay, err := s.updateReplySettings(context.Background(), c.AppID, in)
	require.NoError(t, err)
	require.Equal(t, "unchanged", replay.Status)
	require.Equal(t, result.Version, replay.Version)

	altered := in
	altered.Updates = []ReplySettingsUpdate{{Scope: "private", Mode: replyText("voice")}}
	expectReplyConfigLock(mock, saved)
	mock.ExpectRollback()
	_, err = s.updateReplySettings(context.Background(), c.AppID, altered)
	require.ErrorContains(t, err, "同一操作编号")

	later := saved
	later.ReplySettings.Revision++
	later.ReplySettings.LastOperation = nil
	expectReplyConfigLock(mock, later)
	mock.ExpectRollback()
	_, err = s.updateReplySettings(context.Background(), c.AppID, in)
	require.ErrorIs(t, err, ErrConfigConflict)

	revoked := saved
	revoked.RuntimeChat.PrivateChatEnabled = false
	expectReplyConfigLock(mock, revoked)
	mock.ExpectRollback()
	_, err = s.updateReplySettings(context.Background(), c.AppID, in)
	require.ErrorContains(t, err, "权限已关闭")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplySettingsLegacyAndOtherSectionsPreserveRules(t *testing.T) {
	c := replyTestConfig()
	c.ReplySettings = ReplySettings{Revision: 4, GroupDefaults: ReplyRule{Voice: replyText("御姐")}}
	oldDraft := c
	oldDraft.ReplySettings = ReplySettings{}
	for _, section := range []string{"runtime-chat", "groups", "chat"} {
		merged, err := mergeConfigSection(c, oldDraft, section)
		require.NoError(t, err)
		require.Equal(t, c.ReplySettings, merged.ReplySettings)
	}
	s, mock, _, _ := observationFixture(t)
	s.encryptor, s.encryptionReady = previewEncryptor{}, true
	var saved Config
	expectReplyConfigLock(mock, c)
	expectReplyConfigCommit(mock, &saved, false)
	_, err := s.SaveBasicConfig(context.Background(), oldDraft)
	require.NoError(t, err)
	require.Equal(t, c.ReplySettings, saved.ReplySettings)

	in := runtimeToolRequest{Tool: "update_reply_settings", MemberID: "789", MessageID: "43", OperationID: "43:reply-settings", Version: "stale-version",
		Updates: []ReplySettingsUpdate{{Scope: "group_defaults", Reset: []string{"voice"}}}}
	expectReplyConfigLock(mock, c)
	mock.ExpectRollback()
	_, err = s.updateReplySettings(context.Background(), c.AppID, in)
	require.ErrorIs(t, err, ErrConfigConflict)

	in.Version = sectionVersion(c, "reply-settings")
	expectReplyConfigLock(mock, c)
	expectReplyConfigCommit(mock, &saved, true)
	_, err = s.updateReplySettings(context.Background(), c.AppID, in)
	require.NoError(t, err)
	require.Equal(t, uint64(5), saved.ReplySettings.Revision, "恢复旧规则不能倒退版本")
	require.Nil(t, saved.ReplySettings.GroupDefaults.Voice)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplySettingsGenericSectionCannotBypassVoiceCatalog(t *testing.T) {
	s, mock, _, _ := observationFixture(t)
	for _, voice := range []string{"unknown_voice", "御姐"} {
		c := replyTestConfig()
		c.ReplySettings.GroupDefaults = ReplyRule{Mode: replyText("voice"), Voice: replyText(voice)}
		_, err := s.SaveConfigSection(context.Background(), "reply-settings", sectionVersion(c, "reply-settings"), c)
		require.ErrorContains(t, err, "通过机器人私聊修改")
	}
	require.NoError(t, mock.ExpectationsWereMet(), "通用入口拒绝所有回复设置写入，不启动存储事务")
}

func TestReplySettingsPolicyKeepsKnowledgeAndPersistsInheritance(t *testing.T) {
	c := replyTestConfig()
	c.SiteKnowledge.Enabled = true
	c.SiteKnowledge.Content = "站点知识"
	c.ReplySettings = ReplySettings{GroupDefaults: ReplyRule{Mode: replyText("voice")}, Groups: map[string]ReplyRule{"456": {Prompt: replyText("")}}}
	policy := runtimePolicy(c)
	require.Equal(t, c.SystemPrompt, policy["base_persona"])
	require.Equal(t, "\n\n站点公开知识：\n站点知识", policy["knowledge_prompt"])
	require.Equal(t, "基础人设\n\n站点公开知识：\n站点知识", policy["persona"])
	require.Equal(t, c.ReplySettings, policy["reply_settings"])
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	var restored Config
	require.NoError(t, json.Unmarshal(raw, &restored))
	require.Equal(t, c.ReplySettings, restored.ReplySettings)
	require.NotNil(t, restored.ReplySettings.Groups["456"].Prompt)
}
