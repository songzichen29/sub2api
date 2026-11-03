package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/qqcommunity"
)

func provideQQCommunityService(db *sql.DB, encryptor SecretEncryptor, cfg *config.Config, invalidator APIKeyAuthCacheInvalidator, billing *BillingCacheService, email *EmailService, settings *SettingService, users UserRepository, monitor *ChannelMonitorV2Service, groups GroupRepository, channels ChannelRepository, channelService *ChannelService, priceResolver *ModelPricingResolver, billingService *BillingService, rates UserGroupRateRepository, active *ChannelMonitorService, quality *ModelQualityService) (*qqcommunity.Service, error) {
	s := qqcommunity.New(db, encryptor, cfg.Totp.EncryptionKeyConfigured, func(ctx context.Context, user int64) {
		if invalidator != nil {
			invalidator.InvalidateAuthCacheByUserID(ctx, user)
		}
		if billing != nil {
			_ = billing.InvalidateUserBalance(ctx, user)
		}
	})
	s.SetBindingEmailDelivery(&qqBotBindingEmail{email: email, settings: settings, users: users})
	napcatToken := cfg.QQBotNapCat.WebUIToken
	if cfg.QQBotNapCat.WebUITokenFile != "" {
		raw, err := os.ReadFile(cfg.QQBotNapCat.WebUITokenFile)
		if err != nil {
			return nil, fmt.Errorf("read NapCat WebUI credential file: %w", err)
		}
		napcatToken = strings.TrimSpace(string(raw))
	}
	if err := s.ConfigureNapCatLogin(cfg.QQBotNapCat.WebUIURL, napcatToken); err != nil {
		return nil, err
	}
	status := &qqBotChannelStatus{monitor: monitor, settings: settings, groups: groups}
	s.SetChannelStatusProvider(status)
	if err := s.ConfigureReplyPages(fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port), &qqBotMonitorPages{active: active, quality: quality, status: status}); err != nil {
		return nil, err
	}
	s.SetHealthSnapshotProvider(status)
	s.SetModelPriceProvider(&qqBotModelPrices{groups: groups, users: users, rates: rates, channels: channelService, billing: billingService, resolver: priceResolver})
	s.SetPriceSnapshotProvider(&qqBotPriceSource{channels: channels, groups: groups})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.InitializeLegacy(ctx, fmt.Sprintf("http://127.0.0.1:%d/v1", cfg.Server.Port)); err != nil {
		return nil, fmt.Errorf("migrate QQ community: %w", err)
	}
	s.Start()
	return s, nil
}

type qqBotBindingEmail struct {
	email    *EmailService
	settings *SettingService
	users    UserRepository
}

func (d *qqBotBindingEmail) FrontendURL(ctx context.Context) string {
	return d.settings.GetFrontendURL(ctx)
}

func (d *qqBotBindingEmail) LookupAccount(ctx context.Context, address string) (int64, string, error) {
	u, err := d.users.GetByEmail(ctx, address)
	if err != nil {
		return 0, "", err
	}
	if u == nil || !u.IsActive() || u.DeletedAt != nil {
		return 0, "", ErrUserNotFound
	}
	return u.ID, u.Email, nil
}

func (d *qqBotBindingEmail) SendEmail(ctx context.Context, to, subject, body string) error {
	return d.email.SendEmail(ctx, to, subject, body)
}
