package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// InitializeLegacy is restart safe. The marker is the new configuration row,
// committed with the historical ledgers. It never replays a balance credit.
func (s *Service) InitializeLegacy(ctx context.Context, gateway string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, configKey).Scan(&raw)
	if err == nil {
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='qq_bot_config'`).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	c := DefaultConfig()
	var old struct {
		Enabled             bool     `json:"enabled"`
		BotQQ               string   `json:"bot_qq"`
		AllowedGroups       []string `json:"allowed_groups"`
		AccessToken         string   `json:"access_token"`
		APIKey              string   `json:"api_key"`
		ChatEnabled         bool     `json:"chat_enabled"`
		ChatModel           string   `json:"chat_model"`
		SystemPrompt        string   `json:"system_prompt"`
		MaxOutputTokens     int      `json:"max_output_tokens"`
		HistoryMessages     int      `json:"history_messages"`
		DailyChatLimit      int      `json:"daily_chat_limit"`
		TotalDailyChatLimit int      `json:"total_daily_chat_limit"`
		CooldownSeconds     int      `json:"cooldown_seconds"`
		CheckinReward       float64  `json:"checkin_reward"`
		CheckinRewardMax    float64  `json:"checkin_reward_max"`
	}
	if err == nil {
		if err = json.Unmarshal([]byte(raw), &old); err != nil {
			return err
		}
		c.Enabled, c.AppID, c.AppSecret, c.AllowedGroups = old.Enabled, old.BotQQ, old.AccessToken, old.AllowedGroups
		c.APIKey, c.ChatModel, c.ChatDisabled, c.BaseURL = old.APIKey, old.ChatModel, !old.ChatEnabled, gateway
		c.SystemPrompt, c.MaxOutputTokens, c.HistoryMessages = old.SystemPrompt, old.MaxOutputTokens, old.HistoryMessages
		c.DailyChatLimit, c.TotalDailyChatLimit, c.CooldownSeconds = old.DailyChatLimit, old.TotalDailyChatLimit, old.CooldownSeconds
		c.CheckinReward = old.CheckinReward
		if _, err = tx.ExecContext(ctx, `INSERT INTO qqc_bindings(app_id,member_id,user_id,group_id,created_at) SELECT $1,qq_id,user_id,group_id,created_at FROM qq_bot_bindings`, old.BotQQ); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO qqc_bind_requests(user_id,app_id,code_hash,member_id,group_id,expires_at) SELECT user_id,$1,code_hash,qq_id,group_id,expires_at FROM qq_bot_bind_requests`, old.BotQQ); err != nil {
			return err
		}
		// Historical rewards have already changed user balances. Copy receipts only.
		if _, err = tx.ExecContext(ctx, `INSERT INTO qqc_rewards(reward_key,app_id,group_id,member_id,user_id,kind,amount,created_at) SELECT CONCAT('checkin:',user_id,':',reward_day),$1,group_id,qq_id,user_id,'checkin',amount,created_at FROM qq_bot_rewards`, old.BotQQ); err != nil {
			return err
		}
		// Preserve audit history without replaying previously received commands.
		if _, err = tx.ExecContext(ctx, `INSERT INTO qqc_jobs(id,event_key,app_id,event_type,group_id,member_id,message_id,kind,status,payload,result_text,error,created_at,finished_at,message_preview)
	 SELECT CONCAT('legacy:',id),CONCAT('legacy:',id),$1,'LEGACY_AUDIT',group_id,qq_id,'',
 CASE command WHEN '聊天' THEN 'chat' WHEN '帮助' THEN 'help' WHEN '余额' THEN 'balance' WHEN '签到' THEN 'checkin' WHEN '渠道' THEN 'channel_status' WHEN '清空对话' THEN 'clear' WHEN '绑定' THEN 'binding' ELSE 'event' END,
 CASE WHEN status IN ('success','completed') THEN 'completed' WHEN status IN ('failed','send_failed') THEN 'failed' WHEN status IN ('expired','cancelled') THEN status ELSE 'interrupted' END,
 '{}',result_text,'',created_at,finished_at,command FROM qq_bot_events`, old.BotQQ); err != nil {
			return err
		}
		max := old.CheckinRewardMax
		if max == 0 {
			max = old.CheckinReward
		}
		checkin := CheckinConfig{Enabled: old.CheckinReward > 0, MinAmount: old.CheckinReward, MaxAmount: max, RechargeRequirement: RechargeRequirement{RechargeWindowDays: DefaultRechargeWindowDays}}
		b, err := json.Marshal(checkin)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES($1,$2,NOW())`, checkinConfigKey, string(b)); err != nil {
			return err
		}
	}
	// All new activities, image generation, notifications and moderation retain
	// their disabled defaults. Existing credentials remain encrypted byte-for-byte.
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES($1,$2,NOW())`, configKey, string(b)); err != nil {
		return err
	}
	return tx.Commit()
}
