package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestCodeCampaignMySQL(t *testing.T) {
	dsn := os.Getenv("QQC_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL DSN not configured")
	}
	dbc, err := mysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "qqc_test", dbc.DBName)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(10)
	ctx := context.Background()
	s := New(db, fixtureEncryptor{}, true, nil)
	c := DefaultConfig()
	c.Enabled = true
	c.AppID = randomID()
	c.AppSecret = "fixture-token"
	c.AllowedGroups = []string{"group1", "group2"}
	c.CommandReplyFormat = "text"
	var oldConfig string
	oldErr := db.QueryRow("SELECT value FROM settings WHERE `key`=?", configKey).Scan(&oldConfig)
	raw, _ := json.Marshal(c)
	_, err = db.Exec("INSERT INTO settings (`key`,value,updated_at) VALUES(?,?,NOW()) ON DUPLICATE KEY UPDATE value=VALUES(value)", configKey, string(raw))
	require.NoError(t, err)
	defer func() {
		if oldErr == nil {
			_, _ = db.Exec("UPDATE settings SET value=? WHERE `key`=?", oldConfig, configKey)
		} else {
			_, _ = db.Exec("DELETE FROM settings WHERE `key`=?", configKey)
		}
	}()
	uids := []int64{}
	defer func() {
		_, e := db.Exec(`DELETE FROM redeem_codes WHERE id IN (SELECT redeem_code_id FROM qqc_code_claims WHERE app_id=?)`, c.AppID)
		require.NoError(t, e)
		for _, table := range []string{"qqc_code_claims", "qqc_code_campaign_groups", "qqc_code_campaigns", "qqc_bindings"} {
			_, e = db.Exec("DELETE FROM "+table+" WHERE app_id=?", c.AppID)
			require.NoError(t, e)
		}
		for _, uid := range uids {
			for _, table := range []string{"qqc_lottery_chance_grants", "qqc_lottery_draws", "qqc_rewards"} {
				_, e = db.Exec("DELETE FROM "+table+" WHERE user_id=?", uid)
				require.NoError(t, e)
			}
			_, e = db.Exec("DELETE FROM users WHERE id=?", uid)
			require.NoError(t, e)
		}
	}()
	for i := 0; i < 3; i++ {
		res, e := db.Exec(`INSERT INTO users(email,password_hash,role,status,balance,created_at,updated_at,notes,balance_notify_extra_emails) VALUES(?,'unused','user','active',12,NOW(),NOW(),'','[]')`, fmt.Sprintf("campaign-%s-%d@example.test", c.AppID, i))
		require.NoError(t, e)
		uid, e := res.LastInsertId()
		require.NoError(t, e)
		uids = append(uids, uid)
		_, e = db.Exec(`INSERT INTO qqc_bindings(app_id,member_id,user_id,group_id) VALUES(?,?,?,'group1')`, c.AppID, fmt.Sprintf("member%d", i), uid)
		require.NoError(t, e)
	}
	campaign, err := s.SaveCodeCampaign(ctx, CodeCampaign{Name: "Fixture", Type: "lottery_chances", Value: 3, Enabled: true, Groups: c.AllowedGroups, ClaimMode: "once", StockLimit: 2, CodeValidHours: 24})
	require.NoError(t, err)
	job := Job{AppID: c.AppID, MemberID: "member0", GroupID: "group1"}
	results := make(chan CodeClaim, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, e := s.allocateCampaignCode(ctx, c, job, time.Now())
			results <- claim
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	var claim CodeClaim
	for r := range results {
		if claim.ID != "" {
			require.Equal(t, claim.ID, r.ID)
			require.Equal(t, claim.Code, r.Code)
		}
		claim = r
	}
	require.Len(t, claim.Code, 32)
	job.GroupID = "group2"
	same, err := s.allocateCampaignCode(ctx, c, job, time.Now())
	require.NoError(t, err)
	require.Equal(t, claim.ID, same.ID)
	var winners atomic.Int32
	var unexpected atomic.Int32
	for i := 1; i <= 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.allocateCampaignCode(ctx, c, Job{MemberID: fmt.Sprintf("member%d", i), GroupID: "group1"}, time.Now())
			if e == nil {
				winners.Add(1)
			} else if e.Error() != "本次活动兑换码已领完" {
				unexpected.Add(1)
			}
		}(i)
	}
	wg.Wait()
	require.EqualValues(t, 1, winners.Load())
	require.Zero(t, unexpected.Load())
	list, err := s.CodeCampaigns(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.EqualValues(t, 2, list[0].Claimed)
	claims, err := s.CodeClaims(ctx, campaign.ID, "", 1, 20, false)
	require.NoError(t, err)
	require.Equal(t, 2, claims.Total)
	encoded, err := json.Marshal(claims)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), claim.Code)
	modified := campaign
	modified.Value = 5
	_, err = s.SaveCodeCampaign(ctx, modified)
	require.ErrorContains(t, err, "不可变更")
	campaign.Enabled = false
	campaign, err = s.SaveCodeCampaign(ctx, campaign)
	require.NoError(t, err)
	same, err = s.allocateCampaignCode(ctx, c, job, time.Now())
	require.NoError(t, err)
	require.Equal(t, claim.ID, same.ID)
	_, err = s.SaveCodeCampaign(ctx, modified)
	require.ErrorIs(t, err, ErrConfigConflict)
	var fail atomic.Bool
	fail.Store(true)
	requests := bindingTestBotWithAck(t, s, c, func(map[string]json.RawMessage) map[string]any {
		if fail.Load() {
			return map[string]any{}
		}
		return map[string]any{"message_id": 77}
	})
	msg, err := s.sendCampaignClaim(ctx, c, claim.ID)
	require.NoError(t, err)
	require.Contains(t, msg, "未确认")
	request := bindingTestRequest(t, requests)
	require.Equal(t, `"send_private_msg"`, string(request["action"]))
	require.Contains(t, string(request["params"]), claim.Code)
	msg, err = s.sendCampaignClaim(ctx, c, claim.ID)
	require.NoError(t, err)
	require.Contains(t, msg, "30秒")
	_, err = db.Exec(`UPDATE qqc_code_claims SET last_attempt_at=DATE_SUB(NOW(),INTERVAL 31 SECOND) WHERE id=?`, claim.ID)
	require.NoError(t, err)
	fail.Store(false)
	msg, err = s.sendCampaignClaim(ctx, c, claim.ID)
	require.NoError(t, err)
	require.Contains(t, msg, "已私聊")
	request = bindingTestRequest(t, requests)
	require.Contains(t, string(request["params"]), claim.Code)
	var attempts int
	var state string
	require.NoError(t, db.QueryRow(`SELECT attempts,delivery_status FROM qqc_code_claims WHERE id=?`, claim.ID).Scan(&attempts, &state))
	require.Equal(t, 2, attempts)
	require.Equal(t, "sent", state)
	// A failed resend must not erase an earlier successful delivery timestamp.
	var firstSent time.Time
	require.NoError(t, db.QueryRow(`SELECT sent_at FROM qqc_code_claims WHERE id=?`, claim.ID).Scan(&firstSent))
	_, err = db.Exec(`UPDATE qqc_code_claims SET last_attempt_at=DATE_SUB(NOW(),INTERVAL 31 SECOND) WHERE id=?`, claim.ID)
	require.NoError(t, err)
	fail.Store(true)
	msg, err = s.sendCampaignClaim(ctx, c, claim.ID)
	require.NoError(t, err)
	require.Contains(t, msg, "未确认")
	request = bindingTestRequest(t, requests)
	require.Contains(t, string(request["params"]), claim.Code)
	var retainedSent time.Time
	require.NoError(t, db.QueryRow(`SELECT sent_at FROM qqc_code_claims WHERE id=?`, claim.ID).Scan(&retainedSent))
	require.Equal(t, firstSent, retainedSent)
	list, err = s.CodeCampaigns(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, list[0].Sent)
	// Rebinding the original account to another QQ cannot mint another code.
	_, err = db.Exec(`UPDATE qqc_bindings SET member_id='changed-member' WHERE app_id=? AND user_id=?`, c.AppID, uids[0])
	require.NoError(t, err)
	_, err = s.allocateCampaignCode(ctx, c, Job{MemberID: "changed-member", GroupID: "group1"}, time.Now())
	require.ErrorContains(t, err, "不能换绑")
	_, err = s.sendCampaignClaim(ctx, c, claim.ID)
	require.ErrorContains(t, err, "绑定账号已变化")
	_, err = db.Exec(`UPDATE qqc_bindings SET member_id='member0' WHERE app_id=? AND user_id=?`, c.AppID, uids[0])
	require.NoError(t, err)
	require.NoError(t, s.DisableCampaignCode(ctx, claim.ID))
	same, err = s.allocateCampaignCode(ctx, c, job, time.Now())
	require.NoError(t, err)
	require.Equal(t, "disabled", same.Status)
	// Deleting the underlying code keeps its claim and reward details auditable.
	_, err = db.Exec(`DELETE FROM redeem_codes WHERE id=?`, claim.CodeID)
	require.NoError(t, err)
	same, err = s.allocateCampaignCode(ctx, c, job, time.Now())
	require.NoError(t, err)
	require.Equal(t, claim.ID, same.ID)
	require.Equal(t, "deleted", same.Status)
	require.Equal(t, "lottery_chances", same.Type)
	require.Equal(t, float64(3), same.Value)
	for _, uid := range uids {
		var balance float64
		var grants int
		require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id=?`, uid).Scan(&balance))
		require.Equal(t, 12.0, balance)
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qqc_lottery_chance_grants WHERE user_id=?`, uid).Scan(&grants))
		require.Zero(t, grants)
	}
	// A second daily campaign resets at Beijing midnight, never within one day.
	daily, err := s.SaveCodeCampaign(ctx, CodeCampaign{Name: "Daily", Type: "balance", Value: 1, Enabled: true, Groups: []string{"group2"}, ClaimMode: "daily", StockLimit: 10, CodeValidHours: 24})
	require.NoError(t, err)
	before := time.Date(2026, 11, 7, 15, 59, 59, 0, time.UTC)
	a, err := s.allocateCampaignCode(ctx, c, job, before)
	require.NoError(t, err)
	b, err := s.allocateCampaignCode(ctx, c, job, before)
	require.NoError(t, err)
	require.Equal(t, a.ID, b.ID)
	b, err = s.allocateCampaignCode(ctx, c, job, before.Add(time.Second))
	require.NoError(t, err)
	require.NotEqual(t, a.ID, b.ID)
	require.Equal(t, daily.ID, b.CampaignID)
	// Same request ID cannot create a duplicate campaign after an uncertain save.
	daily.Version = 0
	_, err = s.SaveCodeCampaign(ctx, daily)
	require.ErrorIs(t, err, ErrConfigConflict)
	// Statistics include unbound site users, exclude disabled holders, and use
	// the same actual-CNY calculation as the member's current lottery state.
	filter := ActivityStatsFilter{Date: day(time.Now()), Search: c.AppID, Page: 1, PageSize: 20}
	baseline, err := s.ActivityStats(ctx, filter, false)
	require.NoError(t, err)
	for i := 1; i <= 2; i++ {
		_, err = db.Exec(`INSERT INTO qqc_lottery_chance_grants(source_key,user_id,chances) VALUES(?,?,5)`, c.AppID+fmt.Sprint(i), uids[i])
		require.NoError(t, err)
	}
	_, err = db.Exec(`UPDATE users SET status='disabled' WHERE id=?`, uids[1])
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM qqc_bindings WHERE app_id=? AND user_id=?`, c.AppID, uids[2])
	require.NoError(t, err)
	for i, app := range []string{c.AppID, "website"} {
		_, err = db.Exec(`INSERT INTO qqc_rewards(reward_key,app_id,group_id,member_id,user_id,kind,amount) VALUES(?,?,'group1',?,?,'checkin',0.1)`, c.AppID+fmt.Sprint(i), app, fmt.Sprintf("member%d", i), uids[i])
		require.NoError(t, err)
	}
	stats, err := s.ActivityStats(ctx, filter, false)
	require.NoError(t, err)
	require.Equal(t, 3, stats.Total)
	require.Equal(t, baseline.Summary.CheckinUsers+2, stats.Summary.CheckinUsers)
	require.Equal(t, baseline.Summary.ChanceUsers+1, stats.Summary.ChanceUsers)
	require.Equal(t, baseline.Summary.RemainingChances+5, stats.Summary.RemainingChances)
	filter.Filter = "chances"
	stats, err = s.ActivityStats(ctx, filter, false)
	require.NoError(t, err)
	require.Len(t, stats.Items, 1)
	require.Equal(t, uids[2], stats.Items[0].UserID)
	require.Empty(t, stats.Items[0].Members)
	state2, err := s.LotteryState(ctx, uids[2])
	require.NoError(t, err)
	require.Equal(t, state2, stats.Items[0].LotteryState)
}
