package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// usageLogInsertArgTypes must stay in the same order as:
//  1. prepareUsageLogInsert().args
//  2. every INSERT/CTE VALUES column list in this file
//  3. execUsageLogInsertNoResult placeholder positions
//  4. scanUsageLog selected column order (via usageLogSelectColumns)
//
// When adding a usage_logs column, update all of those call sites together.
var usageLogInsertArgTypes = [...]string{
	"bigint",      // user_id
	"bigint",      // api_key_id
	"bigint",      // account_id
	"text",        // request_id
	"text",        // model
	"text",        // requested_model
	"text",        // upstream_model
	"text",        // upstream_response_model
	"boolean",     // upstream_model_mismatch
	"bigint",      // group_id
	"bigint",      // subscription_id
	"integer",     // input_tokens
	"integer",     // output_tokens
	"integer",     // cache_creation_tokens
	"integer",     // cache_read_tokens
	"integer",     // cache_creation_5m_tokens
	"integer",     // cache_creation_1h_tokens
	"integer",     // image_output_tokens
	"numeric",     // image_output_cost
	"integer",     // image_input_tokens
	"numeric",     // image_input_cost
	"numeric",     // input_cost
	"numeric",     // output_cost
	"numeric",     // cache_creation_cost
	"numeric",     // cache_read_cost
	"numeric",     // total_cost
	"numeric",     // actual_cost
	"numeric",     // rate_multiplier
	"numeric",     // account_rate_multiplier
	"smallint",    // billing_type
	"smallint",    // request_type
	"boolean",     // stream
	"boolean",     // openai_ws_mode
	"integer",     // duration_ms
	"integer",     // first_token_ms
	"integer",     // upstream_first_event_ms
	"text",        // user_agent
	"text",        // ip_address
	"integer",     // image_count
	"text",        // image_size
	"text",        // image_input_size
	"text",        // image_output_size
	"text",        // image_size_source
	"json",        // image_size_breakdown
	"text",        // service_tier
	"text",        // reasoning_effort
	"text",        // inbound_endpoint
	"text",        // upstream_endpoint
	"boolean",     // cache_ttl_overridden
	"boolean",     // long_context_billing_applied
	"bigint",      // channel_id
	"text",        // model_mapping_chain
	"text",        // billing_tier
	"text",        // billing_mode
	"numeric",     // account_stats_cost
	"text",        // session_id
	"datetime(6)", // created_at
}

const (
	usageLogCreateBatchMaxSize  = 64
	usageLogCreateBatchWindow   = 3 * time.Millisecond
	usageLogCreateBatchQueueCap = 4096
	usageLogCreateCancelWait    = 2 * time.Second

	usageLogBestEffortBatchMaxSize  = 256
	usageLogBestEffortBatchWindow   = 20 * time.Millisecond
	usageLogBestEffortBatchQueueCap = 32768
	usageLogBestEffortRecentTTL     = 30 * time.Second
)

type usageLogCreateRequest struct {
	log      *service.UsageLog
	prepared usageLogInsertPrepared
	shared   *usageLogCreateShared
	resultCh chan usageLogCreateResult
}

type usageLogCreateResult struct {
	inserted bool
	err      error
}

type usageLogBestEffortRequest struct {
	prepared usageLogInsertPrepared
	apiKeyID int64
	resultCh chan error
}

type usageLogInsertPrepared struct {
	createdAt      time.Time
	apiKeyID       int64
	requestID      string
	rateMultiplier float64
	requestType    int16
	args           []any
}

type usageLogBatchState struct {
	ID        int64
	CreatedAt time.Time
}

type usageLogBatchRow struct {
	RequestID string    `json:"request_id"`
	APIKeyID  int64     `json:"api_key_id"`
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Inserted  bool      `json:"inserted"`
}

type usageLogCreateShared struct {
	state atomic.Int32
}

const (
	usageLogCreateStateQueued int32 = iota
	usageLogCreateStateProcessing
	usageLogCreateStateCompleted
	usageLogCreateStateCanceled
)

