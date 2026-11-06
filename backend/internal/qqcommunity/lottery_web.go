package qqcommunity

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strconv"
)

var (
	ErrLotteryClosed    = errors.New("抽奖活动暂未开放")
	ErrLotteryNoChances = errors.New("抽奖次数不足")
	ErrLotteryAccount   = errors.New("账户不可用")
	ErrLotteryRequest   = errors.New("抽奖请求标识不正确")
	lotteryRequestID    = regexp.MustCompile(`^[a-zA-Z0-9-]{16,64}$`)
)

// Prizes and PrizeIndex describe the actual pool at settlement, even if an
// administrator changes it while the wheel is open. Never select prizes in JS.
type UserLotterySpin struct {
	RequestID    string         `json:"request_id"`
	Prize        LotteryPrize   `json:"prize"`
	PrizeIndex   int            `json:"prize_index"`
	Prizes       []LotteryPrize `json:"prizes"`
	IsGuaranteed bool           `json:"is_guaranteed"`
	Remaining    int64          `json:"remaining"`
}

func (s *Service) UserLotterySpin(ctx context.Context, uid int64, requestID string) (UserLotterySpin, error) {
	var out UserLotterySpin
	if uid <= 0 || !lotteryRequestID.MatchString(requestID) {
		return out, ErrLotteryRequest
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	// The same order and user lock as QQ draws prevents spending one chance
	// twice across tabs, requests, or the two entry points (also on MySQL).
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared(71427101)`); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared($1)`, lotteryConfigLock); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,274))`, strconv.FormatInt(uid, 10)); err != nil {
		return out, err
	}
	var activeID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL FOR UPDATE`, uid).Scan(&activeID)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrLotteryAccount
	}
	if err != nil {
		return out, err
	}
	requestKey := "web:lottery:" + strconv.FormatInt(uid, 10) + ":" + requestID
	var saved string
	err = tx.QueryRowContext(ctx, `SELECT result_text FROM qqc_lottery_draws WHERE request_key=$1 AND user_id=$2`, requestKey, uid).Scan(&saved)
	if err == nil {
		if err = json.Unmarshal([]byte(saved), &out); err != nil {
			return out, err
		}
		if err = tx.Commit(); err != nil {
			return out, err
		}
		s.invalidate(ctx, uid)
		return out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	cfg, err := loadLotteryConfig(ctx, tx)
	if err != nil {
		return out, err
	}
	if !cfg.Enabled {
		return out, ErrLotteryClosed
	}
	if err = validateLottery(cfg); err != nil {
		return out, err
	}
	state, err := lotteryState(ctx, tx, cfg, uid, true)
	if err != nil {
		return out, err
	}
	if state.Remaining < 1 {
		return out, ErrLotteryNoChances
	}
	pool := cfg.Prizes
	guaranteed := cfg.drawsUntilGuarantee(state.Used) == 1
	if guaranteed {
		pool = cfg.GuaranteePrizes
	}
	n, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return out, err
	}
	index, threshold := 0, int64(0)
	for i, prize := range pool {
		threshold += int64(prize.ProbabilityBPS)
		if n.Int64() < threshold {
			index = i
			break
		}
	}
	out = UserLotterySpin{RequestID: requestID, Prize: pool[index], PrizeIndex: index, Prizes: pool, IsGuaranteed: guaranteed, Remaining: state.Remaining - 1}
	raw, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	// A distinct app namespace records the website origin without inventing QQ
	// identities or sending group messages. Entitlement is always user-scoped.
	err = settleLotteryDraws(ctx, tx, Config{AppID: "website"}, Job{}, uid, cfg, state, []lotteryOutcome{{prize: out.Prize, guaranteed: guaranteed}}, requestKey, string(raw))
	if err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	s.invalidate(ctx, uid)
	return out, nil
}
