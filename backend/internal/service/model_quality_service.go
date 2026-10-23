package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/google/uuid"
)

type qualityGroupReader interface {
	GetByID(context.Context, int64) (*Group, error)
	ListActive(context.Context) ([]Group, error)
}
type qualityKeyReader interface {
	GetByID(context.Context, int64) (*APIKey, error)
	GetByKey(context.Context, string) (*APIKey, error)
	ListByUserID(context.Context, int64, pagination.PaginationParams, APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error)
}
type qualityChannelReader interface {
	GetChannelForGroup(context.Context, int64) (*Channel, error)
}
type qualityVisibility interface {
	GetAvailableGroups(context.Context, int64) ([]Group, error)
}
type qualityMonitorReader interface {
	List(context.Context, ChannelMonitorListParams) ([]*ChannelMonitor, int64, error)
	Get(context.Context, int64) (*ChannelMonitor, error)
}

type ModelQualityService struct {
	repo       ModelQualityRepository
	keys       qualityKeyReader
	groups     qualityGroupReader
	channels   qualityChannelReader
	visibility qualityVisibility
	monitors   qualityMonitorReader
	client     ManxueClient
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewModelQualityService(repo ModelQualityRepository, keys APIKeyRepository, groups GroupRepository, channels *ChannelService, apiKeys *APIKeyService, monitors *ChannelMonitorService) *ModelQualityService {
	return &ModelQualityService{repo: repo, keys: keys, groups: groups, channels: channels, visibility: apiKeys, monitors: monitors, client: NewManxueClient()}
}

type ModelQualityInput struct {
	Name            string               `json:"name"`
	GroupID         int64                `json:"group_id"`
	Config          ModelQualityConfig   `json:"config"`
	Configs         []ModelQualityConfig `json:"configs"`
	Enabled         bool                 `json:"enabled"`
	PublicVisible   bool                 `json:"public_visible"`
	IntervalMinutes int                  `json:"interval_minutes"`
}

func qualityInvalid(message string) error { return fmt.Errorf("%w: %s", ErrQualityInvalid, message) }

func modelQualityConfigID(c ModelQualityConfig) string {
	if c.ID != "" {
		return c.ID
	}
	return modelQualityConfigKey(c)
}

func modelQualityConfigKey(c ModelQualityConfig) string {
	return QualityConfigID(c.Model, c.Benchmark, c.ReasoningEffort, c.ServiceTier)
}

func QualityConfigID(model, benchmark, effort, tier string) string {
	key := strings.Join([]string{strings.TrimSpace(model), benchmark, effort, tier}, "\x00")
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", sum[:12])
}

func qualityConfigs(p *ModelQualityPlan) []ModelQualityConfig {
	if len(p.Configs) > 0 {
		return p.Configs
	}
	return []ModelQualityConfig{p.Config}
}

func qualityConfigForRun(p *ModelQualityPlan, configID string) (ModelQualityConfig, bool) {
	if configID == "" {
		configs := qualityConfigs(p)
		if len(configs) > 0 {
			return configs[0], true
		}
	}
	for _, c := range qualityConfigs(p) {
		if modelQualityConfigID(c) == configID {
			return c, true
		}
	}
	return ModelQualityConfig{}, false
}

func qualityConfigViews(configs []ModelQualityConfig) []ModelQualityConfigView {
	out := make([]ModelQualityConfigView, 0, len(configs))
	for _, c := range configs {
		out = append(out, ModelQualityConfigView{
			ID: modelQualityConfigID(c), Benchmark: c.Benchmark, Model: c.Model,
			ReasoningEffort: c.ReasoningEffort, ServiceTier: c.ServiceTier,
		})
	}
	return out
}

func validateQualityConfig(c *ModelQualityConfig) error {
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.Model = strings.TrimSpace(c.Model)
	if c.MonitorID < 0 {
		return qualityInvalid("请选择检测渠道")
	}
	if c.MonitorID == 0 {
		u, err := url.Parse(c.BaseURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(c.BaseURL) > 2048 {
			return qualityInvalid("本站 Base URL 必须是公网 HTTPS 地址，不能包含密码、查询参数或片段")
		}
		host := strings.ToLower(u.Hostname())
		ip := net.ParseIP(host)
		if host == "localhost" || strings.HasSuffix(host, ".localhost") || !strings.Contains(host, ".") || (ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast())) {
			return qualityInvalid("Manxue 必须能从公网访问本站 Base URL")
		}
	}
	if c.Benchmark != "candy" && c.Benchmark != "pelican" && c.Benchmark != "creative" {
		return qualityInvalid("请选择糖果、鹈鹕或自由创作检测")
	}
	if c.Model == "" || len(c.Model) > 200 {
		return qualityInvalid("模型名称不能为空且最多 200 字节")
	}
	if c.Benchmark != "candy" && c.Model != "gpt-6-astra" {
		return qualityInvalid("Manxue 的鹈鹕和自由创作模式目前只接受 gpt-6-astra")
	}
	if c.ReasoningEffort == "" {
		c.ReasoningEffort = "low"
	}
	allowed := c.ReasoningEffort == "low" || c.ReasoningEffort == "medium" || c.ReasoningEffort == "high"
	if c.Benchmark == "candy" {
		allowed = allowed || c.ReasoningEffort == "xhigh" || c.ReasoningEffort == "max" || c.ReasoningEffort == "ultra"
	}
	if !allowed {
		return qualityInvalid("该模式不支持所选思考强度")
	}
	if c.ServiceTier != "" && c.ServiceTier != "priority" && c.ServiceTier != "ultrafast" {
		return qualityInvalid("服务等级仅支持默认、priority 或 ultrafast")
	}
	return nil
}

func (s *ModelQualityService) validateKey(ctx context.Context, p *ModelQualityPlan, config ModelQualityConfig) (*APIKey, error) {
	k, err := s.keys.GetByID(ctx, config.APIKeyID)
	if err != nil || k == nil {
		return nil, qualityInvalid("检测 Key 不存在或已删除")
	}
	if k.UserID != p.OwnerID || k.GroupID == nil || *k.GroupID != p.GroupID {
		return nil, qualityInvalid("必须选择计划创建者拥有且绑定目标分组的本站 Key")
	}
	if !k.IsActive() || k.IsExpired() || k.IsQuotaExhausted() {
		return nil, qualityInvalid("检测 Key 已停用、过期或额度用尽")
	}
	return k, nil
}

func (s *ModelQualityService) Save(ctx context.Context, id string, owner int64, in ModelQualityInput) (*ModelQualityPlan, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 160 || in.GroupID <= 0 || in.IntervalMinutes < 3 || in.IntervalMinutes > 10080 {
		return nil, qualityInvalid("名称必填，检测间隔须为 3–10080 分钟")
	}
	configs := in.Configs
	if len(configs) == 0 {
		configs = []ModelQualityConfig{in.Config}
	}
	if len(configs) == 0 || len(configs) > 100 {
		return nil, qualityInvalid("至少需要一组检测配置，最多支持 100 组")
	}
	monitorID := configs[0].MonitorID
	seenConfigs := make(map[string]struct{}, len(configs))
	for i := range configs {
		if configs[i].MonitorID != monitorID {
			return nil, qualityInvalid("同一计划的所有检测必须绑定同一渠道")
		}
		if err := validateQualityConfig(&configs[i]); err != nil {
			return nil, err
		}
		configKey := modelQualityConfigKey(configs[i])
		if _, exists := seenConfigs[configKey]; exists {
			return nil, qualityInvalid("不能重复添加相同的模型、模式和思考强度")
		}
		seenConfigs[configKey] = struct{}{}
		configs[i].ID = configKey
	}
	in.Config = configs[0]
	if id == "" && monitorID == 0 {
		return nil, qualityInvalid("请选择已配置的检测渠道")
	}
	if monitorID > 0 {
		for i := range configs {
			configs[i].BaseURL, configs[i].APIKeyID = "", 0
		}
		in.Config = configs[0]
	}
	now := time.Now()
	p := &ModelQualityPlan{ID: id, OwnerID: owner, GroupID: in.GroupID, Name: in.Name, Config: in.Config, Configs: configs, Enabled: in.Enabled, PublicVisible: in.PublicVisible, IntervalMinutes: in.IntervalMinutes, NextRunAt: now.Add(time.Duration(in.IntervalMinutes) * time.Minute), CreatedAt: now}
	validateCredentials := in.Enabled || id == ""
	if id != "" {
		old, err := s.repo.GetPlan(ctx, id)
		if err != nil {
			return nil, err
		}
		if old.OwnerID != owner {
			return nil, qualityInvalid("请由计划创建者修改检测配置")
		}
		if old.GroupID != in.GroupID {
			return nil, qualityInvalid("已有计划不能更换分组")
		}
		if old.Config.MonitorID > 0 && old.Config.MonitorID != monitorID {
			return nil, qualityInvalid("更换检测渠道请新建计划")
		}
		validateCredentials = validateCredentials || old.Config.MonitorID != monitorID
		p.CreatedAt = old.CreatedAt
	}
	in.Config = configs[0]
	p.Config = configs[0]
	p.Configs = configs
	// Disabling a plan must remain possible after its test key expires.
	if validateCredentials {
		for _, config := range configs {
			if _, err := s.target(ctx, p, config); err != nil {
				return nil, err
			}
			if _, err := s.credentials(ctx, p, config); err != nil {
				return nil, err
			}
		}
	}
	if id == "" {
		p.ID = uuid.NewString()
		return p, s.repo.CreatePlan(ctx, p)
	}
	return p, s.repo.UpdatePlan(ctx, p)
}

func (s *ModelQualityService) Delete(ctx context.Context, id string, user int64, admin bool) error {
	p, err := s.repo.GetPlan(ctx, id)
	if err != nil {
		return err
	}
	if !admin && p.OwnerID != user {
		return qualityInvalid("请由计划创建者删除检测计划")
	}
	return s.repo.DeletePlan(ctx, id)
}

func (s *ModelQualityService) target(ctx context.Context, p *ModelQualityPlan, config ModelQualityConfig) (ModelQualityTarget, error) {
	t := ModelQualityTarget{ConfigID: modelQualityConfigID(config), GroupID: p.GroupID, Model: config.Model, Benchmark: config.Benchmark, ReasoningEffort: config.ReasoningEffort, ServiceTier: config.ServiceTier}
	if config.MonitorID > 0 {
		t.MonitorID = config.MonitorID
		if s.monitors != nil {
			if m, err := s.monitors.Get(ctx, config.MonitorID); err == nil && m != nil {
				t.MonitorName = m.Name
			}
		}
	}
	g, err := s.groups.GetByID(ctx, p.GroupID)
	if err != nil {
		return t, err
	}
	if g.Status != StatusActive {
		return t, qualityInvalid("目标分组已停用")
	}
	t.GroupName = g.Name
	ch, err := s.channels.GetChannelForGroup(ctx, p.GroupID)
	if err != nil {
		return t, err
	}
	if ch != nil {
		t.ChannelID, t.ChannelName = ch.ID, ch.Name
	}
	return t, nil
}

type qualityCredentials struct{ baseURL, key string }

func (c qualityCredentials) hash() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(c.baseURL+"\x00"+c.key)))
}

