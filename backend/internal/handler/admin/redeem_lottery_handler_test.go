package admin

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRedeemLotteryGenerateAndExport(t *testing.T) {
	router, adminSvc := setupRedeemExportRouter()
	adminSvc.redeems = []service.RedeemCode{{ID: 7, Code: "LOTTERY-CODE", Type: service.RedeemTypeLotteryChances, Value: 5, Status: service.StatusUnused}}
	router.POST("/api/v1/admin/redeem-codes/generate", NewRedeemHandler(adminSvc, nil).Generate)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/redeem-codes/generate", strings.NewReader(`{"count":1,"type":"lottery_chances","value":5}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"type":"lottery_chances"`)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/redeem-codes/export?type=lottery_chances", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, service.RedeemTypeLotteryChances, adminSvc.lastListRedeemCodes.codeType)
	rows, err := csv.NewReader(strings.NewReader(rec.Body.String())).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "lottery_chances", rows[1][2])
	require.Equal(t, "5", rows[1][3], "chances must not be exported as a currency amount")
}

func TestRedeemLotteryCreateAndRedeemRequestBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"code":"CHANCES","type":"lottery_chances","value":5,"user_id":42}`))
	c.Request.Header.Set("Content-Type", "application/json")
	var req CreateAndRedeemCodeRequest
	require.NoError(t, c.ShouldBindJSON(&req))
	require.Equal(t, service.RedeemTypeLotteryChances, req.Type)
}
