package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type fixtureEncryptor struct{}

type fixtureEmail struct{}

func (fixtureEmail) FrontendURL(context.Context) string { return "https://example.test" }
func (fixtureEmail) LookupAccount(_ context.Context, email string) (int64, string, error) {
	if email == "qqc-two@example.test" {
		return 990002, email, nil
	}
	return 0, "", fmt.Errorf("unknown fixture")
}
func (fixtureEmail) SendEmail(context.Context, string, string, string) error { return nil }

type fixtureSnapshots struct{ revision int }

func (p *fixtureSnapshots) PriceSnapshot(_ context.Context, now time.Time) (PriceSnapshot, error) {
	return PriceSnapshot{NextCheck: now.Add(15 * time.Second), States: []PriceState{{Key: "fixture", Kind: "group", Name: "测试分组", Status: "active", Fields: []PriceField{{Key: "rate", Label: "倍率", Value: fmt.Sprint(p.revision)}}}}}, nil
}
func (p *fixtureSnapshots) HealthSnapshot(_ context.Context, now time.Time) (HealthSnapshot, error) {
	return HealthSnapshot{UpdatedAt: now, Complete: true, Groups: []HealthMetric{{Key: "fixture", Name: "测试分组", Requests: 100, ErrorRate: .9}}}, nil
}

func (fixtureEncryptor) Encrypt(s string) (string, error) { return s, nil }
func (fixtureEncryptor) Decrypt(s string) (string, error) { return s, nil }

