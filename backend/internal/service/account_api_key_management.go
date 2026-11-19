package service

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type AccountAPIKeySummary struct {
	ID         string     `json:"id"`
	Masked     string     `json:"masked"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type AccountAPIKeyPool struct {
	Revision string                 `json:"revision"`
	Keys     []AccountAPIKeySummary `json:"keys"`
}

type AccountAPIKeyEntry struct {
	ID    string `json:"id,omitempty"`
	Value string `json:"value,omitempty"`
}

type AccountAPIKeyUpdate struct {
	Revision string               `json:"revision"`
	Entries  []AccountAPIKeyEntry `json:"entries"`
}

func accountAPIKeyID(accountID int64, key string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s", accountID, key))))
}

// APIKeyPoolSummary 仅返回脱敏标识；编辑保留的 Key 在服务端解析。
func (a *Account) APIKeyPoolSummary() *AccountAPIKeyPool {
	if a.Type != AccountTypeAPIKey {
		return nil
	}
	keys := a.OrderedAPIKeys()
	fingerprint := accountAPIKeyPoolFingerprint(a.GetCredential("base_url"), keys)
	pool := &AccountAPIKeyPool{Revision: fmt.Sprintf("%x", fingerprint), Keys: make([]AccountAPIKeySummary, 0, len(keys))}
	for i, key := range keys {
		masked := "••••••••"
		characters := []rune(key)
		if len(characters) > 8 {
			masked += string(characters[len(characters)-4:])
		}
		item := AccountAPIKeySummary{ID: accountAPIKeyID(a.ID, key), Masked: masked}
		if recent, ok := recentAccountAPIKeyUses.Load(a.ID); ok {
			use := recent.(accountAPIKeyUse)
			if use.poolFingerprint == fingerprint && use.index == i {
				item.LastUsedAt = &use.usedAt
			}
		}
		pool.Keys = append(pool.Keys, item)
	}
	return pool
}

// ResolveAPIKeyUpdate 校验编辑快照并解析保留项，不修改现有凭据。
func (a *Account) ResolveAPIKeyUpdate(update *AccountAPIKeyUpdate) (map[string]any, error) {
	if a.Type != AccountTypeAPIKey || a.IsCredentialShadow() {
		return nil, infraerrors.BadRequest("INVALID_ACCOUNT_API_KEYS", "only API-key accounts support key editing")
	}
	pool := a.APIKeyPoolSummary()
	if update.Revision != pool.Revision {
		return nil, infraerrors.New(http.StatusConflict, "ACCOUNT_API_KEYS_CHANGED", "API keys have changed; reload the account before saving")
	}
	existing := make(map[string]string, len(pool.Keys))
	for _, key := range a.OrderedAPIKeys() {
		existing[accountAPIKeyID(a.ID, key)] = key
	}
	keys := make([]string, 0, len(update.Entries))
	for _, entry := range update.Entries {
		if entry.ID == "" {
			keys = append(keys, strings.TrimSpace(entry.Value))
			continue
		}
		key, ok := existing[entry.ID]
		if !ok || entry.Value != "" {
			return nil, infraerrors.BadRequest("INVALID_ACCOUNT_API_KEYS", "invalid saved API key reference")
		}
		keys = append(keys, key)
	}
	credentials := map[string]any{"api_keys": keys}
	if err := NormalizeAccountAPIKeys(a.Type, credentials); err != nil {
		return nil, err
	}
	return credentials, nil
}

func (a *Account) APIKeyByID(id string) (string, error) {
	for _, key := range a.OrderedAPIKeys() {
		if accountAPIKeyID(a.ID, key) == id {
			return key, nil
		}
	}
	return "", infraerrors.New(http.StatusConflict, "ACCOUNT_API_KEYS_CHANGED", "API key no longer exists; reload the account")
}
