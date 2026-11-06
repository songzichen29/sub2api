package qqcommunity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrCheckinDay         = errors.New("签到日期已更新，请刷新后重试")
	ErrCheckinMonth       = errors.New("签到月份不正确")
	ErrCheckinClosed      = errors.New("签到活动暂未开放")
	ErrCheckinAccount     = errors.New("账户不可用")
	ErrCheckinEligibility = errors.New("尚未满足签到充值门槛")
)

var checkinZone = time.FixedZone("Asia/Shanghai", 8*60*60)

type CheckinDay struct {
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
	Source string  `json:"source"`
}

type CheckinBonus struct {
	Reward GrowthReward `json:"reward"`
	Status string       `json:"status"`
}

type UserCheckin struct {
	Today          string        `json:"today"`
	Month          string        `json:"month"`
	ServerTime     time.Time     `json:"server_time"`
	NextResetAt    time.Time     `json:"next_reset_at"`
	Enabled        bool          `json:"enabled"`
	Eligible       bool          `json:"eligible"`
	CheckedIn      bool          `json:"checked_in"`
	CanCheckin     bool          `json:"can_checkin"`
	TodayAmount    float64       `json:"today_amount"`
	TodaySource    string        `json:"today_source"`
	MinAmount      float64       `json:"min_amount"`
	MaxAmount      float64       `json:"max_amount"`
	TotalDays      int           `json:"total_days"`
	StreakDays     int           `json:"streak_days"`
	TotalAmount    float64       `json:"total_amount"`
	StreakInterval int           `json:"streak_interval"`
	StreakReward   *GrowthReward `json:"streak_reward"`
	TodayBonus     *CheckinBonus `json:"today_bonus"`
	Days           []CheckinDay  `json:"days"`
	RechargeRequirement
}

type UserCheckinResult struct {
	CheckinDay
	AlreadyCheckedIn bool          `json:"already_checked_in"`
	Bonus            *CheckinBonus `json:"bonus"`
}

func checkinMonth(month string, at time.Time) (time.Time, error) {
	if month == "" {
		month = at.In(checkinZone).Format("2006-01")
	}
	start, err := time.ParseInLocation("2006-01", month, checkinZone)
	if err != nil || start.Year() < 2000 || month > at.In(checkinZone).Format("2006-01") {
		return time.Time{}, ErrCheckinMonth
	}
	return start, nil
}

// The website and QQ share account/day records, including calendar and streaks.
// Only public activity settings are returned; no QQ credentials or identities.
func (s *Service) UserCheckin(ctx context.Context, uid int64, month string) (UserCheckin, error) {
	out := UserCheckin{Days: []CheckinDay{}}
	if uid <= 0 {
		return out, ErrCheckinAccount
	}
	at := time.Now()
	start, err := checkinMonth(month, at)
	if err != nil {
		return out, err
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
	cfg, err := loadCheckinConfig(ctx, tx, c.CheckinReward)
	if err != nil {
		return out, err
	}
	out.Today, out.Month, out.ServerTime = day(at), start.Format("2006-01"), at
	today, _ := time.ParseInLocation(time.DateOnly, out.Today, checkinZone)
	out.NextResetAt = today.AddDate(0, 0, 1)
	out.Enabled, out.MinAmount, out.MaxAmount = cfg.Enabled, cfg.MinAmount, cfg.MaxAmount
	out.RechargeRequirement = cfg.RechargeRequirement
	out.Eligible = true
	if cfg.RechargeRequirement.enabled() {
		eligibility, e := cfg.eligibility(ctx, tx, uid, false)
		if e != nil {
			return out, e
		}
		out.Eligible = cfg.satisfied(eligibility)
	}
	record, err := readCheckinDay(ctx, tx, uid, out.Today)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	out.CheckedIn = err == nil
	out.TodayAmount, out.TodaySource = record.Amount, record.Source
	out.CanCheckin = cfg.Enabled && out.Eligible && !out.CheckedIn
	out.TotalDays, out.StreakDays, err = checkinProgress(ctx, tx, uid, at)
	if err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount),0) FROM qqc_rewards WHERE user_id=$1 AND kind='checkin'`, uid).Scan(&out.TotalAmount)
	if err != nil {
		return out, err
	}
	out.StreakInterval = cfg.streakDays()
	if reward := cfg.streakReward(); reward.enabled() {
		out.StreakReward = &reward
	}
	if out.CheckedIn {
		out.TodayBonus, err = readCheckinBonus(ctx, tx, uid, out.Today)
		if err != nil {
			return out, err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT created_at,amount,app_id FROM qqc_rewards WHERE user_id=$1 AND kind='checkin' AND created_at >= $2 AND created_at < $3 ORDER BY created_at`, uid, start, start.AddDate(0, 1, 0))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var created time.Time
		var amount float64
		var app string
		if err = rows.Scan(&created, &amount, &app); err != nil {
			return out, err
		}
		out.Days = append(out.Days, CheckinDay{Date: day(created), Amount: amount, Source: checkinSource(app)})
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func checkinSource(app string) string {
	if app == "website" {
		return "website"
	}
	return "qq"
}

