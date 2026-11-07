package repository

import (
	"context"
	"database/sql"
	"errors"
)

// numberedSQLExecutor keeps upstream numbered SQL portable without changing
// transaction ownership. Repeated and reordered parameters must be rebound
// together with their arguments; replacing only $n would corrupt CAS writes.
type numberedSQLExecutor struct {
	exec  sqlExecutor
	mysql bool
}

func (e numberedSQLExecutor) bind(query string, args []any) (string, []any) {
	if !e.mysql {
		return query, args
	}
	return bindNumberedSQL(query, args...)
}

func (e numberedSQLExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	query, args = e.bind(query, args)
	return e.exec.ExecContext(ctx, query, args...)
}

func (e numberedSQLExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	query, args = e.bind(query, args)
	return e.exec.QueryContext(ctx, query, args...)
}

// QueryRowContext is used only with database/sql DB or Tx executors.
func (e numberedSQLExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	query, args = e.bind(query, args)
	return e.exec.(interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	}).QueryRowContext(ctx, query, args...)
}

// matchedUpdate preserves PostgreSQL's matched-row semantics for idempotent
// updates. MySQL reports changed rows by default, so an unchanged timestamp
// must be checked against the same authorization predicate before rejecting it.
func (e numberedSQLExecutor) matchedUpdate(ctx context.Context, result sql.Result, execErr error, existsQuery string, args ...any) (bool, error) {
	if execErr != nil {
		return false, execErr
	}
	n, err := result.RowsAffected()
	if err != nil || n > 0 || !e.mysql {
		return n > 0, err
	}
	var found int
	err = e.QueryRowContext(ctx, existsQuery, args...).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
