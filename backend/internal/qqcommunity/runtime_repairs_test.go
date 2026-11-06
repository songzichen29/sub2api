package qqcommunity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRuntimeGroupAdminsKeepExactScope(t *testing.T) {
	c := DefaultConfig()
	c.AppID = "123"
	c.GroupAdmins = []GroupAdmin{{AppID: "123", GroupID: "456", MemberID: "789"}, {AppID: "999", GroupID: "456", MemberID: "888"}}
	p := runtimePolicy(c)
	require.Empty(t, p["owners"])
	require.Equal(t, []string{"789"}, p["private_users"])
	require.Equal(t, c.GroupAdmins, p["group_admins"])
	c.Enabled, c.RuntimeChat.Enabled = true, true
	require.False(t, runtimeToolScope(c, runtimeToolRequest{MemberID: "888", MessageID: "1"}))
}

func TestRuntimeEditPreservesOriginalThroughDurablePayloadAndNeverFallsBack(t *testing.T) {
	original := testInputPNG(t)
	in := runtimeToolRequest{Tool: "edit_image", GroupID: "456", MemberID: "789", MessageID: "-42", Prompt: "背景改成蓝色", SourceImage: &runtimeSourceImage{MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(original)}}
	e, err := runtimeImageEvent(in)
	require.NoError(t, err)
	raw, err := json.Marshal(e)
	require.NoError(t, err)
	var queued Event
	require.NoError(t, json.Unmarshal(raw, &queued))
	require.Equal(t, e.ID, queued.ID)
	require.Equal(t, e.RuntimeEditImage, queued.RuntimeEditImage)
	for _, fail := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			require.Equal(t, "/v1/images/edits", r.URL.Path)
			require.NoError(t, r.ParseMultipartForm(16<<20))
			defer r.MultipartForm.RemoveAll()
			require.Equal(t, in.Prompt, r.FormValue("prompt"))
			f, _, err := r.FormFile("image")
			require.NoError(t, err)
			defer f.Close()
			received, err := io.ReadAll(f)
			require.NoError(t, err)
			require.Equal(t, original, received)
			if fail {
				w.WriteHeader(400)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(original)}}})
		}))
		s := New(nil, nil, false, nil)
		c := DefaultConfig()
		c.ImageBaseURL = server.URL + "/v1"
		result, err := s.runtimeImage(context.Background(), c, queued, in.Prompt)
		if fail {
			require.Error(t, err)
			require.Empty(t, result)
		} else {
			require.NoError(t, err)
			require.NotEmpty(t, result)
		}
		require.Equal(t, 1, calls)
		server.Close()
	}
	for _, source := range []*runtimeSourceImage{nil, {MimeType: "image/jpeg", Data: in.SourceImage.Data}, {MimeType: "image/png", Data: "not-base64"}, {MimeType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("not image"))}} {
		bad := in
		bad.SourceImage = source
		_, err = runtimeImageEvent(bad)
		require.Error(t, err)
	}
	in.Tool = "generate_image"
	_, err = runtimeImageEvent(in)
	require.Error(t, err)
}

func TestRuntimeDiagnosticsAreBoundedAndFreshnessIsExplicit(t *testing.T) {
	c := DefaultConfig()
	c.RuntimeChat.Enabled = true
	now := time.Now()
	report := ChatRuntimeStatus{Revision: runtimeRevision(runtimePolicy(c)), LastSeenAt: &now, Day: day(now), DailyCalls: 11, DailySent: 9, DailyFailed: 1, Unconfirmed: 1, Recent: []ChatRuntimeRecord{{ID: "id", Scope: "123:group:456", Sender: "789", MessageID: "-42", Status: "partial", Reason: "qq_delivery_unconfirmed", Created: 1, Updated: 2}}}
	require.True(t, validRuntimeReport(report))
	s := New(nil, nil, false, nil)
	s.chatRuntimeStatus = report
	require.True(t, s.ChatRuntimeStatus(c).MetricsAvailable)
	stale := now.Add(-time.Minute)
	s.chatRuntimeStatus.LastSeenAt = &stale
	require.False(t, s.ChatRuntimeStatus(c).MetricsAvailable)
	s.chatRuntimeStatus = report
	s.chatRuntimeStatus.Day = day(now.Add(-24 * time.Hour))
	require.False(t, s.ChatRuntimeStatus(c).MetricsAvailable)
	report.DailyCalls = -1
	require.False(t, validRuntimeReport(report))
	report.DailyCalls = 11
	report.Recent[0].Reason = strings.Repeat("x", 101)
	require.False(t, validRuntimeReport(report))
	report.Recent = make([]ChatRuntimeRecord, 21)
	require.False(t, validRuntimeReport(report))
}