func (s *ModelQualityService) credentials(ctx context.Context, p *ModelQualityPlan, config ModelQualityConfig) (qualityCredentials, error) {
	if config.MonitorID == 0 {
		key, err := s.validateKey(ctx, p, config)
		if err != nil {
			return qualityCredentials{}, err
		}
		return qualityCredentials{config.BaseURL, key.Key}, nil
	}
	if s.monitors == nil {
		return qualityCredentials{}, qualityInvalid("渠道配置不可用")
	}
	m, err := s.monitors.Get(ctx, config.MonitorID)
	if err != nil || m == nil {
		return qualityCredentials{}, qualityInvalid("检测渠道不存在或已删除")
	}
	if !m.Enabled || m.Provider != MonitorProviderOpenAI || m.APIKeyDecryptFailed || strings.TrimSpace(m.APIKey) == "" {
		return qualityCredentials{}, qualityInvalid("请选择已启用且 Key 有效的 OpenAI 渠道")
	}
	k, keyErr := s.keys.GetByKey(ctx, m.APIKey)
	if keyErr != nil && !errors.Is(keyErr, ErrAPIKeyNotFound) {
		return qualityCredentials{}, qualityInvalid("暂时无法核对渠道 Key 所属分组")
	}
	if keyErr == nil && k != nil {
		if k.GroupID == nil || *k.GroupID != p.GroupID {
			return qualityCredentials{}, qualityInvalid("渠道 Key 所属分组与检测分组不一致")
		}
		if !k.IsActive() || k.IsExpired() || k.IsQuotaExhausted() {
			return qualityCredentials{}, qualityInvalid("渠道 Key 已停用、过期或额度用尽")
		}
	}
	baseURL := strings.TrimRight(strings.TrimSpace(m.Endpoint), "/")
	u, err := url.Parse(baseURL)
	if err != nil {
		return qualityCredentials{}, qualityInvalid("渠道地址无效")
	}
	if !strings.HasSuffix(u.EscapedPath(), "/v1") {
		baseURL += "/v1"
	}
	check := config
	check.MonitorID, check.BaseURL = 0, baseURL
	if err := validateQualityConfig(&check); err != nil {
		return qualityCredentials{}, qualityInvalid("渠道需要公网 HTTPS 地址并支持 Responses 接口")
	}
	return qualityCredentials{baseURL, m.APIKey}, nil
}

