package repository

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func transientDatabaseError(code string) error {
	return &pq.Error{Code: pq.ErrorCode(code), Message: "database is temporarily unavailable"}
}

func TestIsTransientDatabaseInitializationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "postgres is starting", err: transientDatabaseError("57P03"), want: true},
		{name: "postgres connection failure", err: fmt.Errorf("wrapped: %w", transientDatabaseError("08006")), want: true},
		{name: "mysql server shutdown", err: &mysql.MySQLError{Number: 1053, Message: "server shutdown"}, want: true},
		{name: "mysql connection refused", err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, want: true},
		{name: "mysql authentication failure", err: &mysql.MySQLError{Number: 1045, Message: "access denied"}, want: false},
		{name: "postgres authentication failure", err: transientDatabaseError("28P01"), want: false},
		{name: "migration error", err: errors.New("migration checksum mismatch"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isTransientDatabaseInitializationError(tt.err))
		})
	}
}

func TestInitializeDatabaseWithRetryEventuallySucceeds(t *testing.T) {
	attempts := 0
	var delays []time.Duration
	err := initializeDatabaseWithRetryWithWait(context.Background(), func(context.Context) error {
		attempts++
		if attempts <= 3 {
			return transientDatabaseError("57P03")
		}
		return nil
	}, func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 4, attempts)
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, delays)
}

func TestInitializeDatabaseWithRetryFailsFastForPermanentError(t *testing.T) {
	attempts := 0
	waitCalled := false
	permanentErr := transientDatabaseError("28P01")
	err := initializeDatabaseWithRetryWithWait(context.Background(), func(context.Context) error {
		attempts++
		return permanentErr
	}, func(_ context.Context, _ time.Duration) error {
		waitCalled = true
		return nil
	})

	require.ErrorIs(t, err, permanentErr)
	require.Equal(t, 1, attempts)
	require.False(t, waitCalled)
}

func TestInitializeDatabaseWithRetryStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := 0
	err := initializeDatabaseWithRetryWithWait(ctx, func(context.Context) error {
		attempts++
		return transientDatabaseError("57P03")
	}, func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	})

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, attempts)
}

func TestInitializeDatabaseWithRetryReturnsLastErrorAfterLimit(t *testing.T) {
	attempts := 0
	lastErr := transientDatabaseError("08006")
	var delays []time.Duration
	err := initializeDatabaseWithRetryWithWait(context.Background(), func(context.Context) error {
		attempts++
		return lastErr
	}, func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	})

	require.ErrorIs(t, err, lastErr)
	require.Equal(t, maxDatabaseInitializationRetries+1, attempts)
	require.Len(t, delays, maxDatabaseInitializationRetries)
	require.Equal(t, databaseInitializationRetryMax, delays[len(delays)-1])
}

func TestInitializeDatabaseWithRetryAllowsIdempotentMigrationRetry(t *testing.T) {
	applied := make(map[string]bool)
	attempts := 0
	err := initializeDatabaseWithRetryWithWait(context.Background(), func(context.Context) error {
		attempts++
		if !applied["001_init.sql"] {
			applied["001_init.sql"] = true
			return transientDatabaseError("57P03")
		}
		return nil
	}, func(_ context.Context, _ time.Duration) error { return nil })

	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.True(t, applied["001_init.sql"])
}
