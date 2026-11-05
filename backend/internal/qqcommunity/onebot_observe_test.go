package qqcommunity

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func observationFixture(t *testing.T) (*Service, sqlmock.Sqlmock, Config, obEvent) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	c := DefaultConfig()
	c.Enabled, c.AppID, c.AllowedGroups = true, "123", []string{"456"}
	c.HistoryMessages = 12
	c.Autonomous.Enabled, c.Autonomous.Groups = true, []string{"456"}
	e := obEvent{ObserveOnly: true, SelfID: "123", UserID: "789", GroupID: "456", MessageID: "42", PostType: "message", MessageType: "group", Time: time.Now().Unix()}
	e.Sender.Role = "member"
	e.Message = []obSegment{{Type: "text", Data: map[string]json.RawMessage{"text": json.RawMessage(`"你好"`)}}}
	return New(db, nil, false, nil), mock, c, e
}

func expectObservationConfig(mock sqlmock.Sqlmock, c Config) {
	raw, _ := json.Marshal(c)
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
}

func TestOneBotObservationRecordsHistoryWithoutAnyChatOrBusinessJob(t *testing.T) {
	s, mock, c, e := observationFixture(t)
	expectObservationConfig(mock, c)
	mock.ExpectExec("INSERT INTO qqc_groups").WithArgs("123", "456", groupMessageCreate).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock_shared").WillReturnResult(sqlmock.NewResult(0, 1))
	expectObservationConfig(mock, c)
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM qqc_group_messages").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO qqc_group_messages").WithArgs("123", "456", "user:42", "user", sqlmock.AnyArg(), sqlmock.AnyArg(), "789").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM qqc_group_messages").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	require.NoError(t, s.enqueueOneBot(context.Background(), e))
	require.NoError(t, mock.ExpectationsWereMet(), "observation must record history and never enter autonomous or business queues")
}

func TestOneBotObservationStillRunsModeration(t *testing.T) {
	s, mock, c, e := observationFixture(t)
	c.Moderation.Enabled, c.Moderation.Groups = true, []string{"456"}
	expectObservationConfig(mock, c)
	failure := errors.New("moderation database unavailable")
	mock.ExpectBegin().WillReturnError(failure)
	require.ErrorIs(t, s.enqueueOneBot(context.Background(), e), failure)
	require.NoError(t, mock.ExpectationsWereMet(), "the observation flag cannot bypass moderation")
}
