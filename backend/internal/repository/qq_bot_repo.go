package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/qqbot"
	"github.com/google/uuid"
)

// The configuration row is the first lock in all bot mutations. Besides making
// reward/quota operations atomic, it serializes disable/reconfigure with them.
type qqBotRepository struct {
	db    *sql.DB
	mysql bool
}

func NewQQBotRepository(db *sql.DB) qqbot.Repository {
	return &qqBotRepository{db: db, mysql: opsIsMySQLDB(db)}
}
func (r *qqBotRepository) q(s string) string {
	if r.mysql {
		return s
	}
	return opsRebindQuestionPlaceholders(s)
}
func (r *qqBotRepository) key() string {
	if r.mysql {
		return "`key`"
	}
	return `"key"`
}
func (r *qqBotRepository) ensure(ctx context.Context) error {
	raw, _ := json.Marshal(qqbot.DefaultConfig())
	q := "INSERT INTO settings (" + r.key() + ", value, updated_at) VALUES (?, ?, ?)"
	if r.mysql {
		q += " ON DUPLICATE KEY UPDATE " + r.key() + " = " + r.key()
	} else {
		q += " ON CONFLICT (\"key\") DO NOTHING"
	}
	_, err := r.db.ExecContext(ctx, r.q(q), qqbot.ConfigKey, string(raw), time.Now())
	return err
}
func (r *qqBotRepository) LoadConfig(ctx context.Context) (string, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, r.q("SELECT value FROM settings WHERE "+r.key()+" = ?"), qqbot.ConfigKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		rawBytes, _ := json.Marshal(qqbot.DefaultConfig())
		return string(rawBytes), nil
	}
	return raw, err
}
func (r *qqBotRepository) transaction(ctx context.Context, fn func(*sql.Tx, qqbot.Config) error) error {
	if err := r.ensure(ctx); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if err = tx.QueryRowContext(ctx, r.q("SELECT value FROM settings WHERE "+r.key()+" = ? FOR UPDATE"), qqbot.ConfigKey).Scan(&raw); err != nil {
		return err
	}
	c := qqbot.DefaultConfig()
	if err = json.Unmarshal([]byte(raw), &c); err != nil {
		return err
	}
	if err = fn(tx, c); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *qqBotRepository) SaveConfig(ctx context.Context, raw string, version int) error {
	return r.transaction(ctx, func(tx *sql.Tx, c qqbot.Config) error {
		if c.Version != version {
			return qqbot.ErrConflict
		}
		_, err := tx.ExecContext(ctx, r.q("UPDATE settings SET value = ?, updated_at = ? WHERE "+r.key()+" = ?"), raw, time.Now(), qqbot.ConfigKey)
		return err
	})
}
func (r *qqBotRepository) active(ctx context.Context, tx *sql.Tx, id int64) (float64, error) {
	var balance float64
	var status string
	err := tx.QueryRowContext(ctx, r.q("SELECT balance, status FROM users WHERE id = ? AND deleted_at IS NULL FOR UPDATE"), id).Scan(&balance, &status)
	if errors.Is(err, sql.ErrNoRows) || err == nil && status != "active" {
		return 0, qqbot.ErrInactiveUser
	}
	return balance, err
}
func (r *qqBotRepository) BindingStatus(ctx context.Context, id int64) (*qqbot.BindingStatus, error) {
	s := &qqbot.BindingStatus{}
	b := &qqbot.Binding{UserID: id}
	err := r.db.QueryRowContext(ctx, r.q("SELECT qq_id, group_id, created_at FROM qq_bot_bindings WHERE user_id = ?"), id).Scan(&b.QQ, &b.GroupID, &b.CreatedAt)
	if err == nil {
		s.Binding = b
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var expiry time.Time
	err = r.db.QueryRowContext(ctx, r.q("SELECT qq_id, expires_at FROM qq_bot_bind_requests WHERE user_id = ? AND expires_at > ?"), id, time.Now()).Scan(&s.PendingQQ, &expiry)
	if err == nil {
		s.ExpiresAt = &expiry
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return s, nil
}
func (r *qqBotRepository) CreateBindCode(ctx context.Context, id int64, hash string, expiry time.Time) error {
	return r.transaction(ctx, func(tx *sql.Tx, c qqbot.Config) error {
		if !c.Enabled {
			return qqbot.ErrDisabled
		}
		if _, err := r.active(ctx, tx, id); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRowContext(ctx, r.q("SELECT COUNT(*) FROM qq_bot_bindings WHERE user_id = ?"), id).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return qqbot.ErrBindingConflict
		}
		if _, err := tx.ExecContext(ctx, r.q("DELETE FROM qq_bot_bind_requests WHERE user_id = ?"), id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, r.q("INSERT INTO qq_bot_bind_requests (user_id, code_hash, expires_at) VALUES (?, ?, ?)"), id, hash, expiry)
		return err
	})
}
func (r *qqBotRepository) ClaimBindCode(ctx context.Context, hash, qq, group, bot string) error {
	return r.transaction(ctx, func(tx *sql.Tx, c qqbot.Config) error {
		if !c.Allows(bot, group) {
			return qqbot.ErrDisabled
		}
		var id int64
		var claimed string
		var expiry time.Time
		err := tx.QueryRowContext(ctx, r.q("SELECT user_id, qq_id, expires_at FROM qq_bot_bind_requests WHERE code_hash = ? FOR UPDATE"), hash).Scan(&id, &claimed, &expiry)
		if errors.Is(err, sql.ErrNoRows) || err == nil && (claimed != "" || !expiry.After(time.Now())) {
			return qqbot.ErrInvalidCode
		}
		if err != nil {
			return err
		}
		var count int
		if err = tx.QueryRowContext(ctx, r.q("SELECT COUNT(*) FROM qq_bot_bindings WHERE user_id = ? OR qq_id = ?"), id, qq).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return qqbot.ErrBindingConflict
		}
		if _, err = r.active(ctx, tx, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, r.q("UPDATE qq_bot_bind_requests SET qq_id = ?, group_id = ?, bot_id = ? WHERE user_id = ?"), qq, group, bot, id)
		return err
	})
}
func (r *qqBotRepository) ConfirmBinding(ctx context.Context, id int64, expectedQQ string) (*qqbot.Binding, error) {
	b := &qqbot.Binding{UserID: id, CreatedAt: time.Now()}
	err := r.transaction(ctx, func(tx *sql.Tx, c qqbot.Config) error {
		var bot string
		var expiry time.Time
		err := tx.QueryRowContext(ctx, r.q("SELECT qq_id, group_id, bot_id, expires_at FROM qq_bot_bind_requests WHERE user_id = ? FOR UPDATE"), id).Scan(&b.QQ, &b.GroupID, &bot, &expiry)
		if errors.Is(err, sql.ErrNoRows) || err == nil && (b.QQ == "" || !expiry.After(time.Now())) {
			return qqbot.ErrInvalidCode
		}
		if err != nil {
			return err
		}
		if !c.Allows(bot, b.GroupID) {
			return qqbot.ErrDisabled
		}
		if b.QQ != expectedQQ {
			return qqbot.ErrConflict
		}
		if _, err = r.active(ctx, tx, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, r.q("INSERT INTO qq_bot_bindings (user_id, qq_id, group_id, created_at) VALUES (?, ?, ?, ?)"), id, b.QQ, b.GroupID, b.CreatedAt); err != nil {
			if isUniqueConstraintViolation(err) {
				return qqbot.ErrBindingConflict
			}
			return err
		}
		_, err = tx.ExecContext(ctx, r.q("DELETE FROM qq_bot_bind_requests WHERE user_id = ?"), id)
		return err
	})
	return b, err
}
func (r *qqBotRepository) Unbind(ctx context.Context, id int64) error {
	return r.transaction(ctx, func(tx *sql.Tx, _ qqbot.Config) error {
		// Remove old conversations as well; a later binding must not inherit them.
		if _, err := tx.ExecContext(ctx, r.q("DELETE FROM qq_bot_conversations WHERE qq_id IN (SELECT qq_id FROM qq_bot_bindings WHERE user_id = ?)"), id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, r.q("DELETE FROM qq_bot_bind_requests WHERE user_id = ?"), id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, r.q("DELETE FROM qq_bot_bindings WHERE user_id = ?"), id)
		return err
	})
}
func (r *qqBotRepository) Account(ctx context.Context, qq string) (*qqbot.Account, error) {
	a := &qqbot.Account{}
	var status string
	err := r.db.QueryRowContext(ctx, r.q("SELECT u.id, u.balance, u.status FROM users u JOIN qq_bot_bindings b ON b.user_id = u.id WHERE b.qq_id = ? AND u.deleted_at IS NULL"), qq).Scan(&a.UserID, &a.Balance, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, qqbot.ErrNotBound
	}
	if err != nil {
		return nil, err
	}
	a.Active = status == "active"
	if !a.Active {
		return nil, qqbot.ErrInactiveUser
	}
	return a, nil
}
func (r *qqBotRepository) boundUser(ctx context.Context, tx *sql.Tx, qq string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, r.q("SELECT user_id FROM qq_bot_bindings WHERE qq_id = ? FOR UPDATE"), qq).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, qqbot.ErrNotBound
	}
	return id, err
}
func (r *qqBotRepository) Checkin(ctx context.Context, qq, group, bot string, now time.Time) (*qqbot.Reward, error) {
	reward := &qqbot.Reward{ID: uuid.NewString(), QQ: qq, GroupID: group, Day: now.In(qqbot.Shanghai).Format("2006-01-02"), CreatedAt: now}
	err := r.transaction(ctx, func(tx *sql.Tx, c qqbot.Config) error {
		if !c.Allows(bot, group) {
			return qqbot.ErrDisabled
		}
		if c.CheckinReward <= 0 {
			return qqbot.ErrRewardsDisabled
		}
		id, err := r.boundUser(ctx, tx, qq)
		if err != nil {
			return err
		}
		reward.UserID = id
		_, err = r.active(ctx, tx, id)
		if err != nil {
			return err
		}
		reward.Amount, err = c.RewardAmount()
		if err != nil {
			return err
		}
		amount := strconv.FormatFloat(reward.Amount, 'f', 8, 64)
		if _, err = tx.ExecContext(ctx, r.q("INSERT INTO qq_bot_rewards (id,user_id,qq_id,group_id,reward_day,amount,balance_after,created_at) VALUES (?,?,?,?,?,?,?,?)"), reward.ID, id, qq, group, reward.Day, amount, 0, now); err != nil {
			if isUniqueConstraintViolation(err) {
				return qqbot.ErrAlreadyCheckedIn
			}
			return err
		}
		_, err = tx.ExecContext(ctx, r.q("UPDATE users SET balance = balance + CAST(? AS DECIMAL(20,8)), updated_at = ? WHERE id = ?"), amount, now, id)
		if err != nil {
			return err
		}
		// Read the database's decimal result rather than persisting float addition
		// as the final ledger balance (large balances can lose a decimal place).
		if err = tx.QueryRowContext(ctx, r.q("SELECT balance FROM users WHERE id = ?"), id).Scan(&reward.BalanceAfter); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, r.q("UPDATE qq_bot_rewards SET balance_after = (SELECT balance FROM users WHERE id = ?) WHERE id = ?"), id, reward.ID)
		return err
	})
	if errors.Is(err, qqbot.ErrAlreadyCheckedIn) {
		// A failed insert aborts PostgreSQL transactions, so load the existing
		// claim only after transaction() has rolled back. Never redraw a reward.
		if lookupErr := r.db.QueryRowContext(ctx, r.q("SELECT amount, balance_after FROM qq_bot_rewards WHERE user_id = ? AND reward_day = ?"), reward.UserID, reward.Day).Scan(&reward.Amount, &reward.BalanceAfter); lookupErr != nil {
			return nil, lookupErr
		}
	}
	return reward, err
}
func (r *qqBotRepository) BeginEvent(ctx context.Context, e qqbot.EventRecord) (bool, error) {
	_, err := r.db.ExecContext(ctx, r.q("INSERT INTO qq_bot_events (id,qq_id,group_id,command,status,result_text,created_at) VALUES (?,?,?,?,?,?,?)"), e.ID, e.QQ, e.GroupID, e.Command, "processing", "", time.Now())
	if isUniqueConstraintViolation(err) {
		return false, nil
	}
	return err == nil, err
}
func (r *qqBotRepository) FinishEvent(ctx context.Context, id, status, result string) error {
	_, err := r.db.ExecContext(ctx, r.q("UPDATE qq_bot_events SET status = ?, result_text = ?, finished_at = ? WHERE id = ?"), status, result, time.Now(), id)
	return err
}
func (r *qqBotRepository) ReserveChat(ctx context.Context, qq, group, bot, event string, now time.Time, version int) ([]qqbot.Message, error) {
	history := []qqbot.Message{}
	err := r.transaction(ctx, func(tx *sql.Tx, c qqbot.Config) error {
		if !c.Allows(bot, group) || !c.ChatEnabled {
			return qqbot.ErrDisabled
		}
		if c.Version != version {
			return qqbot.ErrConflict
		}
		id, err := r.boundUser(ctx, tx, qq)
		if err != nil {
			return err
		}
		if _, err = r.active(ctx, tx, id); err != nil {
			return err
		}
		// Configuration lock serializes inserts, so no dialect-specific upsert is needed.
		day := now.In(qqbot.Shanghai).Format("2006-01-02")
		scopes := []string{"site", fmt.Sprintf("user:%d", id)}
		limits := []int{c.TotalDailyChatLimit, c.DailyChatLimit}
		for i, scope := range scopes {
			var count int
			var last time.Time
			err = tx.QueryRowContext(ctx, r.q("SELECT request_count,last_request_at FROM qq_bot_daily_usage WHERE usage_day = ? AND scope = ? FOR UPDATE"), day, scope).Scan(&count, &last)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if count >= limits[i] {
				return qqbot.ErrChatLimit
			}
			if i == 1 && count > 0 && now.Sub(last) < time.Duration(c.CooldownSeconds)*time.Second {
				return qqbot.ErrCooldown
			}
			if errors.Is(err, sql.ErrNoRows) {
				_, err = tx.ExecContext(ctx, r.q("INSERT INTO qq_bot_daily_usage (usage_day,scope,request_count,last_request_at) VALUES (?,?,1,?)"), day, scope, now)
			} else {
				_, err = tx.ExecContext(ctx, r.q("UPDATE qq_bot_daily_usage SET request_count = request_count + 1, last_request_at = ? WHERE usage_day = ? AND scope = ?"), now, day, scope)
			}
			if err != nil {
				return err
			}
		}
		var raw string
		var until time.Time
		err = tx.QueryRowContext(ctx, r.q("SELECT messages_json,lease_until FROM qq_bot_conversations WHERE qq_id = ? AND group_id = ? FOR UPDATE"), qq, group).Scan(&raw, &until)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.ExecContext(ctx, r.q("INSERT INTO qq_bot_conversations (qq_id,group_id,messages_json,lease_token,lease_until,updated_at) VALUES (?,?,?,?,?,?)"), qq, group, "[]", event, now.Add(3*time.Minute), now)
		} else if err == nil {
			if until.After(now) {
				return qqbot.ErrCooldown
			}
			if err = json.Unmarshal([]byte(raw), &history); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, r.q("UPDATE qq_bot_conversations SET lease_token = ?, lease_until = ?, updated_at = ? WHERE qq_id = ? AND group_id = ?"), event, now.Add(3*time.Minute), now, qq, group)
		}
		return err
	})
	return history, err
}
func (r *qqBotRepository) FinishChat(ctx context.Context, qq, group, event string, messages []qqbot.Message) error {
	raw, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, r.q("UPDATE qq_bot_conversations SET messages_json = ?, lease_token = '', lease_until = ?, updated_at = ? WHERE qq_id = ? AND group_id = ? AND lease_token = ?"), string(raw), time.Now().Add(-time.Minute), time.Now(), qq, group, event)
	return err
}
func (r *qqBotRepository) ClearHistory(ctx context.Context, qq, group string) error {
	return r.transaction(ctx, func(tx *sql.Tx, c qqbot.Config) error {
		// Do not let an in-flight completion restore a cleared history.
		var until time.Time
		err := tx.QueryRowContext(ctx, r.q("SELECT lease_until FROM qq_bot_conversations WHERE qq_id = ? AND group_id = ? FOR UPDATE"), qq, group).Scan(&until)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if until.After(time.Now()) {
			return qqbot.ErrCooldown
		}
		_, err = tx.ExecContext(ctx, r.q("DELETE FROM qq_bot_conversations WHERE qq_id = ? AND group_id = ?"), qq, group)
		return err
	})
}

