package handler

import (
	"bytes"
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/qqcommunity"
	"github.com/gin-gonic/gin"
)

func campaignAPIError(c *gin.Context, err error) {
	var rule qqcommunity.CampaignRuleError
	switch {
	case errors.Is(err, qqcommunity.ErrConfigConflict):
		response.Error(c, http.StatusConflict, "配置已变化，请刷新后重试")
	case errors.As(err, &rule):
		response.BadRequest(c, rule.Error())
	default:
		response.InternalError(c, "活动处理失败，请稍后重试")
	}
}
func (h *QQCommunityHandler) ListCodeCampaigns(c *gin.Context) {
	if !requireQQBotAdmin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	out, err := h.bot.CodeCampaigns(c.Request.Context())
	if err != nil {
		campaignAPIError(c, err)
		return
	}
	response.Success(c, out)
}
func (h *QQCommunityHandler) SaveCodeCampaign(c *gin.Context) {
	if !requireQQBotAdmin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var req qqcommunity.CodeCampaign
	if c.ShouldBindJSON(&req) != nil {
		response.BadRequest(c, "活动配置格式不正确")
		return
	}
	out, err := h.bot.SaveCodeCampaign(c.Request.Context(), req)
	if err != nil {
		campaignAPIError(c, err)
		return
	}
	response.Success(c, out)
}
func (h *QQCommunityHandler) ListCodeClaims(c *gin.Context) {
	if !requireQQBotAdmin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	f := struct {
		Campaign string `form:"campaign_id"`
		Search   string `form:"search"`
		Page     int    `form:"page"`
		PageSize int    `form:"page_size"`
	}{Page: 1, PageSize: 20}
	if c.ShouldBindQuery(&f) != nil {
		response.BadRequest(c, "查询参数不正确")
		return
	}
	export := strings.HasSuffix(c.FullPath(), "/export")
	out, err := h.bot.CodeClaims(c.Request.Context(), f.Campaign, strings.TrimSpace(f.Search), f.Page, f.PageSize, export)
	if err != nil {
		campaignAPIError(c, err)
		return
	}
	if !export {
		response.Success(c, out)
		return
	}
	records := [][]string{{"活动", "账号ID", "邮箱", "QQ", "来源群", "码尾号", "兑换类型", "面值", "订阅天数", "发送状态", "兑换状态", "领取时间", "发送时间", "兑换时间"}}
	for _, r := range out.Items {
		records = append(records, []string{r.CampaignName, strconv.FormatInt(r.UserID, 10), r.Email, r.MemberID, r.GroupID, r.CodeHint, r.Type, strconv.FormatFloat(r.Value, 'f', -1, 64), strconv.Itoa(r.ValidityDays), r.DeliveryStatus, r.Status, r.CreatedAt.Format(time.RFC3339), campaignCSVTime(r.SentAt), campaignCSVTime(r.UsedAt)})
	}
	writeCampaignCSV(c, "code-claims.csv", records)
}
func (h *QQCommunityHandler) ResendCodeClaim(c *gin.Context) {
	if !requireQQBotAdmin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	message, err := h.bot.ResendCampaignCode(c.Request.Context(), c.Param("id"))
	if err != nil {
		campaignAPIError(c, err)
		return
	}
	response.Success(c, gin.H{"message": message})
}
func (h *QQCommunityHandler) DisableCodeClaim(c *gin.Context) {
	if !requireQQBotAdmin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	if err := h.bot.DisableCampaignCode(c.Request.Context(), c.Param("id")); err != nil {
		campaignAPIError(c, err)
		return
	}
	response.Success(c, gin.H{"disabled": true})
}
func (h *QQCommunityHandler) ActivityStats(c *gin.Context) {
	if !requireQQBotAdmin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	f := qqcommunity.ActivityStatsFilter{Page: 1, PageSize: 20}
	if c.ShouldBindQuery(&f) != nil {
		response.BadRequest(c, "查询参数不正确")
		return
	}
	export := strings.HasSuffix(c.FullPath(), "/export")
	out, err := h.bot.ActivityStats(c.Request.Context(), f, export)
	if err != nil {
		campaignAPIError(c, err)
		return
	}
	if !export {
		response.Success(c, out)
		return
	}
	records := [][]string{{"统计日期(北京时间)", "账号ID", "邮箱", "QQ", "账号状态", "已删除", "当日签到", "签到奖励USD", "当日抽奖次数", "抽奖奖励USD", "当前期人民币本金CNY", "当前期累计赠送次数", "当前期已用次数", "当前剩余次数"}}
	for _, r := range out.Items {
		records = append(records, []string{out.Date, strconv.FormatInt(r.UserID, 10), r.Email, strings.Join(r.Members, " / "), r.Status, strconv.FormatBool(r.Deleted), strconv.FormatBool(r.CheckedIn), strconv.FormatFloat(r.CheckinAmount, 'f', 8, 64), strconv.FormatInt(r.DayDraws, 10), strconv.FormatFloat(r.DayDrawAmount, 'f', 8, 64), strconv.FormatFloat(r.RechargeTotal, 'f', 8, 64), strconv.FormatInt(r.BonusChances, 10), strconv.FormatInt(r.Used, 10), strconv.FormatInt(r.Remaining, 10)})
	}
	writeCampaignCSV(c, "activity-stats-"+out.Date+".csv", records)
}
func campaignCSVTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
func writeCampaignCSV(c *gin.Context, name string, records [][]string) {
	var b bytes.Buffer
	b.WriteString("\xEF\xBB\xBF")
	w := csv.NewWriter(&b)
	for _, row := range records {
		for i, cell := range row {
			trimmed := strings.TrimLeft(cell, " \t\r\n")
			if strings.HasPrefix(cell, "\t") || strings.HasPrefix(cell, "\r") || strings.HasPrefix(cell, "\n") || strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "@") {
				row[i] = "'" + cell
			}
		}
		_ = w.Write(row)
	}
	w.Flush()
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", b.Bytes())
}