func TestCommunityMySQL(t *testing.T) {
	dsn := os.Getenv("QQC_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL DSN not configured")
	}
	cfg, err := mysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "qqc_test", cfg.DBName, "test must never target a production database")
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx := context.Background()
	// This database contains schema only and is disposable. Test account IDs
	// and all funds are isolated from the live site.
	for _, q := range []string{
		`INSERT INTO users(id,email,password_hash,role,status,balance,created_at,updated_at,notes,balance_notify_extra_emails) VALUES(990001,'qqc-one@example.test','unused','user','active',10,NOW(),NOW(),'','[]'),(990002,'qqc-two@example.test','unused','user','active',20,NOW(),NOW(),'','[]') ON DUPLICATE KEY UPDATE balance=VALUES(balance)`,
		`DELETE FROM settings WHERE ` + "`key`" + ` LIKE 'qqc_%' OR ` + "`key`" + `='qq_bot_config'`,
		`DELETE FROM qq_bot_bindings`, `DELETE FROM qq_bot_bind_requests`, `DELETE FROM qq_bot_rewards`,
		`DELETE FROM qq_bot_events`,
	} {
		_, err = db.ExecContext(ctx, q)
		require.NoError(t, err)
	}
	old := map[string]any{"enabled": true, "bot_qq": "1849138657", "allowed_groups": []string{"1025362951"}, "access_token": "test-token-at-least-24-characters", "chat_enabled": true, "api_key": "operator-key", "chat_model": "test-chat", "system_prompt": "test", "max_output_tokens": 1024, "history_messages": 10, "daily_chat_limit": 20, "total_daily_chat_limit": 200, "cooldown_seconds": 0, "checkin_reward": .05, "checkin_reward_max": .10}
	raw, _ := json.Marshal(old)
	_, err = db.ExecContext(ctx, "INSERT INTO settings (`key`,value,updated_at) VALUES('qq_bot_config',?,NOW())", string(raw))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO qq_bot_bindings(user_id,qq_id,group_id,created_at) VALUES(990001,'123456789','1025362951',NOW()),(990002,'987654321','1025362951',NOW())`)
	require.NoError(t, err)
	s := New(db, fixtureEncryptor{}, true, nil)
	_, err = db.ExecContext(ctx, `INSERT INTO qq_bot_events(id,qq_id,group_id,command,status,result_text,created_at) VALUES(REPEAT('a',64),'123456789','1025362951','help','completed','legacy help',NOW())`)
	require.NoError(t, err)
	require.NoError(t, s.InitializeLegacy(ctx, "http://127.0.0.1:8080/v1"))
	require.NoError(t, s.InitializeLegacy(ctx, "http://127.0.0.1:8080/v1"))
	c, err := s.Config(ctx)
	require.NoError(t, err)
	require.True(t, c.Enabled)
	require.False(t, c.ImageEnabled)
	require.Zero(t, c.InviteReward)
	require.False(t, c.TransferEnabled)
	require.False(t, c.Autonomous.Enabled)
	require.False(t, c.Moderation.Enabled)
	require.False(t, c.PriceNotifications.Enabled)
	require.False(t, c.HealthNotifications.Enabled)
	checkin, err := s.CheckinConfig(ctx)
	require.NoError(t, err)
	require.True(t, checkin.Enabled)
	require.Equal(t, .10, checkin.MaxAmount)
	lot, err := s.LotteryConfig(ctx)
	require.NoError(t, err)
	require.False(t, lot.Enabled)
	ranking, err := s.RankingConfig(ctx)
	require.NoError(t, err)
	require.False(t, ranking.Enabled)
	require.False(t, ranking.RewardsEnabled)
	_, err = s.SaveConfigSection(ctx, "limits", sectionVersion(c, "limits"), c.Public())
	require.NoError(t, err)
	_, err = s.SaveConfigSection(ctx, "limits", "outdated", c.Public())
	require.ErrorIs(t, err, ErrConfigConflict)
	j := Job{AppID: c.AppID, GroupID: c.AllowedGroups[0], MemberID: "123456789", ID: "fixture-checkin", CreatedAt: time.Now()}
	t.Run("concurrent checkin credits once", func(t *testing.T) {
		var wg sync.WaitGroup
		errs := make(chan error, 16)
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, e := s.reward(ctx, c, j, "checkin"); errs <- e }()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			require.NoError(t, e)
		}
		var count int
		var amount, balance float64
		require.NoError(t, db.QueryRow(`SELECT COUNT(*),SUM(amount) FROM qqc_rewards WHERE user_id=990001 AND kind='checkin'`).Scan(&count, &amount))
		require.Equal(t, 1, count)
		require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id=990001`).Scan(&balance))
		require.InDelta(t, 10+amount, balance, 1e-8)
	})
	t.Run("all administration read paths", func(t *testing.T) {
		_, err := s.Runtime(ctx, c)
		require.NoError(t, err)
		_, err = s.Overview(ctx)
		require.NoError(t, err)
		_, err = s.ListMembers(ctx, MemberDirectoryFilter{Page: 1, PageSize: 20})
		require.NoError(t, err)
		_, err = s.ListJobs(ctx, JobFilter{Page: 1, PageSize: 20})
		require.NoError(t, err)
		_, err = s.ListModerationRecords(ctx, JobFilter{Page: 1, PageSize: 20})
		require.NoError(t, err)
		_, err = s.ListBindings(ctx, "", 1, 20)
		require.NoError(t, err)
		for _, kind := range []string{"checkin", "lottery", "invite"} {
			_, err = s.ListActivityRecords(ctx, kind, "", 1, 20)
			require.NoError(t, err, kind)
		}
		_, err = s.ListLotteryDraws(ctx, "", 1, 20)
		require.NoError(t, err)
		_, err = s.LotteryState(ctx, 990001)
		require.NoError(t, err)
		_, err = s.TransferHistory(ctx, "", 1)
		require.NoError(t, err)
		_, err = s.PreviewActivities(ctx, 990001)
		require.NoError(t, err)
	})
	t.Run("durable jobs and bounded quotas", func(t *testing.T) {
		e := Event{ID: "fixture-message-1", GroupID: j.GroupID, Content: "/余额"}
		e.Author.MemberID = j.MemberID
		b, _ := json.Marshal(e)
		p := Payload{ID: e.ID, Type: groupMessageCreate, Data: b}
		require.NoError(t, s.Enqueue(ctx, c, p))
		require.NoError(t, s.Enqueue(ctx, c, p))
		job, err := s.claimJob(ctx)
		require.NoError(t, err)
		require.Equal(t, e.ID, job.MessageID)
		_, err = s.db.ExecContext(ctx, `UPDATE qqc_jobs SET status='completed',finished_at=NOW() WHERE id=$1`, job.ID)
		require.NoError(t, err)
		c.TotalDailyChatLimit = 1
		c.DailyChatLimit = 1
		require.NoError(t, reserveCounterFixture(ctx, s, c.AppID, "member", 1))
		require.Error(t, reserveCounterFixture(ctx, s, c.AppID, "member", 1))
	})
	t.Run("lottery growth and transfer configuration", func(t *testing.T) {
		lot.Enabled = true
		lot.Prizes = []LotteryPrize{{Name: "谢谢参与", ProbabilityBPS: 10000}}
		require.NoError(t, s.SaveLotteryConfig(ctx, lot))
		_, err := s.db.ExecContext(ctx, `INSERT INTO qqc_lottery_chance_grants(source_key,user_id,chances) VALUES('fixture-grant',990001,2)`)
		require.NoError(t, err)
		_, err = s.drawLottery(ctx, c, j)
		require.NoError(t, err)
		_, err = s.drawLottery(ctx, c, j)
		require.NoError(t, err)
		require.NoError(t, s.SaveTransferConfig(ctx, TransferConfig{TransferEnabled: true, TransferFeePercent: 1, TransferMinAmount: .01}))
		_, err = s.db.ExecContext(ctx, `INSERT INTO qqc_transfer_history(app_id,from_member_id,from_user_id,to_user_id,from_email,to_email,amount,fee,net_amount) VALUES($1,$2,990001,990002,'a@test','b@test',1,.01,.99)`, c.AppID, j.MemberID)
		require.NoError(t, err)
		_, err = s.TransferHistory(ctx, "a@test", 1)
		require.NoError(t, err)
	})
	t.Run("transfer confirmation debits exactly once", func(t *testing.T) {
		s.SetBindingEmailDelivery(fixtureEmail{})
		c, err := s.Config(ctx)
		require.NoError(t, err)
		var beforeFrom, beforeTo float64
		require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id=990001`).Scan(&beforeFrom))
		require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id=990002`).Scan(&beforeTo))
		e := Event{ID: "transfer-test", GroupID: j.GroupID}
		e.Author.MemberID = j.MemberID
		message, pending := s.prepareTransfer(ctx, c, e, "1 qqc-two@example.test")
		require.NotNil(t, pending, message)
		_, err = s.PreviewTransfer(ctx, pending.token)
		require.NoError(t, err)
		var wg sync.WaitGroup
		results := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); results <- s.ConfirmTransfer(ctx, pending.token) }()
		}
		wg.Wait()
		close(results)
		success := 0
		for e := range results {
			if e == nil {
				success++
			}
		}
		require.Equal(t, 1, success)
		var afterFrom, afterTo float64
		require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id=990001`).Scan(&afterFrom))
		require.InDelta(t, beforeFrom-1, afterFrom, 1e-8)
		require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id=990002`).Scan(&afterTo))
		require.InDelta(t, beforeTo+.99, afterTo, 1e-8)
	})
	t.Run("growth rewards and rankings", func(t *testing.T) {
		checkin.Growth.Tasks = []GrowthTask{{ID: "fixture_binding", Name: "绑定奖励", Kind: "binding", Target: 1, Enabled: true, Reward: &GrowthReward{Type: "balance", Amount: .25}}}
		require.NoError(t, s.SaveCheckinConfig(ctx, checkin))
		_, n, err := s.processGrowthTasks(ctx, c, j, time.Now(), true)
		require.NoError(t, err)
		require.Equal(t, 1, n)
		_, n, err = s.processGrowthTasks(ctx, c, j, time.Now(), true)
		require.NoError(t, err)
		require.Zero(t, n)
		_, err = s.todayRankings(ctx, c, time.Now())
		require.NoError(t, err)
		_, err = s.saveRankingConfigAt(ctx, defaultRankingConfig(), time.Now().AddDate(0, 0, -2))
		require.NoError(t, err)
		require.NoError(t, s.settleDueRankings(ctx, time.Now()))
	})
	t.Run("native chat through model and OneBot", func(t *testing.T) {
		var calls atomic.Int32
		var imageCalls atomic.Int32
		picture := testInputPNG(t)
		model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/images/generations") {
				imageCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(picture)}}})
				return
			}
			calls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"你好，测试成功"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
		}))
		defer model.Close()
		cfg, err := s.Config(ctx)
		require.NoError(t, err)
		version := sectionVersion(cfg, "chat")
		cfg.BaseURL = model.URL + "/v1"
		cfg.APIKey = "isolated-model-key"
		cfg.ChatProtocol = "chat_completions"
		_, err = s.SaveConfigSection(ctx, "chat", version, cfg)
		require.NoError(t, err)
		napcat := httptest.NewServer(http.HandlerFunc(s.ServeOneBot))
		defer napcat.Close()
		headers := http.Header{"Authorization": {"Bearer " + cfg.AppSecret}, "X-Self-ID": {cfg.AppID}, "X-Client-Role": {"Universal"}}
		ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(napcat.URL, "http"), headers)
		require.NoError(t, err)
		defer ws.Close()
		go func() {
			for {
				var request map[string]json.RawMessage
				if ws.ReadJSON(&request) != nil {
					return
				}
				if ws.WriteJSON(map[string]any{"status": "ok", "retcode": 0, "echo": json.RawMessage(request["echo"]), "data": map[string]any{"message_id": "native-reply"}}) != nil {
					return
				}
			}
		}()
		e := obEvent{PostType: "message", MessageType: "group", MessageID: "native-chat", GroupID: obID(j.GroupID), UserID: obID(j.MemberID), SelfID: obID(cfg.AppID), Time: time.Now().Unix(), Message: []obSegment{{Type: "text", Data: map[string]json.RawMessage{"text": json.RawMessage(`"/聊天 测试"`)}}}}
		require.NoError(t, s.enqueueOneBot(ctx, e))
		s.runOne(ctx)
		var status, detail string
		require.NoError(t, db.QueryRow(`SELECT status,error FROM qqc_jobs WHERE message_id='native-chat'`).Scan(&status, &detail))
		require.Equal(t, "completed", status, detail)
		require.EqualValues(t, 1, calls.Load())
		require.NoError(t, s.enqueueOneBot(ctx, e))
		s.runOne(ctx)
		require.EqualValues(t, 1, calls.Load())
		var history int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qqc_group_messages`).Scan(&history))
		require.Positive(t, history)
		cfg, err = s.Config(ctx)
		require.NoError(t, err)
		version = sectionVersion(cfg, "images")
		cfg.ImageEnabled = true
		cfg.ImageModel = "fixture-image"
		_, err = s.SaveConfigSection(ctx, "images", version, cfg)
		require.NoError(t, err)
		e.MessageID = "native-image"
		e.Message[0].Data["text"] = json.RawMessage(`"/生图 测试小猫"`)
		require.NoError(t, s.enqueueOneBot(ctx, e))
		s.runOne(ctx)
		require.NoError(t, db.QueryRow(`SELECT status,error FROM qqc_jobs WHERE message_id='native-image'`).Scan(&status, &detail))
		require.Equal(t, "completed", status, detail)
		require.EqualValues(t, 1, imageCalls.Load())
	})
	t.Run("native invitations and membership", func(t *testing.T) {
		require.NoError(t, s.SaveInviteConfig(ctx, InviteConfig{InviteReward: .3, InviteDailyLimit: 10, RechargeRequirement: RechargeRequirement{RechargeWindowDays: 30}}))
		for _, notice := range []struct {
			kind, sub string
			delta     int64
		}{{"group_increase", "invite", -3}, {"group_decrease", "leave", -2}, {"group_increase", "approve", -1}} {
			in := obEvent{SelfID: obID(c.AppID), PostType: "notice", NoticeType: notice.kind, SubType: notice.sub, UserID: "987654321", OperatorID: "123456789", GroupID: obID(j.GroupID), Time: time.Now().Unix() + notice.delta}
			require.NoError(t, s.enqueueOneBot(ctx, in))
		}
		for i := 0; i < 4; i++ {
			s.runOne(ctx)
		}
		var inviter string
		var joined, left sql.NullTime
		require.NoError(t, db.QueryRow(`SELECT inviter_id,joined_at,left_at FROM qqc_members WHERE app_id=? AND member_id='987654321'`, c.AppID).Scan(&inviter, &joined, &left))
		require.Equal(t, "123456789", inviter)
		require.True(t, joined.Valid)
		require.False(t, left.Valid)
		require.NoError(t, s.SettleInvites(ctx, "987654321"))
		var count int
		var amount float64
		require.NoError(t, db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(amount),0) FROM qqc_rewards WHERE kind='invite' AND user_id=990001`).Scan(&count, &amount))
		require.Equal(t, 1, count)
		require.InDelta(t, .3, amount, 1e-8)
	})
	t.Run("zero balance checkin remains valid", func(t *testing.T) {
		cfg, err := s.CheckinConfig(ctx)
		require.NoError(t, err)
		cfg.Enabled = true
		cfg.MinAmount = 0
		cfg.MaxAmount = 0
		require.NoError(t, s.SaveCheckinConfig(ctx, cfg))
		job := j
		job.MemberID = "987654321"
		job.ID = "zero-checkin"
		_, err = s.reward(ctx, c, job, "checkin")
		require.NoError(t, err)
		_, err = s.reward(ctx, c, job, "checkin")
		require.NoError(t, err)
		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qqc_rewards WHERE kind='checkin' AND user_id=990002`).Scan(&count))
		require.Equal(t, 1, count)
	})
	t.Run("moderation queue without external punishment", func(t *testing.T) {
		cfg, err := s.Config(ctx)
		require.NoError(t, err)
		version := sectionVersion(cfg, "moderation")
		cfg.Moderation.Enabled = true
		cfg.Moderation.Groups = cfg.AllowedGroups
		cfg, err = s.SaveConfigSection(ctx, "moderation", version, cfg.Public())
		require.NoError(t, err)
		for i := 0; i < 6; i++ {
			e := Event{ID: fmt.Sprintf("moderation-%d", i), GroupID: j.GroupID, Content: "重复测试消息"}
			e.Author.MemberID = j.MemberID
			e.Author.MemberRole = "member"
			e.Timestamp, _ = json.Marshal(time.Now().UTC().Format(time.RFC3339Nano))
			_, err = s.moderateIncoming(ctx, cfg, e)
			require.NoError(t, err)
		}
		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qqc_moderation_incidents`).Scan(&count))
		require.Positive(t, count)
	})
	t.Run("incoming picture references", func(t *testing.T) {
		c.ImageEnabled = true
		e := Event{ID: "incoming-picture", GroupID: j.GroupID, Attachments: []Attachment{{URL: "https://example.test/picture.png", ContentType: "image/png"}}}
		e.Author.MemberID = j.MemberID
		require.NoError(t, s.rememberIncomingImages(ctx, c, e))
		require.NoError(t, s.rememberIncomingImages(ctx, c, e))
		quote := Event{GroupID: j.GroupID}
		quote.MessageReference.MessageID = e.ID
		_, images, err := s.referencedImageSources(ctx, c, quote, nil)
		require.NoError(t, err)
		require.Len(t, images, 1)
		require.Equal(t, e.Attachments[0].URL, images[0].URL)
	})
	t.Run("background growth and notifications", func(t *testing.T) {
		cfg, err := s.Config(ctx)
		require.NoError(t, err)
		_, err = s.settleGrowthBatch(ctx, cfg, 0)
		require.NoError(t, err)
		reward, err := rechargeSum(ctx, s.db, 990001, nil, false)
		require.NoError(t, err)
		require.True(t, reward.IsZero())
		version := sectionVersion(cfg, "notifications")
		cfg.PriceNotifications.Enabled = true
		cfg.HealthNotifications.Enabled = true
		cfg.HealthNotifications.FailureMinutes = 1
		_, err = s.SaveConfigSection(ctx, "notifications", version, cfg)
		require.NoError(t, err)
		provider := &fixtureSnapshots{revision: 1}
		s.SetHealthSnapshotProvider(provider)
		s.SetPriceSnapshotProvider(provider)
		now := time.Now()
		_, err = s.checkPriceChanges(ctx, now)
		require.NoError(t, err)
		require.NoError(t, s.checkHealthChanges(ctx, now))
		provider.revision = 2
		_, err = s.checkPriceChanges(ctx, now.Add(2*time.Minute))
		require.NoError(t, err)
		require.NoError(t, s.checkHealthChanges(ctx, now.Add(2*time.Minute)))
		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qqc_jobs WHERE kind IN ('price_notification','health_notification')`).Scan(&count))
		require.Positive(t, count)
	})
	t.Run("maintenance", func(t *testing.T) {
		var errs []error
		s.db.execError = func(e error) { errs = append(errs, e) }
		defer func() { s.db.execError = nil }()
		s.maintenance(ctx)
		require.Empty(t, errs)
	})
}
func reserveCounterFixture(ctx context.Context, s *Service, app, scope string, limit int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = reserveCounter(ctx, tx, app, scope, "chat", limit); err != nil {
		return err
	}
	return tx.Commit()
}
