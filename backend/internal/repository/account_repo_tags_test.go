package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountTagsListFilterUsesDialectAndAnyTag(t *testing.T) {
	for _, dbDialect := range []string{dialect.Postgres, dialect.MySQL} {
		t.Run(dbDialect, func(t *testing.T) {
			var queries []string
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_, actual string) error {
				queries = append(queries, normalizeSQLWhitespace(actual))
				return nil
			})))
			require.NoError(t, err)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dbDialect, db)))
			t.Cleanup(func() { _ = client.Close() })
			repo := newAccountRepositoryWithSQL(client, db, nil)

			args := []driver.Value{service.PlatformAnthropic, `["vip"]`, `["生产"]`}
			if dbDialect == dialect.MySQL {
				args = []driver.Value{service.PlatformAnthropic, `["vip"]`, 1, `["生产"]`, 1}
			}
			mock.ExpectQuery("count").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			mock.ExpectQuery("list").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id"}))

			accounts, page, err := repo.ListWithFilters(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 10},
				service.PlatformAnthropic, "", "", "", 0, "", []string{"vip", "生产"})
			require.NoError(t, err)
			require.Empty(t, accounts)
			require.Zero(t, page.Total)
			require.NoError(t, mock.ExpectationsWereMet())
			require.Len(t, queries, 2)
			for _, query := range queries {
				require.NotContains(t, query, "vip", "tags must be bound, not interpolated")
				require.Contains(t, query, "deleted_at")
				if dbDialect == dialect.Postgres {
					require.NotContains(t, query, "JSON_CONTAINS")
					require.Contains(t, query, `"platform" = $1 AND ("accounts"."tags" @> $2 OR "accounts"."tags" @> $3)`)
				} else {
					require.Contains(t, query, "`platform` = ? AND (JSON_CONTAINS(`accounts`.`tags`, ?, '$') = ? OR JSON_CONTAINS(`accounts`.`tags`, ?, '$') = ?)")
				}
			}
		})
	}
}

func TestAccountTagsBulkUpdateReplacementAndClear(t *testing.T) {
	for _, dbDialect := range []string{dialect.Postgres, dialect.MySQL} {
		for _, tc := range []struct {
			name string
			tags []string
			json string
		}{
			{name: "replace", tags: []string{"prod", "生产"}, json: `["prod","生产"]`},
			{name: "clear", tags: []string{}, json: `[]`},
			{name: "nil_slice_clears", tags: nil, json: `[]`},
		} {
			t.Run(dbDialect+"/"+tc.name, func(t *testing.T) {
				exec := &recordingSQLExecutor{result: rowsAffectedResult(2)}
				repo := newAccountRepositoryWithSQL(nil, exec, nil)
				update := repo.BulkUpdate
				if dbDialect == dialect.MySQL {
					update = repo.bulkUpdateMySQL
				}
				rows, err := update(context.Background(), []int64{27, 28}, service.AccountBulkUpdate{Tags: &tc.tags})
				require.NoError(t, err)
				require.EqualValues(t, 2, rows)
				require.NotEmpty(t, exec.execQueries)
				query := normalizeSQLWhitespace(exec.execQueries[0])
				if dbDialect == dialect.Postgres {
					require.Contains(t, query, "tags = $1::jsonb, updated_at = NOW() WHERE id = ANY($2) AND deleted_at IS NULL")
				} else {
					require.Contains(t, query, "tags = CAST(? AS JSON), updated_at = NOW() WHERE id IN (?,?) AND deleted_at IS NULL")
				}
				payload, ok := exec.execArgs[0][0].([]byte)
				require.True(t, ok)
				require.JSONEq(t, tc.json, string(payload))
			})
		}
	}
}

func TestAccountTagsBulkUpdateNilLeavesTagsUnchanged(t *testing.T) {
	for _, dbDialect := range []string{dialect.Postgres, dialect.MySQL} {
		t.Run(dbDialect, func(t *testing.T) {
			exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
			repo := newAccountRepositoryWithSQL(nil, exec, nil)
			update := repo.BulkUpdate
			if dbDialect == dialect.MySQL {
				update = repo.bulkUpdateMySQL
			}
			rows, err := update(context.Background(), []int64{27}, service.AccountBulkUpdate{})
			require.NoError(t, err)
			require.Zero(t, rows)
			require.Empty(t, exec.execQueries)

			name := "renamed"
			_, err = update(context.Background(), []int64{27}, service.AccountBulkUpdate{Name: &name})
			require.NoError(t, err)
			require.NotContains(t, exec.execQueries[0], "tags =")
		})
	}
}

func TestAccountTagsBulkUpdatePostgresMixedFieldsKeepParameterOrder(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	name, tags := "renamed", []string{"vip"}
	_, err := repo.BulkUpdate(context.Background(), []int64{27}, service.AccountBulkUpdate{
		Name: &name, Extra: map[string]any{"note": "keep"}, Tags: &tags,
	})
	require.NoError(t, err)
	query := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, query, "name = $1")
	require.Contains(t, query, "extra = COALESCE(extra, '{}'::jsonb) || $2::jsonb")
	require.Contains(t, query, "tags = $3::jsonb")
	require.Contains(t, query, "id = ANY($4)")
	require.Equal(t, 4, strings.Count(query, "$"))
	require.Len(t, exec.execArgs[0], 4)
	require.Equal(t, name, exec.execArgs[0][0])
	require.JSONEq(t, `{"note":"keep"}`, string(exec.execArgs[0][1].([]byte)))
	require.JSONEq(t, `["vip"]`, string(exec.execArgs[0][2].([]byte)))
}
