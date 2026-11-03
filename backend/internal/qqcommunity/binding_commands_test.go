package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func expectNewPrivateBinding(t *testing.T, mock sqlmock.Sqlmock, c Config, e Event, j *Job, content string) {
	t.Helper()
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock_shared").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
	mock.ExpectExec("SELECT pg_advisory_xact_lock\\(hashtextextended").WithArgs(c.AppID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT EXISTS.*FROM qqc_bindings").WithArgs(c.AppID, e.Author.MemberID).WillReturnRows(sqlmock.NewRows([]string{"bound"}).AddRow(false))
	mock.ExpectQuery("SELECT request_id,group_id,nickname,expires_at,user_id FROM qqc_binding_commands").WithArgs(c.AppID, e.Author.MemberID).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT request_id,group_id,nickname,expires_at FROM qqc_private_bind_sessions").WithArgs(c.AppID, e.Author.MemberID).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO qqc_private_bind_sessions").WithArgs(c.AppID, e.Author.MemberID, sqlmock.AnyArg(), e.GroupID, e.Author.Username, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM qqc_binding_commands").WithArgs(c.AppID, e.Author.MemberID).WillReturnResult(sqlmock.NewResult(0, 0))
	if j != nil {
		mock.ExpectExec("INSERT INTO qqc_binding_commands").WithArgs(c.AppID, e.Author.MemberID, sqlmock.AnyArg(), j.ID, e.GroupID, j.MessageID, e.Author.Username, content, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
}

// A local OneBot peer validates the actual action, target and receipt without
// sending a message or using any real QQ credential.
func bindingTestRequest(t *testing.T, requests <-chan map[string]json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(5 * time.Second):
		t.Fatal("expected OneBot fixture request was not sent")
		return nil
	}
}

func bindingTestBot(t *testing.T, s *Service, c Config, ackData map[string]any) <-chan map[string]json.RawMessage {
	return bindingTestBotWithAck(t, s, c, func(map[string]json.RawMessage) map[string]any { return ackData })
}

func bindingTestBotWithAck(t *testing.T, s *Service, c Config, ackData func(map[string]json.RawMessage) map[string]any) <-chan map[string]json.RawMessage {
	t.Helper()
	requests := make(chan map[string]json.RawMessage, 10)
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
			if ws.WriteJSON(map[string]any{"echo": json.RawMessage(request["echo"]), "status": "ok", "retcode": 0, "data": ackData(request)}) != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })
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
	return requests
}

