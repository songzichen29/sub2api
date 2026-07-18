package service

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

// ApplyProviderDefaultSettingsOnFirstBind applies provider-specific bootstrap
// settings the first time a user binds a third-party identity. The grant is
// idempotent per user/provider pair.
func (s *AuthService) ApplyProviderDefaultSettingsOnFirstBind(
	ctx context.Context,
	userID int64,
	providerType string,
) error {
	if s == nil || s.entClient == nil || s.settingService == nil || userID <= 0 {
		return nil
	}

	if dbent.TxFromContext(ctx) != nil {
		_, err := s.applyProviderDefaultSettingsOnFirstBind(ctx, userID, providerType)
		return err
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin first bind defaults transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	assignedGroups, err := s.applyProviderDefaultSettingsOnFirstBind(txCtx, userID, providerType)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.invalidateAssignedSubscriptionCaches(userID, assignedGroups)
	return nil
}

func (s *AuthService) applyProviderDefaultSettingsOnFirstBind(
	ctx context.Context,
	userID int64,
	providerType string,
) ([]int64, error) {
	providerDefaults, enabled, err := s.settingService.ResolveAuthSourceGrantSettings(ctx, providerType, true)
	if err != nil {
		return nil, fmt.Errorf("load auth source defaults: %w", err)
	}
	if !enabled {
		return nil, nil
	}

	client := s.entClient
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
	}

	var result entsql.Result
	insertGrantSQL := firstBindProviderGrantInsertSQL(client)
	if err := client.Driver().Exec(
		ctx,
		insertGrantSQL,
		[]any{userID, strings.TrimSpace(providerType), "first_bind"},
		&result,
	); err != nil {
		return nil, fmt.Errorf("record first bind provider grant: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read first bind provider grant result: %w", err)
	}
	if affected == 0 {
		return nil, nil
	}

	if providerDefaults.Balance != 0 {
		if err := client.User.UpdateOneID(userID).AddBalance(providerDefaults.Balance).Exec(ctx); err != nil {
			return nil, fmt.Errorf("apply first bind balance default: %w", err)
		}
	}
	if providerDefaults.Concurrency != 0 {
		if err := client.User.UpdateOneID(userID).AddConcurrency(providerDefaults.Concurrency).Exec(ctx); err != nil {
			return nil, fmt.Errorf("apply first bind concurrency default: %w", err)
		}
	}
	assignedGroups := make([]int64, 0, len(providerDefaults.Subscriptions))
	if s.defaultSubAssigner != nil {
		for _, item := range providerDefaults.Subscriptions {
			if _, _, err := s.defaultSubAssigner.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
				UserID:       userID,
				GroupID:      item.GroupID,
				ValidityDays: item.ValidityDays,
				StartsAt:     parseDefaultSubscriptionTime(item.StartsAt),
				ExpiresAt:    parseDefaultSubscriptionTime(item.ExpiresAt),
				Notes:        "auto assigned by first bind defaults",
			}); err != nil {
				return nil, fmt.Errorf("apply first bind subscription default: %w", err)
			}
			assignedGroups = append(assignedGroups, item.GroupID)
		}
	}

	return assignedGroups, nil
}

func (s *AuthService) invalidateAssignedSubscriptionCaches(userID int64, groupIDs []int64) {
	if s == nil || userID <= 0 || len(groupIDs) == 0 {
		return
	}
	invalidator, ok := s.defaultSubAssigner.(interface {
		invalidateSubscriptionCaches(userID, groupID int64) error
	})
	if !ok {
		return
	}
	seen := make(map[int64]struct{}, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		if _, exists := seen[groupID]; exists {
			continue
		}
		seen[groupID] = struct{}{}
		if err := invalidator.invalidateSubscriptionCaches(userID, groupID); err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to invalidate assigned subscription cache: user_id=%d group_id=%d err=%v", userID, groupID, err)
		}
	}
}

func firstBindProviderGrantInsertSQL(client *dbent.Client) string {
	switch client.Driver().Dialect() {
	case dialect.MySQL:
		return `INSERT IGNORE INTO user_provider_default_grants (user_id, provider_type, grant_reason)
VALUES (?, ?, ?)`
	case dialect.SQLite:
		return `INSERT OR IGNORE INTO user_provider_default_grants (user_id, provider_type, grant_reason)
VALUES (?, ?, ?)`
	default:
		return `INSERT INTO user_provider_default_grants (user_id, provider_type, grant_reason)
VALUES (?, ?, ?)
ON CONFLICT (user_id, provider_type, grant_reason) DO NOTHING`
	}
}
