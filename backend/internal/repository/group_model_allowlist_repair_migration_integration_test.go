//go:build integration

package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const groupModelAllowlistRepairMigration = "100_group_model_allowlist_repair.sql"

// 236 是可重放的修复迁移：235 的重命名一旦被记账就不会重跑，数据库若回到旧结构
// （手工改回列名、按旧结构部分恢复）应用仍能启动，但所有关联 groups 的查询都会
// 报 column groups.model_allowlist does not exist（issue #6780）。
func TestMigration236RenamesLegacyModelsListConfigColumn(t *testing.T) {
	tx := newGroupRepairTestDB(t)
	ctx := context.Background()

	_, err := tx.ExecContext(ctx, "ALTER TABLE `groups` RENAME COLUMN model_allowlist TO models_list_config")
	require.NoError(t, err)

	resultgroupID, err := tx.ExecContext(ctx, `
INSERT INTO `+"`groups`"+` (name, platform, rate_multiplier, status, models_list_config)
VALUES ('migration-236-rename', 'anthropic', 1, 'active', '{"enabled":true,"models":["claude-sonnet-5"]}')
`)
	require.NoError(t, err)
	groupID, err := resultgroupID.LastInsertId()
	require.NoError(t, err)

	applyGroupModelAllowlistRepair(ctx, t, tx)

	// 重命名保留原数据，且新列恢复 NOT NULL DEFAULT '{}' 的形状。
	var allowlist string
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT model_allowlist FROM `groups` WHERE id = ?", groupID).Scan(&allowlist))
	require.JSONEq(t, `{"enabled":true,"models":["claude-sonnet-5"]}`, allowlist)
	requireModelAllowlistColumnShape(ctx, t, tx)

	// 可重放：重复执行不报错也不改变结果。
	applyGroupModelAllowlistRepair(ctx, t, tx)
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT model_allowlist FROM `groups` WHERE id = ?", groupID).Scan(&allowlist))
	require.JSONEq(t, `{"enabled":true,"models":["claude-sonnet-5"]}`, allowlist)
}

func TestMigration236BackfillsWhenBothColumnsExist(t *testing.T) {
	tx := newGroupRepairTestDB(t)
	ctx := context.Background()

	_, err := tx.ExecContext(ctx,
		"ALTER TABLE `groups` ADD COLUMN models_list_config JSON NOT NULL DEFAULT (JSON_OBJECT())")
	require.NoError(t, err)

	// 新列仍是默认空值：旧列里的配置应该被补回来。
	resultstaleID, err := tx.ExecContext(ctx, `
INSERT INTO `+"`groups`"+` (name, platform, rate_multiplier, status, model_allowlist, models_list_config)
VALUES ('migration-236-backfill', 'anthropic', 1, 'active', '{}', '{"enabled":true,"models":["legacy-model"]}')
`)
	require.NoError(t, err)
	staleID, err := resultstaleID.LastInsertId()
	require.NoError(t, err)

	// 新列已有配置：不能被旧列覆盖。
	resultcurrentID, err := tx.ExecContext(ctx, `
INSERT INTO `+"`groups`"+` (name, platform, rate_multiplier, status, model_allowlist, models_list_config)
VALUES ('migration-236-keep', 'anthropic', 1, 'active', '{"enabled":true,"models":["current-model"]}', '{"enabled":true,"models":["legacy-model"]}')
`)
	require.NoError(t, err)
	currentID, err := resultcurrentID.LastInsertId()
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, "ALTER TABLE `groups` MODIFY COLUMN model_allowlist JSON NULL")
	require.NoError(t, err)
	nullResult, err := tx.ExecContext(ctx, "INSERT INTO `groups` (name, model_allowlist, models_list_config) VALUES ('legacy-null', NULL, ?)", `{"enabled":true,"models":["null-legacy-model"]}`)
	require.NoError(t, err)
	nullID, err := nullResult.LastInsertId()
	require.NoError(t, err)

	applyGroupModelAllowlistRepair(ctx, t, tx)

	var backfilled, kept string
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT model_allowlist FROM `groups` WHERE id = ?", staleID).Scan(&backfilled))
	require.JSONEq(t, `{"enabled":true,"models":["legacy-model"]}`, backfilled)
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT model_allowlist FROM `groups` WHERE id = ?", currentID).Scan(&kept))
	require.JSONEq(t, `{"enabled":true,"models":["current-model"]}`, kept)
	var nullBackfilled string
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT model_allowlist FROM `groups` WHERE id = ?", nullID).Scan(&nullBackfilled))
	require.JSONEq(t, `{"enabled":true,"models":["null-legacy-model"]}`, nullBackfilled)
	requireModelAllowlistColumnShape(ctx, t, tx)
}

func TestMigration236RecreatesMissingModelAllowlistColumn(t *testing.T) {
	tx := newGroupRepairTestDB(t)
	ctx := context.Background()

	_, err := tx.ExecContext(ctx, "ALTER TABLE `groups` DROP COLUMN model_allowlist")
	require.NoError(t, err)

	resultgroupID, err := tx.ExecContext(ctx, `
INSERT INTO `+"`groups`"+` (name, platform, rate_multiplier, status)
VALUES ('migration-236-recreate', 'anthropic', 1, 'active')
`)
	require.NoError(t, err)
	groupID, err := resultgroupID.LastInsertId()
	require.NoError(t, err)

	applyGroupModelAllowlistRepair(ctx, t, tx)

	var allowlist string
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT model_allowlist FROM `groups` WHERE id = ?", groupID).Scan(&allowlist))
	require.JSONEq(t, `{}`, allowlist)
	requireModelAllowlistColumnShape(ctx, t, tx)
}

func applyGroupModelAllowlistRepair(ctx context.Context, t *testing.T, tx *sql.Conn) {
	t.Helper()

	migrationSQL, err := dbmigrations.MySQLFS.ReadFile(groupModelAllowlistRepairMigration)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
}

func requireModelAllowlistColumnShape(ctx context.Context, t *testing.T, tx *sql.Conn) {
	t.Helper()

	var isNullable, columnDefault string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT is_nullable, COALESCE(column_default, '')
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'model_allowlist'
`).Scan(&isNullable, &columnDefault))
	require.Equal(t, "NO", isNullable)
	require.Contains(t, strings.ToLower(columnDefault), "json_object")
}
func newGroupRepairTestDB(t *testing.T) *sql.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := integrationDB.Conn(ctx)
	require.NoError(t, err)
	var originalDB string
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&originalDB))
	name := "group_repair_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = conn.ExecContext(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = conn.ExecContext(context.Background(), "USE "+originalDB)
		_, _ = conn.ExecContext(context.Background(), "DROP DATABASE "+name)
		_ = conn.Close()
	})
	_, err = conn.ExecContext(ctx, "USE "+name)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, "CREATE TABLE "+"`groups`"+" (id BIGINT PRIMARY KEY AUTO_INCREMENT, name VARCHAR(255), platform VARCHAR(50), rate_multiplier DECIMAL(10,4), status VARCHAR(50), model_allowlist JSON NOT NULL DEFAULT (JSON_OBJECT()))")
	require.NoError(t, err)
	return conn
}
