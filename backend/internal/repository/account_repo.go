// Package repository 实现数据访问层（Repository Pattern）。
//
// 该包提供了与数据库交互的所有操作，包括 CRUD、复杂查询和批量操作。
// 采用 Repository 模式将数据访问逻辑与业务逻辑分离，便于测试和维护。
//
// 主要特性：
//   - 使用 Ent ORM 进行类型安全的数据库操作
//   - 对于复杂查询（如批量更新、聚合统计）使用原生 SQL
//   - 提供统一的错误翻译机制，将数据库错误转换为业务错误
//   - 支持软删除，所有查询自动过滤已删除记录
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
	dbproxy "github.com/Wei-Shaw/sub2api/ent/proxy"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
)

// accountRepository 实现 service.AccountRepository 接口。
// 提供 AI API 账户的完整数据访问功能。
//
// 设计说明：
//   - client: Ent 客户端，用于类型安全的 ORM 操作
//   - sql: 原生 SQL 执行器，用于复杂查询和批量操作
//   - schedulerCache: 调度器缓存，用于在账号状态变更时同步快照
type accountRepository struct {
	client *dbent.Client // Ent ORM 客户端
	sql    sqlExecutor   // 原生 SQL 执行接口
	// schedulerCache 用于在账号状态变更时主动同步快照到缓存，
	// 确保粘性会话能及时感知账号不可用状态。
	// Used to proactively sync account snapshot to cache when status changes,
	// ensuring sticky sessions can promptly detect unavailable accounts.
	schedulerCache service.SchedulerCache
}

func buildAccountInt64InClause(ids []int64) (string, []any) {
	if len(ids) == 0 {
		return "", nil
	}
	ph := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		ph = append(ph, "?")
		args = append(args, id)
	}
	return strings.Join(ph, ","), args
}

var schedulerNeutralExtraKeyPrefixes = []string{
	"codex_primary_",
	"codex_secondary_",
	"codex_5h_",
	"codex_7d_",
	"codex_reset_credit_",
	"passive_usage_",
	"upstream_billing_probe",
	"upstream_billing_rate_sync",
	"ollama_cloud_usage",
}

var schedulerNeutralExtraKeys = map[string]struct{}{
	"codex_usage_updated_at":     {},
	"grok_billing_snapshot":      {},
	"session_window_utilization": {},
}

const queryParameterBatchSize = 50000

// NewAccountRepository 创建账户仓储实例。
// 这是对外暴露的构造函数，返回接口类型以便于依赖注入。
func NewAccountRepository(client *dbent.Client, sqlDB *sql.DB, schedulerCache service.SchedulerCache) service.AccountRepository {
	return newAccountRepositoryWithSQL(client, sqlDB, schedulerCache)
}

// NewAdminAccountRepository exposes the account repository's atomic duplication capability
// as an explicit dependency of the admin service.
func NewAdminAccountRepository(client *dbent.Client, sqlDB *sql.DB, schedulerCache service.SchedulerCache) service.AdminAccountRepository {
	return newAccountRepositoryWithSQL(client, sqlDB, schedulerCache)
}

// newAccountRepositoryWithSQL 是内部构造函数，支持依赖注入 SQL 执行器。
// 这种设计便于单元测试时注入 mock 对象。
func newAccountRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor, schedulerCache service.SchedulerCache) *accountRepository {
	return &accountRepository{client: client, sql: sqlq, schedulerCache: schedulerCache}
}

func (r *accountRepository) Create(ctx context.Context, account *service.Account) error {
	if err := createAccountRecord(ctx, r.client, account); err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, buildSchedulerGroupPayload(account.GroupIDs)); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue account create failed: account=%d err=%v", account.ID, err)
	}
	return nil
}

func createAccountRecord(ctx context.Context, client *dbent.Client, account *service.Account) error {
	if account == nil {
		return service.ErrAccountNilInput
	}

	builder := client.Account.Create().
		SetName(account.Name).
		SetNillableNotes(account.Notes).
		SetPlatform(account.Platform).
		SetType(account.Type).
		SetCredentials(normalizeJSONMap(account.Credentials)).
		SetExtra(normalizeJSONMap(account.Extra)).
		SetTags(normalizeJSONStringSlice(account.Tags)).
		SetConcurrency(account.Concurrency).
		SetPriority(account.Priority).
		SetStatus(account.Status).
		SetErrorMessage(account.ErrorMessage).
		SetSchedulable(account.Schedulable).
		SetAutoPauseOnExpired(account.AutoPauseOnExpired)

	if account.RateMultiplier != nil {
		builder.SetRateMultiplier(*account.RateMultiplier)
	}
	if account.LoadFactor != nil {
		builder.SetLoadFactor(*account.LoadFactor)
	}

	if account.ProxyID != nil {
		builder.SetProxyID(*account.ProxyID)
	}
	if account.LastUsedAt != nil {
		builder.SetLastUsedAt(*account.LastUsedAt)
	}
	if account.ExpiresAt != nil {
		builder.SetExpiresAt(*account.ExpiresAt)
	}
	if account.RateLimitedAt != nil {
		builder.SetRateLimitedAt(*account.RateLimitedAt)
	}
	if account.RateLimitResetAt != nil {
		builder.SetRateLimitResetAt(*account.RateLimitResetAt)
	}
	if account.OverloadUntil != nil {
		builder.SetOverloadUntil(*account.OverloadUntil)
	}
	if account.SessionWindowStart != nil {
		builder.SetSessionWindowStart(*account.SessionWindowStart)
	}
	if account.SessionWindowEnd != nil {
		builder.SetSessionWindowEnd(*account.SessionWindowEnd)
	}
	if account.SessionWindowStatus != "" {
		builder.SetSessionWindowStatus(account.SessionWindowStatus)
	}

	builder.SetQuotaDimension(dbaccount.QuotaDimension(account.QuotaDimensionOrDefault()))
	if account.ParentAccountID != nil {
		builder.SetParentAccountID(*account.ParentAccountID)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	account.ID = created.ID
	account.CreatedAt = created.CreatedAt
	account.UpdatedAt = created.UpdatedAt
	return nil
}

// CreateWithAccountGroups atomically persists an account, its exact per-group priorities,
// and the scheduler outbox event used to publish the new routing snapshot.
func (r *accountRepository) CreateWithAccountGroups(ctx context.Context, account *service.Account, groups []service.AccountGroup) error {
	if account == nil {
		return service.ErrAccountNilInput
	}
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		// Reuse a caller-owned transaction when this repository is already transactional.
		txClient = r.client
	}

	if err := createAccountRecord(ctx, txClient, account); err != nil {
		return err
	}
	groupIDs := make([]int64, 0, len(groups))
	if len(groups) > 0 {
		builders := make([]*dbent.AccountGroupCreate, 0, len(groups))
		for i := range groups {
			groups[i].AccountID = account.ID
			groupIDs = append(groupIDs, groups[i].GroupID)
			builders = append(builders, txClient.AccountGroup.Create().
				SetAccountID(account.ID).
				SetGroupID(groups[i].GroupID).
				SetPriority(groups[i].Priority),
			)
		}
		if _, err := txClient.AccountGroup.CreateBulk(builders...).Save(ctx); err != nil {
			return err
		}
	}
	account.GroupIDs = groupIDs
	account.AccountGroups = append([]service.AccountGroup(nil), groups...)
	if err := enqueueSchedulerOutbox(ctx, txClient, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, buildSchedulerGroupPayload(groupIDs)); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (r *accountRepository) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	m, err := r.client.Account.Query().Where(dbaccount.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	accounts, err := r.accountsToService(ctx, []*dbent.Account{m})
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, service.ErrAccountNotFound
	}
	return &accounts[0], nil
}

func (r *accountRepository) GetByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	if len(ids) == 0 {
		return []*service.Account{}, nil
	}

	// De-duplicate while preserving order of first occurrence.
	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return []*service.Account{}, nil
	}

	entAccounts, err := r.client.Account.
		Query().
		Where(dbaccount.IDIn(uniqueIDs...)).
		WithProxy().
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(entAccounts) == 0 {
		return []*service.Account{}, nil
	}

	accountIDs := make([]int64, 0, len(entAccounts))
	entByID := make(map[int64]*dbent.Account, len(entAccounts))
	for _, acc := range entAccounts {
		entByID[acc.ID] = acc
		accountIDs = append(accountIDs, acc.ID)
	}

	groupsByAccount, groupIDsByAccount, accountGroupsByAccount, err := r.loadAccountGroups(ctx, accountIDs)
	if err != nil {
		return nil, err
	}

	outByID := make(map[int64]*service.Account, len(entAccounts))
	for _, entAcc := range entAccounts {
		out := accountEntityToService(entAcc)
		if out == nil {
			continue
		}

		// Prefer the preloaded proxy edge when available.
		if entAcc.Edges.Proxy != nil {
			out.Proxy = proxyEntityToService(entAcc.Edges.Proxy)
		}

		if groups, ok := groupsByAccount[entAcc.ID]; ok {
			out.Groups = groups
		}
		if groupIDs, ok := groupIDsByAccount[entAcc.ID]; ok {
			out.GroupIDs = groupIDs
		}
		if ags, ok := accountGroupsByAccount[entAcc.ID]; ok {
			out.AccountGroups = ags
		}
		outByID[entAcc.ID] = out
	}

	// Preserve input order (first occurrence), and ignore missing IDs.
	out := make([]*service.Account, 0, len(uniqueIDs))
	for _, id := range uniqueIDs {
		if _, ok := entByID[id]; !ok {
			continue
		}
		if acc, ok := outByID[id]; ok && acc != nil {
			out = append(out, acc)
		}
	}

	return out, nil
}

func (r *accountRepository) ListDueUpstreamBillingProbeAccounts(ctx context.Context, now time.Time, limit int) ([]service.Account, error) {
	if accountRepositoryDialect(r) == dialect.MySQL {
		return r.listDueUpstreamBillingProbeAccountsMySQL(ctx, now, limit)
	}
	return r.listDueUpstreamBillingProbeAccountsPostgres(ctx, now, limit)
}

func (r *accountRepository) listDueUpstreamBillingProbeAccountsPostgres(ctx context.Context, now time.Time, limit int) ([]service.Account, error) {
	if limit <= 0 {
		return []service.Account{}, nil
	}
	if r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}
	rows, err := r.sql.QueryContext(ctx, `
		WITH candidates AS (
			SELECT
				id,
				extra #>> '{upstream_billing_probe,status}' AS probe_status,
				extra #>> '{upstream_billing_probe,next_probe_at}' AS next_probe_at
			FROM accounts
			WHERE deleted_at IS NULL
				AND status = 'active'
				AND type = 'apikey'
				AND extra @> '{"upstream_billing_probe_enabled": true}'::jsonb
		), parsed AS MATERIALIZED (
			SELECT
				id,
				probe_status,
				next_probe_at,
				next_probe_at ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$' AS rfc3339_shape,
				jsonb_path_query_first_tz(
					jsonb_build_object(
						'value',
						replace(regexp_replace(regexp_replace(
							next_probe_at,
							'(\.[0-9]{6})[0-9]+(Z|[+-][0-9]{2}:[0-9]{2})$',
							'\1\2'
						), 'Z$', '+00:00'), 'T', ' ')
					),
					'$.value.datetime()',
					'{}'::jsonb,
					true
				) #>> '{}' AS parsed_next_probe_at
			FROM candidates
		), normalized AS (
			SELECT
				id,
				probe_status,
				next_probe_at,
				parsed_next_probe_at,
				rfc3339_shape AND parsed_next_probe_at IS NOT NULL AS valid_next_probe_at
			FROM parsed
		)
		SELECT id
		FROM normalized
		WHERE probe_status NOT IN ('ok', 'unsupported', 'failed')
			OR probe_status IS NULL
			OR next_probe_at IS NULL
			OR NOT valid_next_probe_at
			OR CASE WHEN valid_next_probe_at THEN parsed_next_probe_at::timestamptz <= $1 ELSE FALSE END
		ORDER BY
			CASE
				WHEN probe_status NOT IN ('ok', 'unsupported', 'failed')
					OR probe_status IS NULL
					OR next_probe_at IS NULL
					OR NOT valid_next_probe_at
				THEN 0
				ELSE 1
			END ASC,
			CASE WHEN valid_next_probe_at THEN parsed_next_probe_at::timestamptz END ASC NULLS FIRST,
			id ASC
		LIMIT $2
	`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []service.Account{}, nil
	}
	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			out = append(out, *account)
		}
	}
	return out, nil
}

