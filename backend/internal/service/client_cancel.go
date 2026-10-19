package service

import (
	"context"
	"errors"
)

func isClientCanceledTransportError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return ctx == nil || ctx.Err() == context.Canceled
	}
	return errors.Is(err, context.DeadlineExceeded) && ctx != nil && ctx.Err() == context.DeadlineExceeded
}
