package service

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"time"
)

func hashAdvisoryLockID(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64())
}

func tryAcquireDBAdvisoryLock(ctx context.Context, db *sql.DB, lockID int64) (func(), bool) {
	release, acquired, _ := tryAcquireDBAdvisoryLockWithError(ctx, db, lockID)
	return release, acquired
}

func tryAcquireDBAdvisoryLockWithError(ctx context.Context, db *sql.DB, lockID int64) (func(), bool, error) {
	if db == nil {
		return nil, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("open advisory-lock connection: %w", err)
	}

	lockName := fmt.Sprintf("sub2api:advisory:%d", lockID)

	acquired := false
	var lockResult sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", lockName).Scan(&lockResult); err != nil {
		_ = conn.Close()
		return nil, false, fmt.Errorf("query advisory lock: %w", err)
	}
	acquired = lockResult.Valid && lockResult.Int64 == 1
	if !acquired {
		_ = conn.Close()
		return nil, false, nil
	}

	release := func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var released sql.NullInt64
		_ = conn.QueryRowContext(unlockCtx, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released)
		_ = conn.Close()
	}
	return release, true, nil
}
