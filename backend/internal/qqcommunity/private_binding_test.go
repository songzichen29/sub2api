package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestPublicBindingCodeIsRedactedBeforeQueueing(t *testing.T) {
	code := randomID()
	e := Event{Content: "/绑定 " + code, MsgElements: []MessageElement{{Content: code}}}
	require.True(t, sanitizePublicBinding(&e, Config{}))
	raw, err := json.Marshal(e)
	require.NoError(t, err)
	require.NotContains(t, string(raw), code)
	require.Equal(t, "/绑定 已公开", e.Content)
	require.False(t, sanitizePublicBinding(&Event{Content: "/绑定"}, Config{}))
}

func TestOneBotPrivateBindingRequiresGroupInitiation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	c := DefaultConfig()
	c.Enabled = true
	c.AppID = "bot"
	c.AllowedGroups = []string{"allowed-group"}
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
	mock.ExpectQuery("SELECT request_id,group_id,nickname FROM qqc_private_bind_sessions").WithArgs(c.AppID, "other-qq").
		WillReturnRows(sqlmock.NewRows([]string{"request_id", "group_id", "nickname"}))
	code := randomID()
	codeJSON, err := json.Marshal(code)
	require.NoError(t, err)
	e := obEvent{PostType: "message", MessageType: "private", SelfID: obID(c.AppID), UserID: "other-qq", MessageID: "message-1"}
	e.Message = []obSegment{{Type: "text", Data: map[string]json.RawMessage{"text": codeJSON}}}
	require.NoError(t, s.enqueueOneBot(context.Background(), e))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupBindingStartsPrivateChatAndPrivateCodeRequiresWebsiteApproval(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	c := Config{Enabled: true, AppID: "bot", AppSecret: "test-secret", AllowedGroups: []string{"allowed-group"}}
	requests := make(chan map[string]json.RawMessage, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			var request map[string]json.RawMessage
			if ws.ReadJSON(&request) != nil {
				return
			}
			requests <- request
			if ws.WriteJSON(map[string]any{"echo": json.RawMessage(request["echo"]), "status": "ok", "retcode": 0, "data": map[string]any{"message_id": 1}}) != nil {
				return
			}
		}
	}))
	defer server.Close()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer ws.Close()
	conn := &oneBotConnection{ws: ws, pending: map[string]chan oneBotAck{}, bot: c.AppID, credential: codeHash(c.AppSecret)}
	s.onebot.conn = conn
	go func() {
		for {
			var ack oneBotAck
			if ws.ReadJSON(&ack) != nil {
				return
			}
			conn.mu.Lock()
			ch := conn.pending[ack.Echo]
			conn.mu.Unlock()
			if ch != nil {
				ch <- ack
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	group := Event{GroupID: "allowed-group"}
	group.Author.MemberID = "sender-qq"
	group.Author.Username = "sender"
	mock.ExpectExec("INSERT INTO qqc_private_bind_sessions").
		WithArgs(c.AppID, "sender-qq", sqlmock.AnyArg(), group.GroupID, "sender", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, s.beginPrivateBinding(ctx, c, group))
	first := <-requests
	require.JSONEq(t, `"send_private_msg"`, string(first["action"]))
	require.Contains(t, string(first["params"]), "sender-qq")
	var target struct {
		UserID  string `json:"user_id"`
		GroupID string `json:"group_id"`
	}
	require.NoError(t, json.Unmarshal(first["params"], &target))
	require.Equal(t, "sender-qq", target.UserID)
	require.Equal(t, group.GroupID, target.GroupID)

	code := randomID()
	mock.ExpectQuery("SELECT request_id,group_id,nickname FROM qqc_private_bind_sessions").WithArgs(c.AppID, "sender-qq").
		WillReturnRows(sqlmock.NewRows([]string{"request_id", "group_id", "nickname"}).AddRow("request-1", group.GroupID, "sender"))
	mock.ExpectQuery("SELECT EXISTS.*FROM qqc_bindings").WithArgs(c.AppID, "sender-qq").
		WillReturnRows(sqlmock.NewRows([]string{"bound"}).AddRow(false))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE qqc_bind_requests").WithArgs("sender-qq", "sender", group.GroupID, c.AppID, codeHash(code), "request-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM qqc_private_bind_sessions").WithArgs(c.AppID, "sender-qq", "request-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	codeJSON, err := json.Marshal(code)
	require.NoError(t, err)
	private := obEvent{UserID: "sender-qq", MessageID: "message-2", Message: []obSegment{{Type: "text", Data: map[string]json.RawMessage{"text": codeJSON}}}}
	require.NoError(t, s.handlePrivateBinding(ctx, c, private))
	second := <-requests
	require.JSONEq(t, `"send_private_msg"`, string(second["action"]))
	require.Contains(t, string(second["params"]), "网站个人资料页核对 QQ 号并确认")
	require.NotContains(t, string(second["params"]), code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestExpiredPrivateBindingDoesNotClaimCode(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	c := Config{Enabled: true, AppID: "bot", AllowedGroups: []string{"allowed-group"}}
	mock.ExpectQuery("SELECT request_id,group_id,nickname FROM qqc_private_bind_sessions").WithArgs(c.AppID, "sender-qq").
		WillReturnError(sql.ErrNoRows)
	codeJSON, err := json.Marshal(randomID())
	require.NoError(t, err)
	e := obEvent{UserID: "sender-qq", MessageID: "message-3", Message: []obSegment{{Type: "text", Data: map[string]json.RawMessage{"text": codeJSON}}}}
	require.NoError(t, s.handlePrivateBinding(context.Background(), c, e))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrivateBindingClaimAndSessionConsumptionAreAtomic(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		claimedRows, sessionRows int64
	}{
		{name: "session missing or replaced", claimedRows: 0},
		{name: "concurrent session consumption", claimedRows: 1, sessionRows: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			s := New(db, nil, false, nil)
			c := Config{AppID: "bot"}
			e := Event{GroupID: "group"}
			e.Author.MemberID, e.Author.Username = "qq", "nickname"
			code := randomID()
			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta("AND EXISTS(SELECT 1 FROM qqc_private_bind_sessions s WHERE s.app_id=$4 AND s.member_id=$1 AND s.request_id=$6")).
				WithArgs("qq", "nickname", "group", c.AppID, codeHash(code), "session").
				WillReturnResult(sqlmock.NewResult(0, tc.claimedRows))
			if tc.claimedRows == 1 {
				mock.ExpectExec("DELETE FROM qqc_private_bind_sessions").WithArgs(c.AppID, "qq", "session").
					WillReturnResult(sqlmock.NewResult(0, tc.sessionRows))
			}
			mock.ExpectRollback()
			_, claimed := s.claimBinding(context.Background(), c, e, code, "session")
			require.False(t, claimed)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
