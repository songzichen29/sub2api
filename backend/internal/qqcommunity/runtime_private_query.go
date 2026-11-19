package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const runtimePrivateQueryCreate = "RUNTIME_PRIVATE_QUERY_CREATE"

func (s *Service) enqueueRuntimePrivateQuery(ctx context.Context, c Config, e Event, raw json.RawMessage) error {
	if e.GroupID != "" || !c.allowsPrivateChat(e.Author.MemberID) || !runtimeMessageID.MatchString(e.ID) {
		return ErrDisabled
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(71427102)`); err != nil {
		return err
	}
	var queued int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM qqc_jobs WHERE status='queued'`).Scan(&queued); err != nil {
		return err
	}
	if queued >= 200 {
		return errors.New("机器人队列已满")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO qqc_jobs(id,app_id,event_key,event_type,group_id,member_id,message_id,payload,kind)
 VALUES($1,$2,$3,$4,'',$5,$6,$7,'query') ON CONFLICT(app_id,event_key) DO NOTHING`, randomID(), c.AppID, runtimePrivateQueryCreate+":"+e.Author.MemberID+":"+e.ID, runtimePrivateQueryCreate, e.Author.MemberID, e.ID, string(raw))
	if err != nil {
		return err
	}
	return tx.Commit()
}

// 账户数据只由业务服务私聊发送，不交给模型，也不进入共享聊天记忆。
func (s *Service) runRuntimePrivateQuery(ctx context.Context, c Config, j Job) error {
	var e Event
	if err := json.Unmarshal(j.Payload, &e); err != nil {
		return err
	}
	cmd, arg := command(e.Content)
	kind := map[string]string{"我的余额": "account_balance", "今日用量": "today_usage", "抽奖次数": "lottery_chances"}[cmd]
	if kind == "" || arg != "" || e.Author.MemberID != j.MemberID || e.GroupID != "" {
		return errors.New("私聊仅支持查询本人余额、今日用量和抽奖次数")
	}
	err := s.db.QueryRowContext(ctx, `SELECT b.user_id,b.created_at FROM qqc_bindings b JOIN users u ON u.id=b.user_id WHERE b.app_id=$1 AND b.member_id=$2 AND u.status='active' AND u.deleted_at IS NULL`, c.AppID, j.MemberID).Scan(&j.ExpectedUserID, &j.ExpectedBindingAt)
	if errors.Is(err, sql.ErrNoRows) {
		return s.sendRuntimePrivateQueryNotice(ctx, j, "尚未绑定可用账号，请先在已开放群发送 /绑定，并按私聊提示完成网站确认。")
	}
	if err != nil {
		return errors.Join(err, s.sendRuntimePrivateQueryNotice(ctx, j, "账户绑定状态查询失败，本次未取得账户数据，请稍后重试。"))
	}
	result := s.querySiteAccount(ctx, c, j, siteQuery{Kind: kind}, time.Now())
	if result.Decision.Status == "failed" {
		return errors.Join(errors.New(result.Decision.ErrorCode), s.send(ctx, c, j, result.Text, nil, 1))
	}
	return s.send(ctx, c, j, result.Text, nil, 1)
}

// 不含账户数据的绑定提示同样检查最新私聊权限，但不要求已有绑定。
func (s *Service) sendRuntimePrivateQueryNotice(ctx context.Context, j Job, text string) error {
	c, err := s.Config(ctx)
	if err != nil {
		return err
	}
	if c.AppID != j.AppID || !c.allowsPrivateChat(j.MemberID) {
		return ErrDisabled
	}
	if time.Since(j.CreatedAt) >= passiveReplyWindow {
		return errReplyWindow
	}
	return s.sendOneBotPrivate(ctx, c, j.MemberID, "", text)
}
