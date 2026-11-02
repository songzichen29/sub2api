//go:build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/qqbot"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/migrations"
	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

// A disposable MySQL container exercises the module without unrelated legacy
// integration suites. No external/production DSN is accepted.
func TestQQBotMySQLConcurrentRewardsAndBinding(t *testing.T) {
	ctx := context.Background()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for the disposable MySQL integration test")
	}
	container, err := tcmysql.Run(ctx, "mysql:8.0", tcmysql.WithDatabase("qqbot_test"), tcmysql.WithUsername("root"), tcmysql.WithPassword("local-test-only"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	dsn, err := container.ConnectionString(ctx, "parseTime=true", "charset=utf8mb4")
	require.NoError(t, err)
	integrationDB, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer integrationDB.Close()
	_, err = integrationDB.ExecContext(ctx, "CREATE TABLE users (id BIGINT PRIMARY KEY, balance DECIMAL(20,8) NOT NULL, status VARCHAR(20) NOT NULL, deleted_at DATETIME(6) NULL, updated_at DATETIME(6) NOT NULL)")
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "CREATE TABLE settings (id BIGINT AUTO_INCREMENT PRIMARY KEY, `key` VARCHAR(100) NOT NULL UNIQUE, value LONGTEXT NOT NULL, updated_at DATETIME(6) NOT NULL)")
	require.NoError(t, err)
	migration, err := migrations.MySQLFS.ReadFile("106_qq_community_bot.sql")
	require.NoError(t, err)
	for _, statement := range strings.Split(string(migration), ";") {
		if strings.TrimSpace(statement) != "" {
			_, err = integrationDB.ExecContext(ctx, statement)
			require.NoError(t, err)
		}
	}
	r := repository.NewQQBotRepository(integrationDB)
	c := qqbot.DefaultConfig()
	c.Enabled = true
	c.BotQQ = "123456"
	c.AllowedGroups = []string{"987654", "888888"}
	c.CheckinReward, c.CheckinRewardMax = 0.05, 0.1
	c.Version = 1
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	require.NoError(t, r.SaveConfig(ctx, string(raw), 0))
	_, err = integrationDB.ExecContext(ctx, "INSERT INTO users (id,balance,status,updated_at) VALUES (42,10,'active',?)", time.Now())
	require.NoError(t, err)
	const userID int64 = 42
	require.NoError(t, r.CreateBindCode(ctx, userID, qqbot.Hash("code"), time.Now().Add(time.Minute)))
	require.NoError(t, r.ClaimBindCode(ctx, qqbot.Hash("code"), "777777", "987654", "123456"))
	_, err = r.Account(ctx, "777777")
	require.ErrorIs(t, err, qqbot.ErrNotBound)
	_, err = r.ConfirmBinding(ctx, userID, "777777")
	require.NoError(t, err)
	now := time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	results := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			group := "987654"
			if i%2 == 0 {
				group = "888888"
			}
			_, err := r.Checkin(ctx, "777777", group, "123456", now)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.True(t, errors.Is(err, qqbot.ErrAlreadyCheckedIn), "unexpected reward error: %v", err)
		}
	}
	require.Equal(t, 1, success)
	account, err := r.Account(ctx, "777777")
	require.NoError(t, err)
	var amount, balanceAfter float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT amount,balance_after FROM qq_bot_rewards WHERE user_id = ?", userID).Scan(&amount, &balanceAfter))
	require.GreaterOrEqual(t, amount, 0.05)
	require.LessOrEqual(t, amount, 0.1)
	require.InDelta(t, 10+amount, account.Balance, 1e-8)
	require.Equal(t, account.Balance, balanceAfter)
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM qq_bot_rewards WHERE user_id = ?", userID).Scan(&count))
	require.Equal(t, 1, count)
	// Rebinding the same site user to another QQ cannot reset the daily reward.
	require.NoError(t, r.Unbind(ctx, userID))
	require.NoError(t, r.CreateBindCode(ctx, userID, qqbot.Hash("second"), time.Now().Add(time.Minute)))
	require.NoError(t, r.ClaimBindCode(ctx, qqbot.Hash("second"), "666666", "987654", "123456"))
	_, err = r.ConfirmBinding(ctx, userID, "666666")
	require.NoError(t, err)
	repeated, err := r.Checkin(ctx, "666666", "987654", "123456", now)
	require.ErrorIs(t, err, qqbot.ErrAlreadyCheckedIn)
	require.Equal(t, amount, repeated.Amount)
	account, err = r.Account(ctx, "666666")
	require.NoError(t, err)
	require.Equal(t, balanceAfter, account.Balance)
}
