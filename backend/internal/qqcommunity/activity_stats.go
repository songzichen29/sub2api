package qqcommunity

import (
	"context"
	"database/sql"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type ActivityStatsFilter struct {
	Date     string `form:"date"`
	Search   string `form:"search"`
	Filter   string `form:"filter"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}
type ActivityStatsUser struct {
	LotteryState
	Email         string   `json:"email"`
	Status        string   `json:"status"`
	Deleted       bool     `json:"deleted"`
	Members       []string `json:"members"`
	CheckedIn     bool     `json:"checked_in"`
	CheckinAmount float64  `json:"checkin_amount"`
	DayDraws      int64    `json:"day_draws"`
	DayDrawAmount float64  `json:"day_draw_amount"`
}
type ActivityStatsSummary struct {
	CheckinUsers     int64   `json:"checkin_users"`
	ChanceUsers      int64   `json:"chance_users"`
	RemainingChances int64   `json:"remaining_chances"`
	DrawUsers        int64   `json:"draw_users"`
	DrawCount        int64   `json:"draw_count"`
	CheckinAmount    float64 `json:"checkin_amount"`
	DrawAmount       float64 `json:"draw_amount"`
}
type ActivityStatsPage struct {
	Date        string               `json:"date"`
	RefreshedAt time.Time            `json:"refreshed_at"`
	Summary     ActivityStatsSummary `json:"summary"`
	Items       []ActivityStatsUser  `json:"items"`
	Total       int                  `json:"total"`
	Page        int                  `json:"page"`
	PageSize    int                  `json:"page_size"`
}

func activityDateRange(date string, now time.Time) (time.Time, time.Time, error) {
	if date == "" {
		date = day(now)
	}
	from, err := time.ParseInLocation("2006-01-02", date, time.FixedZone("Asia/Shanghai", 8*3600))
	if err != nil {
		return from, from, CampaignRuleError("日期格式应为 YYYY-MM-DD")
	}
	return from.UTC(), from.AddDate(0, 0, 1).UTC(), nil
}

// All accounts, bindings, payments, grants and activity aggregates use one
// consistent snapshot. Do not call LotteryState once per user (N+1 queries).
func (s *Service) ActivityStats(ctx context.Context, f ActivityStatsFilter, export bool) (ActivityStatsPage, error) {
	out := ActivityStatsPage{Items: []ActivityStatsUser{}, Page: f.Page, PageSize: f.PageSize, RefreshedAt: time.Now()}
	from, to, err := activityDateRange(f.Date, out.RefreshedAt)
	if err != nil {
		return out, err
	}
	out.Date = day(from)
	if f.Page < 1 || f.Page > 1000000 || f.PageSize < 1 || f.PageSize > 100 || len(f.Search) > 254 {
		return out, CampaignRuleError("分页或搜索参数不正确")
	}
	if f.Filter != "" && f.Filter != "chances" && f.Filter != "checkin" && f.Filter != "draws" {
		return out, CampaignRuleError("筛选条件不正确")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()
	c, err := loadLotteryConfig(ctx, tx)
	if err != nil {
		return out, err
	}
	users := map[int64]*ActivityStatsUser{}
	visit := func(query string, args []any, scan func(*sql.Rows) error) error {
		rows, e := tx.QueryContext(ctx, query, args...)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			if e = scan(rows); e != nil {
				return e
			}
		}
		return rows.Err()
	}
	err = visit(`SELECT id,email,status,deleted_at IS NOT NULL FROM users`, nil, func(rows *sql.Rows) error {
		u := &ActivityStatsUser{Members: []string{}}
		e := rows.Scan(&u.UserID, &u.Email, &u.Status, &u.Deleted)
		users[u.UserID] = u
		return e
	})
	if err != nil {
		return out, err
	}
	err = visit(`SELECT DISTINCT user_id,member_id FROM qqc_bindings ORDER BY member_id`, nil, func(rows *sql.Rows) error {
		var id int64
		var member string
		if e := rows.Scan(&id, &member); e != nil {
			return e
		}
		if u := users[id]; u != nil {
			u.Members = append(u.Members, member)
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	totals := map[int64]decimal.Decimal{}
	err = visit(`SELECT user_id,pay_amount,fee_rate,amount,refund_amount,status,COALESCE(provider_key,''),payment_type,provider_snapshot FROM payment_orders WHERE order_type='balance' AND paid_at IS NOT NULL AND completed_at IS NOT NULL AND completed_at >= $1`, []any{lotteryPeriodStart}, func(rows *sql.Rows) error {
		var id int64
		var paid, fee, credited, refunded decimal.Decimal
		var status, provider, method string
		var snapshot []byte
		if e := rows.Scan(&id, &paid, &fee, &credited, &refunded, &status, &provider, &method, &snapshot); e != nil {
			return e
		}
		if lotteryPaymentIsCNY(provider, method, snapshot) {
			totals[id] = totals[id].Add(lotteryOrderPrincipal(paid, fee, credited, refunded, status))
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	err = visit(`SELECT user_id,SUM(chances) FROM qqc_lottery_chance_grants WHERE created_at >= $1 GROUP BY user_id`, []any{lotteryPeriodStart}, func(rows *sql.Rows) error {
		var id, bonus int64
		if e := rows.Scan(&id, &bonus); e != nil {
			return e
		}
		if u := users[id]; u != nil {
			u.BonusChances = bonus
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	err = visit(`SELECT user_id,COUNT(*) FROM qqc_lottery_draws WHERE created_at >= $1 GROUP BY user_id`, []any{lotteryPeriodStart}, func(rows *sql.Rows) error {
		var id, used int64
		if e := rows.Scan(&id, &used); e != nil {
			return e
		}
		if u := users[id]; u != nil {
			u.Used = used
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	checkinTotal, drawTotal := decimal.Zero, decimal.Zero
	err = visit(`SELECT user_id,SUM(amount) FROM qqc_rewards WHERE kind='checkin' AND created_at >= $1 AND created_at < $2 GROUP BY user_id`, []any{from, to}, func(rows *sql.Rows) error {
		var id int64
		var amount decimal.Decimal
		if e := rows.Scan(&id, &amount); e != nil {
			return e
		}
		out.Summary.CheckinUsers++
		checkinTotal = checkinTotal.Add(amount)
		if u := users[id]; u != nil {
			u.CheckedIn = true
			u.CheckinAmount = amount.InexactFloat64()
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	err = visit(`SELECT user_id,COUNT(*),SUM(amount) FROM qqc_lottery_draws WHERE created_at >= $1 AND created_at < $2 GROUP BY user_id`, []any{from, to}, func(rows *sql.Rows) error {
		var id, count int64
		var amount decimal.Decimal
		if e := rows.Scan(&id, &count, &amount); e != nil {
			return e
		}
		out.Summary.DrawUsers++
		out.Summary.DrawCount += count
		drawTotal = drawTotal.Add(amount)
		if u := users[id]; u != nil {
			u.DayDraws = count
			u.DayDrawAmount = amount.InexactFloat64()
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	out.Summary.CheckinAmount = checkinTotal.InexactFloat64()
	out.Summary.DrawAmount = drawTotal.InexactFloat64()
	bonusUsage := map[int64]int64{}
	for _, u := range users {
		if u.BonusChances > 0 && u.Used > 0 {
			bonusUsage, err = lotteryBonusUsage(ctx, tx, 0)
			if err != nil {
				return out, err
			}
			break
		}
	}
	search := strings.ToLower(strings.TrimSpace(f.Search))
	for id, u := range users {
		state, e := calculateLotteryState(c, id, totals[id], u.Used, u.BonusChances, bonusUsage[id])
		if e != nil {
			return out, e
		}
		u.LotteryState = state
		available := u.Status == "active" && !u.Deleted && u.Remaining > 0
		if available {
			out.Summary.ChanceUsers++
			out.Summary.RemainingChances += u.Remaining
		}
		if f.Filter == "chances" && !available || f.Filter == "checkin" && !u.CheckedIn || f.Filter == "draws" && u.DayDraws == 0 {
			continue
		}
		if search != "" && strconv.FormatInt(id, 10) != search && !strings.Contains(strings.ToLower(u.Email), search) && !strings.Contains(strings.Join(u.Members, " "), search) {
			continue
		}
		out.Items = append(out.Items, *u)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		if out.Items[i].Remaining != out.Items[j].Remaining {
			return out.Items[i].Remaining > out.Items[j].Remaining
		}
		return out.Items[i].UserID < out.Items[j].UserID
	})
	out.Total = len(out.Items)
	if export {
		if out.Total > 10000 {
			return out, CampaignRuleError("导出最多10000条，请先缩小筛选范围")
		}
	} else {
		start := min((f.Page-1)*f.PageSize, out.Total)
		out.Items = out.Items[start:min(start+f.PageSize, out.Total)]
	}
	return out, tx.Commit()
}
