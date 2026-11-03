package qqcommunity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
)

const bindFirst = "请先发送“绑定 邮箱或绑定码”绑定网站账号。"

func formatUsageTokens(tokens int64) string {
	switch {
	case tokens >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(tokens)/1_000_000_000)
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(tokens)/1_000_000)
	case tokens >= 1_000:
		return fmt.Sprintf("%.2fK", float64(tokens)/1_000)
	default:
		return strconv.FormatInt(tokens, 10)
	}
}

func boundUser(ctx context.Context, db queryRower, app, member string) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, `SELECT b.user_id FROM qqc_bindings b JOIN users u ON u.id=b.user_id WHERE b.app_id=$1 AND b.member_id=$2 AND u.status='active' AND u.deleted_at IS NULL`, app, member).Scan(&id)
	return id, err
}

func (s *Service) todayUsage(ctx context.Context, c Config, j Job, now time.Time) (string, error) {
	text, _, err := s.todayUsageDocument(ctx, c, j, now)
	return text, err
}

func (s *Service) todayUsageDocument(ctx context.Context, c Config, j Job, now time.Time) (string, *replyDocument, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = tx.Rollback() }()
	uid, err := boundUser(ctx, tx, c.AppID, j.MemberID)
	if errors.Is(err, sql.ErrNoRows) {
		return bindFirst, nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	local := now.In(time.FixedZone("Asia/Shanghai", 8*3600))
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	rows, err := tx.QueryContext(ctx, `SELECT model,COUNT(*),COALESCE(SUM(input_tokens),0),COALESCE(SUM(output_tokens),0),COALESCE(SUM(cache_creation_tokens),0),COALESCE(SUM(cache_read_tokens),0),COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE user_id=$1 AND created_at >= $2 AND created_at <= $3 GROUP BY model ORDER BY SUM(actual_cost) DESC,COUNT(*) DESC,model`, uid, start, now)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = rows.Close() }()
	doc := &replyDocument{UserID: uid, Title: "模型用量 · " + local.Format("2006-01-02"), ChartNote: "北京时间 · 本人绑定账号 · 按模型汇总"}
	text := "📊 **" + doc.Title + "**"
	for rows.Next() {
		var m ModelUsageStat
		if err = rows.Scan(&m.Model, &m.Requests, &m.Input, &m.Output, &m.CacheCreation, &m.CacheRead, &m.Cost); err != nil {
			return "", nil, err
		}
		m.Total = m.Input + m.Output + m.CacheCreation + m.CacheRead
		doc.Models = append(doc.Models, m)
		doc.CostBars = append(doc.CostBars, replyBar{Label: m.Model, Value: m.Cost, Display: fmt.Sprintf("$%.4f", m.Cost)})
		text += fmt.Sprintf("\n\n%s：%d 次 · %s Token · USD %.4f", replyPlainLabel(m.Model), m.Requests, formatUsageTokens(m.Total), m.Cost)
	}
	if err = rows.Err(); err != nil {
		return "", nil, err
	}
	if err = rows.Close(); err != nil {
		return "", nil, err
	}
	if len(doc.Models) == 0 {
		text += "\n今日暂无模型用量。"
	}
	if err = tx.Commit(); err != nil {
		return "", nil, err
	}
	return text, doc, nil
}
