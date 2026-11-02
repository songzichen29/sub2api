// Package qqbot connects QQ group commands to scoped Sub2API account services.
package qqbot

import (
	"context"
	"errors"
	"time"
)

const ConfigKey = "qq_bot_config"

var (
	ErrDisabled         = errors.New("QQ 机器人尚未启用或当前群未获授权")
	ErrInvalidCode      = errors.New("绑定码无效、已使用或已过期，请在网站重新生成")
	ErrBindingConflict  = errors.New("QQ 或站点账号已绑定，请先在网站解除原绑定")
	ErrNotBound         = errors.New("请先在网站个人资料生成绑定码，发送 /绑定 验证码 后回网站确认")
	ErrInactiveUser     = errors.New("站点账号不可用，请联系管理员")
	ErrAlreadyCheckedIn = errors.New("今天已经签到，请明天再来")
	ErrRewardsDisabled  = errors.New("签到奖励尚未开启")
	ErrChatLimit        = errors.New("今日聊天次数已用完，请明天再来")
	ErrCooldown         = errors.New("操作太频繁或上一条聊天仍在处理中，请稍后再试")
	ErrConflict         = errors.New("配置或绑定状态已变化，请刷新后重试")
	ErrInvalidConfig    = errors.New("机器人配置不正确")
	ErrModelRequest     = errors.New("模型调用失败")
)

type Config struct {
	Version             int      `json:"version"`
	Enabled             bool     `json:"enabled"`
	BotQQ               string   `json:"bot_qq"`
	AllowedGroups       []string `json:"allowed_groups"`
	AccessToken         string   `json:"access_token"`
	AccessTokenSet      bool     `json:"access_token_set"`
	ClearAccessToken    bool     `json:"clear_access_token,omitempty"`
	CheckinReward       float64  `json:"checkin_reward"`
	CheckinRewardMax    float64  `json:"checkin_reward_max"`
	ChatEnabled         bool     `json:"chat_enabled"`
	APIKey              string   `json:"api_key"`
	APIKeySet           bool     `json:"api_key_set"`
	ClearAPIKey         bool     `json:"clear_api_key,omitempty"`
	ChatModel           string   `json:"chat_model"`
	SystemPrompt        string   `json:"system_prompt"`
	MaxOutputTokens     int      `json:"max_output_tokens"`
	HistoryMessages     int      `json:"history_messages"`
	DailyChatLimit      int      `json:"daily_chat_limit"`
	TotalDailyChatLimit int      `json:"total_daily_chat_limit"`
	CooldownSeconds     int      `json:"cooldown_seconds"`
}

func DefaultConfig() Config {
	return Config{AllowedGroups: []string{}, MaxOutputTokens: 1024, HistoryMessages: 10,
		DailyChatLimit: 20, TotalDailyChatLimit: 200, CooldownSeconds: 5,
		SystemPrompt: "你是本站 QQ 群助手，请用简洁友好的中文回答。余额、绑定、签到和渠道状态由站点固定指令处理；不得声称已修改账户或发放余额。"}
}

func (c Config) Public() Config {
	c.AccessTokenSet, c.APIKeySet = c.AccessToken != "", c.APIKey != ""
	c.AccessToken, c.APIKey = "", ""
	c.ClearAccessToken, c.ClearAPIKey = false, false
	return c
}

func (c Config) Allows(bot, group string) bool {
	if !c.Enabled || c.BotQQ != bot {
		return false
	}
	for _, allowed := range c.AllowedGroups {
		if allowed == group {
			return true
		}
	}
	return false
}

type Binding struct {
	UserID    int64     `json:"user_id"`
	QQ        string    `json:"qq"`
	GroupID   string    `json:"group_id"`
	CreatedAt time.Time `json:"created_at"`
}

type BindingStatus struct {
	Enabled   bool       `json:"enabled"`
	BotQQ     string     `json:"bot_qq"`
	Binding   *Binding   `json:"binding"`
	PendingQQ string     `json:"pending_qq"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type Account struct {
	UserID  int64
	Balance float64
	Active  bool
}
type Reward struct {
	ID           string    `json:"id"`
	UserID       int64     `json:"user_id"`
	QQ           string    `json:"qq"`
	GroupID      string    `json:"group_id"`
	Day          string    `json:"day"`
	Amount       float64   `json:"amount"`
	BalanceAfter float64   `json:"balance_after"`
	CreatedAt    time.Time `json:"created_at"`
}

type EventRecord struct {
	ID         string     `json:"id"`
	QQ         string     `json:"qq"`
	GroupID    string     `json:"group_id"`
	Command    string     `json:"command"`
	Status     string     `json:"status"`
	Result     string     `json:"result"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type Repository interface {
	LoadConfig(context.Context) (string, error)
	SaveConfig(context.Context, string, int) error
	BindingStatus(context.Context, int64) (*BindingStatus, error)
	CreateBindCode(context.Context, int64, string, time.Time) error
	ClaimBindCode(context.Context, string, string, string, string) error
	ConfirmBinding(context.Context, int64, string) (*Binding, error)
	Unbind(context.Context, int64) error
	Account(context.Context, string) (*Account, error)
	Checkin(context.Context, string, string, string, time.Time) (*Reward, error)
	BeginEvent(context.Context, EventRecord) (bool, error)
	FinishEvent(context.Context, string, string, string) error
	ReserveChat(context.Context, string, string, string, string, time.Time, int) ([]Message, error)
	FinishChat(context.Context, string, string, string, []Message) error
	ClearHistory(context.Context, string, string) error
	ListBindings(context.Context, int, int) ([]Binding, int64, error)
	ListRewards(context.Context, int, int) ([]Reward, int64, error)
	ListEvents(context.Context, int, int) ([]EventRecord, int64, error)
}

type Encryptor interface {
	Encrypt(string) (string, error)
	Decrypt(string) (string, error)
}
type Host interface {
	ChannelStatus(context.Context, int64) (string, error)
	InvalidateBalance(context.Context, int64)
}

var Shanghai = time.FixedZone("Asia/Shanghai", 8*60*60)
