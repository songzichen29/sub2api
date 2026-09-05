package service

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// OpenAIOAuthErrorAlertService sends alert emails when an OpenAI OAuth account enters error state.
// Recipients are loaded from ops_email_notification_config.alert.recipients.
type OpenAIOAuthErrorAlertService struct {
	settingRepo  SettingRepository
	emailService *EmailService
	accountRepo  openAIOAuthAlertAccountReader
	usageRepo    openAIOAuthAlertUsageReader
}

type openAIOAuthAlertAccountReader interface {
	ListByPlatform(ctx context.Context, platform string) ([]Account, error)
}

type openAIOAuthAlertUsageReader interface {
	GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error)
}

func NewOpenAIOAuthErrorAlertService(
	settingRepo SettingRepository,
	emailService *EmailService,
	accountRepo openAIOAuthAlertAccountReader,
	usageRepo openAIOAuthAlertUsageReader,
) *OpenAIOAuthErrorAlertService {
	return &OpenAIOAuthErrorAlertService{
		settingRepo:  settingRepo,
		emailService: emailService,
		accountRepo:  accountRepo,
		usageRepo:    usageRepo,
	}
}

func (s *OpenAIOAuthErrorAlertService) NotifyAccountError(ctx context.Context, account *Account, trigger, errorMsg string) {
	s.notifyAccountIssue(ctx, account, "error", trigger, errorMsg)
}

func (s *OpenAIOAuthErrorAlertService) NotifyAccountRateLimited(ctx context.Context, account *Account, trigger, detail string) {
	s.notifyAccountIssue(ctx, account, "rate_limited", trigger, detail)
}

func (s *OpenAIOAuthErrorAlertService) notifyAccountIssue(ctx context.Context, account *Account, issueType, trigger, detail string) {
	if s == nil || s.settingRepo == nil || s.emailService == nil || account == nil {
		return
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return
	}

	recipients := s.loadRecipients(ctx)
	if len(recipients) == 0 {
		return
	}

	subject := fmt.Sprintf("[OpenAI OAuth 账号%s] %s (#%d)", issueTypeLabel(issueType), strings.TrimSpace(account.Name), account.ID)
	adminURL := s.buildAdminAccountsURL(ctx)
	availableCount := s.availableOpenAIOAuthAccountCount(ctx)
	body := buildOpenAIOAuthAccountIssueEmailBody(account, issueType, trigger, detail, adminURL, availableCount)

	for _, recipient := range recipients {
		if err := s.emailService.SendEmail(ctx, recipient, subject, body); err != nil {
			slog.Warn("openai_oauth_error_alert_send_failed",
				"account_id", account.ID,
				"recipient", recipient,
				"error", err,
			)
			continue
		}
		slog.Info("openai_oauth_error_alert_sent",
			"account_id", account.ID,
			"recipient", recipient,
			"trigger", strings.TrimSpace(trigger),
		)
	}
}

//nolint:unused // retained for richer OpenAI OAuth alert snapshots.
type openAIOAuthAccountSnapshot struct {
	AccountID   int64
	AccountName string
	StatusLabel string
	Detail      string
	Usage5h     string
	Usage7d     string
	IsAvailable bool
}

//nolint:unused // retained for richer OpenAI OAuth alert snapshots.
type openAIOAuthSnapshotSummary struct {
	Total             int
	Available         int
	RateLimited       int
	Error             int
	TempUnschedulable int
	Overloaded        int
	UnavailableOther  int
}

//nolint:unused // retained for richer OpenAI OAuth alert snapshots.
func (s *OpenAIOAuthErrorAlertService) buildPlatformSnapshot(ctx context.Context, current *Account) []openAIOAuthAccountSnapshot {
	if s == nil || s.accountRepo == nil {
		return nil
	}
	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil || len(accounts) == 0 {
		return nil
	}

	snapshots := make([]openAIOAuthAccountSnapshot, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		if account.Type != AccountTypeOAuth {
			continue
		}
		snapshots = append(snapshots, openAIOAuthAccountSnapshot{
			AccountID:   account.ID,
			AccountName: account.Name,
			StatusLabel: s.describeAccountStatus(&account, current),
			Detail:      s.describeAccountDetail(&account),
			Usage5h:     s.describeUsageWindow(ctx, account.ID, 5*time.Hour),
			Usage7d:     s.describeUsageWindow(ctx, account.ID, 7*24*time.Hour),
			IsAvailable: account.IsSchedulable(),
		})
	}
	return snapshots
}

