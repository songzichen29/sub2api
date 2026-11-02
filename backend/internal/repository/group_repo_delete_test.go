package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRemoveModerationGroupID(t *testing.T) {
	for _, tt := range []struct {
		name, value, want string
		id                int64
	}{
		{"remove duplicates and preserve fields", `{"group_ids":[7,85,9,85],"enabled":true,"future":{"limit":9007199254740993}}`, `{"enabled":true,"future":{"limit":9007199254740993},"group_ids":[7,9]}`, 85},
		{"last group becomes empty array", `{"group_ids":[85]}`, `{"group_ids":[]}`, 85},
		{"large IDs retain precision", `{"group_ids":[9007199254740992,9007199254740993]}`, `{"group_ids":[9007199254740992]}`, 9007199254740993},
		{"preserve non-ID values", `{"group_ids":["85",null,{"id":85},85]}`, `{"group_ids":["85",null,{"id":85}]}`, 85},
		{"null is not group zero", `{"group_ids":[null]}`, "", 0},
		{"unrelated ID", ` {"group_ids":[7,9]} `, "", 85},
		{"malformed JSON", `{"group_ids":[85`, "", 85},
		{"non-array scope", `{"group_ids":85}`, "", 85},
		{"missing scope", `{"enabled":true}`, "", 85},
		{"null scope", `{"group_ids":null}`, "", 85},
		{"null config", `null`, "", 85},
		{"array config", `[85]`, "", 85},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := removeModerationGroupID(tt.value, tt.id)
			require.Equal(t, tt.want != "", changed)
			if changed {
				// JSONEq would decode numbers as float64, hiding precision loss.
				require.Equal(t, tt.want, got)
			} else {
				require.Equal(t, tt.value, got)
			}
		})
	}
}

func newGroupDeleteMySQLMock(t *testing.T) (*groupRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.MySQL, db)))
	t.Cleanup(func() { _ = client.Close() })
	return newGroupRepositoryWithSQL(client, db), mock
}

func expectGroupDeleteMySQL(mock sqlmock.Sqlmock, requireEmpty bool) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, subscription_type FROM `groups` WHERE id = ? AND deleted_at IS NULL FOR UPDATE")).
		WithArgs(int64(85)).WillReturnRows(sqlmock.NewRows([]string{"id", "subscription_type"}).AddRow(85, service.SubscriptionTypeStandard))
	if requireEmpty {
		mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(85)).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	}
	for _, query := range []string{
		"DELETE FROM user_allowed_groups WHERE group_id = ?",
		"DELETE FROM account_groups WHERE group_id = ?",
		"UPDATE composite_model_routes SET deleted_at = NOW() WHERE group_id = ? AND deleted_at IS NULL",
	} {
		mock.ExpectExec(regexp.QuoteMeta(query)).WithArgs(int64(85)).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec("UPDATE `groups` SET").WillReturnResult(sqlmock.NewResult(0, 1))
}

const moderationLockMySQL = "SELECT `settings`.`id`, `settings`.`key`, `settings`.`value`, `settings`.`updated_at` FROM `settings` WHERE `settings`.`key` = ? LIMIT 2 FOR UPDATE"
const moderationUpdateMySQL = "UPDATE `settings` SET `value` = ?, `updated_at` = ? WHERE `settings`.`id` = ?"

func TestGroupDeleteCascadeMySQLModeration(t *testing.T) {
	for _, requireEmpty := range []bool{false, true} {
		method := "DeleteCascade"
		if requireEmpty {
			method = "DeleteCascadeIfEmpty"
		}
		for _, tt := range []struct {
			name, value, updated string
			missing              bool
		}{
			{"remove deleted group", `{"group_ids":[85,9],"enabled":true}`, `{"enabled":true,"group_ids":[9]}`, false},
			{"missing setting", "", "", true},
			{"invalid setting", `{"group_ids":[85`, "", false},
			{"unrelated scope", `{"group_ids":[9]}`, "", false},
		} {
			t.Run(method+"/"+tt.name, func(t *testing.T) {
				repo, mock := newGroupDeleteMySQLMock(t)
				expectGroupDeleteMySQL(mock, requireEmpty)
				rows := sqlmock.NewRows([]string{"id", "key", "value", "updated_at"})
				if !tt.missing {
					rows.AddRow(12, service.SettingKeyContentModerationConfig, tt.value, time.Now())
				}
				mock.ExpectQuery(regexp.QuoteMeta(moderationLockMySQL)).
					WithArgs(service.SettingKeyContentModerationConfig).WillReturnRows(rows)
				if tt.updated != "" {
					mock.ExpectExec(regexp.QuoteMeta(moderationUpdateMySQL)).
						WithArgs(tt.updated, sqlmock.AnyArg(), int64(12)).WillReturnResult(sqlmock.NewResult(0, 1))
				}
				mock.ExpectCommit()
				mock.ExpectExec("INSERT INTO scheduler_outbox").WillReturnResult(sqlmock.NewResult(1, 1))

				_, err := repo.deleteCascade(context.Background(), 85, requireEmpty)
				require.NoError(t, err)
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func TestGroupDeleteCascadeRollsBackOnModerationUpdateError(t *testing.T) {
	repo, mock := newGroupDeleteMySQLMock(t)
	expectGroupDeleteMySQL(mock, false)
	mock.ExpectQuery(regexp.QuoteMeta(moderationLockMySQL)).
		WithArgs(service.SettingKeyContentModerationConfig).
		WillReturnRows(sqlmock.NewRows([]string{"id", "key", "value", "updated_at"}).
			AddRow(12, service.SettingKeyContentModerationConfig, `{"group_ids":[85]}`, time.Now()))
	wantErr := errors.New("setting update failed")
	mock.ExpectExec(regexp.QuoteMeta(moderationUpdateMySQL)).
		WithArgs(`{"group_ids":[]}`, sqlmock.AnyArg(), int64(12)).WillReturnError(wantErr)
	mock.ExpectRollback()

	_, err := repo.DeleteCascade(context.Background(), 85)
	require.ErrorIs(t, err, wantErr)
	require.NoError(t, mock.ExpectationsWereMet())
}