func (s *ModelQualityService) Queue(ctx context.Context, id string, scheduled bool) ([]*ModelQualityRun, error) {
	p, err := s.repo.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	batchID := uuid.NewString()
	runs := make([]*ModelQualityRun, 0, len(qualityConfigs(p)))
	for _, config := range qualityConfigs(p) {
		if _, err := s.credentials(ctx, p, config); err != nil {
			return nil, err
		}
		target, err := s.target(ctx, p, config)
		if err != nil {
			return nil, err
		}
		runs = append(runs, &ModelQualityRun{ID: uuid.NewString(), PlanID: id, BatchID: batchID, ConfigID: target.ConfigID, Target: target, Status: "queued", Verdict: "pending", StartedAt: now})
	}
	if err := s.repo.QueueBatch(ctx, runs, scheduled); err != nil {
		return nil, err
	}
	return runs, nil
}

func (s *ModelQualityService) visiblePlans(ctx context.Context, userID int64, admin bool) ([]*ModelQualityPlan, error) {
	plans, err := s.repo.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	if admin {
		return plans, nil
	}
	groups, err := s.visibility.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	allowed := make(map[int64]bool, len(groups))
	for _, g := range groups {
		allowed[g.ID] = true
	}
	out := make([]*ModelQualityPlan, 0)
	for _, p := range plans {
		if p.PublicVisible && allowed[p.GroupID] {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *ModelQualityService) List(ctx context.Context, userID int64, admin bool) ([]*ModelQualityView, error) {
	plans, err := s.visiblePlans(ctx, userID, admin)
	if err != nil {
		return nil, err
	}
	items := make([]*ModelQualityView, 0, len(plans))
	bindings := s.monitorBindings(ctx, plans)
	for _, p := range plans {
		configs := qualityConfigs(p)
		t, _ := s.target(ctx, p, configs[0])
		stats, err := s.repo.Stats(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		recent, err := s.repo.History(ctx, p.ID, 60, 0)
		if err != nil {
			return nil, err
		}
		v := &ModelQualityView{ModelQualityPlan: p, Target: t, Configs: qualityConfigViews(configs), Stats: stats, Recent: recent, MonitorIDs: bindings[p.GroupID]}
		v.Batches, err = s.repo.Batches(ctx, p.ID, 60)
		if err != nil {
			return nil, err
		}
		if p.Config.MonitorID > 0 {
			v.MonitorIDs = []int64{p.Config.MonitorID}
		}
		if admin {
			v.ActiveRunID = p.ActiveRunID
			v.CanEdit = p.OwnerID == userID
			cfg := p.Config
			v.AdminConfig = &cfg
		}
		items = append(items, v)
	}
	return items, nil
}

// Legacy probe names and group labels are free text. Bind by the probe key's
// actual group, never by similar names, and expose only public monitor IDs.
func (s *ModelQualityService) monitorBindings(ctx context.Context, plans []*ModelQualityPlan) map[int64][]int64 {
	bindings := make(map[int64][]int64)
	if s.monitors == nil || len(plans) == 0 {
		return bindings
	}
	visible := make(map[int64]bool)
	for _, p := range plans {
		visible[p.GroupID] = true
	}
	enabled := true
	for page := 1; ; page++ {
		monitors, total, err := s.monitors.List(ctx, ChannelMonitorListParams{Page: page, PageSize: 200, Enabled: &enabled})
		if err != nil {
			return bindings
		}
		for _, m := range monitors {
			if m.APIKey == "" || m.APIKeyDecryptFailed {
				continue
			}
			key, err := s.keys.GetByKey(ctx, m.APIKey)
			if err == nil && key != nil && key.GroupID != nil && visible[*key.GroupID] {
				bindings[*key.GroupID] = append(bindings[*key.GroupID], m.ID)
			}
		}
		if len(monitors) == 0 || int64(page*200) >= total {
			return bindings
		}
	}
}

func (s *ModelQualityService) authorize(ctx context.Context, planID string, userID int64, admin bool) error {
	if admin {
		_, err := s.repo.GetPlan(ctx, planID)
		return err
	}
	plans, err := s.visiblePlans(ctx, userID, false)
	if err != nil {
		return err
	}
	for _, p := range plans {
		if p.ID == planID {
			return nil
		}
	}
	return ErrQualityNotFound
}

func (s *ModelQualityService) History(ctx context.Context, id string, user int64, admin bool, page int) ([]*ModelQualityRun, error) {
	if err := s.authorize(ctx, id, user, admin); err != nil {
		return nil, err
	}
	return s.repo.History(ctx, id, 50, (page-1)*50)
}

func (s *ModelQualityService) GroupHistory(ctx context.Context, groupID, user int64, admin bool, page int) ([]*ModelQualityRun, error) {
	plans, err := s.visiblePlans(ctx, user, admin)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	for _, p := range plans {
		if p.GroupID == groupID {
			ids = append(ids, p.ID)
		}
	}
	if len(ids) == 0 {
		return nil, ErrQualityNotFound
	}
	if page < 1 || page > 100000 {
		return nil, qualityInvalid("invalid page")
	}
	return s.repo.HistoryForPlans(ctx, ids, 50, (page-1)*50)
}

func (s *ModelQualityService) Detail(ctx context.Context, id string, user int64, admin bool) (*ModelQualityRun, error) {
	r, err := s.repo.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, r.PlanID, user, admin); err != nil {
		return nil, err
	}
	return r, nil
}

// Results pages are filtered before pagination, so an older benchmark cannot
// disappear behind pages of a more frequently scheduled benchmark.
func (s *ModelQualityService) GroupResults(ctx context.Context, groupID, user int64, admin bool, benchmark string, page int) (*ModelQualityResults, error) {
	if page < 1 || page > 100000 {
		return nil, qualityInvalid("invalid result filter")
	}
	plans, err := s.visiblePlans(ctx, user, admin)
	if err != nil {
		return nil, err
	}
	byBenchmark := make(map[string][]string)
	for _, p := range plans {
		if p.GroupID == groupID {
			for _, config := range qualityConfigs(p) {
				ids := byBenchmark[config.Benchmark]
				if len(ids) == 0 || ids[len(ids)-1] != p.ID {
					byBenchmark[config.Benchmark] = append(ids, p.ID)
				}
			}
		}
	}
	if len(byBenchmark) == 0 {
		return nil, ErrQualityNotFound
	}
	out := &ModelQualityResults{Items: []*ModelQualityRun{}, Benchmarks: []string{}}
	for mode := range byBenchmark {
		out.Benchmarks = append(out.Benchmarks, mode)
	}
	sort.Strings(out.Benchmarks)
	if benchmark == "" {
		benchmark = out.Benchmarks[0]
	}
	out.Benchmark = benchmark
	ids := byBenchmark[benchmark]
	if len(ids) == 0 {
		return nil, qualityInvalid("invalid result filter")
	}
	const pageSize = 12
	rows, err := s.repo.ResultsForPlans(ctx, ids, benchmark, pageSize+1, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	out.HasMore = len(rows) > pageSize
	if out.HasMore {
		rows = rows[:pageSize]
	}
	out.Items = rows
	return out, nil
}

// batchID selects only the clicked round. Without it, history is scoped to this
// plan and can exclude the clicked round. Dimensions come from snapshots so
// editing a plan never relabels or hides results from earlier configurations.
func (s *ModelQualityService) PlanResults(ctx context.Context, planID, batchID, excludeBatchID, model, benchmark string, user int64, admin bool, page int) (*ModelQualityResults, error) {
	if page < 1 || page > 100000 || len(batchID) > 36 || len(excludeBatchID) > 36 || (batchID != "" && excludeBatchID != "") || len(model) > 200 || len(benchmark) > 32 {
		return nil, qualityInvalid("invalid result filter")
	}
	if err := s.authorize(ctx, planID, user, admin); err != nil {
		return nil, err
	}
	dimensions, err := s.repo.ResultDimensions(ctx, planID, batchID, excludeBatchID)
	if err != nil {
		return nil, err
	}
	out := &ModelQualityResults{BatchID: batchID, Items: []*ModelQualityRun{}, Models: []string{}, Benchmarks: []string{}}
	models, benchmarks := map[string]bool{}, map[string]bool{}
	for _, d := range dimensions {
		models[d.Model] = true
	}
	for m := range models {
		out.Models = append(out.Models, m)
	}
	sort.Strings(out.Models)
	if len(out.Models) == 0 {
		return out, nil
	}
	if !models[model] {
		model = out.Models[0]
	}
	out.Model = model
	for _, d := range dimensions {
		if d.Model == model {
			benchmarks[d.Benchmark] = true
		}
	}
	for b := range benchmarks {
		out.Benchmarks = append(out.Benchmarks, b)
	}
	sort.Strings(out.Benchmarks)
	if !benchmarks[benchmark] {
		benchmark = out.Benchmarks[0]
	}
	out.Benchmark = benchmark
	const pageSize = 12
	rows, err := s.repo.PlanResults(ctx, planID, batchID, excludeBatchID, model, benchmark, pageSize+1, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	out.HasMore = len(rows) > pageSize
	if out.HasMore {
		rows = rows[:pageSize]
	}
	out.Items = rows
	return out, nil
}

func (s *ModelQualityService) Cancel(ctx context.Context, id string) error {
	if _, err := s.repo.GetRun(ctx, id); err != nil {
		return err
	}
	return s.repo.RequestCancel(ctx, id)
}

type QualityKeyOption struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	GroupID int64  `json:"group_id"`
}
type QualityGroupOption struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ChannelName string `json:"channel_name"`
}
type ModelQualityOptions struct {
	Groups   []QualityGroupOption   `json:"groups"`
	Keys     []QualityKeyOption     `json:"keys"`
	Monitors []QualityMonitorOption `json:"monitors"`
}

type QualityMonitorOption struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	GroupID int64  `json:"group_id"`
	Model   string `json:"model"`
}

func (s *ModelQualityService) Options(ctx context.Context, owner int64) (*ModelQualityOptions, error) {
	out := &ModelQualityOptions{Groups: []QualityGroupOption{}, Keys: []QualityKeyOption{}, Monitors: []QualityMonitorOption{}}
	groups, err := s.groups.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		option := QualityGroupOption{ID: g.ID, Name: g.Name}
		ch, err := s.channels.GetChannelForGroup(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		if ch != nil {
			option.ChannelName = ch.Name
		}
		out.Groups = append(out.Groups, option)
	}
	if s.monitors != nil {
		enabled := true
		for page := 1; ; page++ {
			monitors, total, err := s.monitors.List(ctx, ChannelMonitorListParams{Page: page, PageSize: 200, Enabled: &enabled})
			if err != nil {
				return nil, err
			}
			for _, m := range monitors {
				if m.Provider != MonitorProviderOpenAI || m.APIKey == "" || m.APIKeyDecryptFailed {
					continue
				}
				option := QualityMonitorOption{ID: m.ID, Name: m.Name, Model: m.PrimaryModel}
				if k, err := s.keys.GetByKey(ctx, m.APIKey); err == nil && k != nil && k.GroupID != nil {
					option.GroupID = *k.GroupID
				}
				out.Monitors = append(out.Monitors, option)
			}
			if len(monitors) == 0 || int64(page*200) >= total {
				break
			}
		}
	}
	return out, nil
}

func (s *ModelQualityService) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(3)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cycle, done := context.WithTimeout(ctx, 20*time.Second)
				plans, err := s.repo.ListPlans(cycle)
				if err == nil {
					for _, p := range plans {
						if p.Enabled && !p.NextRunAt.After(time.Now()) {
							_, err = s.Queue(cycle, p.ID, true)
							if err != nil && !errors.Is(err, ErrQualityBusy) {
								log.Printf("[ModelQuality] schedule failed: %s", logredact.RedactText(err.Error()))
							}
						}
					}
				}
				done()
			}
		}
	}()
	for i := 0; i < 2; i++ {
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					step, done := context.WithTimeout(ctx, 40*time.Second)
					r, err := s.repo.Claim(step, time.Now(), uuid.NewString())
					if err == nil {
						s.step(step, r)
					} else if !errors.Is(err, ErrQualityNotFound) && ctx.Err() == nil {
						log.Printf("[ModelQuality] claim failed: %s", logredact.RedactText(err.Error()))
					}
					done()
				}
			}
		}()
	}
}

