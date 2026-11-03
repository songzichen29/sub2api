package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func currentTimeMessage(now time.Time) Message {
	now = now.In(time.FixedZone("Asia/Shanghai", 8*3600))
	weekdays := []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
	return Message{Role: "system", Content: fmt.Sprintf("后端本轮当前时间：%s，%s，时区 Asia/Shanghai（北京时间 UTC+8）。今天、日期、星期和相对日期以此为准，无需联网；这不表示具备外网检索能力。", now.Format("2006-01-02 15:04:05"), weekdays[now.Weekday()])}
}

// Refresh identity each explicit turn. It must survive history truncation, and
// disappear immediately after unbinding. No personal account data enters chat.
func (s *Service) withBindingContext(ctx context.Context, c Config, j Job, messages []Message) []Message {
	state, role := "unknown", ""
	if c.Enabled && c.AppID != "" && j.AppID == c.AppID && c.allows(j.GroupID) && j.MemberID != "" && s.db != nil {
		err := s.db.QueryRowContext(ctx, `SELECT u.role FROM qqc_bindings b JOIN users u ON u.id=b.user_id WHERE b.app_id=$1 AND b.member_id=$2 AND u.status='active' AND u.deleted_at IS NULL`, c.AppID, j.MemberID).Scan(&role)
		if err == nil {
			state = "bound"
			if role != "admin" {
				role = "user"
			}
		} else if errors.Is(err, sql.ErrNoRows) {
			state, role = "unbound", ""
		} else {
			role = ""
		}
	}
	raw, _ := json.Marshal(map[string]string{"speaker": groupSpeaker(c.AppID, j.GroupID, j.MemberID), "binding": state, "site_role": role})
	content := "后端本轮验证的当前成员绑定状态 JSON：" + string(raw) + "。bound=已绑定有效站点账号，unbound=未绑定有效账号，unknown=暂未能核实，不能当作未绑定。site_role 仅说明站点身份，不授予群管理员或资金操作权限；群管理员以本轮单独的后端验证为准。无需依靠旧聊天猜测身份，不披露内部标识。"
	boundary := 0
	for boundary < len(messages) && (messages[boundary].Role == "system" || messages[boundary].Role == "developer") {
		boundary++
	}
	result := append([]Message{}, messages[:boundary]...)
	result = append(result, Message{Role: "system", Content: content})
	return append(result, messages[boundary:]...)
}
