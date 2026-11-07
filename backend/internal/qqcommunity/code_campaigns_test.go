package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestCampaignAllocationFailureRollsBackCode(t *testing.T) {
	for _, isMySQL := range []bool{false, true} {
		db, m, err := sqlmock.New()
		require.NoError(t, err)
		s := New(db, fixtureEncryptor{}, true, nil)
		s.db.mysql = isMySQL
		c := Config{AppID: "bot", Enabled: true, AllowedGroups: []string{"group"}}
		config, _ := json.Marshal(c)
		campaign, _ := json.Marshal(CodeCampaign{Name: "Test", Type: "lottery_chances", Value: 1, Enabled: true, ClaimMode: "once", StockLimit: 10, CodeValidHours: 24})
		m.ExpectBegin()
		if isMySQL {
			m.ExpectQuery("SELECT id FROM qqc_transaction_lock").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			m.ExpectExec("DO 0").WillReturnResult(sqlmock.NewResult(0, 1))
		} else {
			m.ExpectExec("SELECT pg_advisory_xact_lock_shared").WillReturnResult(sqlmock.NewResult(0, 1))
		}
		m.ExpectQuery("SELECT value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(config)))
		if isMySQL {
			m.ExpectExec("DO 0").WillReturnResult(sqlmock.NewResult(0, 1))
		} else {
			m.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
		}
		m.ExpectQuery("SELECT b.user_id FROM qqc_bindings").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
		m.ExpectQuery("SELECT c.id,c.config_json FROM qqc_code_campaign_groups").WillReturnRows(sqlmock.NewRows([]string{"id", "config"}).AddRow("campaign", string(campaign)))
		m.ExpectQuery("SELECT id FROM qqc_code_claims").WillReturnError(sql.ErrNoRows)
		m.ExpectQuery("SELECT COUNT.*FROM qqc_code_claims").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		if isMySQL {
			m.ExpectExec("INSERT INTO redeem_codes").WillReturnResult(sqlmock.NewResult(7, 1))
		} else {
			m.ExpectQuery("INSERT INTO redeem_codes").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
		}
		m.ExpectExec("INSERT INTO qqc_code_claims").WillReturnError(errors.New("claim failed"))
		m.ExpectRollback()
		_, err = s.allocateCampaignCode(context.Background(), c, Job{GroupID: "group", MemberID: "member"}, time.Now())
		require.ErrorContains(t, err, "claim failed")
		require.NoError(t, m.ExpectationsWereMet())
		_ = db.Close()
	}
}

func TestCampaignValidationAndPrivateCommand(t *testing.T) {
	c := CodeCampaign{Name: "test", Type: "lottery_chances", Value: 1, ClaimMode: "once", StockLimit: 5, CodeValidHours: 24}
	require.NoError(t, validateCodeCampaign(c))
	for _, v := range []float64{0, -1, 1.5, 1001, math.NaN(), math.Inf(1)} {
		c.Value = v
		require.Error(t, validateCodeCampaign(c))
	}
	c.Type = "balance"
	c.Value = .00000001
	require.NoError(t, validateCodeCampaign(c))
	c.Value = .000000001
	require.Error(t, validateCodeCampaign(c))
	cmd, arg := command("/领码")
	require.Equal(t, "领码", cmd)
	require.Empty(t, arg)
	require.True(t, requiresBinding(cmd))
	require.True(t, isBuiltinCommand(cmd))
	require.True(t, privateCommandText("/领码"))
	require.Error(t, validateCustomCommands([]CustomCommand{{Command: "/领码", Reply: "override", Enabled: true}}))
	require.NotContains(t, campaignErrorMessage(errors.New("SQL secret code")), "secret")
}
func TestCampaignDateUsesBeijingHalfOpenDay(t *testing.T) {
	from, to, err := activityDateRange("2026-11-07", time.Now())
	require.NoError(t, err)
	require.Equal(t, "2026-11-06T16:00:00Z", from.Format(time.RFC3339))
	require.Equal(t, 24*time.Hour, to.Sub(from))
	_, _, err = activityDateRange("2026-02-30", time.Now())
	require.Error(t, err)
	from, _, err = activityDateRange("", time.Date(2026, 11, 6, 16, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, "2026-11-07", day(from))
}
func TestCampaignCommandRejectsParametersWithoutAllocation(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, fixtureEncryptor{}, true, nil)
	c := Config{Enabled: true, AppID: "bot", AppSecret: "secret", AllowedGroups: []string{"group"}, CommandReplyFormat: "text"}
	requests := bindingTestBot(t, s, c, map[string]any{"message_id": 1})
	j := Job{ID: "param-job", AppID: c.AppID, GroupID: "group", MemberID: "member", MessageID: "msg", CreatedAt: time.Now(), EventType: groupAtMessageCreate, TransientReply: true}
	e := Event{ID: "msg", GroupID: "group", Content: "/领码 余额"}
	e.Author.MemberID = "member"
	require.NoError(t, s.message(context.Background(), c, &j, e))
	req := bindingTestRequest(t, requests)
	require.Contains(t, string(req["params"]), "用法：/领码")
	require.NoError(t, m.ExpectationsWereMet())
}
