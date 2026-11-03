package qqcommunity

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func autonomousConversationFixture(t *testing.T) (*Service, sqlmock.Sqlmock, Config) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	c := DefaultConfig()
	c.Enabled, c.AppID, c.AllowedGroups = true, "bot", []string{"group"}
	c.Autonomous.Enabled, c.Autonomous.Groups = true, []string{"group"}
	c.Autonomous.CooldownSeconds, c.Autonomous.MemberCooldownSeconds = 5, 5
	c.Autonomous.MinMessages, c.Autonomous.SamplePercent = 1, 100
	c.Autonomous.StartHour, c.Autonomous.EndHour = 0, 0
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock_shared").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
	return New(db, nil, false, nil), mock, c
}

func expectAutonomousConversationState(mock sqlmock.Sqlmock, latestID string) {
	mock.ExpectExec("INSERT INTO qqc_autonomous_state").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT messages,last_message_id").WillReturnRows(sqlmock.NewRows([]string{
		"messages", "last_message_id", "last_message_at", "last_text_hash", "last_candidate_at", "last_reply_at", "last_reply_member", "last_reply_kind", "last_reply_trigger", "last_reply_counter",
	}).AddRow(6, latestID, time.Now(), "previous-text", time.Now().Add(-7*time.Second), time.Now().Add(-7*time.Second), "member", "chat", "name", 5))
}

func TestAutonomousQueuesShortChatAfterFiveSecondCooldown(t *testing.T) {
	s, mock, c := autonomousConversationFixture(t)
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	expectAutonomousConversationState(mock, "previous-message")
	mock.ExpectExec("INSERT INTO qqc_autonomous_seen").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE qqc_autonomous_state SET messages").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COUNT.*FROM qqc_jobs").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT COUNT.*FROM qqc_jobs").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO qqc_jobs").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE qqc_autonomous_state SET last_candidate_at").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO qqc_autonomous_decisions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	e := Event{ID: "greeting", GroupID: "group", Content: "你好"}
	e.Author.MemberID = "member"
	raw, err := json.Marshal(e)
	require.NoError(t, err)
	require.NoError(t, s.routeAutonomous(context.Background(), c, Payload{Type: groupMessageCreate, Data: raw}, e, true))
	require.NoError(t, mock.ExpectationsWereMet(), "a seven-second-old candidate must not impose the old ten-second gate")
}

func TestAutonomousAnswerSurvivesNewMessagesAndHonorsMemberCooldown(t *testing.T) {
	for _, recent := range []int{0, 1} {
		t.Run(boolText(recent > 0), func(t *testing.T) {
			s, mock, c := autonomousConversationFixture(t)
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			expectAutonomousConversationState(mock, "another-members-new-message")
			mock.ExpectQuery("SELECT COUNT.*FROM qqc_jobs").
				WithArgs("bot", "group", "member", 5, day(time.Now()), 5).
				WillReturnRows(sqlmock.NewRows([]string{"member_recent", "hourly", "daily", "group_recent"}).AddRow(recent, 0, 0, 0))
			if recent == 0 {
				mock.ExpectExec("UPDATE qqc_jobs SET reply_reserved_at").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			j := Job{ID: "job", AppID: "bot", GroupID: "group", MemberID: "member", MessageID: "original-question", CreatedAt: time.Now(), TriggerReason: "question", ResultText: "这是原问题的回答。"}
			_, reason, err := s.reserveAutonomous(context.Background(), c, j, true)
			require.NoError(t, err)
			if recent == 0 {
				require.Empty(t, reason, "later messages must not cancel the original answer")
			} else {
				require.Equal(t, "member_cooldown", reason)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
