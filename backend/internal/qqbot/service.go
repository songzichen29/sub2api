package qqbot

import (
	"bytes"
	"context"
 "github.com/Wei-Shaw/sub2api/internal/qqcommunity"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

var numericID = regexp.MustCompile(`^[1-9][0-9]{4,19}$`)
var bindCodePattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Service struct {
 Community *qqcommunity.Service
	repo      Repository
	encryptor Encryptor
	host      Host
	gateway   string
	client    *http.Client
	mu        sync.Mutex
	wg        sync.WaitGroup
	sessions  map[*connection]struct{}
	stopped   bool
}

func NewService(repo Repository, encryptor Encryptor, host Host, gateway string) *Service {
	return &Service{repo: repo, encryptor: encryptor, host: host, gateway: gateway,
		client: &http.Client{Timeout: 110 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, sessions: make(map[*connection]struct{})}
}
func Hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func (s *Service) config(ctx context.Context) (Config, error) {
	c := DefaultConfig()
	raw, err := s.repo.LoadConfig(ctx)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal([]byte(raw), &c)
	return c, err
}
func (s *Service) Config(ctx context.Context) (Config, error) {
	c, err := s.config(ctx)
	return c.Public(), err
}
func Validate(c Config) error {
	if c.Version < 0 {
		return ErrInvalidConfig
	}
	for _, amount := range []float64{c.CheckinReward, c.CheckinRewardMax} {
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 || amount > 100 || math.Abs(amount*1e8-math.Round(amount*1e8)) > 0.00001 {
			return errors.New("签到奖励需为 0～100，最多 8 位小数")
		}
	}
	if c.CheckinRewardMax > 0 && (c.CheckinReward <= 0 || c.CheckinRewardMax < c.CheckinReward) {
		return errors.New("随机签到奖励下限需大于 0，且上限不能小于下限")
	}
	if c.BotQQ != "" && !numericID.MatchString(c.BotQQ) {
		return errors.New("机器人 QQ 格式不正确")
	}
	if len(c.AllowedGroups) > 100 {
		return errors.New("最多配置 100 个群")
	}
	seen := map[string]bool{}
	for _, g := range c.AllowedGroups {
		if !numericID.MatchString(g) || seen[g] {
			return errors.New("群号格式不正确或重复")
		}
		seen[g] = true
	}
	if c.Enabled && (c.BotQQ == "" || len(c.AllowedGroups) == 0 || c.AccessToken == "") {
		return errors.New("启用前请填写机器人 QQ、群白名单和访问令牌")
	}
	if c.MaxOutputTokens < 1 || c.MaxOutputTokens > 4096 || c.HistoryMessages < 0 || c.HistoryMessages > 30 || c.DailyChatLimit < 1 || c.DailyChatLimit > 1000 || c.TotalDailyChatLimit < 1 || c.TotalDailyChatLimit > 100000 || c.CooldownSeconds < 1 || c.CooldownSeconds > 3600 {
		return errors.New("聊天次数、历史、输出长度或冷却时间超出允许范围")
	}
	if len(c.ChatModel) > 200 || len(c.SystemPrompt) > 8000 {
		return errors.New("模型名称或系统提示词过长")
	}
	if c.ChatEnabled && (c.APIKey == "" || strings.TrimSpace(c.ChatModel) == "") {
		return errors.New("开启聊天前请填写站点专用 API Key 和模型")
	}
	return nil
}

// RewardAmount draws once for a new ledger entry. The repository persists the
// amount in the same transaction as the balance; repeated claims reuse it.
func (c Config) RewardAmount() (float64, error) {
	if c.CheckinReward <= 0 {
		return 0, ErrRewardsDisabled
	}
	if c.CheckinRewardMax == 0 || c.CheckinRewardMax == c.CheckinReward {
		return c.CheckinReward, nil
	}
	min := int64(math.Round(c.CheckinReward * 1e8))
	max := int64(math.Round(c.CheckinRewardMax * 1e8))
	if max < min {
		return 0, ErrInvalidConfig
	}
	n, err := rand.Int(rand.Reader, big.NewInt(max-min+1))
	if err != nil {
		return 0, err
	}
	return float64(min+n.Int64()) / 1e8, nil
}
func (s *Service) prepare(ctx context.Context, c Config) (Config, error) {
	old, err := s.config(ctx)
	if err != nil {
		return c, err
	}
	if c.Version != old.Version {
		return c, ErrConflict
	}
	c.BotQQ = strings.TrimSpace(c.BotQQ)
	c.ChatModel = strings.TrimSpace(c.ChatModel)
	for i := range c.AllowedGroups {
		c.AllowedGroups[i] = strings.TrimSpace(c.AllowedGroups[i])
	}
	// New plaintext is encrypted exactly once; blank preserves the ciphertext.
	for _, p := range []struct {
		value *string
		old   string
		clear bool
		min   int
	}{{&c.AccessToken, old.AccessToken, c.ClearAccessToken, 24}, {&c.APIKey, old.APIKey, c.ClearAPIKey, 1}} {
		if p.clear {
			*p.value = ""
			continue
		}
		if *p.value == "" {
			*p.value = p.old
			continue
		}
		if len(*p.value) < p.min || len(*p.value) > 4096 || strings.ContainsAny(*p.value, "\r\n") {
			return c, fmt.Errorf("%w：凭据格式不正确；访问令牌至少 24 个字符", ErrInvalidConfig)
		}
		encrypted, err := s.encryptor.Encrypt(*p.value)
		if err != nil {
			return c, errors.New("凭据加密失败")
		}
		*p.value = encrypted
	}
	c.ClearAccessToken = false
	c.ClearAPIKey = false
	c.AccessTokenSet = false
	c.APIKeySet = false
	if err := Validate(c); err != nil {
		return c, fmt.Errorf("%w：%s", ErrInvalidConfig, err)
	}
	return c, nil
}
func (s *Service) SaveConfig(ctx context.Context, c Config) (Config, error) {
	c, err := s.prepare(ctx, c)
	if err != nil {
		return c.Public(), err
	}
	expected := c.Version
	c.Version++
	raw, err := json.Marshal(c)
	if err != nil {
		return c.Public(), err
	}
	if err = s.repo.SaveConfig(ctx, string(raw), expected); err != nil {
		return c.Public(), err
	}
	// Also revoke sessions for non-secret scope changes. NapCat will reconnect.
	s.disconnect()
	return c.Public(), nil
}
func (s *Service) BindingStatus(ctx context.Context, id int64) (*BindingStatus, error) {
	b, err := s.repo.BindingStatus(ctx, id)
	if err != nil {
		return nil, err
	}
	c, err := s.config(ctx)
	b.Enabled = c.Enabled
	b.BotQQ = c.BotQQ
	return b, err
}
func (s *Service) CreateCode(ctx context.Context, id int64) (string, time.Time, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	code := hex.EncodeToString(b)
	expiry := time.Now().Add(5 * time.Minute)
	return code, expiry, s.repo.CreateBindCode(ctx, id, Hash(code), expiry)
}
func (s *Service) Confirm(ctx context.Context, id int64, qq string) (*Binding, error) {
	return s.repo.ConfirmBinding(ctx, id, qq)
}
func (s *Service) Unbind(ctx context.Context, id int64) error { return s.repo.Unbind(ctx, id) }
func (s *Service) ListBindings(ctx context.Context, p, n int) ([]Binding, int64, error) {
	return s.repo.ListBindings(ctx, p, n)
}
func (s *Service) ListRewards(ctx context.Context, p, n int) ([]Reward, int64, error) {
	return s.repo.ListRewards(ctx, p, n)
}
func (s *Service) ListEvents(ctx context.Context, p, n int) ([]EventRecord, int64, error) {
	return s.repo.ListEvents(ctx, p, n)
}

func (s *Service) completion(ctx context.Context, c Config, messages []Message) (string, error) {
	key, err := s.encryptor.Decrypt(c.APIKey)
	if err != nil {
		return "", fmt.Errorf("%w：站点聊天 Key 无法解密，请管理员重新配置", ErrModelRequest)
	}
	body, err := json.Marshal(map[string]any{"model": c.ChatModel, "messages": messages, "max_tokens": c.MaxOutputTokens, "stream": false})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.gateway+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w：聊天请求超时或站点网关暂不可用，请稍后再试", ErrModelRequest)
	}
	defer resp.Body.Close()
	// Never expose gateway/upstream errors, which may include account identifiers.
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w（HTTP %d），请管理员检查专用 Key、余额、模型及分组", ErrModelRequest, resp.StatusCode)
	}
	var result struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result); err != nil || len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("%w：模型没有返回可用的文字回复", ErrModelRequest)
	}
	return truncate(result.Choices[0].Message.Content, 6000), nil
}
func (s *Service) TestModel(ctx context.Context, c Config) (string, error) {
	c, err := s.prepare(ctx, c)
	if err != nil {
		return "", err
	}
	if c.APIKey == "" || c.ChatModel == "" {
		return "", fmt.Errorf("%w：请填写站点专用 Key 和模型", ErrInvalidConfig)
	}
	c.MaxOutputTokens = 32
	return s.completion(ctx, c, []Message{{Role: "user", Content: "请只回复 OK"}})
}
func truncate(v string, n int) string {
	r := []rune(v)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return v
}

