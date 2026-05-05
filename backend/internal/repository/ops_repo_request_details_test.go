package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryListRequestDetails_BindsCTETimeWindowForBothBranches(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 4, 25, 15, 30, 54, 464000000, time.UTC)
	end := time.Date(2026, 4, 25, 16, 30, 54, 464000000, time.UTC)
	filter := &service.OpsRequestDetailFilter{
		StartTime: &start,
		EndTime:   &end,
		Kind:      "error",
		Page:      1,
		PageSize:  10,
		Sort:      "created_at_desc",
	}

	mock.ExpectQuery(`SELECT COUNT\(1\) FROM combined WHERE kind = \?`).
		WithArgs(start, end, start, end, "error").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	mock.ExpectQuery(`FROM combined\s+WHERE kind = \?\s+ORDER BY created_at DESC\s+LIMIT \? OFFSET \?`).
		WithArgs(start, end, start, end, "error", 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"kind",
			"created_at",
			"request_id",
			"platform",
			"model",
			"duration_ms",
			"status_code",
			"error_id",
			"phase",
			"severity",
			"message",
			"user_id",
			"api_key_id",
			"account_id",
			"account_name",
			"group_id",
			"stream",
		}).AddRow(
			"error",
			end,
			"req-1",
			"openai",
			"gpt-5.4",
			1250,
			500,
			int64(9),
			"gateway",
			"error",
			"boom",
			int64(1),
			int64(2),
			int64(3),
			"claude-pro-01",
			int64(4),
			true,
		))

	items, total, err := repo.ListRequestDetails(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, service.OpsRequestKindError, items[0].Kind)
	require.Equal(t, "req-1", items[0].RequestID)
	require.NotNil(t, items[0].StatusCode)
	require.Equal(t, 500, *items[0].StatusCode)
	require.Equal(t, "claude-pro-01", items[0].AccountName)

	require.NoError(t, mock.ExpectationsWereMet())
}
