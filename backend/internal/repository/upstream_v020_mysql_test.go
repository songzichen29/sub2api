package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

// An isolated database is created and removed on the explicitly supplied test
// instance. Normal unit runs skip this test when no MySQL instance is supplied.
func newUpstreamMySQLFixture(t *testing.T) (*sql.DB, *dbent.Client) {
	t.Helper()
	dsn := os.Getenv("SUB2API_MERGE_MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("set SUB2API_MERGE_MYSQL_TEST_DSN for real MySQL compatibility checks")
	}
	cfg, err := mysql.ParseDSN(dsn)
	require.NoError(t, err)
	cfg.DBName = ""
	cfg.ParseTime = true
	base, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	name := fmt.Sprintf("sub2api_merge_test_%d", time.Now().UnixNano())
	_, err = base.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4")
	require.NoError(t, err)
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.MySQL, db)))
	t.Cleanup(func() {
		_ = client.Close()
		_, err := base.Exec("DROP DATABASE `" + name + "`")
		if err != nil {
			t.Errorf("remove isolated test database: %v", err)
		}
		_ = base.Close()
	})
	for _, ddl := range []string{
		`CREATE TABLE users(id BIGINT PRIMARY KEY,email VARCHAR(255),username VARCHAR(100),role VARCHAR(20),status VARCHAR(20),deleted_at DATETIME(6),updated_at DATETIME(6))`,
		"CREATE TABLE `groups`(id BIGINT PRIMARY KEY,platform VARCHAR(32),allow_image_generation BOOLEAN DEFAULT FALSE)",
		`CREATE TABLE proxies(id BIGINT PRIMARY KEY,name VARCHAR(100),protocol VARCHAR(10),host VARCHAR(100),port INT,status VARCHAR(20),expires_at DATETIME(6),deleted_at DATETIME(6))`,
		`CREATE TABLE accounts(id BIGINT PRIMARY KEY,name VARCHAR(100),notes TEXT,platform VARCHAR(32),type VARCHAR(32),status VARCHAR(32),schedulable BOOLEAN,concurrency INT,priority INT,credentials JSON,extra JSON,proxy_id BIGINT,expires_at DATETIME(6),rate_limit_reset_at DATETIME(6),overload_until DATETIME(6),temp_unschedulable_until DATETIME(6),created_at DATETIME(6),updated_at DATETIME(6),deleted_at DATETIME(6),INDEX account_proxy(proxy_id))`,
		`CREATE TABLE account_groups(account_id BIGINT,group_id BIGINT,priority INT,created_at DATETIME(6))`,
		`INSERT INTO users(id,email,username,role,status) VALUES (1,'admin@example.com','admin','admin','active'),(2,'A.B+tag@gmail.com','user','user','active')`,
		"INSERT INTO `groups`(id,platform) VALUES (1,'gemini'),(2,'composite')",
	} {
		_, err := db.Exec(ddl)
		require.NoError(t, err)
	}
	content, err := migrations.MySQLFS.ReadFile("111_upstream_v0_2_20.sql")
	require.NoError(t, err)
	// PREPARE and its session variables must stay on the same connection.
	conn, err := db.Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	for range 2 {
		for _, statement := range strings.Split(string(content), ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			_, err = conn.ExecContext(context.Background(), statement)
			require.NoError(t, err, "migration statement: %s", statement)
		}
	}
	var gemini, composite bool
	require.NoError(t, db.QueryRow("SELECT allow_image_generation FROM `groups` WHERE id=1").Scan(&gemini))
	require.NoError(t, db.QueryRow("SELECT allow_image_generation FROM `groups` WHERE id=2").Scan(&composite))
	require.True(t, gemini)
	require.False(t, composite)
	return db, client
}

