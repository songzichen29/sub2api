package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestUpstreamV020MySQLScopedAccountTransaction(t *testing.T) {
	dsn := os.Getenv("SUB2API_MERGE_MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("set SUB2API_MERGE_MYSQL_TEST_DSN for real MySQL compatibility checks")
	}
	cfg, err := mysql.ParseDSN(dsn)
	require.NoError(t, err)
	cfg.DBName = ""
	base, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	name := fmt.Sprintf("sub2api_merge_lock_test_%d", time.Now().UnixNano())
	_, err = base.Exec("CREATE DATABASE `" + name + "`")
	require.NoError(t, err)
	t.Cleanup(func() {
		defer base.Close()
		_, err := base.Exec("DROP DATABASE `" + name + "`")
		require.NoError(t, err)
	})
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.MySQL, db)))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	for _, ddl := range []string{
		`CREATE TABLE accounts(id BIGINT PRIMARY KEY, marker INT, deleted_at DATETIME)`,
		`CREATE TABLE account_groups(account_id BIGINT, group_id BIGINT, PRIMARY KEY(account_id,group_id))`,
		`INSERT INTO accounts(id,marker) VALUES(1,0)`,
		`INSERT INTO account_groups VALUES(1,1)`,
	} {
		_, err := db.Exec(ddl)
		require.NoError(t, err)
	}
	s := &adminServiceImpl{entClient: client}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	locked, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := s.scopedAdminTransaction(ctx, 1, false, func(txctx context.Context) (*Account, error) {
			close(locked)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			_, err := dbent.TxFromContext(txctx).Client().ExecContext(txctx, `UPDATE accounts SET marker=1 WHERE id=1`)
			return &Account{ID: 1}, err
		})
		done <- err
	}()
	select {
	case <-locked:
	case err := <-done:
		t.Fatalf("first transaction did not acquire the account lock: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	blockedCtx, blockedCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer blockedCancel()
	_, err = s.scopedAdminTransaction(blockedCtx, 1, false, func(context.Context) (*Account, error) {
		return nil, errors.New("concurrent callback bypassed the account row lock")
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	close(release)
	require.NoError(t, <-done)
	_, err = s.scopedAdminTransaction(ctx, 1, false, func(txctx context.Context) (*Account, error) {
		rows, err := dbent.TxFromContext(txctx).Client().QueryContext(txctx, `SELECT marker FROM accounts WHERE id=1`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		require.True(t, rows.Next())
		var marker int
		require.NoError(t, rows.Scan(&marker))
		require.Equal(t, 1, marker, "permission revalidation must see the committed account state")
		return &Account{ID: 1}, nil
	})
	require.NoError(t, err)
	rollback := errors.New("force rollback")
	_, err = s.scopedAdminTransaction(ctx, 1, false, func(txctx context.Context) (*Account, error) {
		_, err := dbent.TxFromContext(txctx).Client().ExecContext(txctx, `UPDATE accounts SET marker=2 WHERE id=1`)
		require.NoError(t, err)
		return nil, rollback
	})
	require.ErrorIs(t, err, rollback)
	var marker int
	require.NoError(t, db.QueryRow(`SELECT marker FROM accounts WHERE id=1`).Scan(&marker))
	require.Equal(t, 1, marker)
}
