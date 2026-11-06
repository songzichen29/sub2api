package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/qqcommunity"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserCheckinRequiresAuthentication(t *testing.T) {
	h := NewQQCommunityHandler(nil)
	for _, handle := range []gin.HandlerFunc{h.UserCheckin, h.ClaimUserCheckin} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		handle(c)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	}
}

func TestUserCheckinIgnoresForgedUserAndAmount(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := NewQQCommunityHandler(qqcommunity.New(db, nil, false, nil))
	date := time.Now().In(time.FixedZone("Beijing", 8*3600)).Format(time.DateOnly)
	m.ExpectBegin()
	m.ExpectExec("SELECT pg_advisory").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectQuery("SELECT value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"app_id":"bot"}`))
	for i := 0; i < 2; i++ {
		m.ExpectExec("SELECT pg_advisory").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	m.ExpectQuery("SELECT id FROM users").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	m.ExpectQuery("SELECT amount,app_id FROM qqc_rewards WHERE user_id").WithArgs(int64(42), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"amount", "app_id"}).AddRow(.12, "bot"))
	m.ExpectQuery("SELECT reward_type").WillReturnError(sql.ErrNoRows)
	m.ExpectQuery("SELECT chances").WillReturnError(sql.ErrNoRows)
	m.ExpectCommit()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/user/checkin?user_id=99", strings.NewReader(`{"day":"`+date+`","user_id":99,"amount":1000}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	h.ClaimUserCheckin(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Contains(t, w.Body.String(), `"amount":0.12`)
	require.Contains(t, w.Body.String(), `"already_checked_in":true`)
	require.NoError(t, m.ExpectationsWereMet())
}

func TestUserCheckinRejectsMalformedBodyAndMonth(t *testing.T) {
	h := NewQQCommunityHandler(qqcommunity.New(nil, nil, false, nil))
	for _, body := range []string{`{}`, `{"day":"2026-02-30"}`, `{"day":"tomorrow"}`} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/user/checkin", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		h.ClaimUserCheckin(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/user/checkin?month=2026-13", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	h.UserCheckin(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