func readCheckinDay(ctx context.Context, tx *transaction, uid int64, date string) (CheckinDay, error) {
	out := CheckinDay{Date: date}
	start, err := time.ParseInLocation(time.DateOnly, date, checkinZone)
	if err != nil {
		return out, ErrCheckinDay
	}
	var app string
	err = tx.QueryRowContext(ctx, `SELECT amount,app_id FROM qqc_rewards WHERE user_id=$1 AND kind='checkin' AND created_at >= $2 AND created_at < $3`, uid, start, start.AddDate(0, 0, 1)).Scan(&out.Amount, &app)
	if err == nil {
		out.Source = checkinSource(app)
	}
	return out, err
}

func readCheckinBonus(ctx context.Context, tx *transaction, uid int64, date string) (*CheckinBonus, error) {
	reward, status, err := activityRewardSnapshot(ctx, tx, fmt.Sprintf("qq-streak:%d:%s", uid, date))
	if err != nil || status == "" {
		return nil, err
	}
	return &CheckinBonus{Reward: reward, Status: status}, nil
}

func (s *Service) ClaimUserCheckin(ctx context.Context, uid int64, requestedDay string) (UserCheckinResult, error) {
	return s.claimUserCheckin(ctx, uid, requestedDay, time.Now)
}

func (s *Service) claimUserCheckin(ctx context.Context, uid int64, requestedDay string, now func() time.Time) (UserCheckinResult, error) {
	var out UserCheckinResult
	if uid <= 0 {
		return out, ErrCheckinAccount
	}
	if _, err := time.Parse(time.DateOnly, requestedDay); err != nil {
		return out, ErrCheckinDay
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	c, err := s.lockConfig(ctx, tx)
	if err != nil {
		return out, err
	}
	// Same lock order as QQ check-ins and growth settlement, on both dialects.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,272))`, c.AppID); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared($1)`, checkinConfigLock); err != nil {
		return out, err
	}
	var active int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL FOR UPDATE`, uid).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrCheckinAccount
	}
	if err != nil {
		return out, err
	}
	// Compute the day after all blocking locks. A retry across midnight must
	// never silently consume tomorrow's check-in.
	at := now()
	if requestedDay != day(at) {
		return out, ErrCheckinDay
	}
	out.CheckinDay, err = readCheckinDay(ctx, tx, uid, requestedDay)
	if err == nil {
		out.AlreadyCheckedIn = true
		out.Bonus, err = readCheckinBonus(ctx, tx, uid, requestedDay)
		if err != nil {
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
	cfg, err := loadCheckinConfig(ctx, tx, c.CheckinReward)
	if err != nil {
		return out, err
	}
	// This activity is independent of the QQ transport and QQ membership.
	if !cfg.Enabled {
		return out, ErrCheckinClosed
	}
	if cfg.RechargeRequirement.enabled() {
		eligibility, e := cfg.eligibility(ctx, tx, uid, true)
		if e != nil {
			return out, e
		}
		if !cfg.satisfied(eligibility) {
			return out, ErrCheckinEligibility
		}
	}
	amount, err := randomCheckinAmount(cfg)
	if err != nil {
		return out, err
	}
	key := fmt.Sprintf("checkin:%d:%s", uid, requestedDay)
	var rewardID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO qqc_rewards(reward_key,app_id,group_id,member_id,user_id,kind,amount,created_at) VALUES($1,'website','','',$2,'checkin',$3,$4) ON CONFLICT(reward_key) DO NOTHING RETURNING id`, key, uid, amount, at).Scan(&rewardID)
	if err != nil {
		return out, err
	}
	if amount > 0 {
		res, e := tx.ExecContext(ctx, `UPDATE users SET balance=balance+$1,updated_at=NOW() WHERE id=$2 AND status='active' AND deleted_at IS NULL`, amount, uid)
		if e != nil {
			return out, e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return out, e
		}
		if n != 1 {
			return out, ErrCheckinAccount
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO redeem_codes(code,type,value,status,used_by,used_at,notes,created_at) VALUES($1,'admin_balance',$2,'used',$3,$4,$5,$4)`, "QQ"+randomID(), amount, uid, at, fmt.Sprintf("Website checkin reward #%d", rewardID)); err != nil {
			return out, err
		}
	}
	// Keep ancillary rewards in the existing admin fulfillment namespace.
	if _, _, err = grantStreak(ctx, tx, c, Job{}, uid, at, cfg.streakReward(), cfg.streakDays()); err != nil {
		return out, err
	}
	var bound bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM qqc_bindings WHERE app_id=$1 AND user_id=$2)`, c.AppID, uid).Scan(&bound)
	if err != nil {
		return out, err
	}
	if !bound {
		cfg = withoutBindingTask(cfg)
	}
	if _, _, _, err = settleGrowthForUser(ctx, tx, c, Job{}, uid, cfg, at); err != nil {
		return out, err
	}
	out = UserCheckinResult{CheckinDay: CheckinDay{Date: requestedDay, Amount: amount, Source: "website"}}
	out.Bonus, err = readCheckinBonus(ctx, tx, uid, requestedDay)
	if err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	s.invalidate(ctx, uid)
	return out, nil
}

// Growth's QQ caller already proves binding. A website caller must not gain
// that reward merely by signing in. Copy the slice; never change saved settings.
func withoutBindingTask(cfg CheckinConfig) CheckinConfig {
	tasks := make([]GrowthTask, 0)
	for _, task := range cfg.Growth.tasks() {
		if task.Kind != "binding" {
			tasks = append(tasks, task)
		}
	}
	cfg.Growth.Tasks = tasks
	return cfg
}
