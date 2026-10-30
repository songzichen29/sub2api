package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadModelQualityJudge(t *testing.T) {
	for _, tc := range []struct{ name, keyEnv, modelEnv, key, model string }{
		{"defaults", "", "", "测试降智", "gpt-6.1-sol"},
		{"environment", "备用裁判", "gpt-6.1-sol", "备用裁判", "gpt-6.1-sol"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("MODEL_QUALITY_JUDGE_KEY_NAME", tc.keyEnv)
			t.Setenv("MODEL_QUALITY_JUDGE_MODEL", tc.modelEnv)
			cfg, err := Load()
			require.NoError(t, err)
			require.Equal(t, tc.key, cfg.ModelQuality.JudgeKeyName)
			require.Equal(t, tc.model, cfg.ModelQuality.JudgeModel)
		})
	}
}
