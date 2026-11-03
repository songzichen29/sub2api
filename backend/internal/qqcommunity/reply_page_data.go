package qqcommunity

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type ModelUsageStat struct {
	Model         string  `json:"model"`
	Requests      int64   `json:"requests"`
	Input         int64   `json:"input_tokens"`
	Output        int64   `json:"output_tokens"`
	CacheCreation int64   `json:"cache_creation_tokens"`
	CacheRead     int64   `json:"cache_read_tokens"`
	Total         int64   `json:"total_tokens"`
	Cost          float64 `json:"actual_cost"`
}

func balanceReplyPage(balance float64, user ...int64) *ReplyPage {
	uid := int64(0)
	if len(user) > 0 {
		uid = user[0]
	}
	return &ReplyPage{BoundUserID: uid, Kind: "balance", Title: "我的余额", SourcePath: "/key-usage", Data: map[string]any{"balance": balance}}
}

func usageReplyPage(doc *replyDocument) *ReplyPage {
	if doc == nil {
		return nil
	}
	models := doc.Models
	if models == nil {
		models = []ModelUsageStat{}
	}
	return &ReplyPage{BoundUserID: doc.UserID, Kind: "usage", Title: doc.Title, SourcePath: "/key-usage", Data: map[string]any{"models": models}}
}

func priceReplyPage(model, filter string, blocks []string) *ReplyPage {
	selected := make([]string, 0, len(blocks))
	for _, block := range blocks {
		selected = append(selected, cardPlainText(block))
	}

	return &ReplyPage{Kind: "prices", Title: "模型价格 · " + model, SourcePath: "/model-plaza", Data: map[string]any{"model": model, "filter": filter, "blocks": selected}}
}

// An administrator can review real pages and captures without sending messages
// to QQ, invoking a paid model or changing a member's balance.
func (s *Service) PreviewReplyPage(ctx context.Context, kind, model, filter, group, member string) (string, error) {
	c, err := s.Config(ctx)
	if err != nil {
		return "", err
	}
	if !c.Enabled || !c.allows(group) {
		return "", errors.New("请选择已启用的群")
	}
	j := Job{AppID: c.AppID, GroupID: group, MemberID: member, CreatedAt: time.Now()}
	uid, err := boundUser(ctx, s.db, c.AppID, member)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var page *ReplyPage
	switch kind {
	case "monitor":
		page, err = s.prepareMonitorPage(ctx, c, j)
	case "usage":
		_, doc, e := s.todayUsageDocument(ctx, c, j, time.Now())
		err = e
		page = usageReplyPage(doc)
	case "balance":
		var balance float64
		err = s.db.QueryRowContext(ctx, `SELECT u.balance FROM users u WHERE u.id=$1 AND u.status='active' AND u.deleted_at IS NULL`, uid).Scan(&balance)
		page = balanceReplyPage(balance)
	case "prices":
		if s.modelPrices == nil || model == "" || len(model) > 200 {
			return "", errors.New("请输入模型名称")
		}
		var blocks []string
		blocks, err = s.modelPrices.ModelPrice(ctx, model, uid, time.Now())
		page = priceReplyPage(model, filter, blocks)
	default:
		return "", errors.New("未知页面类型")
	}
	if err != nil {
		return "", err
	}
	if page == nil {
		return "", errors.New(bindFirst)
	}
	if page.BoundUserID == 0 {
		page.BoundUserID = uid
	}
	token, err := s.rememberReplyPage(c, j, uid, *page)
	if err != nil {
		return "", err
	}
	return "/qq/report#" + token, nil
}