func TestUpstreamV020MySQLSecurityRepositories(t *testing.T) {
	db, client := newUpstreamMySQLFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	keys := NewScopedAdminKeyRepository(db)
	k := &service.ScopedAdminKey{ID: "test-key", Hash: strings.Repeat("a", 64), KeyPrefix: "adm_sc_test-key", CreatedBy: 1, CreatedAt: now, UpdatedAt: now}
	k.Name, k.Enabled = "test", true
	k.AllowedIPs = []string{}
	require.NoError(t, keys.InsertScopedAdminKey(ctx, k))
	loaded, err := keys.GetScopedAdminKey(ctx, k.ID)
	require.NoError(t, err)
	loaded.ExpectedUpdatedAt = loaded.UpdatedAt
	loaded.UpdatedAt = now.Add(time.Second)
	loaded.Name = "updated"
	require.NoError(t, keys.UpdateScopedAdminKey(ctx, loaded))
	require.ErrorIs(t, keys.UpdateScopedAdminKey(ctx, loaded), service.ErrScopedAdminKeyConflict)
	require.NoError(t, keys.TouchScopedAdminKey(ctx, k.ID, k.Hash, loaded.UpdatedAt, "127.0.0.1", now.Add(2*time.Second)))
	require.NoError(t, keys.TouchScopedAdminKey(ctx, k.ID, k.Hash, loaded.UpdatedAt, "127.0.0.1", now.Add(2*time.Second)), "an unchanged audit timestamp is still a valid key")
	require.Error(t, keys.TouchScopedAdminKey(ctx, k.ID, k.Hash, now, "127.0.0.1", now.Add(3*time.Second)))
	require.NoError(t, keys.RotateScopedAdminKeyHash(ctx, k.ID, k.Hash, strings.Repeat("b", 64), now.Add(4*time.Second)))
	require.ErrorIs(t, keys.RotateScopedAdminKeyHash(ctx, k.ID, k.Hash, strings.Repeat("c", 64), now.Add(5*time.Second)), service.ErrScopedAdminKeyConflict)
	require.NoError(t, keys.RevokeScopedAdminKey(ctx, k.ID, now.Add(6*time.Second)))
	require.NoError(t, keys.DeleteScopedAdminKeyRecord(ctx, k.ID, now.Add(7*time.Second)))
	items, err := keys.ListScopedAdminKeys(ctx)
	require.NoError(t, err)
	require.Empty(t, items)

	oauth := NewMCPOAuthRepository(client, db)
	grant := &service.MCPOAuthGrant{ID: "grant-one", ClientID: "client", ClientName: "test", UserID: 2, TokenVersion: 1, Resource: "test-resource", Scope: "user", ReadOnly: true, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	code := &service.MCPOAuthCode{Hash: "code-one", GrantID: grant.ID, ClientID: grant.ClientID, RedirectURI: "http://localhost/callback", Resource: grant.Resource, CodeChallenge: strings.Repeat("c", 43), ExpiresAt: now.Add(time.Minute)}
	require.NoError(t, oauth.CreateAuthorization(ctx, grant, code, 5))
	_, err = oauth.GetCode(ctx, code.Hash)
	require.NoError(t, err)
	access := &service.MCPOAuthCredential{Hash: "access-one", Kind: "access", ExpiresAt: now.Add(time.Minute)}
	refresh := &service.MCPOAuthCredential{Hash: "refresh-one", Kind: "refresh", ExpiresAt: now.Add(time.Hour)}
	require.NoError(t, oauth.ExchangeCode(ctx, code.Hash, grant.ID, now, access, refresh))
	_, err = oauth.GetCredential(ctx, refresh.Hash)
	require.NoError(t, err)
	refresh2 := &service.MCPOAuthCredential{Hash: "refresh-two", Kind: "refresh", ExpiresAt: now.Add(time.Hour)}
	require.NoError(t, oauth.RotateRefresh(ctx, refresh.Hash, grant.ID, now.Add(time.Second), nil, refresh2))
	require.ErrorIs(t, oauth.RotateRefresh(ctx, refresh.Hash, grant.ID, now.Add(2*time.Second), nil, nil), service.ErrMCPOAuthNotFound)
	got, err := oauth.GetGrant(ctx, grant.ID)
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt, "refresh replay must persist family revocation")
	require.NoError(t, oauth.RevokeGrant(ctx, 2, grant.ID, now.Add(3*time.Second)), "revocation is idempotent")
	require.ErrorIs(t, oauth.RevokeGrant(ctx, 1, grant.ID, now.Add(3*time.Second)), service.ErrMCPOAuthNotFound)

	grant.ID, code.Hash, code.GrantID = "grant-two", "code-two", "grant-two"
	require.NoError(t, oauth.CreateAuthorization(ctx, grant, code, 5))
	require.NoError(t, oauth.TouchGrant(ctx, grant.ID, now))
	require.NoError(t, oauth.TouchGrant(ctx, grant.ID, now), "identical touch timestamps must not reject a valid grant")
	users := newUserRepositoryWithSQL(client, db)
	matched, err := users.ListActiveUsersByEmailAliasProbeKeys(ctx, []string{"ab@gmailcom"})
	require.NoError(t, err)
	require.Len(t, matched, 1)
	require.Equal(t, int64(2), matched[0].ID)
	disabled, err := users.BatchDisableUsers(ctx, []int64{1, 2})
	require.NoError(t, err)
	require.Equal(t, []int64{2}, disabled, "administrators must never be batch-disabled")
	got, err = oauth.GetGrant(ctx, grant.ID)
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt)
}

