package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// openAITransportErrorTempUnschedDuration is reserved for durable credential or
// configuration failures. Recoverable network failures use the short stepped
// backoff below instead.
const openAITransportErrorTempUnschedDuration = 10 * time.Minute

var openAITransportNetworkBackoffSteps = [...]time.Duration{
	5 * time.Second,
	10 * time.Second,
	15 * time.Second,
}

// A quiet period resets the stepped network backoff, so an isolated outage does
// not make a much later failure start at the maximum delay.
const openAITransportNetworkBackoffResetAfter = 30 * time.Second

type openAITransportBackoffState struct {
	failures    int
	lastFailure time.Time
}

// openAITransportFailoverBody is the OpenAI-format error body attached to the
// failover error for a transport-level failure. Kept identical to the legacy
// inline 502 body so the client-visible payload is unchanged if failover is
// ultimately exhausted.
var openAITransportFailoverBody = []byte(`{"error":{"type":"upstream_error","message":"Upstream request failed"}}`)

// openAITransportErrorClass describes how to react to a transport-level upstream
// failure — i.e. the HTTP round-trip never completed (proxy / DNS / TCP / TLS
// error, no HTTP status code received).
type openAITransportErrorClass struct {
	// Persistent marks failures where retrying the same proxy/account is
	// pointless: expired or rejected proxy credentials, a dead proxy endpoint,
	// or DNS/routing failure. Such accounts should be temporarily unscheduled
	// (and alerted on) instead of being repeatedly scheduled into hard failures.
	Persistent bool
	// ShortBackoff marks network-availability failures that should temporarily
	// leave scheduling for only 5/10/15 seconds. It may be true together with
	// Persistent to preserve the existing durable-fault classification used by
	// metrics and tests.
	ShortBackoff bool
}

// Credential failures retain the longer cooldown because an immediate retry
// cannot recover without a configuration change.
var openAIPersistentTransportErrorMarkers = []string{
	"authentication failed",         // SOCKS5 RFC1929 / proxy credentials rejected (expired account)
	"proxy authentication required", // HTTP proxy 407
}

// Network availability failures recover quickly during local proxy/container
// restarts, so they use the short stepped backoff.
var openAIShortBackoffTransportErrorMarkers = []string{
	"connection refused", // proxy/upstream endpoint down
	"no route to host",
	"network is unreachable",
	"no such host", // DNS resolution failure (bad/expired proxy hostname)
}

var openAIShortBackoffTimeoutMarkers = []string{
	"i/o timeout",
	"client.timeout exceeded",
	"context deadline exceeded",
}

// classifyOpenAITransportError separates long-lived credential/configuration
// failures from recoverable network failures. The latter still fail over, but
// only pause scheduling for the short stepped backoff.
//
// Motivating incident: a SOCKS5 proxy whose subscription lapsed returned
// `username/password authentication failed`; the account was nonetheless
// rescheduled on every request, hard-failing users with 502s.
//
// Classification strategy (mirrors sanitizeStreamError in gateway_service.go):
//  1. Typed-error checks first (syscall constants, *net.DNSError) — portable and
//     unambiguous.
//  2. String-marker fallback for errors that have no typed form (e.g. the plain
//     string returned by golang.org/x/net/proxy for SOCKS5 credential rejection).
//     The network-layer string markers ("connection refused", "no route to host",
//     "network is unreachable", "no such host") are kept as a cross-platform safety
//     net even though the typed checks should cover them on modern Go+Linux.
func classifyOpenAITransportError(err error) openAITransportErrorClass {
	if err == nil {
		return openAITransportErrorClass{}
	}

	// — Typed checks (preferred) ——————————————————————————————————————————————
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) {
		return openAITransportErrorClass{Persistent: true, ShortBackoff: true}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return openAITransportErrorClass{Persistent: true, ShortBackoff: true}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return openAITransportErrorClass{ShortBackoff: true}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return openAITransportErrorClass{ShortBackoff: true}
	}

	// — String-marker fallback ————————————————————————————————————————————————
	msg := strings.ToLower(err.Error())
	for _, marker := range openAIPersistentTransportErrorMarkers {
		if strings.Contains(msg, marker) {
			return openAITransportErrorClass{Persistent: true}
		}
	}
	for _, marker := range openAIShortBackoffTransportErrorMarkers {
		if strings.Contains(msg, marker) {
			return openAITransportErrorClass{Persistent: true, ShortBackoff: true}
		}
	}
	for _, marker := range openAIShortBackoffTimeoutMarkers {
		if strings.Contains(msg, marker) {
			return openAITransportErrorClass{ShortBackoff: true}
		}
	}
	return openAITransportErrorClass{}
}

func (s *OpenAIGatewayService) nextOpenAITransportNetworkBackoff(accountID int64, now time.Time) time.Duration {
	if s == nil || accountID <= 0 {
		return openAITransportNetworkBackoffSteps[0]
	}

	s.openaiTransportBackoffMu.Lock()
	defer s.openaiTransportBackoffMu.Unlock()

	if s.openaiTransportBackoffByAccount == nil {
		s.openaiTransportBackoffByAccount = make(map[int64]openAITransportBackoffState)
	}
	state := s.openaiTransportBackoffByAccount[accountID]
	if state.lastFailure.IsZero() || now.Sub(state.lastFailure) > openAITransportNetworkBackoffResetAfter {
		state.failures = 0
	}
	state.failures++
	state.lastFailure = now
	s.openaiTransportBackoffByAccount[accountID] = state

	step := state.failures - 1
	if step >= len(openAITransportNetworkBackoffSteps) {
		step = len(openAITransportNetworkBackoffSteps) - 1
	}
	return openAITransportNetworkBackoffSteps[step]
}

