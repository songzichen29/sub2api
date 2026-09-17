//go:build unit

package admin

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateSettingsPersistsPaymentDiscountRules(t *testing.T) {
	repo := &settingHandlerRepoStub{values: map[string]string{
		service.SettingDiscountRules: `[{"threshold":30,"type":"rate","value":0.99,"enabled":true}]`,
	}}
	settingService := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	paymentConfigService := service.NewPaymentConfigService(nil, repo, nil, nil)
	handler := NewSettingHandler(settingService, nil, nil, nil, paymentConfigService, nil, nil)

	rec := doUpdateSettings(t, handler, map[string]any{
		"payment_discount_rules": []map[string]any{
			{
				"threshold": 30,
				"type":      "rate",
				"value":     0.99,
				"enabled":   false,
			},
		},
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t,
		`[{"threshold":30,"type":"rate","value":0.99,"enabled":false}]`,
		repo.values[service.SettingDiscountRules],
	)
	require.Contains(t, rec.Body.String(), `"enabled":false`)
}
