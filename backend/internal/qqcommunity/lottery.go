package qqcommunity

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type LotteryState struct {
	UserID              int64   `json:"user_id"`
	RechargeTotal       float64 `json:"recharge_total"`
	Earned              int64   `json:"earned"`
	BonusChances        int64   `json:"bonus_chances"`
	Used                int64   `json:"used"`
	Remaining           int64   `json:"remaining"`
	NextRecharge        float64 `json:"next_recharge"`
	GuaranteeEvery      int64   `json:"guarantee_every"`
	DrawsUntilGuarantee int64   `json:"draws_until_guarantee"`
}
type LotteryDraw struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	Email        string    `json:"email"`
	MemberID     string    `json:"member_id"`
	PrizeName    string    `json:"prize_name"`
	IsGuaranteed bool      `json:"is_guaranteed"`
	Amount       float64   `json:"amount"`
	Remaining    int64     `json:"remaining"`
	CreatedAt    time.Time `json:"created_at"`
}
type LotteryDrawsPage struct {
	Items    []LotteryDraw `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

const lotteryMaxDrawCount = 10

const lotteryDrawUsage = "用法：抽奖 [次数]，次数为 1–10；不填默认抽 1 次。"

// This activity was reset for all accounts at the operator's request. Keep
// historical records for audit, while calculating this period from the cutoff.
var lotteryPeriodStart = time.Date(2026, time.November, 3, 9, 48, 32, 263935000, time.UTC)

const lotteryPeriodStartLabel = "2026-11-03 17:48:32（北京时间）"

func lotteryDrawCount(arg string) (int, bool) {
	if arg == "" {
		return 1, true
	}
	for _, digit := range arg {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	count, err := strconv.Atoi(arg)
	return count, err == nil && count >= 1 && count <= lotteryMaxDrawCount
}

type lotteryOutcome struct {
	prize      LotteryPrize
	guaranteed bool
}

func lotteryResult(cfg LotteryConfig, state LotteryState, draws []lotteryOutcome) string {
	var b strings.Builder
	if len(draws) == 1 {
		draw := draws[0]
		fmt.Fprintf(&b, "🎉 **抽奖结果**\n奖品：%s\n奖励额度：USD %.2f\n剩余抽奖次数：%d 次", replyPlainLabel(draw.prize.Name), draw.prize.Amount, state.Remaining-1)
		if draw.guaranteed {
			_, _ = b.WriteString("\n本次触发保底抽奖")
		}
	} else {
		fmt.Fprintf(&b, "🎉 **连抽结果（%d次）**", len(draws))
		names := make([]string, 0, len(draws))
		counts := make(map[string]int, len(draws))
		total := decimal.Zero
		guaranteed := 0
		for _, draw := range draws {
			if counts[draw.prize.Name] == 0 {
				names = append(names, draw.prize.Name)
			}
			counts[draw.prize.Name]++
			total = total.Add(decimal.NewFromFloat(draw.prize.Amount))
			if draw.guaranteed {
				guaranteed++
			}
		}
		for _, name := range names {
			fmt.Fprintf(&b, "\n%s × %d", replyPlainLabel(name), counts[name])
		}
		fmt.Fprintf(&b, "\n合计奖励额度：USD %s\n剩余抽奖次数：%d 次", total.StringFixed(2), state.Remaining-int64(len(draws)))
		if guaranteed > 0 {
			fmt.Fprintf(&b, "\n本次触发保底抽奖 %d 次", guaranteed)
		}
	}
	if cfg.GuaranteeEnabled {
		fmt.Fprintf(&b, "\n距保底 %d 次", cfg.drawsUntilGuarantee(state.Used+int64(len(draws))))
	}
	return b.String()
}

// Count actual CNY principal from completed online balance orders in this period.
// USD balance codes, bonuses, subscriptions and administrative credits do not count.
// Refund-in-progress orders are suspended; partial refunds remove principal pro rata.
func lotteryState(ctx context.Context, db lotteryQuerier, c LotteryConfig, uid int64, lockSources bool) (LotteryState, error) {
	out := LotteryState{UserID: uid}
	total, err := lotteryRechargeSum(ctx, db, uid, lockSources)
	if err != nil {
		return out, err
	}
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM qqc_lottery_draws WHERE user_id=$1 AND created_at >= $2`, uid, lotteryPeriodStart).Scan(&out.Used); err != nil {
		return out, err
	}
	if err = db.QueryRowContext(ctx, `SELECT COALESCE(SUM(chances),0) FROM qqc_lottery_chance_grants WHERE user_id=$1 AND created_at >= $2`, uid, lotteryPeriodStart).Scan(&out.BonusChances); err != nil {
		return out, err
	}
	var bonusUsed int64
	if out.BonusChances > 0 && out.Used > 0 {
		usage, err := lotteryBonusUsage(ctx, db, uid)
		if err != nil {
			return out, err
		}
		bonusUsed = usage[uid]
	}
	return calculateLotteryState(c, uid, total, out.Used, out.BonusChances, bonusUsed)
}

