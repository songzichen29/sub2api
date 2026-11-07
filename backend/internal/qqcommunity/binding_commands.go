package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const bindingCommandEvent = "BINDING_COMMAND_RESUME"
const privateBindingSent = "已私聊，请注意查看。"
const privateBindingFailed = "私聊消息未确认送达，请开启群临时会话或添加机器人为好友后，再发送原指令。"

// Only known account commands can be resumed. Chat, model tools, custom
// commands and raw private messages must never become deferred mutations.
func requiresBinding(cmd string) bool {
	switch cmd {
	case "签到", "我的余额", "今日用量", "我的奖励", "成长任务", "抽奖", "抽奖次数", "我的抽奖", "抽奖活动", "转账", "领码":
		return true
	}
	return false
}

type bindingCommandPayload struct {
	Event     Event     `json:"event"`
	UserID    int64     `json:"user_id"`
	BindingAt time.Time `json:"binding_at"`
}

// ConfirmBinding calls this inside the same transaction as the new binding.
// Consuming the pending row and inserting a unique job either both commit or
// neither does. Repeating confirmation cannot dispatch a second operation.
func enqueueBindingCommand(ctx context.Context, tx *transaction, c Config, user int64, member, group string) error {
	var source, message, nickname, content string
	err := tx.QueryRowContext(ctx, `SELECT source_job_id,message_id,nickname,command_text FROM qqc_binding_commands
 WHERE app_id=$1 AND member_id=$2 AND user_id=$3 AND group_id=$4 AND delivered=TRUE AND expires_at>clock_timestamp() FOR UPDATE`, c.AppID, member, user, group).Scan(&source, &message, &nickname, &content)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		cmd, _ := command(content)
		if !requiresBinding(cmd) {
			return errors.New("待执行指令无效")
		}
		var bindingAt time.Time
		if err = tx.QueryRowContext(ctx, `SELECT created_at FROM qqc_bindings WHERE app_id=$1 AND member_id=$2 AND user_id=$3`, c.AppID, member, user).Scan(&bindingAt); err != nil {
			return err
		}
		e := Event{ID: message, GroupID: group, Content: content, CommandReply: true}
		e.Author.MemberID, e.Author.Username = member, nickname
		raw, err := json.Marshal(bindingCommandPayload{Event: e, UserID: user, BindingAt: bindingAt})
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO qqc_jobs(id,app_id,event_key,event_type,group_id,member_id,message_id,payload,member_nickname,message_preview)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(app_id,event_key) DO NOTHING`, randomID(), c.AppID, "binding-command:"+source, bindingCommandEvent, group, member, message, string(raw), nickname, "/"+cmd)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM qqc_binding_commands WHERE app_id=$1 AND member_id=$2`, c.AppID, member)
	return err
}

// Called again inside money transactions, after their normal serialization
// lock. A queued command cannot use a later replacement binding.
func ensureResumedBinding(ctx context.Context, tx *transaction, c Config, j Job) error {
	if !j.PrivateReply {
		return nil
	}
	var uid int64
	err := tx.QueryRowContext(ctx, `SELECT b.user_id FROM qqc_bindings b JOIN users u ON u.id=b.user_id
 WHERE b.app_id=$1 AND b.member_id=$2 AND b.user_id=$3 AND b.created_at=$4 AND u.status='active' AND u.deleted_at IS NULL FOR SHARE OF b`, c.AppID, j.MemberID, j.ExpectedUserID, j.ExpectedBindingAt).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrBindingChanged
	}
	return err
}

func (s *Service) resumeBindingCommand(ctx context.Context, c Config, j *Job) error {
	var p bindingCommandPayload
	if json.Unmarshal(j.Payload, &p) != nil || p.UserID <= 0 || p.BindingAt.IsZero() || p.Event.Author.MemberID != j.MemberID || p.Event.GroupID != j.GroupID {
		return errors.New("绑定后指令格式无效")
	}
	cmd, _ := command(p.Event.Content)
	if !requiresBinding(cmd) {
		return errors.New("绑定后指令不支持自动执行")
	}
	j.PrivateReply, j.ExpectedUserID, j.ExpectedBindingAt = true, p.UserID, p.BindingAt
	if err := s.checkPrivateReplyBinding(ctx, c, *j); err != nil {
		return err
	}
	if time.Since(j.CreatedAt) >= passiveReplyWindow {
		return errReplyWindow
	}
	return s.message(ctx, c, j, p.Event)
}

func (s *Service) checkPrivateReplyBinding(ctx context.Context, c Config, j Job) error {
	var valid bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM qqc_bindings b JOIN users u ON u.id=b.user_id
 WHERE b.app_id=$1 AND b.member_id=$2 AND b.user_id=$3 AND b.created_at=$4 AND u.status='active' AND u.deleted_at IS NULL)`, c.AppID, j.MemberID, j.ExpectedUserID, j.ExpectedBindingAt).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrBindingChanged
	}
	return nil
}

func (s *Service) checkinWithBalance(ctx context.Context, c Config, j Job) (string, error) {
	text, err := s.reward(ctx, c, j, "checkin")
	if err != nil {
		return text, err
	}
	var balance string
	err = s.db.QueryRowContext(ctx, `SELECT u.balance::text FROM qqc_bindings b JOIN users u ON u.id=b.user_id WHERE b.app_id=$1 AND b.member_id=$2 AND u.status='active' AND u.deleted_at IS NULL`, c.AppID, j.MemberID).Scan(&balance)
	if err == nil {
		text += fmt.Sprintf("\n当前余额：USD %s", balance)
	}
	return text, nil // A balance read failure must never retry a credited reward.
}