const help = "QQ 群助手指令：\n/绑定 网站生成的绑定码（提交后回网站确认）\n/余额\n/签到\n/渠道\n/聊天 你的问题（站点承担费用，需先绑定）\n/清空对话\n/帮助\n也可 @机器人 后输入问题。请勿在群内发送密码或 API Key。"

func (s *Service) execute(ctx context.Context, c Config, e groupEvent, id, command, arg string) (string, error) {
	switch command {
	case "帮助":
		return help, nil
	case "绑定":
		if !bindCodePattern.MatchString(arg) {
			return "", ErrInvalidCode
		}
		err := s.repo.ClaimBindCode(ctx, Hash(arg), string(e.UserID), string(e.GroupID), string(e.SelfID))
		if err != nil {
			return "", err
		}
		return "QQ 身份已提交，请回网站个人资料确认绑定。", nil
	case "签到":
		reward, err := s.repo.Checkin(ctx, string(e.UserID), string(e.GroupID), string(e.SelfID), time.Now())
		if errors.Is(err, ErrAlreadyCheckedIn) && reward != nil && reward.Amount > 0 {
			return fmt.Sprintf("今天已签到，已领取 $%.8f，请明天再来。", reward.Amount), nil
		}
		if err != nil {
			return "", err
		}
		invalidateCtx, invalidateCancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.host.InvalidateBalance(invalidateCtx, reward.UserID)
		invalidateCancel()
		return fmt.Sprintf("签到成功，获得 $%.8f；当前余额 $%.8f。", reward.Amount, reward.BalanceAfter), nil
	}
	a, err := s.repo.Account(ctx, string(e.UserID))
	if err != nil {
		return "", err
	}
	switch command {
	case "余额":
		return fmt.Sprintf("当前余额：$%.8f", a.Balance), nil
	case "渠道":
		return s.host.ChannelStatus(ctx, a.UserID)
	case "清空对话":
		if err = s.repo.ClearHistory(ctx, string(e.UserID), string(e.GroupID)); err != nil {
			return "", err
		}
		return "当前群的个人对话已清空。", nil
	case "聊天":
		if !c.ChatEnabled {
			return "", ErrDisabled
		}
		if arg == "" {
			return "用法：/聊天 你的问题", nil
		}
		if len([]rune(arg)) > 4000 {
			return "", errors.New("问题过长，请控制在 4000 字以内")
		}
		history, err := s.repo.ReserveChat(ctx, string(e.UserID), string(e.GroupID), string(e.SelfID), id, time.Now(), c.Version)
		if err != nil {
			return "", err
		}
		if len(history) > c.HistoryMessages {
			history = history[len(history)-c.HistoryMessages:]
		}
		messages := []Message{{Role: "system", Content: c.SystemPrompt}}
		messages = append(messages, history...)
		messages = append(messages, Message{Role: "user", Content: arg})
		answer, callErr := s.completion(ctx, c, messages)
		if callErr == nil {
			history = append(history, Message{Role: "user", Content: arg}, Message{Role: "assistant", Content: answer})
			if len(history) > c.HistoryMessages {
				history = history[len(history)-c.HistoryMessages:]
			}
		}
		finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err = s.repo.FinishChat(finishCtx, string(e.UserID), string(e.GroupID), id, history); err != nil {
			return "", errors.New("对话保存失败，请稍后再试")
		}
		return answer, callErr
	default:
		return help, nil
	}
}