func (s *ModelQualityService) Stop() {
	if s.cancel != nil {
		s.cancel()
		s.wg.Wait()
	}
}

func (s *ModelQualityService) finish(ctx context.Context, r *ModelQualityRun, status, verdict, reason string) {
	now := time.Now()
	r.Status, r.Verdict, r.Reason, r.Phase, r.FinishedAt = status, verdict, reason, "complete", &now
	s.persist(ctx, r, now)
}

func (s *ModelQualityService) persist(ctx context.Context, r *ModelQualityRun, next time.Time) bool {
	if err := s.repo.SaveRun(ctx, r, next); err != nil {
		if ctx.Err() == nil {
			log.Printf("[ModelQuality] save run %s failed: %s", r.ID, logredact.RedactText(err.Error()))
		}
		return false
	}
	return true
}

// Each claim performs at most one remote operation. Leases outlive the HTTP timeout.
// An interrupted POST is never blindly repeated: without a remote ID its outcome is unknown.
func (s *ModelQualityService) step(ctx context.Context, r *ModelQualityRun) {
	executionStart := r.StartedAt
	if r.ExecutionStartedAt != nil {
		executionStart = *r.ExecutionStartedAt
	}
	if time.Since(executionStart) > 12*time.Minute {
		if r.RemoteID != "" {
			_ = s.client.Cancel(ctx, r.RemoteID)
		}
		s.finish(ctx, r, "failed", "error", "检测超时，未计入通过率")
		return
	}
	if r.CancelRequested {
		if r.RemoteID != "" {
			if err := s.client.Cancel(ctx, r.RemoteID); err != nil {
				var he *manxueHTTPError
				if !errors.As(err, &he) || he.Status != 404 {
					s.deferRun(ctx, r, err, false)
					return
				}
			}
		}
		s.finish(ctx, r, "cancelled", "unknown", "检测已取消；已经发生的上游用量仍可能计费")
		return
	}
	if r.Status == "submitting" && r.RemoteID == "" {
		s.finish(ctx, r, "failed", "error", "提交结果不确定，已停止自动重发以避免重复计费；可手动重新检测")
		return
	}
	if r.RemoteID == "" {
		p, err := s.repo.GetPlan(ctx, r.PlanID)
		if err != nil {
			s.finish(ctx, r, "failed", "error", "无法读取检测配置")
			return
		}
		config, ok := qualityConfigForRun(p, r.ConfigID)
		if !ok {
			s.finish(ctx, r, "failed", "error", "检测配置已被删除")
			return
		}
		if _, err := s.target(ctx, p, config); err != nil {
			s.finish(ctx, r, "failed", "error", "目标分组或渠道配置已失效")
			return
		}
		credential, err := s.credentials(ctx, p, config)
		if err != nil {
			s.finish(ctx, r, "failed", "error", err.Error())
			return
		}
		r.Status = "submitting"
		r.CredentialHash = credential.hash()
		r.Attempts++
		if !s.persist(ctx, r, time.Now().Add(time.Minute)) {
			return
		}
		task, err := s.client.Create(ctx, ManxueCreateRequest{Benchmark: config.Benchmark, BaseURL: credential.baseURL, APIKey: credential.key, Model: config.Model, ReasoningEffort: config.ReasoningEffort, ServiceTier: config.ServiceTier, IdempotencyKey: r.ID})
		if err != nil {
			s.deferRun(ctx, r, err, true)
			return
		}
		if task.ID == "" || len(task.ID) > 256 || strings.ContainsAny(task.ID, "/\\") {
			s.finish(ctx, r, "failed", "error", "Manxue 未返回有效任务 ID；未自动重发")
			return
		}
		r.RemoteID, r.Status, r.Phase, r.Reason, r.Attempts = task.ID, "running", "generating", "", 0
		s.persist(ctx, r, time.Now().Add(4*time.Second))
		return
	}
	task, err := s.client.Get(ctx, r.RemoteID)
	if err != nil {
		s.deferRun(ctx, r, err, false)
		return
	}
	r.Attempts = 0
	if task.Benchmark != "" && task.Benchmark != r.Target.Benchmark {
		s.finish(ctx, r, "failed", "error", "Manxue 返回了不匹配的检测模式")
		return
	}
	if task.ID != "" && task.ID != r.RemoteID {
		s.finish(ctx, r, "failed", "error", "Manxue 返回了不匹配的任务 ID")
		return
	}
	if task.Status == "running" {
		if task.Phase == "generating" || task.Phase == "classifying" {
			r.Phase = task.Phase
		}
		r.Reason = ""
		s.persist(ctx, r, time.Now().Add(4*time.Second))
		return
	}
	status, verdict, reason, result := interpretManxue(task, r.Target.Benchmark)
	// Only known, useful fields are stored; never persist raw remote JSON/URLs/task IDs.
	secrets := []string{r.RemoteID}
	credentialAvailable := false
	if p, e := s.repo.GetPlan(ctx, r.PlanID); e == nil {
		if config, ok := qualityConfigForRun(p, r.ConfigID); ok {
			if credential, e := s.credentials(ctx, p, config); e == nil {
				secrets = append(secrets, credential.baseURL, credential.key)
				credentialAvailable = credential.key != "" && (r.CredentialHash == "" || r.CredentialHash == credential.hash())
			}
		}
	}
	if !credentialAvailable {
		reason = "检测已结束；渠道凭据已变更或失效，未保留远端文本"
	}
	clean := func(v string) string {
		for _, secret := range secrets {
			if secret != "" {
				v = strings.ReplaceAll(v, secret, "[REDACTED]")
			}
		}
		return logredact.RedactText(v)
	}
	reason = clean(reason)
	if result != nil {
		result.Answer, result.HTML, result.Source = clean(result.Answer), clean(result.HTML), clean(result.Source)
		// Deleted credentials cannot be reliably scrubbed from untrusted free text.
		if !credentialAvailable {
			result.Answer, result.HTML, result.Source = "", "", ""
			reason = "检测已结束；检测 Key 已失效，未保留远端文本以避免泄露凭据"
		}
		result.Source = qualityTextLimit(result.Source, 1024)
		if result.ServiceTier != "default" && result.ServiceTier != "priority" && result.ServiceTier != "ultrafast" {
			result.ServiceTier = ""
		}
	}
	reason = qualityTextLimit(reason, 8192)
	r.Result = result
	s.finish(ctx, r, status, verdict, reason)
}