func (r *qqBotRepository) list(ctx context.Context, table, columns string, page, size int, scan func(*sql.Rows) error) (int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&total); err != nil {
		return 0, err
	}
	rows, err := r.db.QueryContext(ctx, r.q("SELECT "+columns+" FROM "+table+" ORDER BY created_at DESC LIMIT ? OFFSET ?"), size, (page-1)*size)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		if err = scan(rows); err != nil {
			return 0, err
		}
	}
	return total, rows.Err()
}
func (r *qqBotRepository) ListBindings(ctx context.Context, page, size int) ([]qqbot.Binding, int64, error) {
	items := []qqbot.Binding{}
	total, err := r.list(ctx, "qq_bot_bindings", "user_id,qq_id,group_id,created_at", page, size, func(rows *sql.Rows) error {
		var b qqbot.Binding
		err := rows.Scan(&b.UserID, &b.QQ, &b.GroupID, &b.CreatedAt)
		items = append(items, b)
		return err
	})
	return items, total, err
}
func (r *qqBotRepository) ListRewards(ctx context.Context, page, size int) ([]qqbot.Reward, int64, error) {
	items := []qqbot.Reward{}
	total, err := r.list(ctx, "qq_bot_rewards", "id,user_id,qq_id,group_id,reward_day,amount,balance_after,created_at", page, size, func(rows *sql.Rows) error {
		var b qqbot.Reward
		var day time.Time
		err := rows.Scan(&b.ID, &b.UserID, &b.QQ, &b.GroupID, &day, &b.Amount, &b.BalanceAfter, &b.CreatedAt)
		b.Day = day.Format("2006-01-02")
		items = append(items, b)
		return err
	})
	return items, total, err
}
func (r *qqBotRepository) ListEvents(ctx context.Context, page, size int) ([]qqbot.EventRecord, int64, error) {
	items := []qqbot.EventRecord{}
	total, err := r.list(ctx, "qq_bot_events", "id,qq_id,group_id,command,status,result_text,created_at,finished_at", page, size, func(rows *sql.Rows) error {
		var b qqbot.EventRecord
		err := rows.Scan(&b.ID, &b.QQ, &b.GroupID, &b.Command, &b.Status, &b.Result, &b.CreatedAt, &b.FinishedAt)
		items = append(items, b)
		return err
	})
	return items, total, err
}
