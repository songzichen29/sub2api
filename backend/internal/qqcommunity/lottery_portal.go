package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Only explicitly published groups are exposed on the user site. Array order
// is the recommendation order; the complete bot allowlist stays private.
type LotteryPublicGroup struct {
	GroupID string `json:"group_id"`
	Name    string `json:"name"`
	JoinURL string `json:"join_url"`
}

var lotteryQQID = regexp.MustCompile(`^[1-9][0-9]{4,19}$`)

func validateLotteryPublicGroups(groups []LotteryPublicGroup) error {
	if len(groups) > 20 {
		return errors.New("最多展示 20 个活动群")
	}
	seen := map[string]bool{}
	for _, g := range groups {
		if !lotteryQQID.MatchString(g.GroupID) || seen[g.GroupID] || strings.TrimSpace(g.Name) == "" || len([]rune(g.Name)) > 60 {
			return errors.New("活动群号须为数字且不能重复，群名称需为 1 到 60 个字符")
		}
		seen[g.GroupID] = true
		if g.JoinURL != "" {
			u, err := url.Parse(g.JoinURL)
			if err != nil || len(g.JoinURL) > 2048 || u.Scheme != "https" || u.User != nil || u.Port() != "" ||
				(u.Host != "qun.qq.com" && u.Host != "qm.qq.com") {
				return errors.New("加群链接须为 https://qun.qq.com 或 https://qm.qq.com 链接；也可留空仅展示群号")
			}
		}
	}
	return nil
}

func lotteryGroupAllowed(c Config, group string) bool {
	return c.allows(group) && !c.groupPolicy(group).DisableActivities
}

type UserLottery struct {
	Enabled         bool                 `json:"enabled"`
	Title           string               `json:"title"`
	PeriodStart     time.Time            `json:"period_start"`
	RechargeStep    float64              `json:"recharge_step"`
	ChancesPerStep  int64                `json:"chances_per_step"`
	Prizes          []LotteryPrize       `json:"prizes"`
	GuaranteePrizes []LotteryPrize       `json:"guarantee_prizes"`
	State           LotteryState         `json:"state"`
	BoundQQ         string               `json:"bound_qq"`
	PublicGroups    []LotteryPublicGroup `json:"public_groups"`
	PendingRecharge bool                 `json:"pending_recharge"`
}

