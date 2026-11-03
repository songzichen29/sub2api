package qqcommunity

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const privateBindingWindow = 10 * time.Minute

type privateBindingSession struct {
	requestID string
	groupID   string
	nickname  string
	expiresAt time.Time
}

// A group message authorizes a short private exchange for exactly that QQ ID.
// The website account owner still has to approve the displayed QQ identity.
func (s *Service) beginPrivateBinding(ctx context.Context, c Config, e Event) error {
	return s.beginPrivateBindingCommand(ctx, c, e, nil, "")
}

func (s *Service) beginPrivateBindingCommand(ctx context.Context, c Config, e Event, job *Job, content string) error {
	if !c.Enabled || !c.allows(e.GroupID) || e.Author.MemberID == "" || e.Author.MemberID == c.AppID {
		return ErrDisabled
	}
	if job != nil {
		cmd, _ := command(content)
		if job.ID == "" || !requiresBinding(cmd) {
			return errors.New("无法暂存该指令")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := s.lockConfig(ctx, tx)
	if err != nil {
		return err
	}
	if !current.Enabled || current.AppID != c.AppID || !current.allows(e.GroupID) {
		return ErrDisabled
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,273))`, c.AppID); err != nil {
		return err
	}
	var bound bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM qqc_bindings WHERE app_id=$1 AND member_id=$2)`, c.AppID, e.Author.MemberID).Scan(&bound); err != nil {
		return err
	}
	if bound {
		return ErrBindingChanged
	}
	var session privateBindingSession
	var claimedUser int64
	err = tx.QueryRowContext(ctx, `SELECT request_id,group_id,nickname,expires_at,user_id FROM qqc_binding_commands WHERE app_id=$1 AND member_id=$2 AND expires_at>clock_timestamp() FOR UPDATE`, c.AppID, e.Author.MemberID).Scan(&session.requestID, &session.groupID, &session.nickname, &session.expiresAt, &claimedUser)
	hasPending := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if !hasPending {
		err = tx.QueryRowContext(ctx, `SELECT request_id,group_id,nickname,expires_at FROM qqc_private_bind_sessions WHERE app_id=$1 AND member_id=$2 AND expires_at>clock_timestamp() FOR UPDATE`, c.AppID, e.Author.MemberID).Scan(&session.requestID, &session.groupID, &session.nickname, &session.expiresAt)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if errors.Is(err, sql.ErrNoRows) || (job != nil && session.groupID != e.GroupID) {
			session = privateBindingSession{requestID: randomID(), groupID: e.GroupID, nickname: limitText(e.Author.Username, 100), expiresAt: time.Now().Add(privateBindingWindow)}
			_, err = tx.ExecContext(ctx, "INSERT INTO qqc_private_bind_sessions(app_id,member_id,request_id,group_id,nickname,expires_at) VALUES($1,$2,$3,$4,$5,$6) "+
				"ON CONFLICT(app_id,member_id) DO UPDATE SET request_id=EXCLUDED.request_id,group_id=EXCLUDED.group_id,nickname=EXCLUDED.nickname,expires_at=EXCLUDED.expires_at,created_at=NOW()",
				c.AppID, e.Author.MemberID, session.requestID, session.groupID, session.nickname, session.expiresAt)
			if err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM qqc_binding_commands WHERE app_id=$1 AND member_id=$2 AND expires_at<=clock_timestamp()`, c.AppID, e.Author.MemberID); err != nil {
			return err
		}
		if job != nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO qqc_binding_commands(app_id,member_id,request_id,source_job_id,group_id,message_id,nickname,command_text,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, c.AppID, e.Author.MemberID, session.requestID, job.ID, session.groupID, job.MessageID, session.nickname, content, session.expiresAt)
			if err != nil {
				return err
			}
			hasPending = true
		}
	}
	if !current.allows(session.groupID) {
		return ErrDisabled
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	notice := "请到网站个人资料页生成绑定码，然后直接在这个私聊中发送绑定码。10 分钟内有效；提交后仍需回网站核对 QQ 号并确认绑定。不要在群里发送绑定码。"
	if claimedUser > 0 {
		notice = "绑定码已受理，请回网站个人资料页核对 QQ 号并确认绑定，无需再次提交绑定码。"
	}
	if hasPending {
		notice += "\n网站确认后，会自动执行本次绑定最先暂存的指令一次，并私聊返回结果。后续发送的其他指令不会累积执行；转账仍需邮件确认。"
	}
	err = s.sendOneBotPrivate(ctx, c, e.Author.MemberID, session.groupID, notice)
	if err != nil {
		return err
	}
	// Only a confirmed private receipt permits this command to be dispatched.
	if hasPending {
		_, err = s.db.ExecContext(ctx, `UPDATE qqc_binding_commands SET delivered=TRUE WHERE app_id=$1 AND member_id=$2 AND request_id=$3`, c.AppID, e.Author.MemberID, session.requestID)
	}
	return err
}

func (s *Service) privateBindingSession(ctx context.Context, c Config, memberID string) (privateBindingSession, error) {
	var session privateBindingSession
	err := s.db.QueryRowContext(ctx,
		"SELECT request_id,group_id,nickname FROM qqc_private_bind_sessions WHERE app_id=$1 AND member_id=$2 AND expires_at>NOW()",
		c.AppID, memberID).Scan(&session.requestID, &session.groupID, &session.nickname)
	return session, err
}

// Private messages are handled in memory and never enter the durable job queue,
// group history, or logs, so the raw binding code is not retained there.
func (s *Service) handlePrivateBinding(ctx context.Context, c Config, in obEvent) error {
	memberID := string(in.UserID)
	if memberID == "" || len(memberID) > 128 || in.MessageID == "" {
		return nil
	}
	var content strings.Builder
	for _, segment := range in.Message {
		if segment.Type == "text" {
			content.WriteString(obValue(segment.Data["text"]))
		}
	}
	code := strings.TrimSpace(content.String())
	if code == "" {
		return nil
	}
	session, err := s.privateBindingSession(ctx, c, memberID)
	if errors.Is(err, sql.ErrNoRows) {
		if code == "/绑定" || code == "绑定" {
			return s.sendOneBotPrivate(ctx, c, memberID, "", "请先在已开放的群里发送 /绑定，再按私聊提示提交网站绑定码。")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !c.allows(session.groupID) {
		return s.sendOneBotPrivate(ctx, c, memberID, session.groupID, "发起绑定的群已停止提供绑定服务，请在其他已开放的群重新发送 /绑定。")
	}
	if strings.HasPrefix(code, "/绑定 ") {
		code = strings.TrimSpace(strings.TrimPrefix(code, "/绑定 "))
	} else if strings.HasPrefix(code, "绑定 ") {
		code = strings.TrimSpace(strings.TrimPrefix(code, "绑定 "))
	}
	if code == "/绑定" || code == "绑定" {
		return s.sendOneBotPrivate(ctx, c, memberID, session.groupID, "请在网站个人资料页生成绑定码，并在当前私聊发送绑定码。")
	}
	if code == "取消" || code == "/取消" {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(ctx, "DELETE FROM qqc_private_bind_sessions WHERE app_id=$1 AND member_id=$2 AND request_id=$3", c.AppID, memberID, session.requestID)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM qqc_binding_commands WHERE app_id=$1 AND member_id=$2 AND request_id=$3`, c.AppID, memberID, session.requestID); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return s.sendOneBotPrivate(ctx, c, memberID, session.groupID, "本次绑定已取消。")
	}
	var bound bool
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM qqc_bindings WHERE app_id=$1 AND member_id=$2)", c.AppID, memberID).Scan(&bound); err != nil {
		return err
	}
	if bound {
		return s.sendOneBotPrivate(ctx, c, memberID, session.groupID, "当前 QQ 已绑定账号。")
	}
	e := Event{GroupID: session.groupID}
	e.Author.MemberID = memberID
	e.Author.Username = session.nickname
	reply, _ := s.claimBinding(ctx, c, e, code, session.requestID)
	return s.sendOneBotPrivate(ctx, c, memberID, session.groupID, reply)
}
