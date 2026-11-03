package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

type bindingFixtureEmail struct{ bodies []string }

func (*bindingFixtureEmail) FrontendURL(context.Context) string { return "https://example.test" }
func (*bindingFixtureEmail) LookupAccount(_ context.Context, email string) (int64, string, error) {
	if email == "binding-recipient@example.test" {
		return 990020, email, nil
	}
	return 0, "", fmt.Errorf("unknown fixture recipient")
}
func (f *bindingFixtureEmail) SendEmail(_ context.Context, _, _, body string) error {
	f.bodies = append(f.bodies, body)
	return nil
}

func TestBindingCommandsMySQL(t *testing.T) {
	dsn := os.Getenv("QQC_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL DSN not configured")
	}
	cfg, err := mysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "qqc_test", cfg.DBName)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx := context.Background()
	s := New(db, fixtureEncryptor{}, true, nil)
	s.db.execError = func(err error) { t.Logf("SQL execution failure: %v", err) }
	c := DefaultConfig()
	c.Enabled = true
	c.AppID = "binding-fixture"
	c.AppSecret = "fixture-token"
	c.AllowedGroups = []string{"binding-group"}
	c.CommandReplyFormat = "text"
	c.HistoryMessages = 0
	c.TransferEnabled = true
	c.TransferFeePercent = 1
	c.TransferMinAmount = .01
	raw, _ := json.Marshal(c)
	_, err = db.Exec("INSERT INTO settings (`key`,value,updated_at) VALUES('qqc_config',?,NOW()) ON DUPLICATE KEY UPDATE value=VALUES(value),updated_at=NOW()", string(raw))
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO users(id,email,password_hash,role,status,balance,created_at,updated_at,notes,balance_notify_extra_emails) VALUES(990020,'binding-recipient@example.test','unused','user','active',20,NOW(),NOW(),'','[]')`)
	require.NoError(t, err)
	checkin, err := s.CheckinConfig(ctx)
	require.NoError(t, err)
	checkin.Enabled = true
	checkin.MinAmount = .05
	checkin.MaxAmount = .05
	require.NoError(t, s.SaveCheckinConfig(ctx, checkin))
	lot, err := s.LotteryConfig(ctx)
	require.NoError(t, err)
	lot.Enabled = true
	lot.Prizes = []LotteryPrize{{Name: "固定测试奖", Amount: .02, ProbabilityBPS: 10000}}
	require.NoError(t, s.SaveLotteryConfig(ctx, lot))
	var failPrivate atomic.Bool
	requests := bindingTestBotWithAck(t, s, c, func(req map[string]json.RawMessage) map[string]any {
		if failPrivate.Load() && string(req["action"]) == `"send_private_msg"` {
			return map[string]any{}
		}
		return map[string]any{"message_id": 77}
	})
	email := &bindingFixtureEmail{}
	s.SetBindingEmailDelivery(email)
	for i, command := range []string{"/签到", "/余额", "/抽奖 2", "/转账 1 binding-recipient@example.test"} {
		t.Run(command, func(t *testing.T) {
			uid := int64(990011 + i)
			member := fmt.Sprintf("binding-member-%d", i)
			_, err = db.Exec(`INSERT INTO users(id,email,password_hash,role,status,balance,created_at,updated_at,notes,balance_notify_extra_emails) VALUES(?,?,'unused','user','active',10,NOW(),NOW(),'','[]')`, uid, fmt.Sprintf("binding-%d@example.test", i))
			require.NoError(t, err)
			if i == 2 {
				_, err = db.Exec(`INSERT INTO qqc_lottery_chance_grants(source_key,user_id,chances) VALUES('binding-draw-chances',?,3)`, uid)
				require.NoError(t, err)
			}
			e := Event{ID: fmt.Sprintf("binding-source-%d", i), GroupID: c.AllowedGroups[0], Content: command}
			e.Author.MemberID = member
			raw, _ := json.Marshal(e)
			p := Payload{ID: e.ID, Type: groupAtMessageCreate, Data: raw}
			require.NoError(t, s.Enqueue(ctx, c, p))
			require.NoError(t, s.Enqueue(ctx, c, p))
			s.runOne(ctx)
			require.JSONEq(t, `"send_private_msg"`, string((bindingTestRequest(t, requests))["action"]))
			group := bindingTestRequest(t, requests)
			require.JSONEq(t, `"send_group_msg"`, string(group["action"]))
			require.Contains(t, string(group["params"]), privateBindingSent)
			var pending int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qqc_binding_commands WHERE app_id=? AND member_id=? AND delivered=TRUE AND user_id=0`, c.AppID, member).Scan(&pending))
			require.Equal(t, 1, pending)
			// Repeated account commands keep the original source, even across groups.
			later := e
			later.ID += "-later"
			later.Content = "/余额"
			require.NoError(t, s.beginPrivateBindingCommand(ctx, c, later, &Job{ID: "later-" + e.ID, MessageID: later.ID}, "/我的余额"))
			bindingTestRequest(t, requests)
			var original string
			require.NoError(t, db.QueryRow(`SELECT command_text FROM qqc_binding_commands WHERE app_id=? AND member_id=?`, c.AppID, member).Scan(&original))
			canonical, arg := commandNameForFixture(command)
			require.Equal(t, "/"+canonical+arg, original)
			code, err := s.CreateBindingCode(ctx, uid)
			require.NoError(t, err)
			codeJSON, _ := json.Marshal(code.Code)
			require.NoError(t, s.handlePrivateBinding(ctx, c, obEvent{UserID: obID(member), MessageID: obID("private-code"), Message: []obSegment{{Type: "text", Data: map[string]json.RawMessage{"text": codeJSON}}}}))
			bindingTestRequest(t, requests)
			var notYet int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qqc_jobs WHERE app_id=? AND event_type=? AND member_id=?`, c.AppID, bindingCommandEvent, member).Scan(&notYet))
			require.Zero(t, notYet)
			var wg sync.WaitGroup
			var success atomic.Int32
			for n := 0; n < 8; n++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if s.ConfirmBinding(ctx, uid, member) == nil {
						success.Add(1)
					}
				}()
			}
			wg.Wait()
			require.Equal(t, int32(1), success.Load())
			var queued int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qqc_jobs WHERE app_id=? AND event_type=? AND member_id=? AND status='queued'`, c.AppID, bindingCommandEvent, member).Scan(&queued))
			require.Equal(t, 1, queued)
			var queuedPayload []byte
			require.NoError(t, db.QueryRow(`SELECT payload FROM qqc_jobs WHERE app_id=? AND event_type=? AND member_id=?`, c.AppID, bindingCommandEvent, member).Scan(&queuedPayload))
			var expected bindingCommandPayload
			require.NoError(t, json.Unmarshal(queuedPayload, &expected))
			tx, err := s.db.BeginTx(ctx, nil)
			require.NoError(t, err)
			require.NoError(t, ensureResumedBinding(ctx, tx, c, Job{PrivateReply: true, MemberID: member, ExpectedUserID: expected.UserID, ExpectedBindingAt: expected.BindingAt}))
			require.NoError(t, tx.Rollback())
			// A failed result receipt must not spend lottery chances twice.
			failPrivate.Store(i == 2)
			s.runOne(ctx)
			failPrivate.Store(false)
			result := bindingTestRequest(t, requests)
			require.JSONEq(t, `"send_private_msg"`, string(result["action"]))
			require.Contains(t, string(result["params"]), member)
			var status, detail string
			require.NoError(t, db.QueryRow(`SELECT status,error FROM qqc_jobs WHERE app_id=? AND event_type=? AND member_id=?`, c.AppID, bindingCommandEvent, member).Scan(&status, &detail))
			if i == 2 {
				require.Equal(t, "delivery_failed", status, detail)
				fallback := bindingTestRequest(t, requests)
				require.JSONEq(t, `"send_group_msg"`, string(fallback["action"]))
				require.Contains(t, string(fallback["params"]), "请开启群临时会话")
				require.NotContains(t, string(fallback["params"]), "固定测试奖")
			} else {
				require.Equal(t, "completed", status, detail)
			}
			require.Error(t, s.ConfirmBinding(ctx, uid, member))
			// A new worker after restart must find no operation to replay.
			require.NoError(t, s.Enqueue(ctx, c, p)) // replay of the original QQ event
			New(db, fixtureEncryptor{}, true, nil).runOne(ctx)
			var balance float64
			require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id=?`, uid).Scan(&balance))
			switch i {
			case 0:
				require.Contains(t, string(result["params"]), "签到成功")
				require.Contains(t, string(result["params"]), "当前余额")
				require.InDelta(t, 10.05, balance, 1e-8)
				var awards int
				require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qqc_rewards WHERE user_id=? AND kind='checkin'`, uid).Scan(&awards))
				require.Equal(t, 1, awards)
			case 1:
				require.Contains(t, string(result["params"]), "10.00")
				require.Equal(t, float64(10), balance)
			case 2:
				require.InDelta(t, 10.04, balance, 1e-8)
				var draws int
				require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qqc_lottery_draws WHERE user_id=?`, uid).Scan(&draws))
				require.Equal(t, 2, draws)
			case 3:
				require.Equal(t, float64(10), balance, "no debit before email confirmation")
				require.Len(t, email.bodies, 1)
				link := regexp.MustCompile(`https://example\.test[^"<\s]+`).FindString(email.bodies[0])
				require.NotEmpty(t, link)
				u, err := url.Parse(link)
				require.NoError(t, err)
				fragment, err := url.ParseQuery(u.Fragment)
				require.NoError(t, err)
				token := fragment.Get("token")
				require.NotEmpty(t, token)
				var confirmations atomic.Int32
				confirmationErrors := make(chan error, 8)
				for n := 0; n < 8; n++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						if err := s.ConfirmTransfer(ctx, token); err == nil {
							confirmations.Add(1)
						} else {
							confirmationErrors <- err
						}
					}()
				}
				wg.Wait()
				close(confirmationErrors)
				var reasons []string
				for err := range confirmationErrors {
					reasons = append(reasons, err.Error())
				}
				require.Equal(t, int32(1), confirmations.Load(), reasons)
				require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id=?`, uid).Scan(&balance))
				require.Equal(t, float64(9), balance)
			}
			var refs int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qqc_message_references WHERE app_id=? AND content LIKE '%余额%'`, c.AppID).Scan(&refs))
			require.Zero(t, refs, "private results must not enter group references")
		})
	}
}

func commandNameForFixture(content string) (string, string) {
	name, arg := command(content)
	if arg != "" {
		arg = " " + arg
	}
	return name, arg
}
