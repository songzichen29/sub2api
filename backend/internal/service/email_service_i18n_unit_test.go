//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmailServiceAuthMessagesUseRequestedLocale(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	emailSvc := NewEmailService(repo, &emailCacheStub{})
	NewNotificationEmailService(repo, emailSvc)

	var subject, body string
	emailSvc.sendFunc = func(_ context.Context, _, gotSubject, gotBody string) error {
		subject = gotSubject
		body = gotBody
		return nil
	}

	require.NoError(t, emailSvc.SendVerifyCode(ctx, "user@example.com", "Sub2API", "zh"))
	require.Contains(t, subject, "邮箱验证码")
	require.Contains(t, body, "您的验证码是")

	require.NoError(t, emailSvc.SendPasswordResetEmail(ctx, "user@example.com", "Sub2API", "https://example.com/reset", "en-US"))
	require.Contains(t, subject, "Password reset request")
	require.Contains(t, body, "We received a request to reset your password")
}