func calculateLotteryState(c LotteryConfig, uid int64, total decimal.Decimal, used, bonus, bonusUsed int64) (LotteryState, error) {
	out := LotteryState{UserID: uid, Used: used, BonusChances: bonus}
	out.DrawsUntilGuarantee = c.drawsUntilGuarantee(out.Used)
	if out.DrawsUntilGuarantee > 0 {
		out.GuaranteeEvery = c.GuaranteeEvery
	}
	step := decimal.NewFromFloat(c.RechargeStep)
	if step.LessThanOrEqual(decimal.Zero) || c.ChancesPerStep < 1 {
		return out, errors.New("抽奖规则不正确")
	}
	quotient, _ := total.QuoRem(step, 0)
	earned := quotient.Mul(decimal.NewFromInt(c.ChancesPerStep)).Add(decimal.NewFromInt(out.BonusChances))
	if earned.GreaterThan(decimal.NewFromInt(1e12)) {
		return out, errors.New("抽奖次数超出支持范围，请联系管理员")
	}
	out.RechargeTotal = total.InexactFloat64()
	out.Earned = earned.IntPart()
	bonusUsed = min(max(bonusUsed, 0), min(used, bonus))
	rechargeUsed := used - bonusUsed
	out.Remaining = max(quotient.IntPart()*c.ChancesPerStep-rechargeUsed, 0) + bonus - bonusUsed
	// When refunds/rule changes put usage ahead of entitlement, new recharge
	// must cover that deficit before another draw becomes available.
	nextTier := max(quotient.IntPart()+1, rechargeUsed/c.ChancesPerStep+1)
	out.NextRecharge = decimal.NewFromInt(nextTier).Mul(step).Sub(total).InexactFloat64()
	return out, nil
}
func (s *Service) LotteryState(ctx context.Context, uid int64) (LotteryState, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return LotteryState{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND deleted_at IS NULL)`, uid).Scan(&exists); err != nil {
		return LotteryState{}, err
	}
	if !exists {
		return LotteryState{}, errors.New("网站账号不存在")
	}
	c, err := loadLotteryConfig(ctx, tx)
	if err != nil {
		return LotteryState{}, err
	}
	out, err := lotteryState(ctx, tx, c, uid, false)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) ListLotteryDraws(ctx context.Context, search string, page, pageSize int) (LotteryDrawsPage, error) {
	out := LotteryDrawsPage{Items: []LotteryDraw{}, Page: page, PageSize: pageSize}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()
	const filter = ` FROM qqc_lottery_draws d JOIN users u ON u.id=d.user_id WHERE ($1='' OR d.user_id::text=$1 OR strpos(lower(u.email),lower($1))>0 OR strpos(lower(d.member_id),lower($1))>0)`
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*)`+filter, search).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT d.id,d.user_id,u.email,d.member_id,d.prize_name,d.amount,d.remaining,d.created_at,COALESCE(d.config_snapshot->'is_guaranteed'='true'::jsonb,false)`+filter+` ORDER BY d.id DESC LIMIT $2 OFFSET $3`, search, pageSize, (page-1)*pageSize)
	if err != nil {
		return out, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var d LotteryDraw
		if err = rows.Scan(&d.ID, &d.UserID, &d.Email, &d.MemberID, &d.PrizeName, &d.Amount, &d.Remaining, &d.CreatedAt, &d.IsGuaranteed); err != nil {
			return out, err
		}
		out.Items = append(out.Items, d)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) lotteryInfo(ctx context.Context, c Config, j Job, showRules bool) (string, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	cfg, err := loadLotteryConfig(ctx, tx)
	if err != nil {
		return "", err
	}
	if !cfg.Enabled {
		return "抽奖活动未开启。", nil
	}
	var uid int64
	// This is a read-only snapshot. Locking users here fails in MySQL read-only
	// transactions and is unnecessary: only the draw transaction spends chances.
	err = tx.QueryRowContext(ctx, `SELECT b.user_id FROM qqc_bindings b JOIN users u ON u.id=b.user_id WHERE b.app_id=$1 AND b.member_id=$2 AND u.status='active' AND u.deleted_at IS NULL`, c.AppID, j.MemberID).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return bindFirst, nil
	}
	if err != nil {
		return "", err
	}
	if showRules {
		var b strings.Builder
		fmt.Fprintf(&b, "🎉 **%s**\n统计起点：%s\n起点后累计实付充值本金每满人民币 %.2f 元送 %d 次（不含手续费）\n每次抽奖消耗 1 次\n\n**普通奖池**", replyPlainLabel(cfg.Title), lotteryPeriodStartLabel, cfg.RechargeStep, cfg.ChancesPerStep)
		for _, p := range cfg.Prizes {
			fmt.Fprintf(&b, "\n\n%s\nUSD %.2f（概率 %.2f%%）", replyPlainLabel(p.Name), p.Amount, float64(p.ProbabilityBPS)/100)
		}
		if cfg.GuaranteeEnabled {
			fmt.Fprintf(&b, "\n\n**保底奖池**\n本期累计每抽 %d 次触发\n普通中奖不重置保底进度", cfg.GuaranteeEvery)
			for _, p := range cfg.GuaranteePrizes {
				fmt.Fprintf(&b, "\n\n%s\nUSD %.2f（概率 %.2f%%）", replyPlainLabel(p.Name), p.Amount, float64(p.ProbabilityBPS)/100)
			}
		}
		return b.String(), tx.Commit()
	}
	state, err := lotteryState(ctx, tx, cfg, uid, false)
	if err != nil {
		return "", err
	}
	var wins int64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM qqc_lottery_draws WHERE user_id=$1 AND amount>0 AND created_at >= $2`, uid, lotteryPeriodStart).Scan(&wins); err != nil {
		return "", err
	}
	result := fmt.Sprintf("🎟 **抽奖次数**\n累计已抽：%d 次\n中奖次数（额度奖励大于0）：%d 次\n剩余抽奖次数：%d 次\n再充值人民币 %.2f 元获赠", state.Used, wins, state.Remaining, state.NextRecharge)
	if state.DrawsUntilGuarantee > 0 {
		result += fmt.Sprintf("\n距保底 %d 次", state.DrawsUntilGuarantee)
	}
	return result, tx.Commit()
}
func (s *Service) drawLottery(ctx context.Context, c Config, j Job) (string, error) {
	return s.drawLotteryCount(ctx, c, j, 1)
}