// No QQ binding is required to see an account's entitlement. Use the exact
// same snapshot/calculation as group commands, including refunds and grants.
func (s *Service) UserLottery(ctx context.Context, uid int64) (UserLottery, error) {
	out := UserLottery{PublicGroups: []LotteryPublicGroup{}, GuaranteePrizes: []LotteryPrize{}}
	if uid <= 0 {
		return out, errors.New("invalid user")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	c, err := s.loadConfig(ctx, tx)
	if err != nil {
		return out, err
	}
	l, err := loadLotteryConfig(ctx, tx)
	if err != nil {
		return out, err
	}
	out.State, err = lotteryState(ctx, tx, l, uid, false)
	if err != nil {
		return out, err
	}
	out.Enabled, out.Title, out.PeriodStart = c.Enabled && l.Enabled, l.Title, lotteryPeriodStart
	out.RechargeStep, out.ChancesPerStep, out.Prizes = l.RechargeStep, l.ChancesPerStep, l.Prizes
	if l.GuaranteeEnabled {
		out.GuaranteePrizes = l.GuaranteePrizes
	}
	for _, g := range l.PublicGroups {
		if lotteryGroupAllowed(c, g.GroupID) {
			out.PublicGroups = append(out.PublicGroups, g)
		}
	}
	err = tx.QueryRowContext(ctx, `SELECT member_id FROM qqc_bindings WHERE app_id=$1 AND user_id=$2`, c.AppID, uid).Scan(&out.BoundQQ)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM payment_orders WHERE user_id=$1 AND order_type='balance' AND status IN ('PAID','RECHARGING') AND completed_at IS NULL)`, uid).Scan(&out.PendingRecharge)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

type UserLotteryDraw struct {
	ID           int64     `json:"id"`
	PrizeName    string    `json:"prize_name"`
	Amount       float64   `json:"amount"`
	IsGuaranteed bool      `json:"is_guaranteed"`
	CreatedAt    time.Time `json:"created_at"`
}
type UserLotteryDraws struct {
	Items    []UserLotteryDraw `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

func (s *Service) UserLotteryDraws(ctx context.Context, uid int64, page, size int) (UserLotteryDraws, error) {
	out := UserLotteryDraws{Items: []UserLotteryDraw{}, Page: page, PageSize: size}
	if uid <= 0 || page < 1 || page > 100000 || size < 1 || size > 100 {
		return out, errors.New("invalid pagination")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	const filter = ` FROM qqc_lottery_draws WHERE user_id=$1 AND created_at >= $2`
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*)`+filter, uid, lotteryPeriodStart).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,prize_name,amount,config_snapshot,created_at`+filter+` ORDER BY id DESC LIMIT $3 OFFSET $4`, uid, lotteryPeriodStart, size, (page-1)*size)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item UserLotteryDraw
		var raw string
		if err = rows.Scan(&item.ID, &item.PrizeName, &item.Amount, &raw, &item.CreatedAt); err != nil {
			return out, err
		}
		var snapshot struct {
			IsGuaranteed bool `json:"is_guaranteed"`
		}
		_ = json.Unmarshal([]byte(raw), &snapshot)
		item.IsGuaranteed = snapshot.IsGuaranteed
		out.Items = append(out.Items, item)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

type LotteryMembership struct {
	Status    string    `json:"status"` // unbound, member, not_member, unknown, disabled
	CheckedAt time.Time `json:"checked_at"`
}

// A binding or an old join event is not proof of current membership. This is a
// read-only probe, never an authority to debit or draw on the website.
func (s *Service) UserLotteryMembership(ctx context.Context, uid int64) (LotteryMembership, error) {
	out := LotteryMembership{Status: "unknown", CheckedAt: time.Now()}
	if uid <= 0 {
		return out, errors.New("invalid user")
	}
	c, err := s.Config(ctx)
	if err != nil {
		return out, err
	}
	l, err := s.LotteryConfig(ctx)
	if err != nil {
		return out, err
	}
	if !c.Enabled || !l.Enabled {
		out.Status = "disabled"
		return out, nil
	}
	var member string
	err = s.db.QueryRowContext(ctx, `SELECT member_id FROM qqc_bindings WHERE app_id=$1 AND user_id=$2`, c.AppID, uid).Scan(&member)
	if errors.Is(err, sql.ErrNoRows) {
		out.Status = "unbound"
		return out, nil
	}
	if err != nil {
		return out, err
	}
	requestCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	// Bound concurrency and overall timeout even if many groups are configured.
	work := make(chan string)
	results := make(chan string, len(c.AllowedGroups))
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for group := range work {
				results <- probeLotteryMembership(ctx, c, group, member, s.oneBotCall)
			}
		}()
	}
	go func() {
		defer close(work)
		for _, group := range c.AllowedGroups {
			if lotteryGroupAllowed(c, group) {
				select {
				case work <- group:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	wg.Wait()
	close(results)
	status := "not_member"
	checked := 0
	for result := range results {
		checked++
		if result == "member" {
			status = "member"
			break
		}
		if result == "unknown" {
			status = "unknown"
		}
	}
	if status != "member" && (checked == 0 || ctx.Err() != nil) {
		status = "unknown"
	}
	// A binding may have changed during the network lookup. Do not apply evidence
	// for the old QQ identity to the user's new binding.
	var same bool
	if err = s.db.QueryRowContext(requestCtx, `SELECT EXISTS(SELECT 1 FROM qqc_bindings WHERE app_id=$1 AND user_id=$2 AND member_id=$3)`, c.AppID, uid, member).Scan(&same); err != nil {
		return out, err
	}
	if !same {
		status = "unknown"
	}
	// Join/leave callbacks received during the probe invalidate older network
	// evidence. A subsequent refresh verifies the member's new state.
	var changed bool
	if err = s.db.QueryRowContext(requestCtx, `SELECT EXISTS(SELECT 1 FROM qqc_members WHERE app_id=$1 AND member_id=$2 AND last_membership_at>$3)`, c.AppID, member, out.CheckedAt).Scan(&changed); err != nil {
		return out, err
	}
	if changed {
		status = "unknown"
	}
	out.Status = status
	out.CheckedAt = time.Now()
	return out, nil
}

type lotteryMemberCall func(context.Context, Config, string, any) (json.RawMessage, error)

func probeLotteryMembership(ctx context.Context, c Config, group, member string, call lotteryMemberCall) string {
	raw, err := call(ctx, c, "get_group_member_info", map[string]any{"group_id": group, "user_id": member, "no_cache": true})
	var info struct {
		Group obID `json:"group_id"`
		User  obID `json:"user_id"`
	}
	if err == nil && json.Unmarshal(raw, &info) == nil && string(info.Group) == group && string(info.User) == member {
		return "member"
	}
	// A failed individual lookup may mean permission or connectivity failure.
	// Only a successful full list containing the bot establishes absence.
	if ctx.Err() != nil {
		return "unknown"
	}
	raw, err = call(ctx, c, "get_group_member_list", map[string]any{"group_id": group, "no_cache": true})
	var members []struct {
		Group obID `json:"group_id"`
		User  obID `json:"user_id"`
	}
	if err != nil || json.Unmarshal(raw, &members) != nil {
		return "unknown"
	}
	botSeen, memberSeen := false, false
	for _, m := range members {
		if string(m.Group) != group || m.User == "" {
			return "unknown"
		}
		botSeen = botSeen || string(m.User) == c.AppID
		memberSeen = memberSeen || string(m.User) == member
	}
	if memberSeen {
		return "member"
	}
	if botSeen {
		return "not_member"
	}
	return "unknown"
}