func (r *accountRepository) listDueUpstreamBillingProbeAccountsMySQL(ctx context.Context, now time.Time, limit int) ([]service.Account, error) {
	if limit <= 0 {
		return []service.Account{}, nil
	}
	if r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}

	rows, err := r.sql.QueryContext(ctx, `
		SELECT
			id,
			JSON_UNQUOTE(JSON_EXTRACT(extra, '$.upstream_billing_probe.status')),
			JSON_UNQUOTE(JSON_EXTRACT(extra, '$.upstream_billing_probe.next_probe_at'))
		FROM accounts
		WHERE deleted_at IS NULL
			AND status = 'active'
			AND type = 'apikey'
			AND JSON_EXTRACT(extra, '$.upstream_billing_probe_enabled') = TRUE
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type dueCandidate struct {
		id          int64
		priority    int
		nextProbeAt time.Time
	}
	candidates := make([]dueCandidate, 0, limit)
	for rows.Next() {
		var id int64
		var status, nextProbeAt sql.NullString
		if err := rows.Scan(&id, &status, &nextProbeAt); err != nil {
			return nil, err
		}

		probeStatus := strings.TrimSpace(status.String)
		validStatus := probeStatus == service.UpstreamBillingProbeStatusOK ||
			probeStatus == service.UpstreamBillingProbeStatusUnsupported ||
			probeStatus == service.UpstreamBillingProbeStatusFailed
		parsedNextProbeAt, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(nextProbeAt.String))
		if !validStatus || !nextProbeAt.Valid || parseErr != nil {
			candidates = append(candidates, dueCandidate{id: id})
			continue
		}
		if !parsedNextProbeAt.After(now.UTC()) {
			candidates = append(candidates, dueCandidate{id: id, priority: 1, nextProbeAt: parsedNextProbeAt})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		if candidates[i].priority == 1 && !candidates[i].nextProbeAt.Equal(candidates[j].nextProbeAt) {
			return candidates[i].nextProbeAt.Before(candidates[j].nextProbeAt)
		}
		return candidates[i].id < candidates[j].id
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	if len(candidates) == 0 {
		return []service.Account{}, nil
	}

	ids := make([]int64, len(candidates))
	for i := range candidates {
		ids[i] = candidates[i].id
	}
	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*service.Account, len(accounts))
	for _, account := range accounts {
		if account != nil {
			byID[account.ID] = account
		}
	}
	out := make([]service.Account, 0, len(accounts))
	for _, id := range ids {
		if account := byID[id]; account != nil {
			out = append(out, *account)
		}
	}
	return out, nil
}

// ExistsByID 检查指定 ID 的账号是否存在。
// 相比 GetByID，此方法性能更优，因为：
//   - 使用 Exist() 方法生成 SELECT EXISTS 查询，只返回布尔值
//   - 不加载完整的账号实体及其关联数据（Groups、Proxy 等）
//   - 适用于删除前的存在性检查等只需判断有无的场景
func (r *accountRepository) ExistsByID(ctx context.Context, id int64) (bool, error) {
	exists, err := r.client.Account.Query().Where(dbaccount.IDEQ(id)).Exist(ctx)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *accountRepository) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*service.Account, error) {
	if crsAccountID == "" {
		return nil, nil
	}

	// 使用 sqljson.ValueEQ 生成 JSON 路径过滤，避免手写 SQL 片段导致语法兼容问题。
	// 排除 spark 影子账号(parent_account_id 非空):影子不持凭据,绝不能被 CRS 当作普通账号
	// 更新而覆盖 type/credentials/proxy。即便影子 Extra 被误写入 crs_account_id 也不会命中
	// (外审第7轮 P1)。
	m, err := r.client.Account.Query().
		Where(dbaccount.ParentAccountIDIsNil()).
		Where(func(s *entsql.Selector) {
			s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, crsAccountID, sqljson.Path("crs_account_id")))
		}).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	accounts, err := r.accountsToService(ctx, []*dbent.Account{m})
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	return &accounts[0], nil
}

func (r *accountRepository) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	// parent_account_id IS NULL 排除 spark 影子账号:影子不是 CRS 账号,绝不能进 CRS 同步映射
	// (否则会被当普通账号更新而覆盖 type/credentials/proxy)(外审第7轮 P1)。
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, JSON_UNQUOTE(JSON_EXTRACT(extra, '$.crs_account_id'))
		FROM accounts
		WHERE deleted_at IS NULL
			AND parent_account_id IS NULL
			AND JSON_EXTRACT(extra, '$.crs_account_id') IS NOT NULL
			AND JSON_UNQUOTE(JSON_EXTRACT(extra, '$.crs_account_id')) != ''
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]int64)
	for rows.Next() {
		var id int64
		var crsID string
		if err := rows.Scan(&id, &crsID); err != nil {
			return nil, err
		}
		result[crsID] = id
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *accountRepository) Update(ctx context.Context, account *service.Account) error {
	return r.updateAccount(ctx, account, nil, nil, account.RateMultiplier)
}

// UpdateWithAccountBillingSettings applies an admin account edit while
// preserving a concurrently probe-synchronized rate unless the request
// explicitly includes a manual rate.
func (r *accountRepository) UpdateWithAccountBillingSettings(
	ctx context.Context,
	account *service.Account,
	probeEnabled *bool,
	rateSyncEnabled *bool,
	rateMultiplier *float64,
) error {
	return r.updateAccount(ctx, account, probeEnabled, rateSyncEnabled, rateMultiplier)
}

func (r *accountRepository) updateAccount(
	ctx context.Context,
	account *service.Account,
	explicitProbeEnabled *bool,
	explicitRateSyncEnabled *bool,
	explicitRateMultiplier *float64,
) error {
	if account == nil {
		return nil
	}

	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	client := r.client
	var tx *dbent.Tx
	if contextTx != nil {
		client = contextTx.Client()
	} else {
		var err error
		tx, err = r.client.Tx(ctx)
		if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
			return err
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			client = tx.Client()
		}
	}

	updated, err := r.updateLockedAccount(
		ctx,
		client,
		account,
		explicitProbeEnabled,
		explicitRateSyncEnabled,
		explicitRateMultiplier,
	)
	if err != nil {
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}
	if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, buildSchedulerGroupPayload(account.GroupIDs)); err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	account.UpdatedAt = updated.UpdatedAt
	// 普通账号编辑（如 model_mapping / credentials）也需要立即刷新单账号快照，
	// 否则网关在 outbox worker 延迟或异常时仍可能读到旧配置。
	if contextTx == nil {
		r.syncSchedulerAccountSnapshot(baseCtx, account.ID)
	}
	return nil
}

func (r *accountRepository) updateLockedAccount(
	ctx context.Context,
	client *dbent.Client,
	account *service.Account,
	explicitProbeEnabled *bool,
	explicitRateSyncEnabled *bool,
	explicitRateMultiplier *float64,
) (*dbent.Account, error) {
	extra, err := lockAndMergeAccountProbeExtra(ctx, client, account, explicitProbeEnabled, explicitRateSyncEnabled)
	if err != nil {
		return nil, err
	}
	account.Extra = extra

	schedulable := account.Schedulable
	if account.Status == service.StatusError {
		schedulable = false
	}

	builder := client.Account.UpdateOneID(account.ID).
		SetName(account.Name).
		SetNillableNotes(account.Notes).
		SetPlatform(account.Platform).
		SetType(account.Type).
		SetCredentials(normalizeJSONMap(account.Credentials)).
		SetExtra(normalizeJSONMap(account.Extra)).
		SetTags(normalizeJSONStringSlice(account.Tags)).
		SetConcurrency(account.Concurrency).
		SetPriority(account.Priority).
		SetStatus(account.Status).
		SetErrorMessage(account.ErrorMessage).
		SetSchedulable(schedulable).
		SetAutoPauseOnExpired(account.AutoPauseOnExpired)

	if explicitRateMultiplier != nil {
		builder.SetRateMultiplier(*explicitRateMultiplier)
	}
	if account.LoadFactor != nil {
		builder.SetLoadFactor(*account.LoadFactor)
	} else {
		builder.ClearLoadFactor()
	}

	if account.ProxyID != nil {
		builder.SetProxyID(*account.ProxyID)
	} else {
		builder.ClearProxyID()
	}
	if account.LastUsedAt != nil {
		builder.SetLastUsedAt(*account.LastUsedAt)
	} else {
		builder.ClearLastUsedAt()
	}
	if account.ExpiresAt != nil {
		builder.SetExpiresAt(*account.ExpiresAt)
	} else {
		builder.ClearExpiresAt()
	}
	if account.RateLimitedAt != nil {
		builder.SetRateLimitedAt(*account.RateLimitedAt)
	} else {
		builder.ClearRateLimitedAt()
	}
	if account.RateLimitResetAt != nil {
		builder.SetRateLimitResetAt(*account.RateLimitResetAt)
	} else {
		builder.ClearRateLimitResetAt()
	}
	if account.OverloadUntil != nil {
		builder.SetOverloadUntil(*account.OverloadUntil)
	} else {
		builder.ClearOverloadUntil()
	}
	if account.SessionWindowStart != nil {
		builder.SetSessionWindowStart(*account.SessionWindowStart)
	} else {
		builder.ClearSessionWindowStart()
	}
	if account.SessionWindowEnd != nil {
		builder.SetSessionWindowEnd(*account.SessionWindowEnd)
	} else {
		builder.ClearSessionWindowEnd()
	}
	if account.SessionWindowStatus != "" {
		builder.SetSessionWindowStatus(account.SessionWindowStatus)
	} else {
		builder.ClearSessionWindowStatus()
	}
	if account.Notes == nil {
		builder.ClearNotes()
	}

	builder.SetQuotaDimension(dbaccount.QuotaDimension(account.QuotaDimensionOrDefault()))
	builder.SetNillableParentAccountID(account.ParentAccountID)

	return builder.Save(ctx)
}

func lockAndMergeAccountProbeExtra(ctx context.Context, client *dbent.Client, account *service.Account, explicitProbeEnabled, explicitRateSyncEnabled *bool) (map[string]any, error) {
	if client != nil && client.Driver() != nil && client.Driver().Dialect() == dialect.MySQL {
		return lockAndMergeAccountProbeExtraMySQL(ctx, client, account, explicitProbeEnabled, explicitRateSyncEnabled)
	}
	return lockAndMergeAccountProbeExtraPostgres(ctx, client, account, explicitProbeEnabled, explicitRateSyncEnabled)
}

func lockAndMergeAccountProbeExtraPostgres(ctx context.Context, client *dbent.Client, account *service.Account, explicitProbeEnabled, explicitRateSyncEnabled *bool) (map[string]any, error) {
	credentials, err := json.Marshal(normalizeJSONMap(account.Credentials))
	if err != nil {
		return nil, err
	}
	var proxyID any
	if account.ProxyID != nil {
		proxyID = *account.ProxyID
	}
	rows, err := client.QueryContext(ctx, `
		SELECT
			platform = $2
			AND type = $3
			AND credentials = $4::jsonb
			AND proxy_id IS NOT DISTINCT FROM $5,
			COALESCE(
				platform IN ('openai', 'anthropic')
				AND $2 IN ('openai', 'anthropic')
				AND type = 'apikey'
				AND $3 = 'apikey'
				AND credentials -> 'api_key' IS NOT DISTINCT FROM $4::jsonb -> 'api_key'
				AND `+ollamaCloudBaseURLMatchesSQL("credentials ->> 'base_url'")+`
				AND `+ollamaCloudBaseURLMatchesSQL("$4::jsonb ->> 'base_url'")+`,
				false
			),
			proxy_id IS NOT DISTINCT FROM $5,
			extra -> 'upstream_billing_probe_enabled',
			extra -> 'upstream_billing_rate_sync_enabled',
			extra -> 'upstream_billing_probe',
			extra -> 'ollama_cloud_usage_session',
			extra -> 'ollama_cloud_usage_auto_refresh',
			extra -> 'ollama_cloud_usage_snapshot'
		FROM accounts
		WHERE id = $1 AND deleted_at IS NULL
		FOR NO KEY UPDATE
	`, account.ID, account.Platform, account.Type, string(credentials), proxyID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAccountNotFound
	}
	return mergeLockedAccountProbeExtraRows(rows, account, explicitProbeEnabled, explicitRateSyncEnabled)
}

func lockAndMergeAccountProbeExtraMySQL(ctx context.Context, client *dbent.Client, account *service.Account, explicitProbeEnabled, explicitRateSyncEnabled *bool) (map[string]any, error) {
	credentials, err := json.Marshal(normalizeJSONMap(account.Credentials))
	if err != nil {
		return nil, err
	}
	var proxyID any
	if account.ProxyID != nil {
		proxyID = *account.ProxyID
	}
	rows, err := client.QueryContext(ctx, `
		SELECT
			platform = ?
			AND type = ?
			AND credentials = CAST(? AS JSON)
			AND proxy_id <=> ?,
			COALESCE(
				platform IN ('openai', 'anthropic')
				AND ? IN ('openai', 'anthropic')
				AND type = 'apikey'
				AND ? = 'apikey'
				AND JSON_EXTRACT(credentials, '$.api_key') <=> JSON_EXTRACT(CAST(? AS JSON), '$.api_key')
				AND `+ollamaCloudBaseURLMatchesMySQL("JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.base_url'))")+`
				AND `+ollamaCloudBaseURLMatchesMySQL("JSON_UNQUOTE(JSON_EXTRACT(CAST(? AS JSON), '$.base_url'))")+`,
				FALSE
			),
			proxy_id <=> ?,
			JSON_EXTRACT(extra, '$.upstream_billing_probe_enabled'),
			JSON_EXTRACT(extra, '$.upstream_billing_rate_sync_enabled'),
			JSON_EXTRACT(extra, '$.upstream_billing_probe'),
			JSON_EXTRACT(extra, '$.ollama_cloud_usage_session'),
			JSON_EXTRACT(extra, '$.ollama_cloud_usage_auto_refresh'),
			JSON_EXTRACT(extra, '$.ollama_cloud_usage_snapshot')
		FROM accounts
		WHERE id = ? AND deleted_at IS NULL
		FOR UPDATE
	`, account.Platform, account.Type, string(credentials), proxyID,
		account.Platform, account.Type, string(credentials), string(credentials), proxyID, account.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAccountNotFound
	}

	return mergeLockedAccountProbeExtraRows(rows, account, explicitProbeEnabled, explicitRateSyncEnabled)
}

func mergeLockedAccountProbeExtraRows(rows *sql.Rows, account *service.Account, explicitProbeEnabled, explicitRateSyncEnabled *bool) (map[string]any, error) {
	var (
		identityUnchanged            bool
		ollamaGroupIdentityUnchanged bool
		ollamaProxyIdentityUnchanged bool
		currentEnabled               []byte
		currentRateSyncEnabled       []byte
		currentSnapshot              []byte
		currentOllamaSession         []byte
		currentOllamaAutoRefresh     []byte
		currentOllamaSnapshot        []byte
	)
	if err := rows.Scan(
		&identityUnchanged,
		&ollamaGroupIdentityUnchanged,
		&ollamaProxyIdentityUnchanged,
		&currentEnabled,
		&currentRateSyncEnabled,
		&currentSnapshot,
		&currentOllamaSession,
		&currentOllamaAutoRefresh,
		&currentOllamaSnapshot,
	); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	extra := copyJSONMap(normalizeJSONMap(account.Extra))
	for _, key := range []string{
		service.UpstreamBillingProbeEnabledExtraKey,
		service.UpstreamBillingRateSyncEnabledExtraKey,
		service.UpstreamBillingProbeExtraKey,
		service.OllamaCloudUsageSessionExtraKey,
		service.OllamaCloudUsageAutoRefreshExtraKey,
		service.OllamaCloudUsageSnapshotExtraKey,
	} {
		delete(extra, key)
	}
	probeAccount := service.IsUpstreamBillingProbeIdentity(account.Platform, account.Type)
	probeEnabled := false
	probeEnabledPresent := false
	if probeAccount {
		if enabled, ok, err := decodeAccountExtraJSON(currentEnabled); err != nil {
			return nil, err
		} else if value, isBool := enabled.(bool); ok && isBool {
			probeEnabled = value
			probeEnabledPresent = true
		}
		if explicitProbeEnabled != nil {
			probeEnabled = *explicitProbeEnabled
			probeEnabledPresent = true
		}
	}
	rateSyncEnabled := false
	rateSyncEnabledPresent := false
	if probeAccount {
		if enabled, ok, err := decodeAccountExtraJSON(currentRateSyncEnabled); err != nil {
			return nil, err
		} else if value, isBool := enabled.(bool); ok && isBool {
			rateSyncEnabled = value
			rateSyncEnabledPresent = true
		}
		if explicitRateSyncEnabled != nil {
			rateSyncEnabled = *explicitRateSyncEnabled
			rateSyncEnabledPresent = true
		}
		if explicitProbeEnabled != nil && !*explicitProbeEnabled {
			rateSyncEnabled = false
			rateSyncEnabledPresent = true
		}
		// 同步依赖探测，方向是单向的：探测关闭（或探测键缺失）一律把同步归零。
		// 不做反向推导——由 rate_sync=true 推出 probe=true 会让一条"同步开、探测键
		// 缺失"的僵尸记录在任意一次无关编辑时静默打开周期性外呼。需要同时打开两个
		// 开关的调用方（管理端编辑）自己显式传 explicitProbeEnabled=true。
		if !probeEnabled {
			rateSyncEnabled = false
		}
		if probeEnabledPresent {
			extra[service.UpstreamBillingProbeEnabledExtraKey] = probeEnabled
		}
		if rateSyncEnabledPresent {
			extra[service.UpstreamBillingRateSyncEnabledExtraKey] = rateSyncEnabled
		}
	}
	probeExplicitlyDisabled := probeEnabledPresent && !probeEnabled
	if identityUnchanged && !probeExplicitlyDisabled {
		if snapshot, ok, err := decodeAccountExtraJSON(currentSnapshot); err != nil {
			return nil, err
		} else if ok {
			extra[service.UpstreamBillingProbeExtraKey] = snapshot
		}
	}

	if service.IsOllamaCloudUsageAccount(account) && ollamaGroupIdentityUnchanged {
		for key, raw := range map[string][]byte{
			service.OllamaCloudUsageSessionExtraKey:     currentOllamaSession,
			service.OllamaCloudUsageAutoRefreshExtraKey: currentOllamaAutoRefresh,
		} {
			if value, ok, err := decodeAccountExtraJSON(raw); err != nil {
				return nil, err
			} else if ok {
				extra[key] = value
			}
		}
		if ollamaProxyIdentityUnchanged {
			if snapshot, ok, err := decodeAccountExtraJSON(currentOllamaSnapshot); err != nil {
				return nil, err
			} else if ok {
				extra[service.OllamaCloudUsageSnapshotExtraKey] = snapshot
			}
		}
	}
	return extra, nil
}

func decodeAccountExtraJSON(raw []byte) (any, bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (r *accountRepository) UpdateCredentials(ctx context.Context, id int64, credentials map[string]any) error {
	if accountRepositoryDialect(r) == dialect.MySQL {
		return r.updateCredentialsMySQL(ctx, id, credentials)
	}
	return r.updateCredentialsPostgres(ctx, id, credentials)
}

func (r *accountRepository) updateCredentialsPostgres(ctx context.Context, id int64, credentials map[string]any) error {
	payload, err := json.Marshal(normalizeJSONMap(credentials))
	if err != nil {
		return err
	}
	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	client := r.client
	var tx *dbent.Tx
	if contextTx != nil {
		client = contextTx.Client()
	} else if r.client != nil {
		var txErr error
		tx, txErr = r.client.Tx(ctx)
		if txErr != nil && !errors.Is(txErr, dbent.ErrTxStarted) {
			return txErr
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			client = tx.Client()
		}
	}
	result, err := client.ExecContext(ctx, `
		UPDATE accounts
		SET
			credentials = $1::jsonb,
			extra = CASE
				WHEN platform IN ('openai', 'anthropic')
					AND type = 'apikey'
					AND credentials IS DISTINCT FROM $1::jsonb
					AND (
						credentials -> 'api_key' IS DISTINCT FROM $1::jsonb -> 'api_key'
						OR NOT (
							COALESCE(`+ollamaCloudBaseURLMatchesSQL("credentials ->> 'base_url'")+`, false)
							AND COALESCE(`+ollamaCloudBaseURLMatchesSQL("$1::jsonb ->> 'base_url'")+`, false)
						)
					)
				THEN COALESCE(extra, '{}'::jsonb)
					- 'upstream_billing_probe'
					- 'ollama_cloud_usage_session'
					- 'ollama_cloud_usage_auto_refresh'
					- 'ollama_cloud_usage_snapshot'
				-- 上游倍率探测已放宽到全部 API-key 平台：凭证变化即视为探测
				-- 身份变化，丢弃 stale 快照。
				WHEN type = 'apikey'
					AND credentials IS DISTINCT FROM $1::jsonb
				THEN COALESCE(extra, '{}'::jsonb) - 'upstream_billing_probe'
				ELSE extra
			END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, string(payload), id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	if contextTx == nil {
		r.syncSchedulerAccountSnapshot(baseCtx, id)
	}
	return nil
}