func (s *Service) drawLotteryCount(ctx context.Context, c Config, j Job, count int) (string, error) {
	if count < 1 || count > lotteryMaxDrawCount {
		return lotteryDrawUsage, nil
	}
	if j.ID == "" {
		return "", errors.New("缺少抽奖请求标识")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	// Match unbinding's reward lock. Protect config changes while settling a draw.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared(71427101)`); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,272))`, c.AppID); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared($1)`, lotteryConfigLock); err != nil {
		return "", err
	}
	current, err := s.loadConfig(ctx, tx)
	if err != nil {
		return "", err
	}
	if !current.Enabled || current.AppID != c.AppID || (!current.allows(j.GroupID) || current.groupPolicy(j.GroupID).DisableActivities) {
		return "当前群未启用机器人。", nil
	}
	cfg, err := loadLotteryConfig(ctx, tx)
	if err != nil {
		return "", err
	}
	var uid int64
	err = tx.QueryRowContext(ctx, `SELECT b.user_id FROM qqc_bindings b JOIN users u ON u.id=b.user_id WHERE b.app_id=$1 AND b.member_id=$2 AND u.status='active' AND u.deleted_at IS NULL`, c.AppID, j.MemberID).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return bindFirst, nil
	}
	if err != nil {
		return "", err
	}
	// User-level serialization also protects lifetime usage across AppID changes.
	if err = ensureResumedBinding(ctx, tx, c, j); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,274))`, strconv.FormatInt(uid, 10)); err != nil {
		return "", err
	}
	requestKey := c.AppID + ":" + j.ID
	if j.MessageID != "" {
		requestKey = c.AppID + ":message:" + j.MessageID
	}
	var result string
	err = tx.QueryRowContext(ctx, `SELECT result_text FROM qqc_lottery_draws WHERE request_key=$1 AND user_id=$2 AND member_id=$3`, requestKey, uid, j.MemberID).Scan(&result)
	if err == nil {
		if _, err = tx.ExecContext(ctx, `UPDATE qqc_jobs SET kind='lottery',result_text=$2 WHERE id=$1`, j.ID, result); err != nil {
			return "", err
		}
		return result, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if !cfg.Enabled {
		return "抽奖活动未开启。", nil
	}
	if err = validateLottery(cfg); err != nil {
		return "", err
	}
	state, err := lotteryState(ctx, tx, cfg, uid, true)
	if err != nil {
		return "", err
	}
	if state.Remaining < int64(count) {
		if count > 1 {
			return fmt.Sprintf("抽奖次数不足，本次未抽奖\n需要抽奖次数：%d 次\n剩余抽奖次数：%d 次\n再充值人民币 %.2f 元获赠", count, state.Remaining, state.NextRecharge), nil
		}
		return fmt.Sprintf("抽奖次数不足\n再充值人民币 %.2f 元获赠", state.NextRecharge), nil
	}
	draws := make([]lotteryOutcome, count)
	for i := range draws {
		n, err := rand.Int(rand.Reader, big.NewInt(10000))
		if err != nil {
			return "", err
		}
		guaranteed := cfg.drawsUntilGuarantee(state.Used+int64(i)) == 1
		pool := cfg.Prizes
		if guaranteed {
			pool = cfg.GuaranteePrizes
		}
		draws[i] = lotteryOutcome{prize: lotteryPrizeAt(pool, n.Int64()), guaranteed: guaranteed}
	}
	result = lotteryResult(cfg, state, draws)
	if err = settleLotteryDraws(ctx, tx, c, j, uid, cfg, state, draws, requestKey, result); err != nil {
		return "", err
	}
	// Persist the result before delivery for auditing and message deduplication.
	if _, err = tx.ExecContext(ctx, `UPDATE qqc_jobs SET kind='lottery',result_text=$2 WHERE id=$1`, j.ID, result); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	s.invalidate(ctx, uid)
	return result, nil
}

// Both entry points settle against the same ledger in the caller's transaction.
// The caller must hold the lottery configuration and user (274) locks.
func settleLotteryDraws(ctx context.Context, tx *transaction, c Config, j Job, uid int64, cfg LotteryConfig, state LotteryState, draws []lotteryOutcome, requestKey, result string) error {
	for i, draw := range draws {
		prize, guaranteed := draw.prize, draw.guaranteed
		// Keep the draw's classification alongside its immutable configuration snapshot.
		// Older records without this field remain ordinary draws, without a migration.
		snapshot, err := json.Marshal(struct {
			LotteryConfig
			IsGuaranteed     bool   `json:"is_guaranteed"`
			DrawNumber       int64  `json:"draw_number"`
			RechargeCurrency string `json:"recharge_currency"`
		}{cfg, guaranteed, state.Used + int64(i) + 1, "CNY"})
		if err != nil {
			return err
		}
		var drawID int64
		// The first draw retains the original key for replaying the whole result.
		// Additional draws use a separate namespace from incoming message/job IDs.
		drawKey := requestKey
		if i > 0 {
			drawKey = fmt.Sprintf("%s:lottery-batch:%s:%d", c.AppID, codeHash(requestKey), i+1)
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO qqc_lottery_draws(request_key,app_id,member_id,group_id,user_id,prize_name,amount,config_snapshot,recharge_total,earned,remaining,result_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`, drawKey, c.AppID, j.MemberID, j.GroupID, uid, prize.Name, prize.Amount, string(snapshot), state.RechargeTotal, state.Earned, state.Remaining-int64(i)-1, result).Scan(&drawID)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO qqc_rewards(reward_key,app_id,group_id,member_id,user_id,kind,amount) VALUES($1,$2,$3,$4,$5,'lottery',$6)`, fmt.Sprintf("lottery:%d", drawID), c.AppID, j.GroupID, j.MemberID, uid, prize.Amount); err != nil {
			return err
		}
		// Zero-value prizes still consume a draw. The active account is locked
		// above; MySQL reports zero changed rows for an unchanged balance.
		if prize.Amount > 0 {
			updated, err := tx.ExecContext(ctx, `UPDATE users SET balance=balance+$1,updated_at=NOW() WHERE id=$2 AND status='active' AND deleted_at IS NULL`, prize.Amount, uid)
			if err != nil {
				return err
			}
			updatedCount, err := updated.RowsAffected()
			if err != nil {
				return err
			}
			if updatedCount != 1 {
				return errors.New("账户不可用")
			}
		}
		if prize.Amount > 0 {
			if _, err = tx.ExecContext(ctx, `INSERT INTO redeem_codes(code,type,value,status,used_by,used_at,notes,created_at) VALUES($1,'admin_balance',$2,'used',$3,NOW(),$4,NOW())`, "QQ"+randomID(), prize.Amount, uid, fmt.Sprintf("QQ lottery reward #%d: %s", drawID, prize.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}
