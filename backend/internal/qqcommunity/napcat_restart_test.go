package qqcommunity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestNapCatRestartProtectsLoginAndDoesNotRetryUncertainDelivery(t *testing.T) {
	for _, tc := range []struct {
		name, phase      string
		accepted, reject bool
		want             error
		calls            int32
	}{
		{name: "cached online can be restarted", phase: "ready", calls: 1},
		{name: "offline can be restarted", phase: "offline", calls: 1},
		{name: "scan accepted", accepted: true, want: ErrNapCatLoginInProgress},
		{name: "scan phase without accepted flag", phase: "qrcode_scanned", want: ErrNapCatLoginInProgress},
		{name: "initializing", phase: "initializing", want: ErrNapCatLoginInProgress},
		{name: "reconnecting", phase: "reconnecting", want: ErrNapCatLoginInProgress},
		{name: "uncertain response", phase: "ready", reject: true, want: ErrNapCatRestartUnconfirmed, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var data any
				switch r.URL.Path {
				case "/api/auth/login":
					data = map[string]any{"Credential": "private-credential"}
				case "/api/QQLogin/CheckLoginStatus":
					data = map[string]any{"isLogin": true, "loginPhase": tc.phase, "qrLoginAccepted": tc.accepted}
				case "/api/QQLogin/RestartNapCat":
					calls.Add(1)
					require.Equal(t, "Bearer private-credential", r.Header.Get("Authorization"))
					if tc.reject {
						_, _ = w.Write([]byte(`{"code":-1,"message":"private-credential secret trace"}`))
						return
					}
					data = map[string]any{"message": "restart requested"}
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
			}))
			defer srv.Close()
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			s := New(db, nil, false, nil)
			require.NoError(t, s.ConfigureNapCatLogin(srv.URL, "private-token"))
			mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"app_id":"1849138657"}`))
			out, err := s.RestartNapCatLogin(context.Background(), "1849138657", true)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				require.NotContains(t, err.Error(), "private-")
			} else {
				require.NoError(t, err)
				require.True(t, out.Accepted)
				require.True(t, out.RestartAvailableAt.After(time.Now()))
			}
			if tc.calls > 0 {
				mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"app_id":"1849138657"}`))
				_, err = s.RestartNapCatLogin(context.Background(), "1849138657", true)
				require.ErrorIs(t, err, ErrNapCatRestartCooldown)
			}
			require.Equal(t, tc.calls, calls.Load())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestNapCatRestartRequiresConfirmationAndCurrentAccount(t *testing.T) {
	s := New(nil, nil, false, nil)
	_, err := s.RestartNapCatLogin(context.Background(), "1849138657", false)
	require.ErrorIs(t, err, ErrNapCatRestartConfirmation)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s = New(db, nil, false, nil)
	mock.ExpectQuery("SELECT value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"app_id":"different-account"}`))
	_, err = s.RestartNapCatLogin(context.Background(), "1849138657", true)
	require.ErrorIs(t, err, ErrNapCatRestartConfirmation)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNapCatConcurrentRestartSendsOnlyOnce(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{}
		switch r.URL.Path {
		case "/api/auth/login":
			data["Credential"] = "credential"
		case "/api/QQLogin/CheckLoginStatus":
			data["loginPhase"] = "ready"
		case "/api/QQLogin/RestartNapCat":
			calls.Add(1)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}))
	defer srv.Close()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	require.NoError(t, s.ConfigureNapCatLogin(srv.URL, "token"))
	for i := 0; i < 2; i++ {
		mock.ExpectQuery("SELECT value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"app_id":"1849138657"}`))
	}
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.RestartNapCatLogin(context.Background(), "1849138657", true)
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	success := 0
	for err := range errors {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, ErrNapCatRestartCooldown)
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, int32(1), calls.Load())
	require.NoError(t, mock.ExpectationsWereMet())
}
