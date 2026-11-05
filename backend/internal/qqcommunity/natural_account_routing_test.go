package qqcommunity

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestNaturalBalanceWithoutMentionEnqueuesBusinessJob(t *testing.T) {
	s, mock, c, e := observationFixture(t)
	c.HistoryMessages, c.Autonomous.Enabled = 0, false
	e.ObserveOnly = false
	e.Message[0].Data["text"] = json.RawMessage(`"查下我的余额"`)
	expectObservationConfig(mock, c)
	mock.ExpectExec("INSERT INTO qqc_groups").WithArgs("123", "456", groupMessageCreate).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COUNT.*FROM qqc_jobs").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO qqc_jobs").WithArgs(sqlmock.AnyArg(), "123", groupAtMessageCreate+":42", groupMessageCreate, "456", "789", "42", sqlmock.AnyArg(), sqlmock.AnyArg(), "").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, s.enqueueOneBot(context.Background(), e))
	require.NoError(t, mock.ExpectationsWereMet(), "a received personal query must become a business job even when chat and history are disabled")
}

func TestNaturalBalanceWithoutMentionReturnsBoundSendersBalance(t *testing.T) {
	for _, text := range []string{"查下我的余额", "我让你查下我的余额"} {
		t.Run(text, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			s := New(db, fixtureEncryptor{}, true, nil)
			c := DefaultConfig()
			c.Enabled, c.AppID, c.AppSecret, c.AllowedGroups = true, "123", "fixture", []string{"456"}
			c.ChatDisabled, c.HistoryMessages, c.CommandReplyFormat = true, 0, "text"
			requests := bindingTestBot(t, s, c, map[string]any{"message_id": 1})
			e := Event{ID: "42", GroupID: "456", Content: text}
			e.Author.MemberID, e.Author.Username = "789", "不是身份凭据的昵称"
			j := Job{ID: "balance", AppID: c.AppID, GroupID: "456", MemberID: "789", MessageID: "42", EventType: groupMessageCreate, CreatedAt: time.Now()}
			mock.ExpectQuery("SELECT b.user_id FROM qqc_bindings").WithArgs(c.AppID, "789").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
			mock.ExpectQuery("SELECT u.balance,u.id FROM qqc_bindings").WithArgs(c.AppID, "789").WillReturnRows(sqlmock.NewRows([]string{"balance", "id"}).AddRow(12.34, 42))
			mock.ExpectExec("INSERT INTO qqc_message_references").WillReturnResult(sqlmock.NewResult(0, 1))
			require.NoError(t, s.message(context.Background(), c, &j, e))
			request := bindingTestRequest(t, requests)
			require.JSONEq(t, `"send_group_msg"`, string(request["action"]))
			require.Contains(t, string(request["params"]), "12.34")
			require.Contains(t, string(request["params"]), `"group_id":"456"`)
			require.True(t, j.CommandReply)
			require.NoError(t, mock.ExpectationsWereMet(), "the authenticated sender's binding supplies identity; no model or chat quota is needed")
		})
	}
}

func TestUnmentionedPersonalQueriesDoNotExecuteQuotesMediaOrForeignMentions(t *testing.T) {
	valid := Event{Content: "查下我的余额"}
	require.True(t, unmentionedGroupCommand(valid))
	require.True(t, shouldQueueGroupMessage(groupMessageCreate, valid))
	quoted := valid
	quoted.MessageReference.MessageID = "another-message"
	attached := valid
	attached.Attachments = []Attachment{{ContentType: "image/png", URL: "https://qpic.cn/image"}}
	mentioned := valid
	mentioned.Mentions = []Mention{{ID: "another-member"}}
	for _, e := range []Event{quoted, attached, mentioned, {Content: "不要查询我的余额"}, {Content: "帮我看一下他的余额"}, {Content: "余额是什么"}} {
		require.False(t, unmentionedGroupCommand(e), e.Content)
	}
}
