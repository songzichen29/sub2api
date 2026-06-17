package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandler_GetSettings_IncludesAffiliateKindSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyAffiliateEnabled:                "true",
			service.SettingKeyAffiliateRebateRate:             "20",
			service.SettingKeyAffiliateRechargeEnabled:        "false",
			service.SettingKeyAffiliateSubscriptionEnabled:    "true",
			service.SettingKeyAffiliateRechargeRebateRate:     "12.5",
			service.SettingKeyAffiliateSubscriptionRebateRate: "7.5",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)

	handler.GetSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["affiliate_recharge_enabled"])
	require.Equal(t, true, data["affiliate_subscription_enabled"])
	require.Equal(t, 12.5, data["affiliate_recharge_rebate_rate"])
	require.Equal(t, 7.5, data["affiliate_subscription_rebate_rate"])
}

func TestSettingHandler_UpdateSettings_PreservesOmittedAffiliateKindSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyPromoCodeEnabled:                "true",
			service.SettingKeyAffiliateEnabled:                "true",
			service.SettingKeyAffiliateRebateRate:             "20",
			service.SettingKeyAffiliateRechargeEnabled:        "false",
			service.SettingKeyAffiliateSubscriptionEnabled:    "true",
			service.SettingKeyAffiliateRechargeRebateRate:     "12.5",
			service.SettingKeyAffiliateSubscriptionRebateRate: "7.5",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled": true,
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyAffiliateRechargeEnabled])
	require.Equal(t, "true", repo.values[service.SettingKeyAffiliateSubscriptionEnabled])
	require.Equal(t, "12.50000000", repo.values[service.SettingKeyAffiliateRechargeRebateRate])
	require.Equal(t, "7.50000000", repo.values[service.SettingKeyAffiliateSubscriptionRebateRate])

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["affiliate_recharge_enabled"])
	require.Equal(t, true, data["affiliate_subscription_enabled"])
	require.Equal(t, 12.5, data["affiliate_recharge_rebate_rate"])
	require.Equal(t, 7.5, data["affiliate_subscription_rebate_rate"])
}

func TestSettingHandler_UpdateSettings_PersistsAffiliateKindSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyPromoCodeEnabled:                "true",
			service.SettingKeyAffiliateEnabled:                "true",
			service.SettingKeyAffiliateRebateRate:             "20",
			service.SettingKeyAffiliateRechargeEnabled:        "true",
			service.SettingKeyAffiliateSubscriptionEnabled:    "false",
			service.SettingKeyAffiliateRechargeRebateRate:     "20",
			service.SettingKeyAffiliateSubscriptionRebateRate: "20",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled":                 true,
		"affiliate_recharge_enabled":         false,
		"affiliate_subscription_enabled":     true,
		"affiliate_recharge_rebate_rate":     33.3,
		"affiliate_subscription_rebate_rate": 44.4,
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyAffiliateRechargeEnabled])
	require.Equal(t, "true", repo.values[service.SettingKeyAffiliateSubscriptionEnabled])
	require.Equal(t, "33.30000000", repo.values[service.SettingKeyAffiliateRechargeRebateRate])
	require.Equal(t, "44.40000000", repo.values[service.SettingKeyAffiliateSubscriptionRebateRate])

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["affiliate_recharge_enabled"])
	require.Equal(t, true, data["affiliate_subscription_enabled"])
	require.Equal(t, 33.3, data["affiliate_recharge_rebate_rate"])
	require.Equal(t, 44.4, data["affiliate_subscription_rebate_rate"])
}

func TestDiffSettings_IncludesAffiliateKindSettings(t *testing.T) {
	changed := diffSettings(
		&service.SystemSettings{
			AffiliateRechargeEnabled:        true,
			AffiliateSubscriptionEnabled:    false,
			AffiliateRechargeRebateRate:     20,
			AffiliateSubscriptionRebateRate: 20,
		},
		&service.SystemSettings{
			AffiliateRechargeEnabled:        false,
			AffiliateSubscriptionEnabled:    true,
			AffiliateRechargeRebateRate:     33.3,
			AffiliateSubscriptionRebateRate: 44.4,
		},
		nil,
		nil,
		UpdateSettingsRequest{},
	)

	require.Contains(t, changed, "affiliate_recharge_enabled")
	require.Contains(t, changed, "affiliate_subscription_enabled")
	require.Contains(t, changed, "affiliate_recharge_rebate_rate")
	require.Contains(t, changed, "affiliate_subscription_rebate_rate")
}