func (s *OpenAIGatewayService) resetOpenAITransportNetworkBackoff(accountID int64) {
	if s == nil || accountID <= 0 {
		return
	}
	s.openaiTransportBackoffMu.Lock()
	delete(s.openaiTransportBackoffByAccount, accountID)
	s.openaiTransportBackoffMu.Unlock()
}

// handleOpenAIUpstreamTransportError handles a transport-level upstream failure
// (Do/DoWithTLS returned a non-HTTP error: proxy/DNS/TCP/TLS). It:
//  1. records the failure in Ops error logs (status 0, kind=request_error);
//  2. for durable faults (expired/rejected proxy creds, dead proxy, DNS/routing)
//     temporarily unschedules the account (DB + in-memory) and logs a stable
//     warn event that alert rules can key on;
//  3. returns an error that is *UpstreamFailoverError (so the handler fails over
//     to a healthy account) for all non-canceled errors, or a plain error for
//     context.Canceled (client gone — no failover, no eviction).
//
// It deliberately does NOT write to the response: the handler owns the response
// (failover, or a protocol-correct error once failover is exhausted).
//
// passthrough tags the Ops error event for the OpenAI passthrough forward path.
func (s *OpenAIGatewayService) handleOpenAIUpstreamTransportError(ctx context.Context, c *gin.Context, account *Account, err error, passthrough bool) error {
	safeErr := sanitizeUpstreamErrorMessage(err.Error())
	setOpsUpstreamError(c, 0, safeErr, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: 0,
		Passthrough:        passthrough,
		Kind:               "request_error",
		Message:            safeErr,
	})

	// Client disconnected: do NOT fail over to another account and do NOT evict
	// this one — the upstream never had a chance to exhibit a fault.
	if errors.Is(err, context.Canceled) {
		return err
	}

	// Transport attempt reached the network path; count as Ollama Cloud activity.
	if s != nil {
		scheduleOllamaCloudUsageActivity(s.deferredService, account)
	}

	errClass := classifyOpenAITransportError(err)
	if errClass.ShortBackoff {
		backoff := s.nextOpenAITransportNetworkBackoff(account.ID, time.Now())
		s.tempUnscheduleOpenAITransportError(ctx, account, safeErr, backoff)
	} else if errClass.Persistent {
		// Authentication/configuration failures still need the original long
		// cooldown; they cannot recover merely by waiting a few seconds.
		s.resetOpenAITransportNetworkBackoff(account.ID)
		s.tempUnscheduleOpenAITransportError(ctx, account, safeErr, openAITransportErrorTempUnschedDuration)
	}

	return &UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: openAITransportFailoverBody,
	}
}

// tempUnscheduleOpenAITransportError marks an account temporarily unschedulable
// after a durable transport failure, both persistently (DB, survives restart)
// and in-memory (immediate scheduler effect before the DB/account cache propagates).
//
// Log semantics:
//   - "openai.account_temp_unscheduled_transport" — emitted ONLY after a
//     successful DB write (both in-memory + persisted).
//   - "openai.account_temp_unscheduled_transport_memory_only" — emitted when
//     accountRepo is nil (in-memory only; no persistence).
//   - "openai.account_temp_unscheduled_transport_failed" — DB write attempted
//     but returned an error.
func (s *OpenAIGatewayService) tempUnscheduleOpenAITransportError(ctx context.Context, account *Account, safeErr string, duration time.Duration) {
	if s == nil || account == nil {
		return
	}
	if duration <= 0 {
		duration = openAITransportErrorTempUnschedDuration
	}
	until := time.Now().Add(duration)
	reason := "upstream transport error (proxy/network): " + safeErr

	// Immediate in-memory block (honoured by the scheduler at selection time),
	// effective even if the DB write below fails or the account cache lags.
	s.BlockAccountScheduling(account, until, "transport_error")

	if s.accountRepo == nil {
		// No DB configured — block is in-memory only; emit a distinct event so
		// operators are not misled into thinking the block survived a restart.
		logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
			"openai.account_temp_unscheduled_transport_memory_only",
			zap.Int64("account_id", account.ID),
			zap.String("account_name", account.Name),
			zap.String("platform", account.Platform),
			zap.Time("until", until),
			zap.Duration("cooldown", duration),
			zap.String("reason", reason),
		)
		return
	}

	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAccountStateUpdateTimeout)
	defer cancel()
	if err := s.accountRepo.SetTempUnschedulable(bgCtx, account.ID, until, reason); err != nil {
		logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
			"openai.account_temp_unscheduled_transport_failed",
			zap.Int64("account_id", account.ID),
			zap.Error(err),
		)
		return
	}

	// DB write succeeded: both in-memory and persisted.
	logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
		"openai.account_temp_unscheduled_transport",
		zap.Int64("account_id", account.ID),
		zap.String("account_name", account.Name),
		zap.String("platform", account.Platform),
		zap.Time("until", until),
		zap.Duration("cooldown", duration),
		zap.String("reason", reason),
	)
}