//nolint:unused // retained for richer OpenAI OAuth alert snapshots.
func (s *OpenAIOAuthErrorAlertService) describeAccountStatus(account *Account, current *Account) string {
	if account == nil {
		return "未知"
	}
	switch {
	case current != nil && account.ID == current.ID:
		if account.Status == StatusError {
			return "当前告警账号（错误）"
		}
		if account.IsRateLimited() {
			return "当前告警账号（限流）"
		}
	}
	switch {
	case account.Status == StatusError:
		return "错误"
	case account.IsRateLimited():
		return "限流中"
	case account.TempUnschedulableUntil != nil && time.Now().Before(*account.TempUnschedulableUntil):
		return "临时不可调度"
	case account.IsOverloaded():
		return "过载"
	case account.IsSchedulable():
		return "可用"
	default:
		return "不可用"
	}
}

//nolint:unused // retained for richer OpenAI OAuth alert snapshots.
func (s *OpenAIOAuthErrorAlertService) describeAccountDetail(account *Account) string {
	if account == nil {
		return ""
	}
	switch {
	case account.Status == StatusError:
		return strings.TrimSpace(account.ErrorMessage)
	case account.IsRateLimited() && account.RateLimitResetAt != nil:
		return "重置时间: " + account.RateLimitResetAt.Format(time.RFC3339)
	case account.TempUnschedulableUntil != nil && time.Now().Before(*account.TempUnschedulableUntil):
		return "解除时间: " + account.TempUnschedulableUntil.Format(time.RFC3339)
	case account.IsOverloaded() && account.OverloadUntil != nil:
		return "恢复时间: " + account.OverloadUntil.Format(time.RFC3339)
	default:
		return ""
	}
}

//nolint:unused // retained for richer OpenAI OAuth alert snapshots.
func (s *OpenAIOAuthErrorAlertService) describeUsageWindow(ctx context.Context, accountID int64, window time.Duration) string {
	if s == nil || s.usageRepo == nil || accountID <= 0 {
		return "-"
	}
	stats, err := s.usageRepo.GetAccountWindowStats(ctx, accountID, time.Now().Add(-window))
	if err != nil || stats == nil {
		return "-"
	}
	return fmt.Sprintf("req=%d, tokens=%d, cost=%.4f", stats.Requests, stats.Tokens, stats.Cost)
}

func (s *OpenAIOAuthErrorAlertService) availableOpenAIOAuthAccountCount(ctx context.Context) *int {
	if s == nil || s.accountRepo == nil {
		return nil
	}
	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		slog.Warn("openai_oauth_error_alert_available_count_failed", "error", err)
		return nil
	}
	count := 0
	for i := range accounts {
		account := accounts[i]
		if account.Type == AccountTypeOAuth && account.IsSchedulable() {
			count++
		}
	}
	return &count
}

func (s *OpenAIOAuthErrorAlertService) loadRecipients(ctx context.Context) []string {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsEmailNotificationConfig)
	if err != nil {
		return nil
	}

	cfg := &OpsEmailNotificationConfig{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil
	}
	normalizeOpsEmailNotificationConfig(cfg)
	if !cfg.Alert.Enabled || len(cfg.Alert.Recipients) == 0 {
		return nil
	}

	recipients := make([]string, 0, len(cfg.Alert.Recipients))
	seen := make(map[string]struct{}, len(cfg.Alert.Recipients))
	for _, recipient := range cfg.Alert.Recipients {
		addr := strings.TrimSpace(recipient)
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		recipients = append(recipients, addr)
	}
	return recipients
}