func (r *accountRepository) updateCredentialsMySQL(ctx context.Context, id int64, credentials map[string]any) error {
	payload, err := json.Marshal(normalizeJSONMap(credentials))
	if err != nil {
		return err
	}
	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	client := r.client
	var tx *dbent.Tx
	if contextTx != nil {
		client = contextTx.Client()
	} else if r.client != nil {
		var txErr error
		tx, txErr = r.client.Tx(ctx)
		if txErr != nil && !errors.Is(txErr, dbent.ErrTxStarted) {
			return txErr
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			client = tx.Client()
		}
	}
	payloadString := string(payload)
	result, err := client.ExecContext(ctx, `
		UPDATE accounts
		SET
			credentials = CAST(? AS JSON),
			extra = CASE
				WHEN platform IN ('openai', 'anthropic')
					AND type = 'apikey'
					AND NOT (credentials <=> CAST(? AS JSON))
					AND (
						NOT (JSON_EXTRACT(credentials, '$.api_key') <=> JSON_EXTRACT(CAST(? AS JSON), '$.api_key'))
						OR NOT (
							COALESCE(`+ollamaCloudBaseURLMatchesMySQL("JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.base_url'))")+`, FALSE)
							AND COALESCE(`+ollamaCloudBaseURLMatchesMySQL("JSON_UNQUOTE(JSON_EXTRACT(CAST(? AS JSON), '$.base_url'))")+`, FALSE)
						)
					)
				THEN JSON_REMOVE(
					CASE
						WHEN platform = 'openai' THEN JSON_REMOVE(COALESCE(extra, JSON_OBJECT()), '$.upstream_billing_probe')
						ELSE COALESCE(extra, JSON_OBJECT())
					END,
					'$.ollama_cloud_usage_session',
					'$.ollama_cloud_usage_auto_refresh',
					'$.ollama_cloud_usage_snapshot'
				)
				WHEN platform = 'openai'
					AND type = 'apikey'
					AND NOT (credentials <=> CAST(? AS JSON))
				THEN JSON_REMOVE(COALESCE(extra, JSON_OBJECT()), '$.upstream_billing_probe')
				ELSE extra
			END,
			updated_at = NOW(6)
		WHERE id = ? AND deleted_at IS NULL
	`, payloadString, payloadString, payloadString, payloadString, payloadString, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	if contextTx == nil {
		r.syncSchedulerAccountSnapshot(baseCtx, id)
	}
	return nil
}

func (r *accountRepository) Delete(ctx context.Context, id int64) error {
	groupIDs, err := r.loadAccountGroupIDs(ctx, id)
	if err != nil {
		return err
	}
	// 使用事务保证账号与关联分组的删除原子性
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		// 已处于外部事务中（ErrTxStarted），复用当前 client
		txClient = r.client
	}

	if _, err := txClient.AccountGroup.Delete().Where(dbaccountgroup.AccountIDEQ(id)).Exec(ctx); err != nil {
		return err
	}
	if _, err := txClient.ExecContext(ctx, "DELETE FROM scheduled_test_plans WHERE account_id = ?", id); err != nil {
		return err
	}
	if _, err := txClient.Account.Delete().Where(dbaccount.IDEQ(id)).Exec(ctx); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	r.deleteSchedulerAccountSnapshot(ctx, id)
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, buildSchedulerGroupPayload(groupIDs)); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue account delete failed: account=%d err=%v", id, err)
	}
	return nil
}

func (r *accountRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "", "", 0, "", nil)
}

func (r *accountRepository) ListAllWithFilters(ctx context.Context, platform, accountType, status, search string, groupID int64, privacyMode string, tags []string) ([]service.Account, error) {
	params := pagination.PaginationParams{Page: 1, PageSize: 1000, SortBy: "id", SortOrder: pagination.SortOrderDesc}
	var all []service.Account
	for {
		accounts, page, err := r.ListWithFilters(ctx, params, platform, accountType, status, search, groupID, privacyMode, tags)
		if err != nil {
			return nil, err
		}
		all = append(all, accounts...)
		if page == nil || int64(len(all)) >= page.Total || len(accounts) == 0 {
			return all, nil
		}
		params.Page++
	}
}

func (r *accountRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string, tags []string) ([]service.Account, *pagination.PaginationResult, error) {
	q := r.client.Account.Query()

	if platform != "" {
		q = q.Where(dbaccount.PlatformEQ(platform))
	}
	if accountType != "" {
		q = q.Where(dbaccount.TypeEQ(accountType))
	}
	if status != "" {
		switch status {
		case service.StatusActive:
			q = q.Where(
				dbaccount.StatusEQ(status),
				dbaccount.SchedulableEQ(true),
				dbaccount.Or(
					dbaccount.RateLimitResetAtIsNil(),
					dbaccount.RateLimitResetAtLTE(time.Now()),
				),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.Or(
						entsql.IsNull(col),
						entsql.LTE(col, entsql.Expr("NOW()")),
					))
				}),
			)
		case "rate_limited":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbaccount.RateLimitResetAtGT(time.Now()),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.Or(
						entsql.IsNull(col),
						entsql.LTE(col, entsql.Expr("NOW()")),
					))
				}),
			)
		case "temp_unschedulable":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.And(
						entsql.Not(entsql.IsNull(col)),
						entsql.GT(col, entsql.Expr("NOW()")),
					))
				}),
			)
		case "unschedulable":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbaccount.SchedulableEQ(false),
				dbaccount.Or(
					dbaccount.RateLimitResetAtIsNil(),
					dbaccount.RateLimitResetAtLTE(time.Now()),
				),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.Or(
						entsql.IsNull(col),
						entsql.LTE(col, entsql.Expr("NOW()")),
					))
				}),
			)
		default:
			q = q.Where(dbaccount.StatusEQ(status))
		}
	}
	if search != "" {
		q = q.Where(
			dbaccount.Or(
				dbaccount.NameContainsFold(search),
				dbaccount.NotesContainsFold(search),
				dbpredicate.Account(func(s *entsql.Selector) {
					s.Where(entsql.P(func(b *entsql.Builder) {
						b.WriteString("LOWER(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(").
							Ident(s.C(dbaccount.FieldExtra)).
							WriteString(", '$.email_address')), '')) LIKE ")
						b.Arg("%" + strings.ToLower(search) + "%")
					}))
				}),
			),
		)
	}
	if groupID == service.AccountListGroupUngrouped {
		q = q.Where(dbaccount.Not(dbaccount.HasAccountGroups()))
	} else if groupID > 0 {
		q = q.Where(dbaccount.HasAccountGroupsWith(dbaccountgroup.GroupIDEQ(groupID)))
	}
	if privacyMode != "" {
		q = q.Where(dbpredicate.Account(func(s *entsql.Selector) {
			path := sqljson.Path("privacy_mode")
			switch privacyMode {
			case service.AccountPrivacyModeUnsetFilter:
				s.Where(entsql.Or(
					entsql.Not(sqljson.HasKey(dbaccount.FieldExtra, path)),
					sqljson.ValueEQ(dbaccount.FieldExtra, "", path),
				))
			default:
				s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, privacyMode, path))
			}
		}))
	}

	// Tags：MySQL JSON 数组包含语义。OR 语义——只要包含任意一个传入标签即命中。
	// MySQL 8 用 `JSON_CONTAINS(tags, '["x"]')`；多个标签时按标签拆成多条单标签谓词，再用 OR 连接。
	if len(tags) > 0 {
		// 预先把每个标签序列化为 JSON_CONTAINS candidate payload。
		perTagPayloads := make([]string, 0, len(tags))
		for _, tag := range tags {
			payload, marshalErr := json.Marshal([]string{tag})
			if marshalErr != nil {
				return nil, nil, marshalErr
			}
			perTagPayloads = append(perTagPayloads, string(payload))
		}
		q = q.Where(dbpredicate.Account(func(s *entsql.Selector) {
			s.Where(entsql.P(func(b *entsql.Builder) {
				b.WriteString("(")
				for i, payload := range perTagPayloads {
					if i > 0 {
						b.WriteString(" OR ")
					}
					b.WriteString("JSON_CONTAINS(").
						Ident(s.C(dbaccount.FieldTags)).
						WriteString(", ").
						Arg(payload).
						WriteString(")")
				}
				b.WriteString(")")
			}))
		}))
	}

	// Clone before Count so interceptor-appended predicates (SoftDeleteMixin deleted_at IS NULL) do not pollute the subsequent list query.
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	accountsQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range accountListOrder(params, r.client.Driver().Dialect()) {
		accountsQuery = accountsQuery.Order(order)
	}

	accounts, err := accountsQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outAccounts, err := r.accountsToService(ctx, accounts)
	if err != nil {
		return nil, nil, err
	}
	return outAccounts, paginationResultFromTotal(int64(total), params), nil
}

// ListAllTags 返回所有未删除账号 tags 字段去重排序后的并集，用于自动补全候选。
//
// 实现策略：SQL 只查每个账号的 tags 字段，Go 层做 unnest + dedupe。
// 避免依赖数据库侧 JSON 展开函数，保持 MySQL 运行路径简单可控。
// 规模评估：N 账号 * 平均标签数（< 20）一次拿回，~100KB 内存可容纳 5k 账号场景。
func (r *accountRepository) ListAllTags(ctx context.Context) ([]string, error) {
	type tagsHolder struct {
		Tags []string `json:"tags"`
	}
	var holders []tagsHolder
	if err := r.client.Account.Query().
		Where(dbaccount.DeletedAtIsNil()).
		Select(dbaccount.FieldTags).
		Scan(ctx, &holders); err != nil {
		return nil, err
	}

	set := make(map[string]struct{}, 64)
	for _, h := range holders {
		for _, tag := range h.Tags {
			if tag == "" {
				continue
			}
			set[tag] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for tag := range set {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out, nil
}

func accountListOrder(params pagination.PaginationParams, dbDialect string) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderAsc)
	if sortBy == "upstream_billing_rate" {
		direction := "ASC"
		tieOrder := entsql.Asc
		if sortOrder == pagination.SortOrderDesc {
			direction = "DESC"
			tieOrder = entsql.Desc
		}
		return []func(*entsql.Selector){func(s *entsql.Selector) {
			extra := s.C(dbaccount.FieldExtra)
			expression := upstreamBillingRateSortExpression(extra, dbDialect)
			if dbDialect == dialect.MySQL {
				// MySQL does not support PostgreSQL's NULLS LAST syntax. Keep
				// missing/unsupported probe values at the end for both directions.
				s.OrderExpr(entsql.Expr(expression + " IS NULL ASC"))
				s.OrderExpr(entsql.Expr(expression + " " + direction))
			} else {
				s.OrderExpr(entsql.Expr(expression + " " + direction + " NULLS LAST"))
			}
			s.OrderBy(tieOrder(s.C(dbaccount.FieldID)))
		}}
	}

	field := dbaccount.FieldName
	defaultOrder := true
	switch sortBy {
	case "", "name":
		field = dbaccount.FieldName
	case "id":
		field = dbaccount.FieldID
		defaultOrder = false
	case "status":
		field = dbaccount.FieldStatus
		defaultOrder = false
	case "schedulable":
		field = dbaccount.FieldSchedulable
		defaultOrder = false
	case "priority":
		field = dbaccount.FieldPriority
		defaultOrder = false
	case "rate_multiplier":
		field = dbaccount.FieldRateMultiplier
		defaultOrder = false
	case "last_used_at":
		direction := "ASC"
		tieOrder := entsql.Asc
		if sortOrder == pagination.SortOrderDesc {
			direction = "DESC"
			tieOrder = entsql.Desc
		}
		return []func(*entsql.Selector){func(s *entsql.Selector) {
			lastUsedAt := s.C(dbaccount.FieldLastUsedAt)
			if dbDialect == dialect.MySQL {
				s.OrderExpr(entsql.Expr(lastUsedAt + " IS NULL ASC"))
				s.OrderExpr(entsql.Expr(lastUsedAt + " " + direction))
			} else {
				s.OrderExpr(entsql.Expr(lastUsedAt + " " + direction + " NULLS LAST"))
			}
			s.OrderBy(tieOrder(s.C(dbaccount.FieldID)))
		}}
	case "expires_at":
		field = dbaccount.FieldExpiresAt
		defaultOrder = false
	case "created_at":
		field = dbaccount.FieldCreatedAt
		defaultOrder = false
	}

	if sortOrder == pagination.SortOrderDesc {
		return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(dbaccount.FieldID)}
	}
	if defaultOrder {
		return []func(*entsql.Selector){dbent.Asc(dbaccount.FieldName), dbent.Asc(dbaccount.FieldID)}
	}
	return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(dbaccount.FieldID)}
}

