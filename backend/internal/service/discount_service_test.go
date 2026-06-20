package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscountServiceApplyThresholdDiscount(t *testing.T) {
	svc := NewDiscountService()

	t.Run("no rules", func(t *testing.T) {
		got := svc.ApplyThresholdDiscount(100, nil)
		require.Equal(t, 0.0, got.DiscountAmount)
		require.Equal(t, 100.0, got.AfterDiscount)
	})

	t.Run("below threshold", func(t *testing.T) {
		got := svc.ApplyThresholdDiscount(30, []DiscountRule{{Threshold: 50, Type: "rate", Value: 0.9, Enabled: true}})
		require.Equal(t, 0.0, got.DiscountAmount)
		require.Equal(t, 30.0, got.AfterDiscount)
	})

	t.Run("highest tier", func(t *testing.T) {
		got := svc.ApplyThresholdDiscount(150, []DiscountRule{
			{Threshold: 50, Type: "rate", Value: 0.95, Enabled: true},
			{Threshold: 100, Type: "rate", Value: 0.90, Enabled: true},
		})
		require.Equal(t, 15.0, got.DiscountAmount)
		require.Equal(t, 135.0, got.AfterDiscount)
		require.NotNil(t, got.AppliedRule)
		require.Equal(t, 100.0, got.AppliedRule.Threshold)
	})

	t.Run("fixed reduce", func(t *testing.T) {
		got := svc.ApplyThresholdDiscount(200, []DiscountRule{{Threshold: 200, Type: "reduce", Value: 30, Enabled: true}})
		require.Equal(t, 30.0, got.DiscountAmount)
		require.Equal(t, 170.0, got.AfterDiscount)
	})

	t.Run("mixed types", func(t *testing.T) {
		got := svc.ApplyThresholdDiscount(120, []DiscountRule{
			{Threshold: 50, Type: "rate", Value: 0.95, Enabled: true},
			{Threshold: 100, Type: "reduce", Value: 20, Enabled: true},
		})
		require.Equal(t, 20.0, got.DiscountAmount)
		require.Equal(t, 100.0, got.AfterDiscount)
	})

	t.Run("floor zero", func(t *testing.T) {
		got := svc.ApplyThresholdDiscount(20, []DiscountRule{{Threshold: 20, Type: "reduce", Value: 50, Enabled: true}})
		require.Equal(t, 20.0, got.DiscountAmount)
		require.Equal(t, 0.0, got.AfterDiscount)
	})

	t.Run("disabled ignored", func(t *testing.T) {
		got := svc.ApplyThresholdDiscount(100, []DiscountRule{{Threshold: 50, Type: "reduce", Value: 20, Enabled: false}})
		require.Equal(t, 0.0, got.DiscountAmount)
		require.Equal(t, 100.0, got.AfterDiscount)
	})
}
