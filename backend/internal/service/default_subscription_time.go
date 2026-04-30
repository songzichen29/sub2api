package service

import (
	"strings"
	"time"
)

func parseDefaultSubscriptionTime(raw *string) *time.Time {
	if raw == nil {
		return nil
	}
	text := strings.TrimSpace(*raw)
	if text == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return nil
	}
	return &t
}
