package httputil

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

type gatewayTimingKey struct{}

// gatewayTiming 只保存阶段统计，不持有请求内容或 Gin Context。
type gatewayTiming struct {
	mu       sync.Mutex
	started  time.Time
	fields   []zap.Field
	attempts int
}

func WithGatewayTiming(ctx context.Context) context.Context {
	return context.WithValue(ctx, gatewayTimingKey{}, &gatewayTiming{started: time.Now()})
}

// RecordGatewayTiming 把阶段结果附加到现有 access 日志，不增加逐块日志。
func RecordGatewayTiming(ctx context.Context, fields ...zap.Field) {
	timing, ok := ctx.Value(gatewayTimingKey{}).(*gatewayTiming)
	if !ok {
		return
	}
	timing.mu.Lock()
	defer timing.mu.Unlock()
	timing.fields = append(timing.fields, fields...)
}

func GatewayTimingFields(ctx context.Context) []zap.Field {
	timing, ok := ctx.Value(gatewayTimingKey{}).(*gatewayTiming)
	if !ok {
		return nil
	}
	timing.mu.Lock()
	defer timing.mu.Unlock()
	return append([]zap.Field(nil), timing.fields...)
}