func TestUpstreamV020MySQLAccountProjectionAndCAS(t *testing.T) {
	db, client := newUpstreamMySQLFixture(t)
	ctx := context.Background()
	_, err := db.Exec(`INSERT INTO accounts(id,name,platform,type,status,schedulable,concurrency,priority,credentials,extra,created_at) VALUES
(1,'test','openai','oauth','active',true,1,1,'{"access_token":"private-token","email":"one@example.com","chatgpt_account_id":" 12345 ","model_mapping":{"one":"two"}}','{"email":"one@example.com","secret":"private-extra","codex_7d_used_percent":42}',NOW()),
(2,'grok','grok','apikey','active',true,1,1,'{}','{"keep":"custom"}',NOW())`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO account_groups(account_id,group_id,priority) VALUES(1,1,1)`)
	require.NoError(t, err)
	repo := newAccountRepositoryWithSQL(client, db, nil)
	policy := adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.read", "accounts.identity.read", "accounts.usage.read", "accounts.credentials"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}}}}
	scoped := adminaccess.WithPrincipal(ctx, &adminaccess.Principal{Policy: policy})
	accounts, total, err := repo.ScopedAdminList(scoped, pagination.PaginationParams{Page: 1, PageSize: 20}, "", "", "", "one@example.com", 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, accounts, 1)
	require.NotContains(t, accounts[0].Credentials, "access_token")
	require.NotContains(t, accounts[0].Extra, "secret")
	require.Equal(t, float64(42), accounts[0].Extra["codex_7d_used_percent"])
	candidates, err := repo.ScopedAdminCodexCandidates(scoped, service.ScopedCodexIdentity{AccountID: "12345"})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	applied, err := repo.CompareAndSwapGrokObservedModels(ctx, 2, nil, json.RawMessage(`{"models":["grok"]}`))
	require.NoError(t, err)
	require.True(t, applied)
	applied, err = repo.CompareAndSwapGrokObservedModels(ctx, 2, nil, json.RawMessage(`{"models":["stale"]}`))
	require.NoError(t, err)
	require.False(t, applied)
	var preserved string
	require.NoError(t, db.QueryRow(`SELECT JSON_UNQUOTE(JSON_EXTRACT(extra,'$.keep')) FROM accounts WHERE id=2`).Scan(&preserved))
	require.Equal(t, "custom", preserved)
}

func TestUpstreamV020MySQLLeastUsedProxySerializesBindings(t *testing.T) {
	db, client := newUpstreamMySQLFixture(t)
	_, err := db.Exec(`INSERT INTO proxies(id,status) VALUES(1,'active'),(2,'active')`)
	require.NoError(t, err)
	repo := &proxyRepository{client: client}
	var ready sync.WaitGroup
	ready.Add(2)
	results := make(chan int64, 2)
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			tx, err := client.Tx(ctx)
			if err != nil {
				ready.Done()
				errors <- err
				return
			}
			defer tx.Rollback()
			// Establish an old consistent snapshot before waiting for the lock.
			rows, err := tx.Client().QueryContext(ctx, `SELECT id FROM accounts`)
			if err == nil {
				err = rows.Close()
			}
			ready.Done()
			ready.Wait()
			if err != nil {
				errors <- err
				return
			}
			id, err := repo.SelectAccountProxy(dbent.NewTxContext(ctx, tx), "least_used")
			if err == nil {
				_, err = tx.Client().ExecContext(ctx, `INSERT INTO accounts(id,proxy_id) VALUES(?,?)`, i+1, id)
			}
			if err == nil {
				err = tx.Commit()
			}
			if err != nil {
				errors <- err
				return
			}
			results <- id
		}(i)
	}
	ids := []int64{}
	for range 2 {
		select {
		case err := <-errors:
			require.NoError(t, err)
		case id := <-results:
			ids = append(ids, id)
		}
	}
	require.ElementsMatch(t, []int64{1, 2}, ids)
}