func (s *OpenAIOAuthErrorAlertService) buildAdminAccountsURL(ctx context.Context) string {
	baseURL, err := s.settingRepo.GetValue(ctx, SettingKeyFrontendURL)
	if err != nil {
		return ""
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ""
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return baseURL + "/admin/accounts"
}

func issueTypeLabel(issueType string) string {
	switch strings.TrimSpace(issueType) {
	case "rate_limited":
		return "限流"
	default:
		return "错误"
	}
}

func buildOpenAIOAuthAccountIssueEmailBody(account *Account, issueType, trigger, detail, adminURL string, availableCount *int) string {
	if account == nil {
		return ""
	}
	accountName := strings.TrimSpace(account.Name)
	accountEmail := strings.TrimSpace(account.GetCredential("email"))
	trigger = conciseOpenAIOAuthAlertText(trigger, 120)
	detail = conciseOpenAIOAuthAlertText(detail, 240)
	occurredAt := time.Now().Format(time.RFC3339)
	adminLinkHTML := ""
	if strings.TrimSpace(adminURL) != "" {
		escapedURL := html.EscapeString(strings.TrimSpace(adminURL))
		adminLinkHTML = fmt.Sprintf(
			`<p><b>后台入口</b>: <a href="%s" target="_blank" rel="noopener noreferrer">打开账号管理</a>，搜索账号 ID <b>%d</b>。</p>`,
			escapedURL,
			account.ID,
		)
	}
	accountEmailHTML := ""
	if accountEmail != "" {
		accountEmailHTML = fmt.Sprintf(`<p><b>账号邮箱</b>: %s</p>`, html.EscapeString(accountEmail))
	}
	availableCountHTML := `<p><b>当前可用 OpenAI OAuth 账号</b>: 未知</p>`
	if availableCount != nil {
		availableCountHTML = fmt.Sprintf(`<p><b>当前可用 OpenAI OAuth 账号</b>: %d 个</p>`, *availableCount)
	}
	return fmt.Sprintf(`
<h2>OpenAI OAuth 账号%s告警</h2>
<p><b>问题</b>: %s</p>
<p><b>账号</b>: #%d %s</p>
%s
%s
<p><b>触发来源</b>: %s</p>
<p><b>错误摘要</b>: %s</p>
<p><b>时间</b>: %s</p>
%s
<h3>建议动作</h3>
<ol>
  <li>进入后台账号管理，定位该 OpenAI OAuth 账号。</li>
  <li>若是 401/403，优先重新授权或检查账号权限；若是 429，等待重置或降低调度频率。</li>
  <li>处理完成后，手动恢复账号调度并观察后续请求。</li>
</ol>
`,
		html.EscapeString(issueTypeLabel(issueType)),
		html.EscapeString(issueTypeLabel(issueType)),
		account.ID,
		html.EscapeString(accountName),
		accountEmailHTML,
		availableCountHTML,
		html.EscapeString(trigger),
		html.EscapeString(detail),
		occurredAt,
		adminLinkHTML,
	)
}

func conciseOpenAIOAuthAlertText(value string, maxLen int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	value = redactContentModerationSecrets(value)
	if value == "" {
		return "-"
	}
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return strings.TrimSpace(truncateString(value, maxLen)) + "…"
}

//nolint:unused // retained for richer OpenAI OAuth alert snapshots.
func buildOpenAIOAuthSnapshotHTML(snapshots []openAIOAuthAccountSnapshot) string {
	if len(snapshots) == 0 {
		return ""
	}
	summary := summarizeOpenAIOAuthSnapshots(snapshots)
	availableRows := make([]openAIOAuthAccountSnapshot, 0, len(snapshots))
	for _, item := range snapshots {
		if item.IsAvailable {
			availableRows = append(availableRows, item)
		}
	}
	var rows strings.Builder
	_, _ = rows.WriteString(`<h3>同平台账号汇总</h3>`)
	_, _ = fmt.Fprintf(&rows,
		`<ul>
<li>总账号数: <b>%d</b></li>
<li>可用账号: <b>%d</b></li>
<li>限流中: <b>%d</b></li>
<li>错误: <b>%d</b></li>
<li>临时不可调度: <b>%d</b></li>
<li>过载: <b>%d</b></li>
<li>其他不可用: <b>%d</b></li>
</ul>`,
		summary.Total,
		summary.Available,
		summary.RateLimited,
		summary.Error,
		summary.TempUnschedulable,
		summary.Overloaded,
		summary.UnavailableOther,
	)
	if len(availableRows) == 0 {
		_, _ = rows.WriteString(`<p><b>剩余可用账号</b>: 无</p>`)
		return rows.String()
	}
	_, _ = rows.WriteString(`<h3>剩余可用账号与使用情况</h3><table border="1" cellpadding="6" cellspacing="0" style="border-collapse:collapse;"><tr><th>ID</th><th>账号</th><th>状态</th><th>说明</th><th>近5h使用</th><th>近7d使用</th></tr>`)
	for _, item := range availableRows {
		_, _ = fmt.Fprintf(&rows,
			`<tr><td>%d</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			item.AccountID,
			html.EscapeString(strings.TrimSpace(item.AccountName)),
			html.EscapeString(strings.TrimSpace(item.StatusLabel)),
			html.EscapeString(strings.TrimSpace(item.Detail)),
			html.EscapeString(strings.TrimSpace(item.Usage5h)),
			html.EscapeString(strings.TrimSpace(item.Usage7d)),
		)
	}
	_, _ = rows.WriteString(`</table>`)
	return rows.String()
}

//nolint:unused // retained for richer OpenAI OAuth alert snapshots.
func summarizeOpenAIOAuthSnapshots(snapshots []openAIOAuthAccountSnapshot) openAIOAuthSnapshotSummary {
	summary := openAIOAuthSnapshotSummary{}
	for _, item := range snapshots {
		summary.Total++
		switch item.StatusLabel {
		case "可用":
			summary.Available++
		case "限流中", "当前告警账号（限流）":
			summary.RateLimited++
		case "错误", "当前告警账号（错误）":
			summary.Error++
		case "临时不可调度":
			summary.TempUnschedulable++
		case "过载":
			summary.Overloaded++
		default:
			if !item.IsAvailable {
				summary.UnavailableOther++
			}
		}
	}
	return summary
}
