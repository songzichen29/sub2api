package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
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
	repo         ModelQualityRepository
	keys         qualityKeyReader
	groups       qualityGroupReader
	channels     qualityChannelReader
	visibility   qualityVisibility
	monitors     qualityMonitorReader
	generator    qualityCompletionClient
	judge        qualityCompletionClient
	judgeOrigin  string
	judgeKeyName string
	judgeModel   string
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

func NewModelQualityService(repo ModelQualityRepository, keys APIKeyRepository, groups GroupRepository, channels *ChannelService, apiKeys *APIKeyService, monitors *ChannelMonitorService, cfg *config.Config) *ModelQualityService {
	port, keyName, model := 8080, "测试降智", "gpt-6.1-sol"
	if cfg != nil {
		if cfg.Server.Port > 0 {
			port = cfg.Server.Port
		}
		if cfg.ModelQuality.JudgeKeyName != "" {
			keyName = cfg.ModelQuality.JudgeKeyName
		}
		if cfg.ModelQuality.JudgeModel != "" {
			model = cfg.ModelQuality.JudgeModel
		}
	}
	return &ModelQualityService{repo: repo, keys: keys, groups: groups, channels: channels, visibility: apiKeys, monitors: monitors,
		generator: newQualityHTTPClient(false), judge: newQualityHTTPClient(true), judgeOrigin: fmt.Sprintf("http://127.0.0.1:%d/v1", port), judgeKeyName: keyName, judgeModel: model}
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
			return qualityInvalid("检测渠道必须使用公网 HTTPS 地址")
		}
	}
	if c.Benchmark != "candy" && c.Benchmark != "pelican" && c.Benchmark != "creative" {
		return qualityInvalid("请选择糖果、鹈鹕或自由创作检测")
	}
	if c.Model == "" || len(c.Model) > 200 {
		return qualityInvalid("模型名称不能为空且最多 200 字节")
	}
	if c.ReasoningEffort == "" {
		c.ReasoningEffort = "low"
	}
	allowed := c.ReasoningEffort == "low" || c.ReasoningEffort == "medium" || c.ReasoningEffort == "high" || c.ReasoningEffort == "xhigh" || c.ReasoningEffort == "max" || c.ReasoningEffort == "ultra"
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
	judgeKey, err := s.qualityJudgeKey(ctx, p.OwnerID, 0)
	if err != nil {
		return nil, err
	}
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
		target.BenchmarkVersion, target.JudgeModel = qualityBenchmarkVersion, s.judgeModel
		runs = append(runs, &ModelQualityRun{ID: uuid.NewString(), PlanID: id, BatchID: batchID, ConfigID: target.ConfigID, Target: target, Status: "queued", Verdict: "pending", StartedAt: now, JudgeKeyID: judgeKey.ID})
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
	now := time.Now()
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
		recentStats, err := s.repo.RecentStats(ctx, p.ID, now)
		if err != nil {
			return nil, err
		}
		for _, stat := range recentStats {
			v.Stats24H.Add(stat)
		}
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
	if len(rows) > 0 {
		stats, err := s.repo.RecentStats(ctx, planID, time.Now())
		if err != nil {
			return nil, err
		}
		for _, run := range rows {
			stat := stats[run.ConfigID]
			run.Stats24H = &stat
		}
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
					step, done := context.WithTimeout(ctx, 9*time.Minute)
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