func (r *usageLogRepository) Create(ctx context.Context, log *service.UsageLog) (bool, error) {
	if log == nil {
		return false, nil
	}

	if tx := dbent.TxFromContext(ctx); tx != nil {
		return r.createSingle(ctx, tx.Client(), log)
	}
	requestID := strings.TrimSpace(log.RequestID)
	if requestID == "" {
		return r.createSingle(ctx, r.sql, log)
	}
	log.RequestID = requestID
	if r.isMySQLDialect() {
		return r.createSingle(ctx, r.sql, log)
	}
	return r.createBatched(ctx, log)
}

func (r *usageLogRepository) CreateBestEffort(ctx context.Context, log *service.UsageLog) error {
	if log == nil {
		return nil
	}

	if tx := dbent.TxFromContext(ctx); tx != nil {
		_, err := r.createSingle(ctx, tx.Client(), log)
		return err
	}
	if r.db == nil {
		_, err := r.createSingle(ctx, r.sql, log)
		return err
	}
	if r.isMySQLDialect() {
		_, err := r.createSingle(ctx, r.sql, log)
		return err
	}

	r.ensureBestEffortBatcher()
	if r.bestEffortBatchCh == nil {
		_, err := r.createSingle(ctx, r.sql, log)
		return err
	}

	req := usageLogBestEffortRequest{
		prepared: prepareUsageLogInsert(log),
		apiKeyID: log.APIKeyID,
		resultCh: make(chan error, 1),
	}
	if key, ok := r.bestEffortRecentKey(req.prepared.requestID, req.apiKeyID); ok {
		if _, exists := r.bestEffortRecent.Get(key); exists {
			return nil
		}
	}

	select {
	case r.bestEffortBatchCh <- req:
	case <-ctx.Done():
		return service.MarkUsageLogCreateDropped(ctx.Err())
	default:
		return service.MarkUsageLogCreateDropped(errors.New("usage log best-effort queue full"))
	}

	select {
	case err := <-req.resultCh:
		return err
	case <-ctx.Done():
		return service.MarkUsageLogCreateDropped(ctx.Err())
	}
}

func (r *usageLogRepository) createSingle(ctx context.Context, sqlq sqlExecutor, log *service.UsageLog) (bool, error) {
	prepared := prepareUsageLogInsert(log)
	if sqlq == nil {
		sqlq = r.sql
	}
	if ctx != nil && ctx.Err() != nil {
		return false, service.MarkUsageLogCreateNotPersisted(ctx.Err())
	}

	inserted, state, err := execUsageLogInsertAndLoadState(ctx, sqlq, prepared)
	if err != nil {
		return false, err
	}
	log.ID = state.ID
	log.CreatedAt = state.CreatedAt
	log.RateMultiplier = prepared.rateMultiplier
	return inserted, nil
}

func (r *usageLogRepository) createBatched(ctx context.Context, log *service.UsageLog) (bool, error) {
	if r.db == nil {
		return r.createSingle(ctx, r.sql, log)
	}
	r.ensureCreateBatcher()
	if r.createBatchCh == nil {
		return r.createSingle(ctx, r.sql, log)
	}

	req := usageLogCreateRequest{
		log:      log,
		prepared: prepareUsageLogInsert(log),
		shared:   &usageLogCreateShared{},
		resultCh: make(chan usageLogCreateResult, 1),
	}

	select {
	case r.createBatchCh <- req:
	case <-ctx.Done():
		return false, service.MarkUsageLogCreateNotPersisted(ctx.Err())
	default:
		return false, service.MarkUsageLogCreateNotPersisted(errors.New("usage log create batch queue full"))
	}

	select {
	case res := <-req.resultCh:
		return res.inserted, res.err
	case <-ctx.Done():
		if req.shared != nil && req.shared.state.CompareAndSwap(usageLogCreateStateQueued, usageLogCreateStateCanceled) {
			return false, service.MarkUsageLogCreateNotPersisted(ctx.Err())
		}
		timer := time.NewTimer(usageLogCreateCancelWait)
		defer timer.Stop()
		select {
		case res := <-req.resultCh:
			return res.inserted, res.err
		case <-timer.C:
			return false, ctx.Err()
		}
	}
}

