package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *userRepository) emailBlacklistMySQL() bool {
	return r.client != nil && r.client.Driver() != nil && r.client.Driver().Dialect() == "mysql"
}

var _ service.UserEmailBlacklistRepository = (*userRepository)(nil)

// listActiveUsersByEmailAliasProbeKeysSQL 把每个用户邮箱折成与 service.EmailAliasProbeKey
// 相同的形态（小写、去首尾空白与域名根点、本地部分去 "+后缀"、本地与域名都去点）后与探针键做
// 哈希连接；去点会在 Gmail 系之外过度匹配，由 service 层按收件箱身份复核。
const listActiveUsersByEmailAliasProbeKeysSQL = `
WITH probes AS (
	SELECT DISTINCT unnest($1::text[]) AS probe
), candidates AS (
	SELECT id, email, username,
		split_part(RTRIM(LOWER(TRIM(email)), '.'), '@', 1) AS local_part,
		split_part(RTRIM(LOWER(TRIM(email)), '.'), '@', 2) AS domain_part
	FROM users
	WHERE deleted_at IS NULL AND status = $2 AND role <> $3
)
SELECT c.id, c.email, c.username
FROM candidates c
JOIN probes p ON p.probe =
	REPLACE(
		CASE WHEN strpos(c.local_part, '+') > 1 THEN split_part(c.local_part, '+', 1) ELSE c.local_part END,
		'.', ''
	) || '@' || REPLACE(c.domain_part, '.', '')
ORDER BY c.id`

// ListActiveUsersByEmailAliasProbeKeys 见 service.UserEmailBlacklistRepository。
func (r *userRepository) ListActiveUsersByEmailAliasProbeKeys(ctx context.Context, keys []string) ([]service.User, error) {
	if len(keys) == 0 {
		return []service.User{}, nil
	}
	if r.sql == nil {
		return nil, fmt.Errorf("sql executor is not configured")
	}
	query := listActiveUsersByEmailAliasProbeKeysSQL
	args := []any{pq.Array(keys), service.StatusActive, service.RoleAdmin}
	if r.emailBlacklistMySQL() {
		encoded, err := json.Marshal(keys)
		if err != nil {
			return nil, err
		}
		query = `WITH probes AS (
 SELECT DISTINCT probe FROM JSON_TABLE(?, '$[*]' COLUMNS (probe VARCHAR(512) CHARACTER SET utf8mb4 PATH '$')) AS probe_keys
), candidates AS (
 SELECT id,email,username,
 SUBSTRING_INDEX(TRIM(TRAILING '.' FROM LOWER(TRIM(email))), '@', 1) AS local_part,
 SUBSTRING_INDEX(TRIM(TRAILING '.' FROM LOWER(TRIM(email))), '@', -1) AS domain_part
 FROM users WHERE deleted_at IS NULL AND status=? AND role<>?
)
SELECT c.id,c.email,c.username FROM candidates c JOIN probes p ON p.probe COLLATE utf8mb4_bin=CONCAT(
 REPLACE(CASE WHEN LOCATE('+',c.local_part)>1 THEN SUBSTRING_INDEX(c.local_part,'+',1) ELSE c.local_part END,'.',''),
 '@',REPLACE(c.domain_part,'.','')) COLLATE utf8mb4_bin ORDER BY c.id`
		args = []any{string(encoded), service.StatusActive, service.RoleAdmin}
	}
	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list users by email alias probes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	users := make([]service.User, 0)
	for rows.Next() {
		var user service.User
		if err := rows.Scan(&user.ID, &user.Email, &user.Username); err != nil {
			return nil, fmt.Errorf("scan user by email alias probe: %w", err)
		}
		user.Status = service.StatusActive
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users by email alias probes: %w", err)
	}
	return users, nil
}

// BatchDisableUsers 见 service.UserEmailBlacklistRepository。
//
// 与 userRepository.Update 停用单个用户的口径一致：停用与吊销 MCP 授权在同一事务内分两条语句
// 执行，恢复账号后旧授权不会复活。吊销语句在停用语句拿到用户行锁之后才取快照，能看到持有该行锁
// 并发新建、随后提交的授权。
func (r *userRepository) BatchDisableUsers(ctx context.Context, userIDs []int64) ([]int64, error) {
	if len(userIDs) == 0 {
		return []int64{}, nil
	}
	if r.client == nil {
		return nil, fmt.Errorf("ent client is not configured")
	}
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return nil, err
	}
	txCtx := ctx
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txCtx = dbent.NewTxContext(ctx, tx)
	}
	exec := txAwareSQLExecutor(txCtx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("sql executor is not configured")
	}

	var disabled []int64
	if r.emailBlacklistMySQL() {
		disabled, err = batchDisableUserRowsMySQL(txCtx, exec, userIDs)
	} else {
		disabled, err = batchDisableUserRows(txCtx, exec, userIDs)
	}
	if err != nil {
		return nil, err
	}
	if len(disabled) > 0 {
		query := `UPDATE mcp_oauth_grants SET revoked_at = $2 WHERE user_id = ANY($1) AND revoked_at IS NULL`
		args := []any{pq.Array(disabled), time.Now().UTC()}
		if r.emailBlacklistMySQL() {
			placeholders, ids := buildGroupInt64InClause(disabled)
			query = `UPDATE mcp_oauth_grants SET revoked_at = ? WHERE user_id IN (` + placeholders + `) AND revoked_at IS NULL`
			args = append([]any{time.Now().UTC()}, ids...)
		}
		if _, err := exec.ExecContext(txCtx,
			query, args...,
		); err != nil {
			return nil, fmt.Errorf("revoke mcp grants of disabled users: %w", err)
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	return disabled, nil
}

// MySQL has no UPDATE RETURNING. Lock the exact eligible users first and
// update/revoke in the same transaction, excluding administrators throughout.
func batchDisableUserRowsMySQL(ctx context.Context, exec sqlQueryExecutor, userIDs []int64) ([]int64, error) {
	ph, args := buildGroupInt64InClause(userIDs)
	args = append(args, service.RoleAdmin, service.StatusDisabled)
	rows, err := exec.QueryContext(ctx, `SELECT id FROM users WHERE id IN (`+ph+`)
AND deleted_at IS NULL AND role<>? AND status<>? ORDER BY id FOR UPDATE`, args...)
	if err != nil {
		return nil, err
	}
	disabled := make([]int64, 0, len(userIDs))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		disabled = append(disabled, id)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(disabled) == 0 {
		return disabled, nil
	}
	ph, args = buildGroupInt64InClause(disabled)
	args = append([]any{service.StatusDisabled}, args...)
	_, err = exec.ExecContext(ctx, `UPDATE users SET status=?,updated_at=NOW() WHERE id IN (`+ph+`)`, args...)
	return disabled, err
}

func batchDisableUserRows(ctx context.Context, exec sqlQueryExecutor, userIDs []int64) ([]int64, error) {
	rows, err := exec.QueryContext(ctx, `
UPDATE users SET status = $1, updated_at = NOW()
WHERE id = ANY($2) AND deleted_at IS NULL AND role <> $3 AND status <> $1
RETURNING id`, service.StatusDisabled, pq.Array(userIDs), service.RoleAdmin)
	if err != nil {
		return nil, fmt.Errorf("batch disable users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	disabled := make([]int64, 0, len(userIDs))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan disabled user id: %w", err)
		}
		disabled = append(disabled, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate disabled user ids: %w", err)
	}
	slices.Sort(disabled)
	return disabled, nil
}
