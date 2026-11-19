package qqcommunity

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func ingressRequest(t *testing.T, s *Service, c Config, phase, token string, e obEvent) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"phase": phase, "event": e})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/runtime/ingress", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Self-ID", c.AppID)
	w := httptest.NewRecorder()
	s.ServeRuntimeIngress(w, req)
	return w
}

func TestRuntimeIngressPrivateUsesBindingSessionNotEnglishWordLength(t *testing.T) {
	for _, tc := range []struct {
		text              string
		session, business bool
	}{
		{"OpenAI", false, false}, {"DeepSeek", false, false}, {"确认一下今天的天气", false, false},
		{"/取消", false, true}, {"取消", true, true}, {"abc123", true, true},
		{"OpenAI", true, true}, {"查下我的余额", false, true}, {"余额是什么", false, false},
	} {
		t.Run(tc.text+":"+map[bool]string{true: "binding", false: "chat"}[tc.session], func(t *testing.T) {
			s, mock, c, e := observationFixture(t)
			s.encryptor = previewEncryptor{}
			c.AppSecret = "secret"
			c.RuntimeChat.Enabled, c.RuntimeChat.PrivateChatEnabled = true, true
			c.RuntimeChat.PrivateUsers = []string{"789"}
			e.MessageType = "private"
			e.Message[0].Data["text"], _ = json.Marshal(tc.text)
			expectObservationConfig(mock, c)
			rows := sqlmock.NewRows([]string{"request_id", "group_id", "nickname"})
			if tc.session {
				rows.AddRow("binding-secret", "456", "member")
			}
			mock.ExpectQuery("SELECT request_id,group_id,nickname").WithArgs(c.AppID, "789").WillReturnRows(rows)
			w := ingressRequest(t, s, c, "route", "secret", e)
			require.Equal(t, 200, w.Code)
			var out map[string]bool
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
			require.True(t, out["allowed"])
			require.Equal(t, tc.business, out["business"])
			require.NotContains(t, w.Body.String(), "binding-secret")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRuntimeIngressRejectsUntrustedIdentityAndUnavailableModeration(t *testing.T) {
	for _, name := range []string{"auth", "bot", "sender", "message", "phase", "route-failure", "delivery-blocked", "binding-disabled-chat"} {
		t.Run(name, func(t *testing.T) {
			s, mock, c, e := observationFixture(t)
			s.encryptor = previewEncryptor{}
			c.AppSecret = "secret"
			c.RuntimeChat.Enabled = true
			token, phase, status := "secret", "route", 400
			switch name {
			case "auth":
				token, status = "wrong", 401
			case "bot":
				e.SelfID = "999"
			case "sender":
				e.UserID = obID(c.AppID)
			case "message":
				e.MessageID = "forged"
			case "phase":
				phase = "unknown"
			case "route-failure", "delivery-blocked":
				c.Moderation.Enabled, c.Moderation.Groups = true, []string{"456"}
				status = 503
				if name == "delivery-blocked" {
					phase, status = "delivery", 200
				}
			case "binding-disabled-chat":
				c.RuntimeChat.Enabled = false
				e.MessageType, status = "private", 200
			}
			expectObservationConfig(mock, c)
			if name == "route-failure" {
				mock.ExpectBegin().WillReturnError(errors.New("database unavailable"))
			}
			if name == "delivery-blocked" {
				mock.ExpectQuery("SELECT EXISTS.*qqc_moderation_events").WithArgs("123", c.Sandbox, "456", "42").WillReturnRows(sqlmock.NewRows([]string{"blocked"}).AddRow(true))
			}
			if name == "binding-disabled-chat" {
				mock.ExpectQuery("SELECT request_id,group_id,nickname").WillReturnRows(sqlmock.NewRows([]string{"id", "group", "name"}).AddRow("binding", "456", "name"))
			}
			w := ingressRequest(t, s, c, phase, token, e)
			require.Equal(t, status, w.Code, w.Body.String())
			if name == "delivery-blocked" {
				require.JSONEq(t, `{"allowed":false,"reason":"moderation_blocked"}`, w.Body.String())
			}
			if name == "binding-disabled-chat" {
				require.JSONEq(t, `{"allowed":true,"business":true}`, w.Body.String())
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestPrivateAccountQueryGrammarAndQueueIdentity(t *testing.T) {
	s, mock, c, e := observationFixture(t)
	c.RuntimeChat.Enabled, c.RuntimeChat.PrivateChatEnabled = true, true
	c.RuntimeChat.PrivateUsers = []string{"789"}
	e.MessageType = "private"
	for text, want := range map[string]string{"查下我的余额": "我的余额", "/今日用量": "今日用量", "抽奖次数": "抽奖次数", "签到": "", "转账 999 10": "", "查他的余额": "", "余额是什么": ""} {
		e.Message[0].Data["text"], _ = json.Marshal(text)
		require.Equal(t, want, privateAccountCommand(e), text)
	}
	e.Message[0].Data["text"] = json.RawMessage(`"查下我的余额"`)
	e.Message = append(e.Message, obSegment{Type: "reply"})
	require.Empty(t, privateAccountCommand(e), "引用内容不能被当成本人查询")
	e.Message = e.Message[:1]
	expectObservationConfig(mock, c)
	mock.ExpectQuery("SELECT request_id,group_id,nickname").WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	converted := oneBotMessageEvent(e)
	converted.Content = "/我的余额"
	raw, err := json.Marshal(converted)
	require.NoError(t, err)
	require.Empty(t, converted.GroupID, "临时私聊附带的群号不得变成群查询")
	mock.ExpectExec("INSERT INTO qqc_jobs.*ON CONFLICT.*DO NOTHING").WithArgs(sqlmock.AnyArg(), c.AppID, runtimePrivateQueryCreate+":789:42", runtimePrivateQueryCreate, "789", "42", string(raw)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, s.enqueueOneBot(context.Background(), e))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRuntimePrivateQueryChecksBindingAndRevocationBeforeSending(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		t.Run(map[bool]string{false: "delivered", true: "revoked"}[revoked], func(t *testing.T) {
			s, mock, c, e := observationFixture(t)
			c.RuntimeChat.Enabled, c.RuntimeChat.PrivateChatEnabled = true, true
			c.RuntimeChat.PrivateUsers = []string{"789"}
			e.MessageType = "private"
			data := oneBotMessageEvent(e)
			data.Content = "/我的余额"
			raw, _ := json.Marshal(data)
			j := Job{ID: "query", AppID: c.AppID, MemberID: "789", MessageID: "42", EventType: runtimePrivateQueryCreate, Payload: raw, PrivateReply: true, CreatedAt: time.Now()}
			boundAt := time.Now().Add(-time.Hour)
			requests := bindingTestBot(t, s, c, map[string]any{"message_id": 1234})
			mock.ExpectQuery("SELECT b.user_id,b.created_at").WithArgs(c.AppID, "789").WillReturnRows(sqlmock.NewRows([]string{"id", "at"}).AddRow(7, boundAt))
			expectObservationConfig(mock, c)
			mock.ExpectQuery("SELECT u.balance,u.id").WithArgs(c.AppID, "789").WillReturnRows(sqlmock.NewRows([]string{"balance", "id"}).AddRow(12.5, 7))
			if revoked {
				c.RuntimeChat.PrivateUsers = []string{}
			}
			expectObservationConfig(mock, c)
			if !revoked {
				mock.ExpectQuery("SELECT EXISTS.*qqc_bindings").WithArgs(c.AppID, "789", int64(7), boundAt).WillReturnRows(sqlmock.NewRows([]string{"valid"}).AddRow(true))
				mock.ExpectExec("UPDATE qqc_jobs SET result_text").WithArgs("query", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			err := s.runRuntimePrivateQuery(context.Background(), c, j)
			if revoked {
				require.ErrorIs(t, err, ErrDisabled)
			}
			if !revoked {
				require.NoError(t, err)
				req := bindingTestRequest(t, requests)
				require.JSONEq(t, `"send_private_msg"`, string(req["action"]))
				require.Contains(t, string(req["params"]), `"user_id":"789"`)
				require.Contains(t, string(req["params"]), "12.50")
				require.NotContains(t, string(req["params"]), "group_id")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestJobLanesReserveBusinessSlotApartFromImages(t *testing.T) {
	for _, lane := range []string{"image", "business"} {
		t.Run(lane, func(t *testing.T) {
			s, mock, _, _ := observationFixture(t)
			operator := "="
			if lane == "business" {
				operator = "<>"
			}
			mock.ExpectBegin()
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT COUNT.*status='running' AND kind" + operator + "'image'").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			mock.ExpectQuery("SELECT .*status='queued' AND q.kind" + operator + "'image'").WillReturnRows(sqlmock.NewRows(strings.Split(jobColumns, ",")))
			mock.ExpectRollback()
			_, err := s.claimJobLane(context.Background(), lane)
			require.ErrorIs(t, err, sql.ErrNoRows)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRuntimePrivateQueryFailureIsNotSilentlyCompleted(t *testing.T) {
	s, mock, c, e := observationFixture(t)
	c.RuntimeChat.Enabled, c.RuntimeChat.PrivateChatEnabled = true, true
	c.RuntimeChat.PrivateUsers = []string{"789"}
	e.MessageType = "private"
	data := oneBotMessageEvent(e)
	data.Content = "/我的余额"
	raw, _ := json.Marshal(data)
	j := Job{ID: "query", AppID: c.AppID, MemberID: "789", MessageID: "42", EventType: runtimePrivateQueryCreate, Payload: raw, PrivateReply: true, CreatedAt: time.Now()}
	requests := bindingTestBot(t, s, c, map[string]any{"message_id": 1234})
	failure := errors.New("database unavailable")
	mock.ExpectQuery("SELECT b.user_id,b.created_at").WillReturnError(failure)
	expectObservationConfig(mock, c)
	require.ErrorIs(t, s.runRuntimePrivateQuery(context.Background(), c, j), failure)
	req := bindingTestRequest(t, requests)
	require.JSONEq(t, `"send_private_msg"`, string(req["action"]))
	require.Contains(t, string(req["params"]), "未取得账户数据")
	require.NotContains(t, string(req["params"]), failure.Error())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRuntimePrivateQueryRetainsExistingQueueCapacity(t *testing.T) {
	s, mock, c, e := observationFixture(t)
	c.RuntimeChat.Enabled, c.RuntimeChat.PrivateChatEnabled = true, true
	c.RuntimeChat.PrivateUsers = []string{"789"}
	e.MessageType = "private"
	data := oneBotMessageEvent(e)
	data.Content = "/我的余额"
	raw, _ := json.Marshal(data)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(200))
	mock.ExpectRollback()
	require.ErrorContains(t, s.enqueueRuntimePrivateQuery(context.Background(), c, data, raw), "队列已满")
	require.NoError(t, mock.ExpectationsWereMet())
}
