//go:build unit

package service

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestClassifyOpenAITransportError pins which transport-level upstream failures
// are "persistent" (retrying the same proxy/account is pointless — evict + alert)
// versus "transient" (a blip — fail over to a healthy account but do not evict).
//
// The motivating incident: a SOCKS5 proxy whose credentials expired returned
// `username/password authentication failed`, yet the account kept being scheduled.
func TestClassifyOpenAITransportError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		persistent bool
	}{
		// Durable — config/credential/routing problems. Retrying same proxy won't help.
		{"socks5 proxy credential rejected", errors.New(`Post "https://chatgpt.com/backend-api/codex/responses": socks connect tcp 85.255.176.68:12324->chatgpt.com:443: username/password authentication failed`), true},
		{"proxy connection refused", errors.New(`proxyconnect tcp: dial tcp 1.2.3.4:1080: connect: connection refused`), true},
		{"no route to host", errors.New(`dial tcp 1.2.3.4:443: connect: no route to host`), true},
		{"dns resolution failure", errors.New(`dial tcp: lookup proxy.example.com: no such host`), true},
		{"network unreachable", errors.New(`dial tcp 1.2.3.4:443: connect: network is unreachable`), true},

		// Transient — a temporary blip. Fail over, but do NOT evict the account.
		{"client timeout", errors.New(`Post "https://chatgpt.com/...": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`), false},
		{"i/o timeout", errors.New(`dial tcp 1.2.3.4:443: i/o timeout`), false},
		{"connection reset by peer", errors.New(`read tcp 10.0.0.1:5->2.2.2.2:443: read: connection reset by peer`), false},
		{"unexpected eof", errors.New(`unexpected EOF`), false},
		{"broken pipe", errors.New(`write tcp 10.0.0.1:5->2.2.2.2:443: write: broken pipe`), false},

		{"nil error", nil, false},

		// ── Typed-error cases ──────────────────────────────────────────────
		// ECONNREFUSED wrapped in the canonical net.OpError shape Go produces.
		{
			"ECONNREFUSED via net.OpError",
			&net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
			},
			true,
		},
		// Bare syscall error (errors.Is traverses the chain).
		{"ECONNREFUSED bare", syscall.ECONNREFUSED, true},
		{"EHOSTUNREACH bare", syscall.EHOSTUNREACH, true},
		{"ENETUNREACH bare", syscall.ENETUNREACH, true},

		// *net.DNSError with IsNotFound — permanent DNS lookup failure.
		{
			"DNS not found (IsNotFound=true)",
			&net.DNSError{Err: "no such host", Name: "proxy.example.com", IsNotFound: true},
			true,
		},
		// *net.DNSError with IsNotFound=false — transient DNS timeout (not persistent).
		{
			"DNS timeout (IsNotFound=false)",
			&net.DNSError{Err: "i/o timeout", Name: "proxy.example.com", IsTimeout: true},
			false,
		},

		// context.Canceled — client gone; NOT classified as persistent.
		{"context.Canceled", context.Canceled, false},
		// context.DeadlineExceeded — slow upstream; NOT persistent.
		{"context.DeadlineExceeded", context.DeadlineExceeded, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyOpenAITransportError(tc.err).Persistent
			if got != tc.persistent {
				t.Fatalf("classifyOpenAITransportError(%q).Persistent = %v, want %v", errString(tc.err), got, tc.persistent)
			}
		})
	}
}

func TestClassifyOpenAITransportError_ShortBackoff(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"connection refused", syscall.ECONNREFUSED, true},
		{"network unreachable", syscall.ENETUNREACH, true},
		{"timeout", context.DeadlineExceeded, true},
		{"proxy credentials rejected", errors.New("proxy authentication required"), false},
		{"connection reset", errors.New("connection reset by peer"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, classifyOpenAITransportError(tc.err).ShortBackoff)
		})
	}
}

func TestOpenAITransportNetworkBackoffStepsAndReset(t *testing.T) {
	svc := &OpenAIGatewayService{}
	const accountID int64 = 2756
	base := time.Unix(1000, 0)

	require.Equal(t, 5*time.Second, svc.nextOpenAITransportNetworkBackoff(accountID, base))
	require.Equal(t, 10*time.Second, svc.nextOpenAITransportNetworkBackoff(accountID, base.Add(5*time.Second)))
	require.Equal(t, 15*time.Second, svc.nextOpenAITransportNetworkBackoff(accountID, base.Add(15*time.Second)))
	require.Equal(t, 15*time.Second, svc.nextOpenAITransportNetworkBackoff(accountID, base.Add(30*time.Second)))

	// A quiet period or an explicit scheduling-block clear starts again at 5s.
	require.Equal(t, 5*time.Second, svc.nextOpenAITransportNetworkBackoff(accountID, base.Add(61*time.Second)))
	svc.resetOpenAITransportNetworkBackoff(accountID)
	require.Equal(t, 5*time.Second, svc.nextOpenAITransportNetworkBackoff(accountID, base.Add(62*time.Second)))
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