func upstreamBillingRateSortExpression(extra, dbDialect string) string {
	if dbDialect == dialect.MySQL {
		return upstreamBillingRateSortExpressionMySQL(extra)
	}
	return upstreamBillingRateSortExpressionPostgres(extra)
}

func upstreamBillingRateSortExpressionPostgres(extra string) string {
	status := extra + " #>> '{upstream_billing_probe,status}'"
	effectiveJSON := extra + " #> '{upstream_billing_probe,data,effective_rate_multiplier}'"
	effective := extra + " #>> '{upstream_billing_probe,data,effective_rate_multiplier}'"
	resolvedJSON := extra + " #> '{upstream_billing_probe,data,resolved_rate_multiplier}'"
	resolved := extra + " #>> '{upstream_billing_probe,data,resolved_rate_multiplier}'"
	peakEnabledJSON := extra + " #> '{upstream_billing_probe,data,peak_rate_enabled}'"
	peakEnabled := extra + " #>> '{upstream_billing_probe,data,peak_rate_enabled}'"
	peakStart := extra + " #>> '{upstream_billing_probe,data,peak_start}'"
	peakEnd := extra + " #>> '{upstream_billing_probe,data,peak_end}'"
	peakMultiplierJSON := extra + " #> '{upstream_billing_probe,data,peak_rate_multiplier}'"
	peakMultiplier := extra + " #>> '{upstream_billing_probe,data,peak_rate_multiplier}'"
	peakMultiplierValue := "(CASE WHEN jsonb_typeof(" + peakMultiplierJSON + ") = 'number' THEN (" + peakMultiplier + ")::numeric END)"
	billingScope := extra + " #>> '{upstream_billing_probe,data,billing_scope}'"
	timezone := extra + " #>> '{upstream_billing_probe,data,timezone}'"
	validClock := "'^([01][0-9]|2[0-3]):[0-5][0-9]$'"
	startMinute := "(CASE WHEN " + peakStart + " ~ " + validClock + " THEN split_part(" + peakStart + ", ':', 1)::numeric * 60 + split_part(" + peakStart + ", ':', 2)::numeric END)"
	endMinute := "(CASE WHEN " + peakEnd + " ~ " + validClock + " THEN split_part(" + peakEnd + ", ':', 1)::numeric * 60 + split_part(" + peakEnd + ", ':', 2)::numeric END)"
	localMinute := "(EXTRACT(HOUR FROM (CURRENT_TIMESTAMP AT TIME ZONE (" + timezone + "))) * 60 + EXTRACT(MINUTE FROM (CURRENT_TIMESTAMP AT TIME ZONE (" + timezone + "))))"
	validPeakWindow := peakStart + " ~ " + validClock + " AND " +
		peakEnd + " ~ " + validClock + " AND " +
		startMinute + " < " + endMinute
	validPeakConfig := validPeakWindow + " AND " + peakMultiplierValue + " >= 0 AND " +
		"EXISTS (SELECT 1 FROM pg_timezone_names WHERE name = " + timezone + ")"
	dynamicRate := "CASE WHEN " + peakEnabled + " = 'false' THEN (" + resolved + ")::numeric WHEN " + peakEnabled + " = 'true' AND " + validPeakConfig +
		" THEN (" + resolved + ")::numeric * CASE WHEN " + localMinute + " >= " + startMinute + " AND " + localMinute + " < " + endMinute +
		" THEN " + peakMultiplierValue + " ELSE 1 END ELSE NULL END"
	legacySnapshot := "jsonb_typeof(" + resolvedJSON + ") IS NULL AND jsonb_typeof(" + peakEnabledJSON + ") IS NULL"

	return "CASE WHEN " + status + " IN ('ok', 'failed') AND (jsonb_typeof(" + resolvedJSON + ") = 'number' OR jsonb_typeof(" + effectiveJSON + ") = 'number') THEN CASE WHEN jsonb_typeof(" +
		resolvedJSON + ") = 'number' AND jsonb_typeof(" + peakEnabledJSON + ") = 'boolean' THEN CASE WHEN " + billingScope + " = 'token' THEN " + dynamicRate + " ELSE NULL END WHEN " + legacySnapshot +
		" AND jsonb_typeof(" + effectiveJSON + ") = 'number' THEN (" + effective + ")::numeric END END"
}

func upstreamBillingRateSortExpressionMySQL(extra string) string {
	statusJSON := "JSON_EXTRACT(" + extra + ", '$.upstream_billing_probe.status')"
	status := "JSON_UNQUOTE(" + statusJSON + ")"
	effectiveJSON := "JSON_EXTRACT(" + extra + ", '$.upstream_billing_probe.data.effective_rate_multiplier')"
	effective := "JSON_UNQUOTE(" + effectiveJSON + ")"
	resolvedJSON := "JSON_EXTRACT(" + extra + ", '$.upstream_billing_probe.data.resolved_rate_multiplier')"
	resolved := "JSON_UNQUOTE(" + resolvedJSON + ")"
	peakEnabledJSON := "JSON_EXTRACT(" + extra + ", '$.upstream_billing_probe.data.peak_rate_enabled')"
	peakEnabled := "JSON_UNQUOTE(" + peakEnabledJSON + ")"
	peakStart := "JSON_UNQUOTE(JSON_EXTRACT(" + extra + ", '$.upstream_billing_probe.data.peak_start'))"
	peakEnd := "JSON_UNQUOTE(JSON_EXTRACT(" + extra + ", '$.upstream_billing_probe.data.peak_end'))"
	peakMultiplierJSON := "JSON_EXTRACT(" + extra + ", '$.upstream_billing_probe.data.peak_rate_multiplier')"
	peakMultiplier := "JSON_UNQUOTE(" + peakMultiplierJSON + ")"
	billingScope := "JSON_UNQUOTE(JSON_EXTRACT(" + extra + ", '$.upstream_billing_probe.data.billing_scope'))"
	timezone := "JSON_UNQUOTE(JSON_EXTRACT(" + extra + ", '$.upstream_billing_probe.data.timezone'))"
	numericTypes := "('INTEGER', 'DOUBLE', 'DECIMAL', 'UNSIGNED INTEGER', 'SIGNED INTEGER')"
	numericResolved := "JSON_TYPE(" + resolvedJSON + ") IN " + numericTypes
	numericEffective := "JSON_TYPE(" + effectiveJSON + ") IN " + numericTypes
	numericPeakMultiplier := "JSON_TYPE(" + peakMultiplierJSON + ") IN " + numericTypes
	startMinute := "(CAST(SUBSTRING(" + peakStart + ", 1, 2) AS UNSIGNED) * 60 + CAST(SUBSTRING(" + peakStart + ", 4, 2) AS UNSIGNED))"
	endMinute := "(CAST(SUBSTRING(" + peakEnd + ", 1, 2) AS UNSIGNED) * 60 + CAST(SUBSTRING(" + peakEnd + ", 4, 2) AS UNSIGNED))"
	localTime := "CONVERT_TZ(UTC_TIMESTAMP(), '+00:00', " + timezone + ")"
	localMinute := "(HOUR(" + localTime + ") * 60 + MINUTE(" + localTime + "))"
	validClock := "(" + peakStart + " REGEXP '^([01][0-9]|2[0-3]):[0-5][0-9]$' AND " + peakEnd + " REGEXP '^([01][0-9]|2[0-3]):[0-5][0-9]$')"
	validPeakConfig := validClock + " AND " + startMinute + " < " + endMinute +
		" AND " + numericPeakMultiplier + " AND CAST(" + peakMultiplier + " AS DECIMAL(30, 12)) >= 0" +
		" AND " + localTime + " IS NOT NULL"
	dynamicRate := "CASE WHEN " + peakEnabled + " = 'false' THEN CAST(" + resolved + " AS DECIMAL(30, 12))" +
		" WHEN " + peakEnabled + " = 'true' AND " + validPeakConfig + " THEN CAST(" + resolved + " AS DECIMAL(30, 12)) * CASE WHEN " +
		localMinute + " >= " + startMinute + " AND " + localMinute + " < " + endMinute + " THEN CAST(" + peakMultiplier + " AS DECIMAL(30, 12)) ELSE 1 END ELSE NULL END"
	legacySnapshot := "JSON_TYPE(" + resolvedJSON + ") IS NULL AND JSON_TYPE(" + peakEnabledJSON + ") IS NULL"

	return "CASE WHEN " + status + " IN ('ok', 'failed') AND (" + numericResolved + " OR " + numericEffective + ") THEN CASE WHEN " +
		numericResolved + " AND JSON_TYPE(" + peakEnabledJSON + ") = 'BOOLEAN' THEN CASE WHEN " + billingScope + " = 'token' THEN " + dynamicRate +
		" ELSE NULL END WHEN " + legacySnapshot + " AND " + numericEffective + " THEN CAST(" + effective + " AS DECIMAL(30, 12)) END END"
}

func (r *accountRepository) ListByGroup(ctx context.Context, groupID int64) ([]service.Account, error) {
	accounts, err := r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status: service.StatusActive,
	})
	if err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *accountRepository) ListActive(ctx context.Context) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(dbaccount.StatusEQ(service.StatusActive)).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListOAuthRefreshCandidates(ctx context.Context) ([]service.Account, error) {
	if r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}
	// 只排除“仍处于 token refresh retry exhausted 临时不可调度窗口”的账号。
	// COALESCE(..., FALSE) 避免 NULL 三值逻辑误排除健康账号（temp_unschedulable_until=NULL）。
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id
		FROM accounts
		WHERE deleted_at IS NULL
			AND schedulable = TRUE
			AND status = 'active'
			AND type IN ('oauth', 'setup-token')
			AND platform IN ('anthropic', 'openai', 'gemini', 'antigravity')
			AND TRIM(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.refresh_token')), '')) <> ''
			AND NOT (
				COALESCE(temp_unschedulable_until > NOW(6), FALSE)
				AND COALESCE(temp_unschedulable_reason LIKE 'token refresh retry exhausted:%', FALSE)
			)
		ORDER BY priority ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []service.Account{}, nil
	}

	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			out = append(out, *account)
		}
	}
	return out, nil
}

func (r *accountRepository) ListByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformEQ(platform),
			dbaccount.StatusEQ(service.StatusActive),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) UpdateLastUsed(ctx context.Context, id int64) error {
	now := time.Now()
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetLastUsedAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"last_used": map[string]int64{
			strconv.FormatInt(id, 10): now.Unix(),
		},
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountLastUsed, &id, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue last used failed: account=%d err=%v", id, err)
	}
	return nil
}

func (r *accountRepository) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	if len(updates) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(updates))
	args := make([]any, 0, len(updates)*2)
	caseSQL := "UPDATE accounts SET last_used_at = CASE id"
	for id, ts := range updates {
		caseSQL += " WHEN ? THEN ?"
		args = append(args, id, ts)
		ids = append(ids, id)
	}
	inClause, inArgs := buildAccountInt64InClause(ids)
	caseSQL += " END, updated_at = NOW() WHERE id IN (" + inClause + ") AND deleted_at IS NULL"
	args = append(args, inArgs...)

	_, err := r.sql.ExecContext(ctx, caseSQL, args...)
	if err != nil {
		return err
	}
	lastUsedPayload := make(map[string]int64, len(updates))
	for id, ts := range updates {
		lastUsedPayload[strconv.FormatInt(id, 10)] = ts.Unix()
	}
	payload := map[string]any{"last_used": lastUsedPayload}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountLastUsed, nil, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue batch last used failed: err=%v", err)
	}
	return nil
}

func (r *accountRepository) SetError(ctx context.Context, id int64, errorMsg string) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetStatus(service.StatusError).
		SetErrorMessage(errorMsg).
		SetSchedulable(false).
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue set error failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

// syncSchedulerAccountSnapshot 在账号状态变更时主动同步快照到调度器缓存。
// 当账号被设置为错误、禁用、不可调度或临时不可调度时调用，
// 确保调度器和粘性会话逻辑能及时感知账号的最新状态，避免继续使用不可用账号。
//
// syncSchedulerAccountSnapshot proactively syncs account snapshot to scheduler cache
// when account status changes. Called when account is set to error, disabled,
// unschedulable, or temporarily unschedulable, ensuring scheduler and sticky session
// logic can promptly detect the latest account state and avoid using unavailable accounts.
func (r *accountRepository) syncSchedulerAccountSnapshot(ctx context.Context, accountID int64) {
	if r == nil || r.schedulerCache == nil || accountID <= 0 {
		return
	}
	account, err := r.GetByID(ctx, accountID)
	if err != nil {
		logger.LegacyPrintf("repository.account", "[Scheduler] sync account snapshot read failed: id=%d err=%v", accountID, err)
		return
	}
	if err := r.schedulerCache.SetAccount(ctx, account); err != nil {
		logger.LegacyPrintf("repository.account", "[Scheduler] sync account snapshot write failed: id=%d err=%v", accountID, err)
	}
}

