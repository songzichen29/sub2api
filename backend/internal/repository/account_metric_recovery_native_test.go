package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	mysqldriver "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Explicit local test DSNs only. Each subtest creates and removes its own
// uniquely named database; existing databases/tables are never migrated.
func TestAccountMetricNativeDatabases(t *testing.T) {
	for _, engine := range []string{"mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			env := "SUB2API_TEST_METRIC_" + strings.ToUpper(engine) + "_DSN"
			dsn := os.Getenv(env)
			if dsn == "" {
				t.Skip(env + " is not configured; native migration/locking verification was not run")
			}
			db := newAccountMetricNativeTestDB(t, engine, dsn)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			for _, statement := range []string{
				"CREATE TABLE accounts (id BIGINT PRIMARY KEY)",
				"CREATE TABLE users (id BIGINT PRIMARY KEY)",
				"INSERT INTO accounts (id) VALUES (17)",
			} {
				_, err := db.ExecContext(ctx, statement)
				require.NoError(t, err)
			}
			migration, err := migrations.FS.ReadFile("253_account_metric_recovery.sql")
			if engine == "mysql" {
				migration, err = migrations.MySQLFS.ReadFile("112_account_metric_recovery.sql")
			}
			require.NoError(t, err)
			for i := 0; i < 2; i++ {
				for _, statement := range strings.Split(string(migration), ";") {
					if strings.TrimSpace(statement) == "" {
						continue
					}
					_, err = db.ExecContext(ctx, statement)
					require.NoError(t, err, "additive migration must execute and be idempotent")
				}
			}
			verifyAccountMetricSnapshotSQL(t, db, engine == "mysql")
			// Permit simultaneous transactions after temporary-table metric checks.
			db.SetMaxOpenConns(8)
			first := &accountMetricRepository{db: db, mysql: engine == "mysql"}
			second := &accountMetricRepository{db: db, mysql: engine == "mysql"}
			now := time.Now().UTC().Truncate(time.Microsecond)
			config := service.AccountMetricRecoveryConfig{
				Enabled: true, CooldownSeconds: 30, RecoverySamples: 3,
				SuccessRate: &service.AccountMetricThreshold{PauseThreshold: 0.8},
			}
			monitor, err := first.SaveAccountMetricRecoveryConfig(ctx, 17, 0, config, nil, now)
			require.NoError(t, err)
			require.Equal(t, int64(1), monitor.Revision)
			_, err = second.SaveAccountMetricRecoveryConfig(ctx, 17, 0, config, nil, now)
			require.ErrorIs(t, err, service.ErrAccountMetricRevisionConflict)
			changed, err := first.PauseAccountForMetricBreach(ctx, 17, monitor.Revision, "test breach", nil, now)
			require.NoError(t, err)
			require.True(t, changed)
			trialTime := now.Add(30 * time.Second)
			type outcome struct {
				allowed bool
				err     error
			}
			results := make(chan outcome, 16)
			start := make(chan struct{})
			for i := 0; i < 16; i++ {
				repo := first
				if i%2 != 0 {
					repo = second
				}
				go func() {
					<-start
					allowed, admitErr := repo.AdmitAccountPerformanceRequest(ctx, 17, trialTime)
					results <- outcome{allowed, admitErr}
				}()
			}
			close(start)
			admitted := 0
			for i := 0; i < 16; i++ {
				result := <-results
				require.NoError(t, result.err)
				if result.allowed {
					admitted++
				}
			}
			require.Equal(t, 1, admitted, "two repository instances must share the trial rate limit")
			for i := 1; i < 9; i++ {
				allowed, admitErr := second.AdmitAccountPerformanceRequest(ctx, 17, trialTime.Add(time.Duration(i)*5*time.Second))
				require.NoError(t, admitErr)
				require.True(t, allowed)
			}
			allowed, err := first.AdmitAccountPerformanceRequest(ctx, 17, trialTime.Add(45*time.Second))
			require.NoError(t, err)
			require.False(t, allowed, "trial budget must stop at recovery_samples * 3")
			monitor, err = first.GetAccountPerformanceMonitor(ctx, 17)
			require.NoError(t, err)
			require.Equal(t, int64(9), monitor.TrialRequests)
			_, err = first.ReleaseAccountMetricPause(ctx, 17, monitor.Revision, monitor.PauseGeneration-1, nil, now)
			require.ErrorIs(t, err, service.ErrAccountMetricPauseConflict)
			monitor, err = first.ReleaseAccountMetricPause(ctx, 17, monitor.Revision, monitor.PauseGeneration, nil, now)
			require.NoError(t, err)
			require.False(t, monitor.Paused)
			require.False(t, monitor.Config.Enabled)
			events, err := first.ListAccountPerformanceEvents(ctx, 17, 0, 50)
			require.NoError(t, err)
			require.Len(t, events, 12, "configure, pause, nine admissions, and release")
		})
	}
}

func newAccountMetricNativeTestDB(t *testing.T, engine, dsn string) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("sub2api_metric_test_%d", time.Now().UnixNano())
	quoted := `"` + name + `"`
	testDSN := ""
	if engine == "mysql" {
		cfg, err := mysqldriver.ParseDSN(dsn)
		if err != nil {
			t.Fatal("invalid local MySQL test DSN")
		}
		host, _, err := net.SplitHostPort(cfg.Addr)
		require.NoError(t, err)
		require.True(t, cfg.Net == "tcp" && metricTestLoopback(host), "native metric tests require a local database")
		cfg.DBName, cfg.ParseTime = name, true
		testDSN = cfg.FormatDSN()
		quoted = "`" + name + "`"
	} else {
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatal("invalid local PostgreSQL test URL")
		}
		require.True(t, (parsed.Scheme == "postgres" || parsed.Scheme == "postgresql") && metricTestLoopback(parsed.Hostname()), "native metric tests require a local PostgreSQL URL")
		// lib/pq URL query parameters can override the URL host/database. Reject
		// overrides so the local-only guard and isolated database stay effective.
		for _, key := range []string{"host", "hostaddr", "port", "dbname", "service", "servicefile"} {
			require.False(t, parsed.Query().Has(key), "native metric test URL must not override %s", key)
		}
		parsed.Path = "/" + name
		parsed.RawPath = ""
		testDSN = parsed.String()
	}
	admin, err := sql.Open(engine, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err = admin.ExecContext(ctx, "CREATE DATABASE "+quoted)
	require.NoError(t, err, "test credentials need CREATE DATABASE on the local test instance")
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		_, cleanupErr := admin.ExecContext(cleanupCtx, "DROP DATABASE "+quoted)
		if cleanupErr != nil {
			t.Errorf("cannot remove isolated metric test database %s", name)
		}
	})
	db, err := sql.Open(engine, testDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	return db
}

func metricTestLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
