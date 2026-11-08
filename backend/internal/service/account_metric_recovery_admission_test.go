package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type accountMetricGuardCache struct {
	ConcurrencyCache
	acquired bool
	releases int
}

func (c *accountMetricGuardCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return c.acquired, nil
}

func (c *accountMetricGuardCache) ReleaseAccountSlot(context.Context, int64, string) error {
	c.releases++
	return nil
}

type accountMetricGuardAdmission struct {
	AccountPerformanceAdmissionRepository
	allowed bool
	err     error
	calls   int
}

func (a *accountMetricGuardAdmission) AdmitAccountPerformanceRequest(context.Context, int64, time.Time) (bool, error) {
	a.calls++
	return a.allowed, a.err
}

func TestAccountMetricGuardDeniesPausedAccountAndReleasesConcurrencySlot(t *testing.T) {
	cache := &accountMetricGuardCache{acquired: true}
	guard := &accountMetricGuardAdmission{allowed: false}
	service := NewConcurrencyService(cache, guard)

	result, err := service.AcquireAccountSlot(context.Background(), 7, 2)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.Equal(t, 1, guard.calls)
	require.Equal(t, 1, cache.releases)
}

func TestAccountMetricGuardFailsClosedOnStateReadError(t *testing.T) {
	cache := &accountMetricGuardCache{acquired: true}
	guard := &accountMetricGuardAdmission{err: errors.New("database down")}
	service := NewConcurrencyService(cache, guard)

	result, err := service.AcquireAccountSlot(context.Background(), 7, 2)
	require.Nil(t, result)
	require.ErrorContains(t, err, "database down")
	require.Equal(t, 1, cache.releases)
}

func TestAccountMetricGuardStillChecksUnlimitedAccounts(t *testing.T) {
	guard := &accountMetricGuardAdmission{allowed: false}
	service := NewConcurrencyService(&accountMetricGuardCache{}, guard)

	result, err := service.AcquireAccountSlot(context.Background(), 7, 0)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.Equal(t, 1, guard.calls)
}