func (r *usageLogRepository) ensureCreateBatcher() {
	if r == nil || r.db == nil || r.createBatchCh != nil {
		return
	}
	r.createBatchOnce.Do(func() {
		r.createBatchCh = make(chan usageLogCreateRequest, usageLogCreateBatchQueueCap)
		go r.runCreateBatcher(r.db)
	})
}

func (r *usageLogRepository) ensureBestEffortBatcher() {
	if r == nil || r.db == nil || r.bestEffortBatchCh != nil {
		return
	}
	r.bestEffortBatchOnce.Do(func() {
		r.bestEffortBatchCh = make(chan usageLogBestEffortRequest, usageLogBestEffortBatchQueueCap)
		go r.runBestEffortBatcher(r.db)
	})
}

func (r *usageLogRepository) runCreateBatcher(db *sql.DB) {
	for {
		first, ok := <-r.createBatchCh
		if !ok {
			return
		}

		batch := make([]usageLogCreateRequest, 0, usageLogCreateBatchMaxSize)
		batch = append(batch, first)

		timer := time.NewTimer(usageLogCreateBatchWindow)
	batchLoop:
		for len(batch) < usageLogCreateBatchMaxSize {
			select {
			case req, ok := <-r.createBatchCh:
				if !ok {
					break batchLoop
				}
				batch = append(batch, req)
			case <-timer.C:
				break batchLoop
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		r.flushCreateBatch(db, batch)
	}
}

func (r *usageLogRepository) runBestEffortBatcher(db *sql.DB) {
	for {
		first, ok := <-r.bestEffortBatchCh
		if !ok {
			return
		}

		batch := make([]usageLogBestEffortRequest, 0, usageLogBestEffortBatchMaxSize)
		batch = append(batch, first)

		timer := time.NewTimer(usageLogBestEffortBatchWindow)
	bestEffortLoop:
		for len(batch) < usageLogBestEffortBatchMaxSize {
			select {
			case req, ok := <-r.bestEffortBatchCh:
				if !ok {
					break bestEffortLoop
				}
				batch = append(batch, req)
			case <-timer.C:
				break bestEffortLoop
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		r.flushBestEffortBatch(db, batch)
	}
}

func (r *usageLogRepository) flushCreateBatch(db *sql.DB, batch []usageLogCreateRequest) {
	if len(batch) == 0 {
		return
	}

	uniqueOrder := make([]string, 0, len(batch))
	preparedByKey := make(map[string]usageLogInsertPrepared, len(batch))
	requestsByKey := make(map[string][]usageLogCreateRequest, len(batch))
	fallback := make([]usageLogCreateRequest, 0)

	for _, req := range batch {
		if req.log == nil {
			completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: false, err: nil})
			continue
		}
		if req.shared != nil && !req.shared.state.CompareAndSwap(usageLogCreateStateQueued, usageLogCreateStateProcessing) {
			if req.shared.state.Load() == usageLogCreateStateCanceled {
				completeUsageLogCreateRequest(req, usageLogCreateResult{
					inserted: false,
					err:      service.MarkUsageLogCreateNotPersisted(context.Canceled),
				})
				continue
			}
		}
		prepared := req.prepared
		if prepared.requestID == "" {
			fallback = append(fallback, req)
			continue
		}
		key := usageLogBatchKey(prepared.requestID, req.log.APIKeyID)
		if _, exists := requestsByKey[key]; !exists {
			uniqueOrder = append(uniqueOrder, key)
			preparedByKey[key] = prepared
		}
		requestsByKey[key] = append(requestsByKey[key], req)
	}

	if len(uniqueOrder) > 0 {
		insertedMap, stateMap, safeFallback, err := r.batchInsertUsageLogs(db, uniqueOrder, preparedByKey)
		if err != nil {
			if safeFallback {
				for _, key := range uniqueOrder {
					fallback = append(fallback, requestsByKey[key]...)
				}
			} else {
				for _, key := range uniqueOrder {
					reqs := requestsByKey[key]
					state, hasState := stateMap[key]
					inserted := insertedMap[key]
					for idx, req := range reqs {
						req.log.RateMultiplier = preparedByKey[key].rateMultiplier
						if hasState {
							req.log.ID = state.ID
							req.log.CreatedAt = state.CreatedAt
						}
						switch {
						case inserted && idx == 0:
							completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: true, err: nil})
						case inserted:
							completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: false, err: nil})
						case hasState:
							completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: false, err: nil})
						case idx == 0:
							completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: false, err: err})
						default:
							completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: false, err: nil})
						}
					}
				}
			}
		} else {
			for _, key := range uniqueOrder {
				reqs := requestsByKey[key]
				state, ok := stateMap[key]
				if !ok {
					for _, req := range reqs {
						completeUsageLogCreateRequest(req, usageLogCreateResult{
							inserted: false,
							err:      fmt.Errorf("usage log batch state missing for key=%s", key),
						})
					}
					continue
				}
				for idx, req := range reqs {
					req.log.ID = state.ID
					req.log.CreatedAt = state.CreatedAt
					req.log.RateMultiplier = preparedByKey[key].rateMultiplier
					completeUsageLogCreateRequest(req, usageLogCreateResult{
						inserted: idx == 0 && insertedMap[key],
						err:      nil,
					})
				}
			}
		}
	}

	if len(fallback) == 0 {
		return
	}

	for _, req := range fallback {
		fallbackCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		inserted, err := r.createSingle(fallbackCtx, db, req.log)
		cancel()
		completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: inserted, err: err})
	}
}