func TestUnboundAccountCommandPrivateReceiptBeforeGroupNotice(t *testing.T) {
	for _, confirmed := range []bool{true, false} {
		name := "confirmed"
		if !confirmed {
			name = "missing private message ID"
		}
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			s := New(db, fixtureEncryptor{}, true, nil)
			c := Config{Enabled: true, AppID: "bot", AppSecret: "secret", AllowedGroups: []string{"group"}, CommandReplyFormat: "text"}
			ack := map[string]any{}
			if confirmed {
				ack["message_id"] = 1
			}
			requests := bindingTestBot(t, s, c, ack)
			e := Event{ID: "original-message", GroupID: "group", Content: "/签到"}
			e.Author.MemberID, e.Author.Username = "member", "sender"
			j := Job{ID: "original-job", AppID: c.AppID, GroupID: e.GroupID, MemberID: e.Author.MemberID, MessageID: e.ID, CreatedAt: time.Now(), EventType: groupAtMessageCreate}
			mock.ExpectQuery("SELECT b.user_id FROM qqc_bindings").WithArgs(c.AppID, j.MemberID).WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery("SELECT EXISTS.*FROM qqc_bindings").WithArgs(c.AppID, j.MemberID).WillReturnRows(sqlmock.NewRows([]string{"bound"}).AddRow(false))
			expectNewPrivateBinding(t, mock, c, e, &j, "/签到")
			if confirmed {
				mock.ExpectExec("UPDATE qqc_binding_commands SET delivered=TRUE").WithArgs(c.AppID, j.MemberID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err = s.message(ctx, c, &j, e)
			if confirmed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			} // group ACK is deliberately missing too
			private, group := <-requests, <-requests
			require.JSONEq(t, `"send_private_msg"`, string(private["action"]))
			require.JSONEq(t, `"send_group_msg"`, string(group["action"]))
			if confirmed {
				require.Contains(t, string(group["params"]), privateBindingSent)
			} else {
				require.Contains(t, string(group["params"]), "开启群临时会话")
				require.NotContains(t, string(group["params"]), privateBindingSent)
			}
			require.NotContains(t, string(group["params"]), "网站个人资料")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestBindingCommandFirstPendingIsNotOverwritten(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, fixtureEncryptor{}, true, nil)
	c := Config{Enabled: true, AppID: "bot", AppSecret: "secret", AllowedGroups: []string{"group"}}
	requests := bindingTestBot(t, s, c, map[string]any{"message_id": 1})
	raw, _ := json.Marshal(c)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock_shared").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
	mock.ExpectExec("SELECT pg_advisory_xact_lock\\(hashtextextended").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT EXISTS.*FROM qqc_bindings").WillReturnRows(sqlmock.NewRows([]string{"bound"}).AddRow(false))
	mock.ExpectQuery("SELECT request_id,group_id,nickname,expires_at,user_id FROM qqc_binding_commands").WillReturnRows(sqlmock.NewRows([]string{"request", "group", "nickname", "expires", "user"}).AddRow("same-session", "group", "sender", time.Now().Add(time.Minute), int64(42)))
	mock.ExpectCommit()
	mock.ExpectExec("UPDATE qqc_binding_commands SET delivered=TRUE").WithArgs(c.AppID, "member", "same-session").WillReturnResult(sqlmock.NewResult(0, 1))
	e := Event{GroupID: "group"}
	e.Author.MemberID = "member"
	require.NoError(t, s.beginPrivateBindingCommand(context.Background(), c, e, &Job{ID: "later-job"}, "/转账 1 recipient@example.test"))
	req := <-requests
	require.Contains(t, string(req["params"]), "无需再次提交绑定码")
	require.NotContains(t, string(req["params"]), "recipient@example.test")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBindingCommandDispatchTransactionRollbackOnQueueFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, fixtureEncryptor{}, true, nil)
	mock.ExpectBegin()
	tx, err := s.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT source_job_id,message_id,nickname,command_text FROM qqc_binding_commands").WithArgs("bot", "member", int64(42), "group").WillReturnRows(sqlmock.NewRows([]string{"source", "message", "nick", "command"}).AddRow("source-job", "original-message", "sender", "/抽奖 3"))
	mock.ExpectQuery("SELECT created_at FROM qqc_bindings").WithArgs("bot", "member", int64(42)).WillReturnRows(sqlmock.NewRows([]string{"created"}).AddRow(time.Now()))
	mock.ExpectExec("INSERT INTO qqc_jobs").WillReturnError(errors.New("queue unavailable"))
	mock.ExpectRollback()
	require.Error(t, enqueueBindingCommand(context.Background(), tx, Config{AppID: "bot"}, 42, "member", "group"))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResumedBalanceUsesPrivateMessageWithoutGroupHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, fixtureEncryptor{}, true, nil)
	c := Config{Enabled: true, AppID: "bot", AppSecret: "secret", AllowedGroups: []string{"group"}, CommandReplyFormat: "text"}
	requests := bindingTestBot(t, s, c, map[string]any{"message_id": 1})
	boundAt := time.Now()
	e := Event{Content: "/我的余额", GroupID: "group"}
	e.Author.MemberID = "member"
	raw, _ := json.Marshal(bindingCommandPayload{Event: e, UserID: 42, BindingAt: boundAt})
	j := Job{ID: "resume", AppID: "bot", GroupID: "group", MemberID: "member", EventType: bindingCommandEvent, CreatedAt: time.Now(), Payload: raw}
	mock.ExpectQuery("SELECT EXISTS.*FROM qqc_bindings").WithArgs(c.AppID, j.MemberID, int64(42), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"valid"}).AddRow(true))
	mock.ExpectQuery("SELECT u.balance,u.id FROM qqc_bindings").WithArgs(c.AppID, j.MemberID).WillReturnRows(sqlmock.NewRows([]string{"balance", "id"}).AddRow(12.34, 42))
	mock.ExpectQuery("SELECT EXISTS.*FROM qqc_bindings").WillReturnRows(sqlmock.NewRows([]string{"valid"}).AddRow(true))
	mock.ExpectExec("UPDATE qqc_jobs SET result_text=").WithArgs("resume", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, s.resumeBindingCommand(context.Background(), c, &j))
	req := <-requests
	require.JSONEq(t, `"send_private_msg"`, string(req["action"]))
	require.Contains(t, string(req["params"]), "12.34")
	require.Contains(t, string(req["params"]), `"user_id":"member"`)
	require.NotContains(t, string(req["params"]), `"type":"at"`)
	require.NoError(t, mock.ExpectationsWereMet()) // no group reference/history INSERT permitted
}

func TestResumedCommandRejectsReplacementBinding(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, fixtureEncryptor{}, true, nil)
	e := Event{GroupID: "group", Content: "/抽奖"}
	e.Author.MemberID = "member"
	raw, _ := json.Marshal(bindingCommandPayload{Event: e, UserID: 42, BindingAt: time.Now()})
	j := Job{AppID: "bot", GroupID: "group", MemberID: "member", Payload: raw}
	mock.ExpectQuery("SELECT EXISTS.*FROM qqc_bindings").WillReturnRows(sqlmock.NewRows([]string{"valid"}).AddRow(false))
	require.ErrorIs(t, s.resumeBindingCommand(context.Background(), Config{AppID: "bot"}, &j), ErrBindingChanged)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResumedCommandImageIsPrivateAndHasNoGroupMention(t *testing.T) {
	s := New(nil, nil, false, nil)
	c := Config{AppID: "bot", AppSecret: "fixture"}
	requests := bindingTestBot(t, s, c, map[string]any{"message_id": 1})
	_, err := s.sendOneBotChunk(context.Background(), c, Job{GroupID: "group", MemberID: "member", PrivateReply: true, ReplyReference: "group-message"}, "签到结果", []byte("fixture-image"))
	require.NoError(t, err)
	request := bindingTestRequest(t, requests)
	require.JSONEq(t, `"send_private_msg"`, string(request["action"]))
	require.Contains(t, string(request["params"]), `"type":"image"`)
	require.Contains(t, string(request["params"]), `"user_id":"member"`)
	require.NotContains(t, string(request["params"]), `"type":"at"`)
	require.NotContains(t, string(request["params"]), `"type":"reply"`)
}
