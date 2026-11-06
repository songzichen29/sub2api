package handler

import (
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

func TestUserLotteryRequiresAuthentication(t *testing.T) {
	h := NewQQCommunityHandler(nil)
	for _, handle := range []gin.HandlerFunc{h.UserLottery, h.UserLotteryDraws, h.UserLotteryMembership, h.UserLotterySpin} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		handle(c)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	}
}

func TestUserLotterySpinUsesOnlyAuthenticatedIdentity(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := NewQQCommunityHandler(qqcommunity.New(db, nil, false, nil))
	m.ExpectBegin()
	for i := 0; i < 3; i++ {
		m.ExpectExec("SELECT pg_advisory").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	m.ExpectQuery("SELECT id FROM users").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	m.ExpectQuery("SELECT result_text FROM qqc_lottery_draws").WithArgs("web:lottery:42:1234567890123456", int64(42)).WillReturnRows(sqlmock.NewRows([]string{"result_text"}).AddRow(`{"request_id":"1234567890123456","prize":{"name":"own prize"},"remaining":0}`))
	m.ExpectCommit()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"request_id":"1234567890123456","user_id":99,"prize_index":9}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	h.UserLotterySpin(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Contains(t, w.Body.String(), "own prize")
	require.NoError(t, m.ExpectationsWereMet())
}
func TestUserLotteryHistoryCannotOverrideAuthenticatedUser(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := NewQQCommunityHandler(qqcommunity.New(db, nil, false, nil))
	m.ExpectBegin()
	m.ExpectQuery(`SELECT COUNT\(\*\) FROM qqc_lottery_draws WHERE user_id=\$1`).WithArgs(int64(42), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	m.ExpectQuery(`SELECT id,prize_name,amount,config_snapshot,created_at`).WithArgs(int64(42), sqlmock.AnyArg(), 20, 0).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "amount", "snapshot", "at"}).AddRow(1, "own prize", 1, `{}`, time.Now()))
	m.ExpectCommit()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/user/qqbot/lottery/draws?user_id=99&search=someone@example.test", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	h.UserLotteryDraws(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Contains(t, w.Body.String(), "own prize")
	require.NoError(t, m.ExpectationsWereMet())
}