func (r *accountRepository) deleteSchedulerAccountSnapshot(ctx context.Context, accountID int64) {
	if r == nil || r.schedulerCache == nil || accountID <= 0 {
		return
	}
	if err := r.schedulerCache.DeleteAccount(ctx, accountID); err != nil {
		logger.LegacyPrintf("repository.account", "[Scheduler] delete account snapshot failed: id=%d err=%v", accountID, err)
	}
}

func (r *accountRepository) syncSchedulerAccountSnapshots(ctx context.Context, accountIDs []int64) {
	if r == nil || r.schedulerCache == nil || len(accountIDs) == 0 {
		return
	}

	uniqueIDs := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]struct{}, len(accountIDs))
	for _, id := range accountIDs {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return
	}

	accounts, err := r.GetByIDs(ctx, uniqueIDs)
	if err != nil {
		logger.LegacyPrintf("repository.account", "[Scheduler] batch sync account snapshot read failed: count=%d err=%v", len(uniqueIDs), err)
		return
	}

	for _, account := range accounts {
		if account == nil {
			continue
		}
		if err := r.schedulerCache.SetAccount(ctx, account); err != nil {
			logger.LegacyPrintf("repository.account", "[Scheduler] batch sync account snapshot write failed: id=%d err=%v", account.ID, err)
		}
	}
}

func (r *accountRepository) ClearError(ctx context.Context, id int64) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetStatus(service.StatusActive).
		SetErrorMessage("").
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear error failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) AddToGroup(ctx context.Context, accountID, groupID int64, priority int) error {
	_, err := r.client.AccountGroup.Create().
		SetAccountID(accountID).
		SetGroupID(groupID).
		SetPriority(priority).
		Save(ctx)
	if err != nil {
		return err
	}
	payload := buildSchedulerGroupPayload([]int64{groupID})
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountGroupsChanged, &accountID, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue add to group failed: account=%d group=%d err=%v", accountID, groupID, err)
	}
	return nil
}

func (r *accountRepository) RemoveFromGroup(ctx context.Context, accountID, groupID int64) error {
	_, err := r.client.AccountGroup.Delete().
		Where(
			dbaccountgroup.AccountIDEQ(accountID),
			dbaccountgroup.GroupIDEQ(groupID),
		).
		Exec(ctx)
	if err != nil {
		return err
	}
	payload := buildSchedulerGroupPayload([]int64{groupID})
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountGroupsChanged, &accountID, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue remove from group failed: account=%d group=%d err=%v", accountID, groupID, err)
	}
	return nil
}

func (r *accountRepository) GetGroups(ctx context.Context, accountID int64) ([]service.Group, error) {
	groups, err := r.client.Group.Query().
		Where(
			dbgroup.HasAccountsWith(dbaccount.IDEQ(accountID)),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		outGroups = append(outGroups, *groupEntityToService(groups[i]))
	}
	return outGroups, nil
}

func (r *accountRepository) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	existingGroupIDs, err := r.loadAccountGroupIDs(ctx, accountID)
	if err != nil {
		return err
	}
	// 使用事务保证删除旧绑定与创建新绑定的原子性
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		// 已处于外部事务中（ErrTxStarted），复用当前 client
		txClient = r.client
	}

	if _, err := txClient.AccountGroup.Delete().Where(dbaccountgroup.AccountIDEQ(accountID)).Exec(ctx); err != nil {
		return err
	}

	if len(groupIDs) == 0 {
		if tx != nil {
			return tx.Commit()
		}
		return nil
	}

	builders := make([]*dbent.AccountGroupCreate, 0, len(groupIDs))
	for i, groupID := range groupIDs {
		builders = append(builders, txClient.AccountGroup.Create().
			SetAccountID(accountID).
			SetGroupID(groupID).
			SetPriority(i+1),
		)
	}

	if _, err := txClient.AccountGroup.CreateBulk(builders...).Save(ctx); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	payload := buildSchedulerGroupPayload(mergeGroupIDs(existingGroupIDs, groupIDs))
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountGroupsChanged, &accountID, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue bind groups failed: account=%d err=%v", accountID, err)
	}
	return nil
}

func (r *accountRepository) ListSchedulable(ctx context.Context) ([]service.Account, error) {
	accounts, err := r.schedulableAccountsQuery(time.Now()).All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableAccountLoads(ctx context.Context) ([]service.AccountWithConcurrency, error) {
	accounts, err := r.schedulableAccountsQuery(time.Now()).
		Select(
			dbaccount.FieldID,
			dbaccount.FieldConcurrency,
			dbaccount.FieldLoadFactor,
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	loads := make([]service.AccountWithConcurrency, 0, len(accounts))
	for _, account := range accounts {
		projection := service.Account{
			ID:          account.ID,
			Concurrency: account.Concurrency,
			LoadFactor:  account.LoadFactor,
		}
		loads = append(loads, service.AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: projection.EffectiveLoadFactor(),
		})
	}
	return loads, nil
}

func (r *accountRepository) schedulableAccountsQuery(now time.Time) *dbent.AccountQuery {
	return r.client.Account.Query().
		Where(
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority))
}

func (r *accountRepository) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]service.Account, error) {
	return r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status:      service.StatusActive,
		schedulable: true,
	})
}

func (r *accountRepository) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformEQ(platform),
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]service.Account, error) {
	// 单平台查询复用多平台逻辑，保持过滤条件与排序策略一致。
	return r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status:      service.StatusActive,
		schedulable: true,
		platforms:   []string{platform},
	})
}

func (r *accountRepository) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	if len(platforms) == 0 {
		return nil, nil
	}
	// 仅返回可调度的活跃账号，并过滤处于过载/限流窗口的账号。
	// 代理与分组信息统一在 accountsToService 中批量加载，避免 N+1 查询。
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformIn(platforms...),
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformEQ(platform),
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			dbaccount.Not(dbaccount.HasAccountGroups()),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	if len(platforms) == 0 {
		return nil, nil
	}
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformIn(platforms...),
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			dbaccount.Not(dbaccount.HasAccountGroups()),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	if len(platforms) == 0 {
		return nil, nil
	}
	// 复用按分组查询逻辑，保证分组优先级 + 账号优先级的排序与筛选一致。
	return r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status:      service.StatusActive,
		schedulable: true,
		platforms:   platforms,
	})
}

