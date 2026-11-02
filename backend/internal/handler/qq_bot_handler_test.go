package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/qqbot"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type qqHandlerRepoStub struct {
	qqbot.Repository
	userID int64
	qq     string
}

func (r *qqHandlerRepoStub) LoadConfig(context.Context) (string, error) {
	b, _ := json.Marshal(qqbot.DefaultConfig())
	return string(b), nil
}
func (r *qqHandlerRepoStub) BindingStatus(_ context.Context, id int64) (*qqbot.BindingStatus, error) {
	r.userID = id
	return &qqbot.BindingStatus{}, nil
}
func (r *qqHandlerRepoStub) ConfirmBinding(_ context.Context, id int64, qq string) (*qqbot.Binding, error) {
	r.userID = id
	r.qq = qq
	return &qqbot.Binding{UserID: id, QQ: qq}, nil
}
func TestQQBotUserRoutesUseOnlyAuthenticatedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &qqHandlerRepoStub{}
	svc := qqbot.NewService(repo, nil, nil, "http://127.0.0.1:1")
	defer svc.Stop()
	h := NewQQBotHandler(svc)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/user/qq-bot?user_id=99", nil)
	h.BindingStatus(ctx)
	require.Equal(t, 401, recorder.Code)
	require.Zero(t, repo.userID)
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/user/qq-bot?user_id=99", nil)
	ctx.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	h.BindingStatus(ctx)
	require.Equal(t, 200, recorder.Code)
	require.EqualValues(t, 42, repo.userID)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/user/qq-bot/confirm", strings.NewReader(`{"qq":"777777","user_id":99}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	h.Confirm(ctx)
	require.Equal(t, 200, recorder.Code)
	require.EqualValues(t, 42, repo.userID)
	require.Equal(t, "777777", repo.qq)
}
