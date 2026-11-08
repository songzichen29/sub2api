package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestUserGroupConcurrencyAtomicAcrossClients(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewConcurrencyCache(client, 15, 900)
	svc := service.NewConcurrencyService(cache)
	secondClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = secondClient.Close() })
	secondService := service.NewConcurrencyService(NewConcurrencyCache(secondClient, 15, 900))
	ctx := context.WithValue(context.Background(), ctxkey.Group, &service.Group{ID: 7, UserConcurrencyLimit: 2})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var held []*service.AcquireResult
	var failures []error
	for i := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker := svc
			if i%2 != 0 {
				worker = secondService
			}
			result, err := worker.AcquireUserSlot(ctx, 42, 3)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, err)
			} else if result.Acquired {
				held = append(held, result)
			}
		}()
	}
	wg.Wait()
	require.Empty(t, failures)
	require.Len(t, held, 2)
	count, err := cache.GetUserConcurrency(ctx, 42)
	require.NoError(t, err)
	require.Equal(t, 2, count, "failed group acquisitions must not leak global slots")
	other := context.WithValue(ctx, ctxkey.Group, &service.Group{ID: 8, UserConcurrencyLimit: 5})
	third, err := svc.AcquireUserSlot(other, 42, 3)
	require.NoError(t, err)
	require.True(t, third.Acquired)
	blocked, err := svc.AcquireUserSlot(other, 42, 3)
	require.NoError(t, err)
	require.False(t, blocked.Acquired, "the global limit still includes every group")
	for _, result := range held {
		result.ReleaseFunc()
		result.ReleaseFunc()
	}
	third.ReleaseFunc()
	count, err = cache.GetUserConcurrency(ctx, 42)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestUserGroupConcurrencyCancellationAndDisabledBehavior(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewConcurrencyCache(client, 15, 900)
	svc := service.NewConcurrencyService(cache)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxkey.Group, &service.Group{ID: 7, UserConcurrencyLimit: 1}))
	result, err := svc.AcquireUserSlot(ctx, 42, 0)
	require.NoError(t, err)
	require.True(t, result.Acquired, "a group limit applies even when global concurrency is unlimited")
	cancel()
	result.ReleaseFunc()
	require.Zero(t, client.ZCard(context.Background(), userGroupSlotKey(42, 7)).Val())
	require.Zero(t, client.ZCard(context.Background(), userSlotKey(42)).Val())
	unlimited := context.WithValue(context.Background(), ctxkey.Group, &service.Group{ID: 7})
	keysBefore := server.Keys()
	for range 4 {
		result, err = svc.AcquireUserSlot(unlimited, 42, 0)
		require.NoError(t, err)
		require.True(t, result.Acquired)
		result.ReleaseFunc()
	}
	require.Equal(t, keysBefore, server.Keys(), "disabled limits must retain the no-cache unlimited path")
}

func TestLiveGroupLeaseRetainsGroupLimitAfterRequestCompletes(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewConcurrencyCache(client, 15, 900)
	group := cache.(service.UserGroupConcurrencyCache)
	live := cache.(service.LiveUserGroupConcurrencyCache)
	ctx := context.Background()
	allowed, err := group.AcquireUserGroupSlot(ctx, 42, 7, 3, 1, "request")
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = live.AcquireLiveLeaseForGroup(ctx, 9, 2, 42, 3, 7, 1, 30, "live", true)
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, group.ReleaseUserGroupSlot(ctx, 42, 7, "request"))
	allowed, err = group.AcquireUserGroupSlot(ctx, 42, 7, 3, 1, "blocked")
	require.NoError(t, err)
	require.False(t, allowed)
	refreshed, err := live.RefreshLiveLeaseForGroup(ctx, 9, 42, 7, 30, "live")
	require.NoError(t, err)
	require.True(t, refreshed)
	require.NoError(t, live.ReleaseLiveLeaseForGroup(ctx, 9, 42, 7, 30, "live"))
	allowed, err = group.AcquireUserGroupSlot(ctx, 42, 7, 3, 1, "next")
	require.NoError(t, err)
	require.True(t, allowed)
	server.FastForward(16 * time.Minute)
	allowed, err = group.AcquireUserGroupSlot(ctx, 42, 7, 3, 1, "after-crash-ttl")
	require.NoError(t, err)
	require.True(t, allowed)
}
