package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type geminiCachedContentSQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type geminiCachedContentRepository struct {
	sql   geminiCachedContentSQLExecutor
	mysql bool
}

func NewGeminiCachedContentRepository(db *sql.DB) service.GeminiCachedContentRepository {
	return &geminiCachedContentRepository{sql: db, mysql: isGeminiCachedContentMySQL(db)}
}

func isGeminiCachedContentMySQL(db *sql.DB) bool {
	if db == nil || db.Driver() == nil {
		return false
	}
	return strings.Contains(strings.ToLower(fmt.Sprintf("%T", db.Driver())), "mysql")
}

// geminiCachedContentQuery keeps sqlmock/PostgreSQL tests on numbered
// placeholders while translating the PostgreSQL-only syntax used by the
// shared query text for MySQL connections.
func (r *geminiCachedContentRepository) geminiCachedContentQuery(query string) string {
	converted, _ := r.geminiCachedContentQueryArgs(query)
	return converted
}

// geminiCachedContentQueryArgs converts PostgreSQL numbered placeholders to
// MySQL markers and reorders arguments to match MySQL's appearance order.
// PostgreSQL permits a query to reference $2 before $1; MySQL binds each '?'
// positionally, so converting the query alone would silently corrupt writes.
func (r *geminiCachedContentRepository) geminiCachedContentQueryArgs(query string, args ...any) (string, []any) {
	if r == nil || !r.mysql {
		return query, args
	}
	query = strings.ReplaceAll(query, "NOW() + make_interval(secs => $3::double precision)", "DATE_ADD(NOW(), INTERVAL $3 SECOND)")
	var b strings.Builder
	order := make([]int, 0, len(args))
	for i := 0; i < len(query); i++ {
		if query[i] == '\'' || query[i] == '"' || query[i] == '`' {
			quote := query[i]
			b.WriteByte(quote)
			for i++; i < len(query); i++ {
				b.WriteByte(query[i])
				if query[i] == quote {
					if i+1 < len(query) && query[i+1] == quote {
						b.WriteByte(query[i+1])
						i++
						continue
					}
					break
				}
			}
			continue
		}
		if query[i] == '$' && i+1 < len(query) && query[i+1] >= '0' && query[i+1] <= '9' {
			b.WriteByte('?')
			index := 0
			i++
			index = int(query[i] - '0')
			for i+1 < len(query) && query[i+1] >= '0' && query[i+1] <= '9' {
				i++
				index = index*10 + int(query[i]-'0')
			}
			order = append(order, index-1)
			continue
		}
		b.WriteByte(query[i])
	}
	reordered := make([]any, 0, len(order))
	for _, index := range order {
		if index >= 0 && index < len(args) {
			reordered = append(reordered, args[index])
		}
	}
	return b.String(), reordered
}

const geminiCachedContentColumns = `id, public_id, user_id, api_key_id, group_id, account_id, upstream_name, model,
 upstream_model, display_name, total_token_count, expire_time, channel_usage, created_at, updated_at`

func (r *geminiCachedContentRepository) Create(ctx context.Context, record *service.GeminiCachedContent) error {
	channelUsage, err := json.Marshal(record.ChannelUsage)
	if err != nil {
		return err
	}
	query := `
INSERT INTO gemini_cached_contents (public_id, user_id, api_key_id, group_id, account_id, upstream_name, model,
 upstream_model, display_name, total_token_count, expire_time, channel_usage)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, created_at, updated_at`
	args := []any{record.PublicID, record.UserID, record.APIKeyID, record.GroupID, record.AccountID, record.UpstreamName,
		record.Model, record.UpstreamModel, record.DisplayName, record.TotalTokenCount, record.ExpireTime, channelUsage}
	if r.mysql {
		insertQuery := strings.TrimSuffix(query, "RETURNING id, created_at, updated_at")
		converted, convertedArgs := r.geminiCachedContentQueryArgs(insertQuery, args...)
		result, err := r.sql.ExecContext(ctx, converted, convertedArgs...)
		if err != nil {
			return translatePersistenceError(err, nil, service.ErrGeminiCachedContentExists)
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		return r.sql.QueryRowContext(ctx, `SELECT id, created_at, updated_at FROM gemini_cached_contents WHERE id = ?`, id).
			Scan(&record.ID, &record.CreatedAt, &record.UpdatedAt)
	}
	row := r.sql.QueryRowContext(ctx, query,
		args...)
	if err := row.Scan(&record.ID, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return translatePersistenceError(err, nil, service.ErrGeminiCachedContentExists)
	}
	return nil
}

func (r *geminiCachedContentRepository) GetForOwner(ctx context.Context, apiKeyID int64, publicID string) (*service.GeminiCachedContent, error) {
	query, args := r.geminiCachedContentQueryArgs(`SELECT `+geminiCachedContentColumns+`
 FROM gemini_cached_contents
	WHERE public_id = $1 AND api_key_id = $2 AND deleted_at IS NULL`, publicID, apiKeyID)
	record, err := scanGeminiCachedContent(r.sql.QueryRowContext(ctx, query, args...))
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGeminiCachedContentNotFound, nil)
	}
	return record, nil
}

