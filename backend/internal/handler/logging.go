package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func requestLogger(c *gin.Context, component string, fields ...zap.Field) *zap.Logger {
	base := logger.L()
	if c != nil && c.Request != nil {
		base = logger.FromContext(c.Request.Context())
	}

	if component != "" {
		fields = append([]zap.Field{zap.String("component", component)}, fields...)
	}
	return base.With(fields...)
}

// bindRequestLogger makes request-scoped fields available to downstream services.
// Streaming work runs below the handler, so this keeps its progress logs correlated
// with the authenticated user, API key, and request identifiers.
func bindRequestLogger(c *gin.Context, l *zap.Logger) {
	if c == nil || c.Request == nil || l == nil {
		return
	}
	c.Request = c.Request.WithContext(logger.IntoContext(c.Request.Context(), l))
}
