package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayServiceGenerateSessionHashScopesContentFallbackByAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(77)
	body := []byte(`{"model":"gpt-5.4","instructions":"You are helpful","input":"Hello"}`)
	makeContext := func(apiKeyID int64) *gin.Context {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Set("api_key", &APIKey{ID: apiKeyID, GroupID: &groupID})
		return c
	}

	svc := &OpenAIGatewayService{}
	first := svc.GenerateSessionHash(makeContext(101), body)
	second := svc.GenerateSessionHash(makeContext(202), body)
	firstAgain := svc.GenerateSessionHash(makeContext(101), body)

	require.NotEmpty(t, first)
	require.NotEqual(t, first, second, "different API keys in the same group must not share content-derived sticky affinity")
	require.Equal(t, first, firstAgain, "the same API key and request must keep stable affinity")
}

func TestOpenAIGatewayServiceLegacyBurstDistributesAcrossEqualPriorityLoadPeers(t *testing.T) {
	now := time.Now()
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 100, Priority: 1, LastUsedAt: fairnessTimePtr(now.Add(-3 * time.Hour))},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 100, Priority: 1, LastUsedAt: fairnessTimePtr(now.Add(-2 * time.Hour))},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 100, Priority: 1, LastUsedAt: fairnessTimePtr(now.Add(-1 * time.Hour))},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	loads := map[int64]*AccountLoadInfo{
		1: {AccountID: 1, LoadRate: 0},
		2: {AccountID: 2, LoadRate: 0},
		3: {AccountID: 3, LoadRate: 0},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{loadMap: loads}),
	}

	selected := map[int64]int{}
	for i := 0; i < 300; i++ {
		selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), nil, "", "gpt-5.1", nil)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.NotNil(t, selection.Account)
		selected[selection.Account.ID]++
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	}

	require.Len(t, selected, len(accounts), "equal priority/load peers must all receive burst traffic even when LastUsedAt differs")
}

type changingOpenAILoadCache struct {
	ConcurrencyCache
	mu    sync.Mutex
	calls int
}

func (c *changingOpenAILoadCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}

func (c *changingOpenAILoadCache) ReleaseAccountSlot(context.Context, int64, string) error {
	return nil
}

func (c *changingOpenAILoadCache) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls == 1 {
		return map[int64]*AccountLoadInfo{
			21: {AccountID: 21, LoadRate: 0},
			22: {AccountID: 22, LoadRate: 90},
		}, nil
	}
	return map[int64]*AccountLoadInfo{
		21: {AccountID: 21, LoadRate: 90},
		22: {AccountID: 22, LoadRate: 0},
	}, nil
}

func TestOpenAILegacySelectionBypassesStaleBatchLoadCache(t *testing.T) {
	accounts := []Account{
		{ID: 21, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10, Priority: 1},
		{ID: 22, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10, Priority: 1},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cache := &changingOpenAILoadCache{}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(cache),
	}

	first, err := svc.SelectAccountWithLoadAwareness(context.Background(), nil, "", "gpt-5.1", nil)
	require.NoError(t, err)
	require.Equal(t, int64(21), first.Account.ID)
	first.ReleaseFunc()

	second, err := svc.SelectAccountWithLoadAwareness(context.Background(), nil, "", "gpt-5.1", nil)
	require.NoError(t, err)
	require.Equal(t, int64(22), second.Account.ID)
	second.ReleaseFunc()
	require.GreaterOrEqual(t, cache.calls, 2)
}

func TestOpenAILegacyAllFullWaitPlanContainsMovableCandidatePool(t *testing.T) {
	accounts := []Account{
		{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.Scheduling.FallbackWaitTimeout = time.Second
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 10
	cache := schedulerTestConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			11: {AccountID: 11, CurrentConcurrency: 1, LoadRate: 100},
			12: {AccountID: 12, CurrentConcurrency: 1, LoadRate: 100},
		},
		acquireResults: map[int64]bool{11: false, 12: false},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(cache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), nil, "", "gpt-5.1", nil)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.WaitPlan)
	require.Len(t, selection.WaitPlan.Candidates, 2)
}

type staleOpenAIWaitCandidateRepo struct {
	schedulerTestOpenAIAccountRepo
	staleID int64
}

func (r staleOpenAIWaitCandidateRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	account, err := r.schedulerTestOpenAIAccountRepo.GetByID(ctx, id)
	if err != nil || account == nil || id != r.staleID {
		return account, err
	}
	stale := *account
	stale.Status = StatusDisabled
	stale.Schedulable = false
	return &stale, nil
}

func TestOpenAILegacyWaitPlanExcludesCandidateRejectedByDBRecheck(t *testing.T) {
	accounts := []Account{
		{ID: 31, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		{ID: 32, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.Scheduling.FallbackWaitTimeout = time.Second
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 10
	cache := schedulerTestConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			31: {AccountID: 31, CurrentConcurrency: 1, LoadRate: 100},
			32: {AccountID: 32, CurrentConcurrency: 1, LoadRate: 100},
		},
		acquireResults: map[int64]bool{31: false, 32: false},
	}
	svc := &OpenAIGatewayService{
		accountRepo: staleOpenAIWaitCandidateRepo{
			schedulerTestOpenAIAccountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
			staleID:                        32,
		},
		schedulerSnapshot: &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{
			snapshotAccounts: []*Account{&accounts[0], &accounts[1]},
			accountsByID: map[int64]*Account{
				31: &accounts[0],
				32: &accounts[1],
			},
		}},
		cache:              &schedulerTestGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(cache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), nil, "", "gpt-5.1", nil)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.WaitPlan)
	require.Len(t, selection.WaitPlan.Candidates, 1)
	require.Equal(t, int64(31), selection.WaitPlan.Candidates[0].Account.ID)
}

func TestOpenAIAdvancedSchedulerEqualScoreTopKDoesNotStarveTailAccounts(t *testing.T) {
	accounts := make([]Account, 0, 10)
	loads := make(map[int64]*AccountLoadInfo, 10)
	for id := int64(1); id <= 10; id++ {
		accounts = append(accounts, Account{
			ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 100, Priority: 1,
		})
		loads[id] = &AccountLoadInfo{AccountID: id, LoadRate: 0, WaitingCount: 0}
	}

	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 7
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 1
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{loadMap: loads}),
	}

	groupID := int64(1)
	selected := map[int64]int{}
	for i := 0; i < 2000; i++ {
		selection, _, err := svc.SelectAccountWithScheduler(
			context.Background(), &groupID, "", fmt.Sprintf("fairness-session-%d", i),
			"gpt-5.1", nil, OpenAIUpstreamTransportAny, false,
		)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.NotNil(t, selection.Account)
		selected[selection.Account.ID]++
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	}

	require.Len(t, selected, len(accounts), "Top-K must be sampled fairly instead of permanently excluding larger account IDs")
	for id := int64(1); id <= 10; id++ {
		require.Positive(t, selected[id], "account %d was starved", id)
	}
}

func fairnessTimePtr(value time.Time) *time.Time { return &value }