func (r *geminiCachedContentRepository) ListForOwner(ctx context.Context, apiKeyID, groupID int64, activeAt time.Time, beforeID int64, limit int) ([]*service.GeminiCachedContent, error) {
	if limit <= 0 {
		limit = service.GeminiCachedContentListDefaultPageSize
	}
	query, args := r.geminiCachedContentQueryArgs(`SELECT `+geminiCachedContentColumns+`
 FROM gemini_cached_contents
 WHERE api_key_id = $1 AND group_id = $2 AND deleted_at IS NULL AND expire_time > $3 AND ($4 <= 0 OR id < $4)
	ORDER BY id DESC
	LIMIT $5`, apiKeyID, groupID, activeAt, beforeID, limit)
	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*service.GeminiCachedContent
	for rows.Next() {
		record, err := scanGeminiCachedContent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (r *geminiCachedContentRepository) BeginMutation(ctx context.Context, id int64, baseline time.Time, token string, lease time.Duration) (bool, error) {
	query, args := r.geminiCachedContentQueryArgs(`UPDATE gemini_cached_contents
 SET mutation_token = $2, mutation_lease_until = NOW() + make_interval(secs => $3::double precision)
	WHERE id = $1 AND deleted_at IS NULL AND mutation_token IS NULL AND expire_time = $4`,
		id, token, lease.Seconds(), baseline)
	result, err := r.sql.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}
	var exists bool
	existsQuery, existsArgs := r.geminiCachedContentQueryArgs(`SELECT EXISTS (
 SELECT 1 FROM gemini_cached_contents WHERE id = $1 AND deleted_at IS NULL)`)
	if err := r.sql.QueryRowContext(ctx, existsQuery, append(existsArgs, id)...).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, service.ErrGeminiCachedContentNotFound
	}
	return false, nil
}

func (r *geminiCachedContentRepository) TakeOverStaleMutation(ctx context.Context, id int64, token string, lease time.Duration) (*service.GeminiCachedContent, error) {
	if r.mysql {
		query, args := r.geminiCachedContentQueryArgs(`UPDATE gemini_cached_contents
 SET mutation_token = $2, mutation_lease_until = NOW() + make_interval(secs => $3::double precision)
	 WHERE id = $1 AND deleted_at IS NULL AND mutation_token IS NOT NULL AND mutation_lease_until < NOW()`,
			id, token, lease.Seconds())
		result, err := r.sql.ExecContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		affected, err := result.RowsAffected()
		if err != nil || affected == 0 {
			return nil, err
		}
		record, err := scanGeminiCachedContent(r.sql.QueryRowContext(ctx,
			`SELECT `+geminiCachedContentColumns+` FROM gemini_cached_contents WHERE id = ? AND mutation_token = ?`, id, token))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return record, err
	}
	record, err := scanGeminiCachedContent(r.sql.QueryRowContext(ctx, `UPDATE gemini_cached_contents
 SET mutation_token = $2, mutation_lease_until = NOW() + make_interval(secs => $3::double precision)
 WHERE id = $1 AND deleted_at IS NULL AND mutation_token IS NOT NULL AND mutation_lease_until < NOW()
 RETURNING `+geminiCachedContentColumns, id, token, lease.Seconds()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (r *geminiCachedContentRepository) CommitMutation(ctx context.Context, id int64, token string, expireTime time.Time) error {
	query, args := r.geminiCachedContentQueryArgs(`UPDATE gemini_cached_contents
 SET expire_time = $3, updated_at = NOW(), mutation_token = NULL, mutation_lease_until = NULL
	WHERE id = $1 AND deleted_at IS NULL AND mutation_token = $2`, id, token, expireTime)
	result, err := r.sql.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrGeminiCachedContentMutationConflict
	}
	return nil
}

func (r *geminiCachedContentRepository) ReleaseMutation(ctx context.Context, id int64, token string) error {
	query, args := r.geminiCachedContentQueryArgs(`UPDATE gemini_cached_contents
 SET mutation_token = NULL, mutation_lease_until = NULL
	WHERE id = $1 AND mutation_token = $2`, id, token)
	_, err := r.sql.ExecContext(ctx, query, args...)
	return err
}

func (r *geminiCachedContentRepository) ListStaleMutations(ctx context.Context, limit int) ([]int64, error) {
	query, args := r.geminiCachedContentQueryArgs(`SELECT id FROM gemini_cached_contents
 WHERE mutation_token IS NOT NULL AND deleted_at IS NULL AND mutation_lease_until < NOW()
 ORDER BY mutation_lease_until
	LIMIT $1`, limit)
	rows, err := r.sql.QueryContext(ctx, query, args...)
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
	return ids, rows.Err()
}

func (r *geminiCachedContentRepository) SoftDelete(ctx context.Context, id int64) error {
	query, args := r.geminiCachedContentQueryArgs(`UPDATE gemini_cached_contents
 SET deleted_at = NOW(), updated_at = NOW()
	WHERE id = $1 AND deleted_at IS NULL`, id)
	_, err := r.sql.ExecContext(ctx, query, args...)
	return err
}

type geminiCachedContentScanner interface {
	Scan(dest ...any) error
}

func scanGeminiCachedContent(row geminiCachedContentScanner) (*service.GeminiCachedContent, error) {
	var record service.GeminiCachedContent
	var channelUsage []byte
	if err := row.Scan(
		&record.ID,
		&record.PublicID,
		&record.UserID,
		&record.APIKeyID,
		&record.GroupID,
		&record.AccountID,
		&record.UpstreamName,
		&record.Model,
		&record.UpstreamModel,
		&record.DisplayName,
		&record.TotalTokenCount,
		&record.ExpireTime,
		&channelUsage,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(channelUsage) > 0 {
		if err := json.Unmarshal(channelUsage, &record.ChannelUsage); err != nil {
			return nil, err
		}
	}
	return &record, nil
}