func qualityTextLimit(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	s = s[:limit]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func (s *ModelQualityService) deferRun(ctx context.Context, r *ModelQualityRun, err error, creating bool) {
	var he *manxueHTTPError
	isHTTP := errors.As(err, &he)
	if isHTTP && he.Status == http.StatusNotFound {
		s.finish(ctx, r, "failed", "error", "Manxue 任务已过期或因重启丢失；未自动重新提交")
		return
	}
	if creating && (!isHTTP || (he.Status != 429 && he.Status != 503)) {
		reason := "提交未成功或结果不确定，未自动重发；请检查配置后手动检测"
		if isHTTP {
			reason = fmt.Sprintf("Manxue 拒绝提交（HTTP %d），请检查配置", he.Status)
		}
		s.finish(ctx, r, "failed", "error", reason)
		return
	}
	if !creating && isHTTP && he.Status != 429 && he.Status < 500 {
		s.finish(ctx, r, "failed", "error", fmt.Sprintf("Manxue 查询异常（HTTP %d）", he.Status))
		return
	}
	if !creating {
		r.Attempts++
	}
	if creating {
		r.Status = "queued"
	}
	delay := time.Duration(1<<min(r.Attempts, 6))*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond
	if isHTTP && he.RetryAfter > delay {
		delay = he.RetryAfter
	}
	// The local deadline still bounds how long a remote outage keeps a plan occupied.
	if left := 12*time.Minute - time.Since(r.StartedAt); delay > left {
		delay = left
	}
	r.Reason = "等待 Manxue 服务恢复，检测结果尚未确定"
	s.persist(ctx, r, time.Now().Add(delay))
}

func interpretManxue(t *ManxueTask, benchmark string) (string, string, string, *ModelQualityResult) {
	if t.Status == "failed" {
		return "failed", "error", "Manxue 检测执行失败，未计入通过率", nil
	}
	if t.Status == "cancelled" {
		return "cancelled", "unknown", "Manxue 检测已取消", nil
	}
	if t.Status != "succeeded" {
		return "failed", "error", "Manxue 返回未知任务状态", nil
	}
	if benchmark == "candy" {
		if t.Candy == nil {
			return "succeeded", "unknown", "缺少糖果判题结果", nil
		}
		c := t.Candy
		r := &ModelQualityResult{Answer: c.Answer, DurationMS: c.DurationMS, InputTokens: c.InputTokens, OutputTokens: c.OutputTokens, ReasoningTokens: c.ReasoningTokens, ServiceTier: c.ServiceTier}
		switch c.Status {
		case "passed":
			return "succeeded", "normal", "糖果检测通过", r
		case "incorrect":
			return "succeeded", "degraded", "糖果答案不符；单次未通过不能证明模型整体能力下降", r
		case "error":
			return "failed", "error", "糖果检测异常，未计入通过率", r
		default:
			return "succeeded", "unknown", "糖果检测无法判定", r
		}
	}
	r := &ModelQualityResult{}
	_ = json.Unmarshal(t.Result, r)
	if t.Assessment == nil {
		return "succeeded", "unknown", "缺少分类判定", r
	}
	a := t.Assessment
	r.Source = a.Source
	verdict := "unknown"
	switch a.Quality {
	case "normal":
		verdict = "normal"
	case "degraded", "suspicious":
		verdict = "degraded"
	}
	return "succeeded", verdict, a.Reason, r
}