func (r *usageLogRepository) flushBestEffortBatch(db *sql.DB, batch []usageLogBestEffortRequest) {
	if len(batch) == 0 {
		return
	}

	type bestEffortGroup struct {
		prepared usageLogInsertPrepared
		apiKeyID int64
		key      string
		reqs     []usageLogBestEffortRequest
	}

	groupsByKey := make(map[string]*bestEffortGroup, len(batch))
	groupOrder := make([]*bestEffortGroup, 0, len(batch))
	preparedList := make([]usageLogInsertPrepared, 0, len(batch))

	for idx, req := range batch {
		prepared := req.prepared
		key := fmt.Sprintf("__best_effort_%d", idx)
		if prepared.requestID != "" {
			key = usageLogBatchKey(prepared.requestID, req.apiKeyID)
		}
		group, exists := groupsByKey[key]
		if !exists {
			group = &bestEffortGroup{
				prepared: prepared,
				apiKeyID: req.apiKeyID,
				key:      key,
			}
			groupsByKey[key] = group
			groupOrder = append(groupOrder, group)
			preparedList = append(preparedList, prepared)
		}
		group.reqs = append(group.reqs, req)
	}

	if len(preparedList) == 0 {
		for _, req := range batch {
			sendUsageLogBestEffortResult(req.resultCh, nil)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query, args := buildUsageLogBestEffortInsertQuery(preparedList)
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		logger.LegacyPrintf("repository.usage_log", "best-effort batch insert failed: %v", err)
		for _, group := range groupOrder {
			singleErr := execUsageLogInsertNoResult(ctx, db, group.prepared)
			if singleErr != nil {
				logger.LegacyPrintf("repository.usage_log", "best-effort single fallback insert failed: %v", singleErr)
			} else if group.prepared.requestID != "" && r != nil && r.bestEffortRecent != nil {
				r.bestEffortRecent.SetDefault(group.key, struct{}{})
			}
			for _, req := range group.reqs {
				sendUsageLogBestEffortResult(req.resultCh, singleErr)
			}
		}
		return
	}
	for _, group := range groupOrder {
		if group.prepared.requestID != "" && r != nil && r.bestEffortRecent != nil {
			r.bestEffortRecent.SetDefault(group.key, struct{}{})
		}
		for _, req := range group.reqs {
			sendUsageLogBestEffortResult(req.resultCh, nil)
		}
	}
}

func sendUsageLogBestEffortResult(ch chan error, err error) {
	if ch == nil {
		return
	}
	select {
	case ch <- err:
	default:
	}
}

func completeUsageLogCreateRequest(req usageLogCreateRequest, res usageLogCreateResult) {
	if req.shared != nil {
		req.shared.state.Store(usageLogCreateStateCompleted)
	}
	sendUsageLogCreateResult(req.resultCh, res)
}

func (r *usageLogRepository) batchInsertUsageLogs(db *sql.DB, keys []string, preparedByKey map[string]usageLogInsertPrepared) (map[string]bool, map[string]usageLogBatchState, bool, error) {
	if len(keys) == 0 {
		return map[string]bool{}, map[string]usageLogBatchState{}, false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	insertedMap := make(map[string]bool, len(keys))
	stateMap := make(map[string]usageLogBatchState, len(keys))
	for _, key := range keys {
		prepared, ok := preparedByKey[key]
		if !ok {
			return insertedMap, stateMap, false, fmt.Errorf("usage log batch prepared missing for key=%s", key)
		}
		inserted, state, err := execUsageLogInsertAndLoadState(ctx, db, prepared)
		if err != nil {
			return insertedMap, stateMap, true, err
		}
		insertedMap[key] = inserted
		stateMap[key] = state
	}
	return insertedMap, stateMap, false, nil
}

func buildUsageLogBestEffortInsertQuery(preparedList []usageLogInsertPrepared) (string, []any) {
	return buildUsageLogMultiInsertQuery(preparedList), flattenUsageLogInsertArgs(preparedList)
}

func execUsageLogInsertNoResult(ctx context.Context, sqlq sqlExecutor, prepared usageLogInsertPrepared) error {
	_, err := execUsageLogInsert(ctx, sqlq, prepared)
	return err
}

func execUsageLogInsert(ctx context.Context, sqlq sqlExecutor, prepared usageLogInsertPrepared) (sql.Result, error) {
	query := `
		INSERT INTO usage_logs (
			user_id,
			api_key_id,
			account_id,
			request_id,
			model,
			requested_model,
			upstream_model,
			upstream_response_model,
			upstream_model_mismatch,
			group_id,
			subscription_id,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			cache_creation_5m_tokens,
			cache_creation_1h_tokens,
			image_output_tokens,
			image_output_cost,
			image_input_tokens,
			image_input_cost,
			input_cost,
			output_cost,
			cache_creation_cost,
			cache_read_cost,
			total_cost,
			actual_cost,
			rate_multiplier,
			account_rate_multiplier,
			billing_type,
			request_type,
			stream,
			openai_ws_mode,
			duration_ms,
			first_token_ms,
			upstream_first_event_ms,
			user_agent,
			ip_address,
			image_count,
			image_size,
			image_input_size,
			image_output_size,
			image_size_source,
			image_size_breakdown,
			service_tier,
			reasoning_effort,
			inbound_endpoint,
			upstream_endpoint,
			cache_ttl_overridden,
			long_context_billing_applied,
			channel_id,
			model_mapping_chain,
			billing_tier,
			billing_mode,
			account_stats_cost,
			session_id,
			created_at
		) VALUES (` + strings.TrimSuffix(strings.Repeat("?,", len(prepared.args)), ",") + `)
		ON DUPLICATE KEY UPDATE id = id
	`
	return sqlq.ExecContext(ctx, query, prepared.args...)
}

func prepareUsageLogInsert(log *service.UsageLog) usageLogInsertPrepared {
	createdAt := log.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	requestID := strings.TrimSpace(log.RequestID)
	log.RequestID = requestID

	rateMultiplier := log.RateMultiplier
	log.SyncRequestTypeAndLegacyFields()
	requestType := int16(log.RequestType)

	groupID := nullInt64(log.GroupID)
	subscriptionID := nullInt64(log.SubscriptionID)
	duration := nullInt(log.DurationMs)
	firstToken := nullInt(log.FirstTokenMs)
	upstreamFirstEvent := nullInt(log.UpstreamFirstEventMs)
	userAgent := nullString(log.UserAgent)
	ipAddress := nullString(log.IPAddress)
	imageSize := nullString(log.ImageSize)
	imageInputSize := nullString(log.ImageInputSize)
	imageOutputSize := nullString(log.ImageOutputSize)
	imageSizeSource := nullString(log.ImageSizeSource)
	imageSizeBreakdown := nullStringIntMapJSON(log.ImageSizeBreakdown)
	serviceTier := nullString(log.ServiceTier)
	reasoningEffort := nullString(log.ReasoningEffort)
	inboundEndpoint := nullString(log.InboundEndpoint)
	upstreamEndpoint := nullString(log.UpstreamEndpoint)
	channelID := nullInt64(log.ChannelID)
	modelMappingChain := nullString(log.ModelMappingChain)
	billingTier := nullString(log.BillingTier)
	billingMode := nullString(log.BillingMode)
	sessionID := nullString(log.SessionID)
	requestedModel := strings.TrimSpace(log.RequestedModel)
	if requestedModel == "" {
		requestedModel = strings.TrimSpace(log.Model)
	}
	upstreamModel := nullString(log.UpstreamModel)
	upstreamResponseModel := nullString(log.UpstreamResponseModel)
	upstreamModelMismatch := nullBool(log.UpstreamModelMismatch)

	var requestIDArg any
	if requestID != "" {
		requestIDArg = requestID
	}

	return usageLogInsertPrepared{
		createdAt:      createdAt,
		apiKeyID:       log.APIKeyID,
		requestID:      requestID,
		rateMultiplier: rateMultiplier,
		requestType:    requestType,
		args: []any{
			log.UserID,
			log.APIKeyID,
			log.AccountID,
			requestIDArg,
			log.Model,
			nullString(&requestedModel),
			upstreamModel,
			upstreamResponseModel,
			upstreamModelMismatch,
			groupID,
			subscriptionID,
			log.InputTokens,
			log.OutputTokens,
			log.CacheCreationTokens,
			log.CacheReadTokens,
			log.CacheCreation5mTokens,
			log.CacheCreation1hTokens,
			log.ImageOutputTokens,
			log.ImageOutputCost,
			log.ImageInputTokens,
			log.ImageInputCost,
			log.InputCost,
			log.OutputCost,
			log.CacheCreationCost,
			log.CacheReadCost,
			log.TotalCost,
			log.ActualCost,
			rateMultiplier,
			log.AccountRateMultiplier,
			log.BillingType,
			requestType,
			log.Stream,
			log.OpenAIWSMode,
			duration,
			firstToken,
			upstreamFirstEvent,
			userAgent,
			ipAddress,
			log.ImageCount,
			imageSize,
			imageInputSize,
			imageOutputSize,
			imageSizeSource,
			imageSizeBreakdown,
			serviceTier,
			reasoningEffort,
			inboundEndpoint,
			upstreamEndpoint,
			log.CacheTTLOverridden,
			log.LongContextBillingApplied,
			channelID,
			modelMappingChain,
			billingTier,
			billingMode,
			log.AccountStatsCost, // account_stats_cost
			sessionID,            // session_id
			createdAt,
		},
	}
}

func buildUsageLogMultiInsertQuery(preparedList []usageLogInsertPrepared) string {
	var query strings.Builder
	_, _ = query.WriteString(`
		INSERT INTO usage_logs (
			user_id,
			api_key_id,
			account_id,
			request_id,
			model,
			requested_model,
			upstream_model,
			upstream_response_model,
			upstream_model_mismatch,
			group_id,
			subscription_id,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			cache_creation_5m_tokens,
			cache_creation_1h_tokens,
			image_output_tokens,
			image_output_cost,
			image_input_tokens,
			image_input_cost,
			input_cost,
			output_cost,
			cache_creation_cost,
			cache_read_cost,
			total_cost,
			actual_cost,
			rate_multiplier,
			account_rate_multiplier,
			billing_type,
			request_type,
			stream,
			openai_ws_mode,
			duration_ms,
			first_token_ms,
			upstream_first_event_ms,
			user_agent,
			ip_address,
			image_count,
			image_size,
			image_input_size,
			image_output_size,
			image_size_source,
			image_size_breakdown,
			service_tier,
			reasoning_effort,
			inbound_endpoint,
			upstream_endpoint,
			cache_ttl_overridden,
			long_context_billing_applied,
			channel_id,
			model_mapping_chain,
			billing_tier,
			billing_mode,
			account_stats_cost,
			session_id,
			created_at
		) VALUES
	`)
	for idx, prepared := range preparedList {
		if idx > 0 {
			_, _ = query.WriteString(",\n")
		}
		_, _ = query.WriteString("(")
		for i := 0; i < len(prepared.args); i++ {
			if i > 0 {
				_, _ = query.WriteString(", ")
			}
			_, _ = query.WriteString("?")
		}
		_, _ = query.WriteString(")")
	}
	_, _ = query.WriteString(`
		ON DUPLICATE KEY UPDATE id = id
	`)
	return query.String()
}

func flattenUsageLogInsertArgs(preparedList []usageLogInsertPrepared) []any {
	args := make([]any, 0, len(preparedList)*len(usageLogInsertArgTypes))
	for _, prepared := range preparedList {
		args = append(args, prepared.args...)
	}
	return args
}

func execUsageLogInsertAndLoadState(ctx context.Context, sqlq sqlExecutor, prepared usageLogInsertPrepared) (bool, usageLogBatchState, error) {
	res, err := execUsageLogInsert(ctx, sqlq, prepared)
	if err != nil {
		return false, usageLogBatchState{}, err
	}

	inserted := false
	if affected, rowsErr := res.RowsAffected(); rowsErr == nil {
		inserted = affected > 0
	}

	var state usageLogBatchState
	if prepared.requestID != "" {
		query := "SELECT id, created_at FROM usage_logs WHERE request_id = ? AND api_key_id = ? ORDER BY id DESC LIMIT 1"
		if err := scanSingleRow(ctx, sqlq, query, []any{prepared.requestID, prepared.apiKeyID}, &state.ID, &state.CreatedAt); err != nil {
			return false, usageLogBatchState{}, err
		}
		return inserted, state, nil
	}

	lastInsertID, err := res.LastInsertId()
	if err != nil {
		return false, usageLogBatchState{}, err
	}
	if err := scanSingleRow(ctx, sqlq, "SELECT id, created_at FROM usage_logs WHERE id = ?", []any{lastInsertID}, &state.ID, &state.CreatedAt); err != nil {
		return false, usageLogBatchState{}, err
	}
	return inserted, state, nil
}

func usageLogBatchKey(requestID string, apiKeyID int64) string {
	return requestID + "\x1f" + strconv.FormatInt(apiKeyID, 10)
}

func sendUsageLogCreateResult(ch chan usageLogCreateResult, res usageLogCreateResult) {
	if ch == nil {
		return
	}
	select {
	case ch <- res:
	default:
	}
}

func (r *usageLogRepository) bestEffortRecentKey(requestID string, apiKeyID int64) (string, bool) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || r == nil || r.bestEffortRecent == nil {
		return "", false
	}
	return usageLogBatchKey(requestID, apiKeyID), true
}