// ListModelAvailabilityCandidates returns the persistently configured account
// pool used to decide whether a model is supported. Unlike scheduling queries,
// it intentionally ignores transient runtime state (rate limits, overload,
// temporary unschedulability, and expiry windows).
func (r *accountRepository) ListModelAvailabilityCandidates(
	ctx context.Context,
	groupID *int64,
	platforms []string,
	includeGrouped bool,
) ([]service.Account, error) {
	if len(platforms) == 0 {
		return []service.Account{}, nil
	}
	if groupID != nil {
		return r.queryAccountsByGroup(ctx, *groupID, accountGroupQueryOptions{
			status:               service.StatusActive,
			schedulable:          true,
			ignoreTransientState: true,
			platforms:            platforms,
		})
	}

	preds := []dbpredicate.Account{
		dbaccount.StatusEQ(service.StatusActive),
		dbaccount.SchedulableEQ(true),
		dbaccount.PlatformIn(platforms...),
	}
	if !includeGrouped {
		preds = append(preds, dbaccount.Not(dbaccount.HasAccountGroups()))
	}
	accounts, err := r.client.Account.Query().
		Where(preds...).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	now := time.Now()
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetRateLimitedAt(now).
		SetRateLimitResetAt(resetAt).
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue rate limit failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

// SetRateLimitedIfLater atomically extends an account-level rate limit. Grok
// requests may finish concurrently, so an older response must not overwrite a
// later reset boundary observed by another request or instance.
func (r *accountRepository) SetRateLimitedIfLater(ctx context.Context, id int64, resetAt time.Time) error {
	now := time.Now()
	updated, err := r.client.Account.Update().
		Where(
			dbaccount.IDEQ(id),
			dbaccount.Or(
				dbaccount.RateLimitResetAtIsNil(),
				dbaccount.RateLimitResetAtLT(resetAt),
			),
		).
		SetRateLimitedAt(now).
		SetRateLimitResetAt(resetAt).
		Save(ctx)
	if err != nil {
		return err
	}
	if updated == 0 {
		// This instance may not have observed the later value written elsewhere.
		// Refresh its local scheduler snapshot even though no outbox event is needed.
		r.syncSchedulerAccountSnapshot(ctx, id)
		return nil
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue extended rate limit failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error {
	if scope == "" {
		return nil
	}
	now := time.Now().UTC()
	payload := map[string]string{
		"rate_limited_at":     now.Format(time.RFC3339),
		"rate_limit_reset_at": resetAt.UTC().Format(time.RFC3339),
	}
	if len(reason) > 0 {
		if value := strings.TrimSpace(reason[0]); value != "" {
			payload["reason"] = value
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(
		ctx,
		`UPDATE accounts SET extra = JSON_SET(COALESCE(extra, JSON_OBJECT()), ?, CAST(? AS JSON)), updated_at = NOW() WHERE id = ? AND deleted_at IS NULL`,
		"$.model_rate_limits."+scope,
		string(raw),
		id,
	)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue model rate limit failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetOverloadUntil(until).
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue overload failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	result, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET temp_unschedulable_until = ?,
			temp_unschedulable_reason = ?,
			updated_at = NOW()
		WHERE id = ?
			AND deleted_at IS NULL
			AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until < ?)
	`, until, reason, id, until)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected <= 0 {
		return nil
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue temp unschedulable failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) ClearTempUnschedulable(ctx context.Context, id int64) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET temp_unschedulable_until = NULL,
			temp_unschedulable_reason = NULL,
			updated_at = NOW()
		WHERE id = ?
			AND deleted_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear temp unschedulable failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) ClearRateLimit(ctx context.Context, id int64) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		ClearRateLimitedAt().
		ClearRateLimitResetAt().
		ClearOverloadUntil().
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear rate limit failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(
		ctx,
		"UPDATE accounts SET extra = JSON_REMOVE(COALESCE(extra, JSON_OBJECT()), '$.antigravity_quota_scopes'), updated_at = NOW() WHERE id = ? AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		// 清理型操作必须保持幂等：当账号存在但 extra 中本来就没有
		// antigravity_quota_scopes 时，MySQL 可能返回 0 rows affected。
		// 这不应被当成账号不存在，否则“重置状态/恢复状态”会在无可清理字段时误报错。
		exists, existsErr := client.Account.Query().Where(dbaccount.IDEQ(id)).Exist(ctx)
		if existsErr != nil {
			return existsErr
		}
		if !exists {
			return service.ErrAccountNotFound
		}
		return nil
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear quota scopes failed: account=%d err=%v", id, err)
	}
	return nil
}

func (r *accountRepository) ClearModelRateLimits(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(
		ctx,
		"UPDATE accounts SET extra = JSON_REMOVE(COALESCE(extra, JSON_OBJECT()), '$.model_rate_limits'), updated_at = NOW() WHERE id = ? AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		// 清理型操作必须保持幂等：当账号存在但 extra 中本来就没有
		// model_rate_limits 时，MySQL 可能返回 0 rows affected。
		// 这不应被当成账号不存在，否则“重置状态/恢复状态”会在无可清理字段时误报错。
		exists, existsErr := client.Account.Query().Where(dbaccount.IDEQ(id)).Exist(ctx)
		if existsErr != nil {
			return existsErr
		}
		if !exists {
			return service.ErrAccountNotFound
		}
		return nil
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear model rate limit failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	builder := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetSessionWindowStatus(status)
	if start != nil {
		builder.SetSessionWindowStart(*start)
	}
	if end != nil {
		builder.SetSessionWindowEnd(*end)
	}
	_, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	// 触发调度器缓存更新（仅当窗口时间有变化时）
	if start != nil || end != nil {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
			logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue session window update failed: account=%d err=%v", id, err)
		}
	}
	return nil
}

func (r *accountRepository) UpdateSessionWindowEnd(ctx context.Context, id int64, end time.Time) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetSessionWindowEnd(end).
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue session window end update failed: account=%d err=%v", id, err)
	}
	return nil
}

func (r *accountRepository) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetSchedulable(schedulable).
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue schedulable change failed: account=%d err=%v", id, err)
	}
	if !schedulable {
		r.syncSchedulerAccountSnapshot(ctx, id)
	}
	return nil
}

func (r *accountRepository) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	// MySQL 无 UPDATE ... RETURNING；两步：先查出待暂停 ID，再按 ID 批量暂停并精确 outbox。
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id
		FROM accounts
		WHERE deleted_at IS NULL
			AND schedulable = TRUE
			AND auto_pause_on_expired = TRUE
			AND expires_at IS NOT NULL
			AND expires_at <= ?
	`, now)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = rows.Close()
	}()

	accountIDs := make([]int64, 0)
	for rows.Next() {
		var accountID int64
		if err := rows.Scan(&accountID); err != nil {
			return 0, err
		}
		accountIDs = append(accountIDs, accountID)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(accountIDs) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(accountIDs))
	args := make([]any, 0, len(accountIDs)+1)
	args = append(args, now)
	for i, id := range accountIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET schedulable = FALSE,
			updated_at = NOW()
		WHERE deleted_at IS NULL
			AND schedulable = TRUE
			AND auto_pause_on_expired = TRUE
			AND expires_at IS NOT NULL
			AND expires_at <= ?
			AND id IN (`+strings.Join(placeholders, ",")+`)
	`, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected > 0 {
		// 只刷新本次暂停的账号，避免少量账号到期触发所有调度桶重建。
		payload := map[string]any{"account_ids": accountIDs}
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); err != nil {
			logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue auto pause account changes failed: err=%v", err)
		}
	}
	return affected, nil
}

func (r *accountRepository) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	// 使用 JSON 合并操作实现原子更新，避免读-改-写的并发丢失更新问题
	payload, err := json.Marshal(updates)
	if err != nil {
		return err
	}

	clearProbeSnapshot := upstreamBillingProbeExplicitlyDisabled(updates) || upstreamBillingProbeSnapshotClearRequested(updates)
	durableSchedulerChange := shouldEnqueueSchedulerOutboxForExtraUpdates(updates) || clearProbeSnapshot
	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	client := clientFromContext(ctx, r.client)
	var tx *dbent.Tx
	if durableSchedulerChange && contextTx == nil {
		var txErr error
		tx, txErr = r.client.Tx(ctx)
		if txErr != nil && !errors.Is(txErr, dbent.ErrTxStarted) {
			return txErr
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			client = tx.Client()
		}
	}
	dbDialect := accountRepositoryDialect(r)
	extraExpression := "JSON_MERGE_PATCH(COALESCE(extra, JSON_OBJECT()), CAST(? AS JSON))"
	query := "UPDATE accounts SET extra = " + extraExpression + ", updated_at = NOW() WHERE id = ? AND deleted_at IS NULL"
	if dbDialect == dialect.MySQL {
		if clearProbeSnapshot {
			extraExpression = "JSON_REMOVE(" + extraExpression + ", '$.upstream_billing_probe')"
		}
		query = "UPDATE accounts SET extra = " + extraExpression + ", updated_at = NOW(6) WHERE id = ? AND deleted_at IS NULL"
	} else {
		extraExpression = "COALESCE(extra, '{}'::jsonb) || $1::jsonb"
		if clearProbeSnapshot {
			extraExpression = "(" + extraExpression + ") - 'upstream_billing_probe'"
		}
		query = "UPDATE accounts SET extra = " + extraExpression + ", updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL"
	}
	result, err := client.ExecContext(
		ctx,
		query,
		string(payload), id,
	)

	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	if durableSchedulerChange {
		if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
			return err
		}
		if tx != nil {
			if err := tx.Commit(); err != nil {
				return err
			}
		}
		if contextTx == nil {
			r.syncSchedulerAccountSnapshot(baseCtx, id)
		}
	} else {
		// 观测型 extra 字段不需要触发 bucket 重建，但仍同步单账号快照，
		// 让 sticky session / GetAccount 命中缓存时也能读到最新数据，
		// 同时避免缓存局部 patch 覆盖掉并发写入的其它账号字段。
		if dbent.TxFromContext(ctx) == nil {
			r.syncSchedulerAccountSnapshot(ctx, id)
		}
	}
	return nil
}

// UpdateUpstreamBillingProbeSnapshot stores a probe result only while the
// network identity used by that probe is still current.
func (r *accountRepository) UpdateUpstreamBillingProbeSnapshot(
	ctx context.Context,
	account *service.Account,
	snapshot *service.UpstreamBillingProbeSnapshot,
	rateMultiplier *float64,
) error {
	if account == nil || snapshot == nil {
		return service.ErrAccountNilInput
	}
	if snapshot.Status != service.UpstreamBillingProbeStatusOK {
		rateMultiplier = nil
	}
	if dbent.TxFromContext(ctx) == nil {
		tx, err := r.client.Tx(ctx)
		if errors.Is(err, dbent.ErrTxStarted) {
			return r.updateUpstreamBillingProbeSnapshotInTx(ctx, account, snapshot, rateMultiplier)
		}
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		if err := r.updateUpstreamBillingProbeSnapshotInTx(dbent.NewTxContext(ctx, tx), account, snapshot, rateMultiplier); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		// The durable outbox event is committed with the snapshot. This direct
		// cache write only reduces visibility latency on the current instance.
		r.syncSchedulerAccountSnapshot(ctx, account.ID)
		return nil
	}
	return r.updateUpstreamBillingProbeSnapshotInTx(ctx, account, snapshot, rateMultiplier)
}

func (r *accountRepository) updateUpstreamBillingProbeSnapshotInTx(
	ctx context.Context,
	account *service.Account,
	snapshot *service.UpstreamBillingProbeSnapshot,
	rateMultiplier *float64,
) error {
	payload, err := json.Marshal(map[string]any{service.UpstreamBillingProbeExtraKey: snapshot})
	if err != nil {
		return err
	}
	credentials, err := json.Marshal(account.Credentials)
	if err != nil {
		return err
	}
	var expectedSnapshot any
	if account.Extra != nil {
		expectedSnapshot = account.Extra[service.UpstreamBillingProbeExtraKey]
	}
	expectedSnapshotJSON, err := json.Marshal(expectedSnapshot)
	if err != nil {
		return err
	}
	var expectedEnabled any
	if account.Extra != nil {
		expectedEnabled = account.Extra[service.UpstreamBillingProbeEnabledExtraKey]
	}
	expectedEnabledJSON, err := json.Marshal(expectedEnabled)
	if err != nil {
		return err
	}
	var expectedRateSyncEnabled any
	if account.Extra != nil {
		expectedRateSyncEnabled = account.Extra[service.UpstreamBillingRateSyncEnabledExtraKey]
	}
	expectedRateSyncEnabledJSON, err := json.Marshal(expectedRateSyncEnabled)
	if err != nil {
		return err
	}
	client := clientFromContext(ctx, r.client)
	proxyMatches, err := lockAndMatchProbeProxyIdentity(ctx, client, account)
	if err != nil {
		return err
	}
	if !proxyMatches {
		return service.ErrUpstreamBillingProbeIdentityChanged
	}
	var proxyID any
	if account.ProxyID != nil {
		proxyID = *account.ProxyID
	}
	var result sql.Result
	if client.Driver() != nil && client.Driver().Dialect() == dialect.MySQL {
		result, err = client.ExecContext(ctx, `
			UPDATE accounts
			SET
				extra = JSON_MERGE_PATCH(COALESCE(extra, JSON_OBJECT()), CAST(? AS JSON)),
				rate_multiplier = CASE
					WHEN ? IS NOT NULL
						AND JSON_EXTRACT(extra, '$.upstream_billing_probe_enabled') = TRUE
						AND JSON_EXTRACT(extra, '$.upstream_billing_rate_sync_enabled') = TRUE
					THEN ?
					ELSE rate_multiplier
				END,
				updated_at = NOW(6)
			WHERE id = ?
				AND platform = ?
				AND type = ?
				AND credentials = CAST(? AS JSON)
				AND proxy_id <=> ?
				AND COALESCE(JSON_EXTRACT(extra, '$.upstream_billing_probe'), JSON_EXTRACT('null', '$')) = CAST(? AS JSON)
				AND COALESCE(JSON_EXTRACT(extra, '$.upstream_billing_probe_enabled'), JSON_EXTRACT('null', '$')) = CAST(? AS JSON)
				AND COALESCE(JSON_EXTRACT(extra, '$.upstream_billing_rate_sync_enabled'), JSON_EXTRACT('null', '$')) = CAST(? AS JSON)
				AND deleted_at IS NULL
		`, string(payload), rateMultiplier, rateMultiplier, account.ID, account.Platform, account.Type, string(credentials), proxyID, string(expectedSnapshotJSON), string(expectedEnabledJSON), string(expectedRateSyncEnabledJSON))
	} else {
		result, err = client.ExecContext(ctx, `
			UPDATE accounts
			SET
				extra = COALESCE(extra, '{}'::jsonb) || $1::jsonb,
				rate_multiplier = CASE
					WHEN $10::numeric IS NOT NULL
						AND extra @> '{"upstream_billing_probe_enabled": true}'::jsonb
						AND extra @> '{"upstream_billing_rate_sync_enabled": true}'::jsonb
					THEN $10::numeric
					ELSE rate_multiplier
				END,
				updated_at = NOW()
			WHERE id = $2
				AND platform = $3
				AND type = $4
				AND credentials = $5::jsonb
				AND proxy_id IS NOT DISTINCT FROM $6
				AND COALESCE(extra -> 'upstream_billing_probe', 'null'::jsonb) = $7::jsonb
				AND COALESCE(extra -> 'upstream_billing_probe_enabled', 'null'::jsonb) = $8::jsonb
				AND COALESCE(extra -> 'upstream_billing_rate_sync_enabled', 'null'::jsonb) = $9::jsonb
				AND deleted_at IS NULL
		`, string(payload), account.ID, account.Platform, account.Type, string(credentials), proxyID, string(expectedSnapshotJSON), string(expectedEnabledJSON), string(expectedRateSyncEnabledJSON), rateMultiplier)
	}
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrUpstreamBillingProbeIdentityChanged
	}
	return enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, nil)
}

func lockAndMatchProbeProxyIdentity(ctx context.Context, client *dbent.Client, account *service.Account) (bool, error) {
	if account.ProxyID == nil {
		return true, nil
	}
	query := `
		SELECT protocol, host, port, COALESCE(username, ''), COALESCE(password, ''), status
		FROM proxies
		WHERE id = ? AND deleted_at IS NULL
		FOR SHARE
	`
	if client == nil || client.Driver() == nil || client.Driver().Dialect() != dialect.MySQL {
		query = `
			SELECT protocol, host, port, COALESCE(username, ''), COALESCE(password, ''), status
			FROM proxies
			WHERE id = $1 AND deleted_at IS NULL
			FOR SHARE
		`
	}
	rows, err := client.QueryContext(ctx, query, *account.ProxyID)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return account.Proxy == nil, nil
	}
	if account.Proxy == nil || account.Proxy.ID != *account.ProxyID {
		return false, nil
	}
	var current proxyProbeIdentity
	if err := rows.Scan(&current.protocol, &current.host, &current.port, &current.username, &current.password, &current.status); err != nil {
		return false, err
	}
	return current == proxyProbeIdentityFromService(account.Proxy), rows.Err()
}

func shouldEnqueueSchedulerOutboxForExtraUpdates(updates map[string]any) bool {
	if len(updates) == 0 {
		return false
	}
	for key := range updates {
		if isSchedulerNeutralExtraKey(key) {
			continue
		}
		return true
	}
	return false
}

func isSchedulerNeutralExtraKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if _, ok := schedulerNeutralExtraKeys[key]; ok {
		return true
	}
	for _, prefix := range schedulerNeutralExtraKeyPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func upstreamBillingProbeExplicitlyDisabled(extra map[string]any) bool {
	enabled, ok := extra[service.UpstreamBillingProbeEnabledExtraKey].(bool)
	return ok && !enabled
}

func upstreamBillingProbeSnapshotClearRequested(extra map[string]any) bool {
	value, ok := extra[service.UpstreamBillingProbeExtraKey]
	return ok && value == nil
}

func ollamaCloudUsageSnapshotClearRequested(extra map[string]any) bool {
	value, ok := extra[service.OllamaCloudUsageSnapshotExtraKey]
	return ok && value == nil
}

func (r *accountRepository) BulkUpdate(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	if accountRepositoryDialect(r) == dialect.MySQL {
		return r.bulkUpdateMySQL(ctx, ids, updates)
	}
	return r.bulkUpdatePostgres(ctx, ids, updates)
}

func (r *accountRepository) bulkUpdatePostgres(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	setClauses := make([]string, 0, 8)
	args := make([]any, 0, 8)
	idx := 1
	ollamaProxyIdentityChanged := ""
	if updates.Name != nil {
		setClauses = append(setClauses, "name = $"+itoa(idx))
		args = append(args, *updates.Name)
		idx++
	}
	if updates.ProxyID != nil {
		if *updates.ProxyID == 0 {
			setClauses = append(setClauses, "proxy_id = NULL")
			ollamaProxyIdentityChanged = "proxy_id IS NOT NULL"
		} else {
			proxyPlaceholder := "$" + itoa(idx)
			setClauses = append(setClauses, "proxy_id = "+proxyPlaceholder)
			ollamaProxyIdentityChanged = "proxy_id IS DISTINCT FROM " + proxyPlaceholder
			args = append(args, *updates.ProxyID)
			idx++
		}
	}
	if updates.Concurrency != nil {
		setClauses = append(setClauses, "concurrency = $"+itoa(idx))
		args = append(args, *updates.Concurrency)
		idx++
	}
	if updates.Priority != nil {
		setClauses = append(setClauses, "priority = $"+itoa(idx))
		args = append(args, *updates.Priority)
		idx++
	}
	if updates.RateMultiplier != nil {
		setClauses = append(setClauses, "rate_multiplier = $"+itoa(idx))
		args = append(args, *updates.RateMultiplier)
		idx++
	}
	if updates.LoadFactor != nil {
		if *updates.LoadFactor <= 0 {
			setClauses = append(setClauses, "load_factor = NULL")
		} else {
			setClauses = append(setClauses, "load_factor = $"+itoa(idx))
			args = append(args, *updates.LoadFactor)
			idx++
		}
	}
	if updates.Status != nil {
		setClauses = append(setClauses, "status = $"+itoa(idx))
		args = append(args, *updates.Status)
		idx++
	}
	if updates.Schedulable != nil {
		setClauses = append(setClauses, "schedulable = $"+itoa(idx))
		args = append(args, *updates.Schedulable)
		idx++
	}
	if updates.ProbeEnabled != nil {
		if updates.Extra == nil {
			updates.Extra = make(map[string]any)
		}
		updates.Extra[service.UpstreamBillingProbeEnabledExtraKey] = *updates.ProbeEnabled
	}
	credentialPlaceholder := ""
	if len(updates.Credentials) > 0 {
		payload, err := json.Marshal(updates.Credentials)
		if err != nil {
			return 0, err
		}
		credentialPlaceholder = "$" + itoa(idx)
		setClauses = append(setClauses, "credentials = COALESCE(credentials, '{}'::jsonb) || "+credentialPlaceholder+"::jsonb")
		args = append(args, payload)
		idx++
	}

	ollamaGroupIdentityChanges := make([]string, 0, 2)
	if _, ok := updates.Credentials["api_key"]; ok {
		ollamaGroupIdentityChanges = append(ollamaGroupIdentityChanges, "credentials -> 'api_key' IS DISTINCT FROM "+credentialPlaceholder+"::jsonb -> 'api_key'")
	}
	if _, ok := updates.Credentials["base_url"]; ok {
		ollamaGroupIdentityChanges = append(ollamaGroupIdentityChanges,
			"NOT (COALESCE("+ollamaCloudBaseURLMatchesSQL("credentials ->> 'base_url'")+", false)"+
				" AND COALESCE("+ollamaCloudBaseURLMatchesSQL(credentialPlaceholder+"::jsonb ->> 'base_url'")+", false))")
	}

	if len(updates.Extra) > 0 || len(ollamaGroupIdentityChanges) > 0 || ollamaProxyIdentityChanged != "" {
		extraExpression := "COALESCE(extra, '{}'::jsonb)"
		if len(updates.Extra) > 0 {
			payload, err := json.Marshal(updates.Extra)
			if err != nil {
				return 0, err
			}
			extraExpression += " || $" + itoa(idx) + "::jsonb"
			args = append(args, payload)
			idx++
			if upstreamBillingProbeExplicitlyDisabled(updates.Extra) || upstreamBillingProbeSnapshotClearRequested(updates.Extra) {
				extraExpression = "(" + extraExpression + ") - 'upstream_billing_probe'"
			}
			if ollamaCloudUsageSnapshotClearRequested(updates.Extra) {
				extraExpression = "(" + extraExpression + ") - 'ollama_cloud_usage_snapshot'"
			}
		}
		eligibleAccount := "platform IN ('openai', 'anthropic') AND type = 'apikey'"
		groupIdentityChanged := ""
		if len(ollamaGroupIdentityChanges) > 0 {
			groupIdentityChanged = "(" + eligibleAccount + " AND (" + joinClauses(ollamaGroupIdentityChanges, " OR ") + "))"
		}
		snapshotIdentityChanged := groupIdentityChanged
		if ollamaProxyIdentityChanged != "" {
			proxyChanged := "(" + eligibleAccount + " AND " + ollamaProxyIdentityChanged + ")"
			if snapshotIdentityChanged == "" {
				snapshotIdentityChanged = proxyChanged
			} else {
				snapshotIdentityChanged = "(" + snapshotIdentityChanged + " OR " + proxyChanged + ")"
			}
		}
		if groupIdentityChanged != "" {
			extraExpression = "CASE" +
				" WHEN " + groupIdentityChanged + " THEN (" + extraExpression + ") - 'ollama_cloud_usage_session' - 'ollama_cloud_usage_auto_refresh' - 'ollama_cloud_usage_snapshot'" +
				" WHEN " + snapshotIdentityChanged + " THEN (" + extraExpression + ") - 'ollama_cloud_usage_snapshot'" +
				" ELSE " + extraExpression + " END"
		} else if snapshotIdentityChanged != "" {
			extraExpression = "CASE WHEN " + snapshotIdentityChanged + " THEN (" + extraExpression + ") - 'ollama_cloud_usage_snapshot' ELSE " + extraExpression + " END"
		}
		setClauses = append(setClauses, "extra = "+extraExpression)
	}

	if len(setClauses) == 0 {
		return 0, nil
	}
	setClauses = append(setClauses, "updated_at = NOW()")
	whereClause := " WHERE id = ANY($" + itoa(idx) + ") AND deleted_at IS NULL"
	args = append(args, pq.Array(ids))
	idx++
	if updates.ProbeEnabled != nil {
		whereClause += " AND type = $" + itoa(idx)
		args = append(args, service.AccountTypeAPIKey)
	}
	query := "UPDATE accounts SET " + joinClauses(setClauses, ", ") + whereClause

	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	exec := r.sql
	var tx *dbent.Tx
	if contextTx != nil {
		exec = contextTx.Client()
	} else if r.client != nil {
		var txErr error
		tx, txErr = r.client.Tx(ctx)
		if txErr != nil && !errors.Is(txErr, dbent.ErrTxStarted) {
			return 0, txErr
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			exec = tx.Client()
		}
	}
	result, err := exec.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if updates.ProbeEnabled != nil {
		expectedRows := int64(0)
		seenIDs := make(map[int64]struct{}, len(ids))
		for _, id := range ids {
			if _, seen := seenIDs[id]; seen {
				continue
			}
			seenIDs[id] = struct{}{}
			expectedRows++
		}
		if rows != expectedRows {
			return 0, service.ErrUpstreamBillingProbeAccountInvalid
		}
	}
	if rows > 0 {
		payload := map[string]any{"account_ids": ids}
		if err := enqueueSchedulerOutbox(ctx, exec, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); err != nil {
			return 0, err
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
	}
	if rows > 0 && contextTx == nil {
		shouldSync := false
		if updates.Status != nil && (*updates.Status == service.StatusError || *updates.Status == service.StatusDisabled) {
			shouldSync = true
		}
		if updates.Schedulable != nil && !*updates.Schedulable {
			shouldSync = true
		}
		if shouldSync {
			r.syncSchedulerAccountSnapshots(baseCtx, ids)
		}
	}
	return rows, nil
}

func (r *accountRepository) bulkUpdateMySQL(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	setClauses := make([]string, 0, 8)
	args := make([]any, 0, 8)

	idx := 1
	if updates.Name != nil {
		setClauses = append(setClauses, "name = $"+itoa(idx))
		args = append(args, *updates.Name)
		idx++
	}
	if updates.ProxyID != nil {
		// 0 表示清除代理（前端发送 0 而不是 null 来表达清除意图）
		if *updates.ProxyID == 0 {
			setClauses = append(setClauses, "proxy_id = NULL")
		} else {
			setClauses = append(setClauses, "proxy_id = $"+itoa(idx))
			args = append(args, *updates.ProxyID)
			idx++
		}
	}
	if updates.Concurrency != nil {
		setClauses = append(setClauses, "concurrency = $"+itoa(idx))
		args = append(args, *updates.Concurrency)
		idx++
	}
	if updates.Priority != nil {
		setClauses = append(setClauses, "priority = $"+itoa(idx))
		args = append(args, *updates.Priority)
		idx++
	}
	if updates.RateMultiplier != nil {
		setClauses = append(setClauses, "rate_multiplier = $"+itoa(idx))
		args = append(args, *updates.RateMultiplier)
		idx++
	}
	if updates.LoadFactor != nil {
		if *updates.LoadFactor <= 0 {
			setClauses = append(setClauses, "load_factor = NULL")
		} else {
			setClauses = append(setClauses, "load_factor = $"+itoa(idx))
			args = append(args, *updates.LoadFactor)
			idx++
		}
	}
	if updates.Status != nil {
		setClauses = append(setClauses, "status = $"+itoa(idx))
		args = append(args, *updates.Status)
		idx++
	}
	if updates.Schedulable != nil {
		setClauses = append(setClauses, "schedulable = $"+itoa(idx))
		args = append(args, *updates.Schedulable)
		idx++
	}
	// JSON 需要合并而非覆盖，使用 raw SQL 保持旧行为。
	if len(updates.Credentials) > 0 {
		payload, err := json.Marshal(updates.Credentials)
		if err != nil {
			return 0, err
		}
		setClauses = append(setClauses, "credentials = JSON_MERGE_PATCH(COALESCE(credentials, JSON_OBJECT()), CAST($"+itoa(idx)+" AS JSON))")
		args = append(args, payload)
		idx++
	}
	if len(updates.Extra) > 0 || updates.ProbeEnabled != nil {
		extraUpdates := copyJSONMap(updates.Extra)
		if extraUpdates == nil {
			extraUpdates = make(map[string]any, 1)
		}
		if updates.ProbeEnabled != nil {
			extraUpdates[service.UpstreamBillingProbeEnabledExtraKey] = *updates.ProbeEnabled
		}
		payload, err := json.Marshal(extraUpdates)
		if err != nil {
			return 0, err
		}
		extraExpression := "JSON_MERGE_PATCH(COALESCE(extra, JSON_OBJECT()), CAST($" + itoa(idx) + " AS JSON))"
		if upstreamBillingProbeExplicitlyDisabled(extraUpdates) || upstreamBillingProbeSnapshotClearRequested(extraUpdates) {
			extraExpression = "JSON_REMOVE(" + extraExpression + ", '$.upstream_billing_probe')"
		}
		setClauses = append(setClauses, "extra = "+extraExpression)
		args = append(args, payload)
		idx++
	}
	// Tags：替换语义。指针非 nil 即落库；空数组允许（清空所有标签）。
	// JSON 字段直接整体覆盖，不做 merge —— 标签的语义是"集合"，merge 会
	// 让"清空"无法表达。前置 service 层已规范化，这里不再校验。
	if updates.Tags != nil {
		payload, err := json.Marshal(normalizeJSONStringSlice(*updates.Tags))
		if err != nil {
			return 0, err
		}
		setClauses = append(setClauses, "tags = CAST($"+itoa(idx)+" AS JSON)")
		args = append(args, payload)
	}

	if len(setClauses) == 0 {
		return 0, nil
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	inClause, inArgs := buildAccountInt64InClause(ids)
	query := "UPDATE accounts SET " + joinClauses(setClauses, ", ") + " WHERE id IN (" + inClause + ") AND deleted_at IS NULL"
	args = append(args, inArgs...)
	if updates.ProbeEnabled != nil {
		query += " AND platform = ? AND type = ?"
		args = append(args, service.PlatformOpenAI, service.AccountTypeAPIKey)
	}
	query = opsReplaceDollarPlaceholders(query)

	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	exec := r.sql
	var tx *dbent.Tx
	if contextTx != nil {
		exec = contextTx.Client()
	} else if r.client != nil {
		var txErr error
		tx, txErr = r.client.Tx(ctx)
		if txErr != nil && !errors.Is(txErr, dbent.ErrTxStarted) {
			return 0, txErr
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			exec = tx.Client()
		}
	}
	if updates.ProbeEnabled != nil {
		if err := validateUpstreamBillingProbeBulkTargets(ctx, exec, ids); err != nil {
			return 0, err
		}
	}
	if err := invalidateOllamaCloudUsageForBulkUpdateMySQL(ctx, exec, ids, updates); err != nil {
		return 0, err
	}

	result, err := exec.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rows > 0 {
		payload := map[string]any{"account_ids": ids}
		if err := enqueueSchedulerOutbox(ctx, exec, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); err != nil {
			return 0, err
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
	}
	if rows > 0 && contextTx == nil {
		// Priority affects every scheduling decision, including whether an
		// existing sticky session may keep its current account. Publish it to
		// the per-account snapshot immediately instead of waiting for outbox
		// polling, otherwise a bulk priority edit can remain invisible on the
		// hot path.
		shouldSync := updates.Priority != nil
		if updates.Status != nil && (*updates.Status == service.StatusError || *updates.Status == service.StatusDisabled) {
			shouldSync = true
		}
		if updates.Schedulable != nil && !*updates.Schedulable {
			shouldSync = true
		}
		if shouldSync {
			r.syncSchedulerAccountSnapshots(baseCtx, ids)
		}
	}
	return rows, nil
}

func validateUpstreamBillingProbeBulkTargets(ctx context.Context, exec sqlExecutor, ids []int64) error {
	uniqueIDs := sortedUniqueAccountIDs(ids)
	if len(uniqueIDs) == 0 {
		return nil
	}
	placeholders, args := buildGroupInt64InClause(uniqueIDs)
	rows, err := exec.QueryContext(ctx, `SELECT id, platform, type FROM accounts
		WHERE id IN (`+placeholders+`) AND deleted_at IS NULL FOR UPDATE`, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	matched := 0
	for rows.Next() {
		var id int64
		var platform, accountType string
		if err := rows.Scan(&id, &platform, &accountType); err != nil {
			return err
		}
		if platform != service.PlatformOpenAI || accountType != service.AccountTypeAPIKey {
			return service.ErrUpstreamBillingProbeAccountInvalid
		}
		matched++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if matched != len(uniqueIDs) {
		return service.ErrUpstreamBillingProbeAccountInvalid
	}
	return nil
}

type accountGroupQueryOptions struct {
	status               string
	schedulable          bool
	ignoreTransientState bool
	platforms            []string // 允许的多个平台，空切片表示不进行平台过滤
}

func (r *accountRepository) queryAccountsByGroup(ctx context.Context, groupID int64, opts accountGroupQueryOptions) ([]service.Account, error) {
	q := r.client.AccountGroup.Query().
		Where(dbaccountgroup.GroupIDEQ(groupID))

	// 通过 account_groups 中间表查询账号，并按需叠加状态/平台/调度能力过滤。
	preds := make([]dbpredicate.Account, 0, 6)
	preds = append(preds, dbaccount.DeletedAtIsNil())
	if opts.status != "" {
		preds = append(preds, dbaccount.StatusEQ(opts.status))
	}
	if len(opts.platforms) > 0 {
		preds = append(preds, dbaccount.PlatformIn(opts.platforms...))
	}
	if opts.schedulable {
		preds = append(preds, dbaccount.SchedulableEQ(true))
		if !opts.ignoreTransientState {
			now := time.Now()
			preds = append(preds,
				tempUnschedulablePredicate(),
				notExpiredPredicate(now),
				dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
				dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
			)
		}
	}

	if len(preds) > 0 {
		q = q.Where(dbaccountgroup.HasAccountWith(preds...))
	}

	groups, err := q.
		Order(
			dbaccountgroup.ByPriority(),
			dbaccountgroup.ByAccountField(dbaccount.FieldPriority),
		).
		WithAccount().
		All(ctx)
	if err != nil {
		return nil, err
	}

	orderedIDs := make([]int64, 0, len(groups))
	accountMap := make(map[int64]*dbent.Account, len(groups))
	for _, ag := range groups {
		if ag.Edges.Account == nil {
			continue
		}
		if _, exists := accountMap[ag.AccountID]; exists {
			continue
		}
		accountMap[ag.AccountID] = ag.Edges.Account
		orderedIDs = append(orderedIDs, ag.AccountID)
	}

	accounts := make([]*dbent.Account, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if acc, ok := accountMap[id]; ok {
			accounts = append(accounts, acc)
		}
	}

	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) accountsToService(ctx context.Context, accounts []*dbent.Account) ([]service.Account, error) {
	if len(accounts) == 0 {
		return []service.Account{}, nil
	}

	accountIDs := make([]int64, 0, len(accounts))
	proxyIDs := make([]int64, 0, len(accounts))
	for _, acc := range accounts {
		accountIDs = append(accountIDs, acc.ID)
		if acc.ProxyID != nil {
			proxyIDs = append(proxyIDs, *acc.ProxyID)
		}
		if acc.ProxyFallbackOriginID != nil {
			proxyIDs = append(proxyIDs, *acc.ProxyFallbackOriginID)
		}
	}

	proxyMap, err := r.loadProxies(ctx, proxyIDs)
	if err != nil {
		return nil, err
	}
	groupsByAccount, groupIDsByAccount, accountGroupsByAccount, err := r.loadAccountGroups(ctx, accountIDs)
	if err != nil {
		return nil, err
	}

	outAccounts := make([]service.Account, 0, len(accounts))
	for _, acc := range accounts {
		out := accountEntityToService(acc)
		if out == nil {
			continue
		}
		if acc.ProxyID != nil {
			if proxy, ok := proxyMap[*acc.ProxyID]; ok {
				out.Proxy = proxy
			}
		}
		out.ProxyFallbackOriginID = acc.ProxyFallbackOriginID
		if acc.ProxyFallbackOriginID != nil {
			if op, ok := proxyMap[*acc.ProxyFallbackOriginID]; ok && op != nil {
				n := op.Name
				out.ProxyFallbackOriginName = &n
			}
		}
		if groups, ok := groupsByAccount[acc.ID]; ok {
			out.Groups = groups
		}
		if groupIDs, ok := groupIDsByAccount[acc.ID]; ok {
			out.GroupIDs = groupIDs
		}
		if ags, ok := accountGroupsByAccount[acc.ID]; ok {
			out.AccountGroups = ags
		}
		outAccounts = append(outAccounts, *out)
	}

	return outAccounts, nil
}

func tempUnschedulablePredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		col := s.C("temp_unschedulable_until")
		s.Where(entsql.Or(
			entsql.IsNull(col),
			entsql.LTE(col, entsql.Expr("NOW()")),
		))
	})
}

func notExpiredPredicate(now time.Time) dbpredicate.Account {
	return dbaccount.Or(
		dbaccount.ExpiresAtIsNil(),
		dbaccount.ExpiresAtGT(now),
		dbaccount.AutoPauseOnExpiredEQ(false),
	)
}

func (r *accountRepository) loadProxies(ctx context.Context, proxyIDs []int64) (map[int64]*service.Proxy, error) {
	proxyMap := make(map[int64]*service.Proxy)
	proxyIDs = uniquePositiveInt64s(proxyIDs)
	if len(proxyIDs) == 0 {
		return proxyMap, nil
	}

	for start := 0; start < len(proxyIDs); start += queryParameterBatchSize {
		end := start + queryParameterBatchSize
		if end > len(proxyIDs) {
			end = len(proxyIDs)
		}
		proxies, err := r.client.Proxy.Query().Where(dbproxy.IDIn(proxyIDs[start:end]...)).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range proxies {
			proxyMap[p.ID] = proxyEntityToService(p)
		}
	}
	return proxyMap, nil
}

func (r *accountRepository) loadAccountGroups(ctx context.Context, accountIDs []int64) (map[int64][]*service.Group, map[int64][]int64, map[int64][]service.AccountGroup, error) {
	groupsByAccount := make(map[int64][]*service.Group)
	groupIDsByAccount := make(map[int64][]int64)
	accountGroupsByAccount := make(map[int64][]service.AccountGroup)

	accountIDs = uniquePositiveInt64s(accountIDs)
	if len(accountIDs) == 0 {
		return groupsByAccount, groupIDsByAccount, accountGroupsByAccount, nil
	}

	for start := 0; start < len(accountIDs); start += queryParameterBatchSize {
		end := start + queryParameterBatchSize
		if end > len(accountIDs) {
			end = len(accountIDs)
		}
		entries, err := r.client.AccountGroup.Query().
			Where(dbaccountgroup.AccountIDIn(accountIDs[start:end]...)).
			Order(dbaccountgroup.ByAccountID(), dbaccountgroup.ByPriority()).
			All(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		groupIDs := make([]int64, 0, len(entries))
		for _, ag := range entries {
			groupIDs = append(groupIDs, ag.GroupID)
		}
		groupMap, err := r.loadGroups(ctx, groupIDs)
		if err != nil {
			return nil, nil, nil, err
		}

		for _, ag := range entries {
			groupSvc := groupMap[ag.GroupID]
			agSvc := service.AccountGroup{
				AccountID: ag.AccountID,
				GroupID:   ag.GroupID,
				Priority:  ag.Priority,
				CreatedAt: ag.CreatedAt,
				Group:     groupSvc,
			}
			accountGroupsByAccount[ag.AccountID] = append(accountGroupsByAccount[ag.AccountID], agSvc)
			groupIDsByAccount[ag.AccountID] = append(groupIDsByAccount[ag.AccountID], ag.GroupID)
			if groupSvc != nil {
				groupsByAccount[ag.AccountID] = append(groupsByAccount[ag.AccountID], groupSvc)
			}
		}
	}

	return groupsByAccount, groupIDsByAccount, accountGroupsByAccount, nil
}

func (r *accountRepository) loadGroups(ctx context.Context, groupIDs []int64) (map[int64]*service.Group, error) {
	groupMap := make(map[int64]*service.Group)
	groupIDs = uniquePositiveInt64s(groupIDs)
	if len(groupIDs) == 0 {
		return groupMap, nil
	}

	for start := 0; start < len(groupIDs); start += queryParameterBatchSize {
		end := start + queryParameterBatchSize
		if end > len(groupIDs) {
			end = len(groupIDs)
		}
		groups, err := r.client.Group.Query().Where(dbgroup.IDIn(groupIDs[start:end]...)).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, g := range groups {
			groupMap[g.ID] = groupEntityToService(g)
		}
	}
	return groupMap, nil
}

func uniquePositiveInt64s(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (r *accountRepository) loadAccountGroupIDs(ctx context.Context, accountID int64) ([]int64, error) {
	entries, err := r.client.AccountGroup.
		Query().
		Where(dbaccountgroup.AccountIDEQ(accountID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.GroupID)
	}
	return ids, nil
}

func mergeGroupIDs(a []int64, b []int64) []int64 {
	seen := make(map[int64]struct{}, len(a)+len(b))
	out := make([]int64, 0, len(a)+len(b))
	for _, id := range a {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range b {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// buildSchedulerGroupPayload 构造 EventAccountChanged / EventAccountGroupsChanged
// 事件的 payload。空 groupIDs 必须返回 untyped nil（any 而非 map[string]any(nil)），
// 否则 enqueueSchedulerOutbox 的 "payload != nil" 接口判空会被 typed-nil 欺骗，
// 把 payload marshal 成 "null" 写入 dedup_key 哈希，破坏与其他 nil-payload 调用的去重一致性。
func buildSchedulerGroupPayload(groupIDs []int64) any {
	if len(groupIDs) == 0 {
		return nil
	}
	return map[string]any{"group_ids": groupIDs}
}

func accountEntityToService(m *dbent.Account) *service.Account {
	if m == nil {
		return nil
	}

	rateMultiplier := m.RateMultiplier

	return &service.Account{
		ID:                      m.ID,
		Name:                    m.Name,
		Notes:                   m.Notes,
		Platform:                m.Platform,
		Type:                    m.Type,
		Credentials:             copyJSONMap(m.Credentials),
		Extra:                   copyJSONMap(m.Extra),
		Tags:                    copyStringSlice(m.Tags),
		ProxyID:                 m.ProxyID,
		ProxyFallbackOriginID:   m.ProxyFallbackOriginID,
		Concurrency:             m.Concurrency,
		Priority:                m.Priority,
		RateMultiplier:          &rateMultiplier,
		LoadFactor:              m.LoadFactor,
		Status:                  m.Status,
		ErrorMessage:            derefString(m.ErrorMessage),
		LastUsedAt:              m.LastUsedAt,
		ExpiresAt:               m.ExpiresAt,
		AutoPauseOnExpired:      m.AutoPauseOnExpired,
		CreatedAt:               m.CreatedAt,
		UpdatedAt:               m.UpdatedAt,
		Schedulable:             m.Schedulable,
		RateLimitedAt:           m.RateLimitedAt,
		RateLimitResetAt:        m.RateLimitResetAt,
		OverloadUntil:           m.OverloadUntil,
		TempUnschedulableUntil:  m.TempUnschedulableUntil,
		TempUnschedulableReason: derefString(m.TempUnschedulableReason),
		SessionWindowStart:      m.SessionWindowStart,
		SessionWindowEnd:        m.SessionWindowEnd,
		SessionWindowStatus:     derefString(m.SessionWindowStatus),
		ParentAccountID:         m.ParentAccountID,
		QuotaDimension:          string(m.QuotaDimension),
	}
}

func normalizeJSONMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}

// normalizeJSONStringSlice 把可能为 nil 的字符串数组兜底成空切片，
// 用于写入 JSON 字段时保证落库为 [] 而不是 NULL。
func normalizeJSONStringSlice(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func copyJSONMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// copyStringSlice 复制字符串切片，避免 ent 内部切片被外部修改污染缓存。
// nil 输入返回 nil（保留 nil 语义供上层做空数组兜底）。
func copyStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func joinClauses(clauses []string, sep string) string {
	if len(clauses) == 0 {
		return ""
	}
	out := clauses[0]
	for i := 1; i < len(clauses); i++ {
		out += sep + clauses[i]
	}
	return out
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

// FindByExtraField 根据 extra 字段中的键值对查找账号。
// 通过 Ent sqljson 生成 MySQL JSON 查询条件。
//
// FindByExtraField finds accounts by key-value pairs in the extra field.
// It uses Ent sqljson predicates so the active MySQL path emits JSON_EXTRACT-based SQL.
func (r *accountRepository) FindByExtraField(ctx context.Context, key string, value any) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.DeletedAtIsNil(),
			func(s *entsql.Selector) {
				path := sqljson.Path(key)
				switch v := value.(type) {
				case string:
					preds := []*entsql.Predicate{sqljson.ValueEQ(dbaccount.FieldExtra, v, path)}
					if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
						preds = append(preds, sqljson.ValueEQ(dbaccount.FieldExtra, parsed, path))
					}
					if len(preds) == 1 {
						s.Where(preds[0])
					} else {
						s.Where(entsql.Or(preds...))
					}
				case int:
					s.Where(entsql.Or(
						sqljson.ValueEQ(dbaccount.FieldExtra, v, path),
						sqljson.ValueEQ(dbaccount.FieldExtra, strconv.Itoa(v), path),
					))
				case int64:
					s.Where(entsql.Or(
						sqljson.ValueEQ(dbaccount.FieldExtra, v, path),
						sqljson.ValueEQ(dbaccount.FieldExtra, strconv.FormatInt(v, 10), path),
					))
				case json.Number:
					if parsed, err := v.Int64(); err == nil {
						s.Where(entsql.Or(
							sqljson.ValueEQ(dbaccount.FieldExtra, parsed, path),
							sqljson.ValueEQ(dbaccount.FieldExtra, v.String(), path),
						))
					} else {
						s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, v.String(), path))
					}
				default:
					s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, value, path))
				}
			},
		).
		All(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	return r.accountsToService(ctx, accounts)
}

// IncrementQuotaUsed 原子递增账号的配额用量（总/日/周三个维度）
// 日/周额度在周期过期时自动重置为 0 再递增。
// 支持滚动窗口（rolling）和固定时间（fixed）两种重置模式。
func (r *accountRepository) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	var extraRaw sql.NullString
	if err := scanSingleRow(ctx, r.sql, "SELECT extra FROM accounts WHERE id = ? AND deleted_at IS NULL", []any{id}, &extraRaw); err != nil {
		if err == sql.ErrNoRows {
			return service.ErrAccountNotFound
		}
		return err
	}
	extra := make(map[string]any)
	if extraRaw.Valid && strings.TrimSpace(extraRaw.String) != "" {
		if err := json.Unmarshal([]byte(extraRaw.String), &extra); err != nil {
			return err
		}
	}
	limit := accountExtraFloat(extra["quota_limit"])
	newUsed := accountExtraFloat(extra["quota_used"]) + amount
	extra["quota_used"] = newUsed
	applyAccountPeriodQuota(extra, "daily", 24*time.Hour, amount, time.Now().UTC())
	applyAccountPeriodQuota(extra, "weekly", 7*24*time.Hour, amount, time.Now().UTC())
	payload, err := json.Marshal(extra)
	if err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "UPDATE accounts SET extra = CAST(? AS JSON), updated_at = NOW() WHERE id = ? AND deleted_at IS NULL", string(payload), id); err != nil {
		return err
	}

	// 任一维度配额刚超限时触发调度快照刷新
	if limit > 0 && newUsed >= limit && (newUsed-amount) < limit {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
			logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue quota exceeded failed: account=%d err=%v", id, err)
		}
	}
	return nil
}

// ResetQuotaUsed 重置账号所有维度的配额用量为 0
// 保留固定重置模式的配置字段（quota_daily_reset_mode 等），仅清零用量和窗口起始时间
func (r *accountRepository) ResetQuotaUsed(ctx context.Context, id int64) error {
	var extraRaw sql.NullString
	if err := scanSingleRow(ctx, r.sql, "SELECT extra FROM accounts WHERE id = ? AND deleted_at IS NULL", []any{id}, &extraRaw); err != nil {
		if err == sql.ErrNoRows {
			return service.ErrAccountNotFound
		}
		return err
	}
	extra := make(map[string]any)
	if extraRaw.Valid && strings.TrimSpace(extraRaw.String) != "" {
		if err := json.Unmarshal([]byte(extraRaw.String), &extra); err != nil {
			return err
		}
	}
	extra["quota_used"] = 0
	extra["quota_daily_used"] = 0
	extra["quota_weekly_used"] = 0
	delete(extra, "quota_daily_start")
	delete(extra, "quota_weekly_start")
	delete(extra, "quota_daily_reset_at")
	delete(extra, "quota_weekly_reset_at")
	payload, err := json.Marshal(extra)
	if err != nil {
		return err
	}
	_, err = r.sql.ExecContext(ctx, "UPDATE accounts SET extra = CAST(? AS JSON), updated_at = NOW() WHERE id = ? AND deleted_at IS NULL", string(payload), id)
	if err != nil {
		return err
	}
	// 重置配额后触发调度快照刷新，使账号重新参与调度
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue quota reset failed: account=%d err=%v", id, err)
	}
	return nil
}

func accountExtraFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f
	default:
		return 0
	}
}

func accountExtraTime(extra map[string]any, key string) time.Time {
	s, _ := extra[key].(string)
	if strings.TrimSpace(s) == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func applyAccountPeriodQuota(extra map[string]any, period string, duration time.Duration, amount float64, now time.Time) {
	limit := accountExtraFloat(extra["quota_"+period+"_limit"])
	if limit <= 0 {
		return
	}
	usedKey := "quota_" + period + "_used"
	startKey := "quota_" + period + "_start"
	resetKey := "quota_" + period + "_reset_at"
	mode, _ := extra["quota_"+period+"_reset_mode"].(string)
	start := accountExtraTime(extra, startKey)
	resetAt := accountExtraTime(extra, resetKey)
	expired := false
	if mode == "fixed" {
		expired = !resetAt.IsZero() && !resetAt.After(now)
	} else {
		expired = start.IsZero() || !start.Add(duration).After(now)
	}
	if expired {
		extra[usedKey] = amount
		extra[startKey] = now.Format(time.RFC3339Nano)
	} else {
		extra[usedKey] = accountExtraFloat(extra[usedKey]) + amount
	}
}

// RevertProxyFallback 将账号的 proxy_id 切回 proxy_fallback_origin_id，并清空 origin 字段。
// 仅当 proxy_fallback_origin_id IS NOT NULL 时执行更新；
// 若影响行数为 0，则返回 ErrAccountNotInFallback（账号存在但不在 fallback 状态）。
func (r *accountRepository) RevertProxyFallback(ctx context.Context, accountID int64) error {
	res, err := r.sql.ExecContext(ctx, `
		UPDATE accounts SET proxy_id=proxy_fallback_origin_id, proxy_fallback_origin_id=NULL, updated_at=NOW()
		WHERE id = ? AND proxy_fallback_origin_id IS NOT NULL AND deleted_at IS NULL`, accountID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return service.ErrAccountNotInFallback
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] revert fallback enqueue failed: account=%d err=%v", accountID, err)
	}
	return nil
}

// ListShadowsByParent 返回指定父账号的影子账号；当前实现仅查 quota_dimension='spark'（唯一预设）。
// 同时过滤 parent_account_id 和 quota_dimension='spark'，防止未来其它 linked 维度被误伤。
// ⚠️ 新增影子维度时：须更新此函数（或新增维度专用列举），并检查所有调用点（级联删除/一母一影校验/type 守卫），否则会静默漏掉新维度。
// 软删除行由 SoftDeleteMixin 拦截器自动排除，无需手写 deleted_at IS NULL。
func (r *accountRepository) ListShadowsByParent(ctx context.Context, parentID int64) ([]*service.Account, error) {
	rows, err := r.client.Account.Query().
		Where(dbaccount.ParentAccountIDEQ(parentID), dbaccount.QuotaDimensionEQ(dbaccount.QuotaDimensionSpark)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.Account, 0, len(rows))
	for _, m := range rows {
		out = append(out, accountEntityToService(m))
	}
	return out, nil
}
