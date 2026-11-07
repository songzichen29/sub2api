package qqcommunity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func helpTestSender(t *testing.T) (*Service, *[]qqTestReply) {
	t.Helper()
	var replies []qqTestReply
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"test-only","expires_in":7200}`))
		case "/v2/groups/group/messages":
			replies = append(replies, readQQTestReply(t, r))
			_, _ = w.Write([]byte(`{"id":"sent"}`))
		default:
			t.Errorf("help unexpectedly called %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	s := New(nil, nil, false, nil)
	s.qqURL, s.tokenURL = server.URL, server.URL+"/token"
	return s, &replies
}

func TestHelpPromotesCodeClaimsAndUsesSlashCommands(t *testing.T) {
	s := New(nil, nil, false, nil)
	c := Config{Enabled: true, AppID: "bot", AppSecret: "secret", AllowedGroups: []string{"group"}, CommandReplyFormat: "text"}
	requests := bindingTestBot(t, s, c, map[string]any{"message_id": 1})
	j := Job{AppID: c.AppID, GroupID: "group", MemberID: "member", MessageID: "help", CreatedAt: time.Now(), TransientReply: true, CommandReply: true}
	require.NoError(t, s.sendHelp(context.Background(), c, j, ""))
	request := bindingTestRequest(t, requests)
	content := string(request["params"])
	for _, command := range []string{"/领码", "/签到", "/抽奖 [次数]", "/绑定", "/我的余额"} {
		require.Contains(t, content, command)
	}
	require.Contains(t, content, "领取不直接到账")
	require.Contains(t, content, "补发原码")
	var quick []string
	for _, item := range helpItems(c)[:helpQuickActions] {
		quick = append(quick, item.command)
	}
	require.Contains(t, quick, "领码")
	require.Contains(t, quick, "绑定")
}
