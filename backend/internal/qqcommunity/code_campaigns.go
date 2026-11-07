package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

type CampaignRuleError string

func (e CampaignRuleError) Error() string { return string(e) }
func campaignErrorMessage(err error) string {
	var rule CampaignRuleError
	if errors.As(err, &rule) {
		return rule.Error()
	}
	return "领码处理失败，请稍后重试；已经分配的兑换码会保留。"
}

type CodeCampaign struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Type                string     `json:"type"`
	Value               float64    `json:"value"`
	SubscriptionGroupID *int64     `json:"subscription_group_id"`
	ValidityDays        int        `json:"validity_days"`
	Enabled             bool       `json:"enabled"`
	Groups              []string   `json:"groups"`
	ClaimMode           string     `json:"claim_mode"`
	StockLimit          int        `json:"stock_limit"`
	CodeValidHours      int        `json:"code_valid_hours"`
	StartsAt            *time.Time `json:"starts_at"`
	EndsAt              *time.Time `json:"ends_at"`
	Version             int64      `json:"version"`
	Claimed             int64      `json:"claimed"`
	Sent                int64      `json:"sent"`
	Redeemed            int64      `json:"redeemed"`
}

func validateCodeCampaign(c CodeCampaign) error {
	if c.ID != "" {
		if _, err := hex.DecodeString(c.ID); err != nil || (len(c.ID) != 24 && len(c.ID) != 32) {
			return CampaignRuleError("活动标识不正确")
		}
	}
	if strings.TrimSpace(c.Name) == "" || utf8.RuneCountInString(c.Name) > 60 || c.StockLimit < 1 || c.StockLimit > 100000 || c.CodeValidHours < 1 || c.CodeValidHours > 8760 || len(c.Groups) > 100 {
		return CampaignRuleError("活动名称、库存（1～100000）或兑换有效期（1～8760小时）不正确")
	}
	if c.ClaimMode != "once" && c.ClaimMode != "daily" {
		return CampaignRuleError("领取频率必须为每活动一次或每天一次")
	}
	if c.StartsAt != nil && c.EndsAt != nil && !c.EndsAt.After(*c.StartsAt) {
		return CampaignRuleError("活动结束时间必须晚于开始时间")
	}
	switch c.Type {
	case "balance":
		if !validRewardAmount(c.Value) || c.Value <= 0 {
			return CampaignRuleError("每码余额需大于0且不超过1000 USD，最多8位小数")
		}
	case "lottery_chances", "concurrency":
		if math.IsNaN(c.Value) || c.Value < 1 || c.Value > 1000 || math.Trunc(c.Value) != c.Value {
			return CampaignRuleError("每码次数或并发数必须是1～1000的整数")
		}
	case "subscription":
		if c.SubscriptionGroupID == nil || *c.SubscriptionGroupID <= 0 || c.ValidityDays < 1 || c.ValidityDays > 3650 {
			return CampaignRuleError("请选择订阅分组，并设置1～3650天")
		}
	default:
		return CampaignRuleError("不支持的兑换类型")
	}
	return nil
}

func (s *Service) CodeCampaigns(ctx context.Context) ([]CodeCampaign, error) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id,config_json,version FROM qqc_code_campaigns WHERE app_id=$1 ORDER BY created_at DESC,id`, cfg.AppID)
	if err != nil {
		return nil, err
	}
	items := []CodeCampaign{}
	for rows.Next() {
		var c CodeCampaign
		var raw string
		var id string
		var version int64
		if err = rows.Scan(&id, &raw, &version); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &c); err != nil {
			_ = rows.Close()
			return nil, err
		}
		c.ID = id
		c.Version = version
		c.Groups = []string{}
		items = append(items, c)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range items {
		c := &items[i]
		groups, e := tx.QueryContext(ctx, `SELECT group_id FROM qqc_code_campaign_groups WHERE app_id=$1 AND campaign_id=$2 ORDER BY group_id`, cfg.AppID, c.ID)
		if e != nil {
			return nil, e
		}
		for groups.Next() {
			var group string
			if e = groups.Scan(&group); e != nil {
				_ = groups.Close()
				return nil, e
			}
			c.Groups = append(c.Groups, group)
		}
		e = groups.Err()
		_ = groups.Close()
		if e != nil {
			return nil, e
		}
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN q.sent_at IS NOT NULL THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN r.status='used' THEN 1 ELSE 0 END),0) FROM qqc_code_claims q LEFT JOIN redeem_codes r ON r.id=q.redeem_code_id WHERE q.campaign_id=$1`, c.ID).Scan(&c.Claimed, &c.Sent, &c.Redeemed)
		if err != nil {
			return nil, err
		}
	}
	return items, tx.Commit()
}

func (s *Service) SaveCodeCampaign(ctx context.Context, c CodeCampaign) (CodeCampaign, error) {
	c.Name = strings.TrimSpace(c.Name)
	if err := validateCodeCampaign(c); err != nil {
		return c, err
	}
	if c.Type != "subscription" {
		c.SubscriptionGroupID = nil
		c.ValidityDays = 0
	} else {
		c.Value = 1
	}
	cfg, err := s.Config(ctx)
	if err != nil {
		return c, err
	}
	seen := map[string]bool{}
	for _, g := range c.Groups {
		if seen[g] || !cfg.allows(g) {
			return c, CampaignRuleError("开放群重复或不在机器人允许名单中")
		}
		seen[g] = true
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return c, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(71427320)`); err != nil {
		return c, err
	}
	if c.Type == "subscription" {
		var valid bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM groups WHERE id=$1 AND subscription_type='subscription' AND status='active' AND deleted_at IS NULL)`, *c.SubscriptionGroupID).Scan(&valid)
		if err != nil {
			return c, err
		}
		if !valid {
			return c, CampaignRuleError("订阅分组不存在或未启用")
		}
	}
	if c.ID == "" {
		c.ID = randomID()
	}
	if c.Version == 0 {
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM qqc_code_campaigns WHERE id=$1)`, c.ID).Scan(&exists); err != nil {
			return c, err
		}
		if exists {
			return c, ErrConfigConflict
		}
		c.Version = 1
	} else {
		var raw string
		var version int64
		err = tx.QueryRowContext(ctx, `SELECT config_json,version FROM qqc_code_campaigns WHERE id=$1 AND app_id=$2 FOR UPDATE`, c.ID, cfg.AppID).Scan(&raw, &version)
		if errors.Is(err, sql.ErrNoRows) {
			return c, CampaignRuleError("活动不存在")
		}
		if err != nil {
			return c, err
		}
		if version != c.Version {
			return c, ErrConfigConflict
		}
		var old CodeCampaign
		if err = json.Unmarshal([]byte(raw), &old); err != nil {
			return c, err
		}
		var issued int64
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM qqc_code_claims WHERE campaign_id=$1`, c.ID).Scan(&issued); err != nil {
			return c, err
		}
		oldGroup, newGroup := int64(0), int64(0)
		if old.SubscriptionGroupID != nil {
			oldGroup = *old.SubscriptionGroupID
		}
		if c.SubscriptionGroupID != nil {
			newGroup = *c.SubscriptionGroupID
		}
		if issued > 0 && (old.Type != c.Type || old.Value != c.Value || old.ClaimMode != c.ClaimMode || oldGroup != newGroup || old.ValidityDays != c.ValidityDays) {
			return c, CampaignRuleError("已有领取记录，兑换内容和领取频率不可变更；请新建活动")
		}
		if int64(c.StockLimit) < issued {
			return c, CampaignRuleError("库存上限不能小于已分配数量")
		}
		c.Version++
	}
	c.Claimed = 0
	c.Sent = 0
	c.Redeemed = 0
	raw, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO qqc_code_campaigns(id,app_id,config_json,version) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET config_json=EXCLUDED.config_json,version=EXCLUDED.version`, c.ID, cfg.AppID, string(raw), c.Version)
	if err != nil {
		return c, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM qqc_code_campaign_groups WHERE app_id=$1 AND campaign_id=$2`, cfg.AppID, c.ID); err != nil {
		return c, err
	}
	for _, g := range c.Groups {
		_, err = tx.ExecContext(ctx, `INSERT INTO qqc_code_campaign_groups(app_id,group_id,campaign_id) VALUES($1,$2,$3) ON CONFLICT(app_id,group_id) DO UPDATE SET campaign_id=EXCLUDED.campaign_id`, cfg.AppID, g, c.ID)
		if err != nil {
			return c, err
		}
	}
	return c, tx.Commit()
}

type CodeClaim struct {
	ID             string     `json:"id"`
	CampaignID     string     `json:"campaign_id"`
	CampaignName   string     `json:"campaign_name"`
	UserID         int64      `json:"user_id"`
	Email          string     `json:"email"`
	MemberID       string     `json:"member_id"`
	GroupID        string     `json:"group_id"`
	CodeID         int64      `json:"code_id"`
	Code           string     `json:"-"`
	CodeHint       string     `json:"code_hint"`
	Type           string     `json:"type"`
	Value          float64    `json:"value"`
	ValidityDays   int        `json:"validity_days"`
	Status         string     `json:"status"`
	DeliveryStatus string     `json:"delivery_status"`
	Attempts       int        `json:"attempts"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      *time.Time `json:"expires_at"`
	UsedAt         *time.Time `json:"used_at"`
	SentAt         *time.Time `json:"sent_at"`
}
type CodeClaimsPage struct {
	Items    []CodeClaim `json:"items"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

const claimSelect = `SELECT q.id,q.campaign_id,q.user_id,COALESCE(u.email,''),q.member_id,q.group_id,q.redeem_code_id,COALESCE(r.code,''),COALESCE(r.type,''),COALESCE(r.value,0),COALESCE(r.validity_days,0),COALESCE(r.status,'deleted'),q.delivery_status,q.attempts,q.created_at,r.expires_at,r.used_at,q.sent_at,c.config_json FROM qqc_code_claims q JOIN qqc_code_campaigns c ON c.id=q.campaign_id LEFT JOIN redeem_codes r ON r.id=q.redeem_code_id LEFT JOIN users u ON u.id=q.user_id`

func scanCodeClaim(scan func(...any) error) (CodeClaim, error) {
	var c CodeClaim
	var raw string
	err := scan(&c.ID, &c.CampaignID, &c.UserID, &c.Email, &c.MemberID, &c.GroupID, &c.CodeID, &c.Code, &c.Type, &c.Value, &c.ValidityDays, &c.Status, &c.DeliveryStatus, &c.Attempts, &c.CreatedAt, &c.ExpiresAt, &c.UsedAt, &c.SentAt, &raw)
	if err != nil {
		return c, err
	}
	var campaign CodeCampaign
	if err = json.Unmarshal([]byte(raw), &campaign); err != nil {
		return c, err
	}
	c.CampaignName = campaign.Name
	if c.Status == "deleted" {
		c.Type = campaign.Type
		c.Value = campaign.Value
		c.ValidityDays = campaign.ValidityDays
	}
	if len(c.Code) > 4 {
		c.CodeHint = "****" + c.Code[len(c.Code)-4:]
	}
	if c.Status == "unused" && c.ExpiresAt != nil && !c.ExpiresAt.After(time.Now()) {
		c.Status = "expired"
	}
	return c, nil
}

func (s *Service) CodeClaims(ctx context.Context, campaign, search string, page, pageSize int, export bool) (CodeClaimsPage, error) {
	out := CodeClaimsPage{Items: []CodeClaim{}, Page: page, PageSize: pageSize}
	if page < 1 || page > 1000000 || pageSize < 1 || pageSize > 100 || len(search) > 254 {
		return out, CampaignRuleError("分页或搜索参数不正确")
	}
	c, err := s.Config(ctx)
	if err != nil {
		return out, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()
	filter := ` WHERE q.app_id=$1 AND ($2='' OR q.campaign_id=$2) AND ($3='' OR q.member_id=$3 OR q.user_id::text=$3 OR strpos(lower(COALESCE(u.email,'')),lower($3))>0)`
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM qqc_code_claims q LEFT JOIN users u ON u.id=q.user_id`+filter, c.AppID, campaign, search).Scan(&out.Total)
	if err != nil {
		return out, err
	}
	if export {
		if out.Total > 10000 {
			return out, CampaignRuleError("导出最多10000条，请先缩小筛选范围")
		}
		page, pageSize = 1, 10000
	}
	rows, err := tx.QueryContext(ctx, claimSelect+filter+` ORDER BY q.created_at DESC,q.id DESC LIMIT $4 OFFSET $5`, c.AppID, campaign, search, pageSize, (page-1)*pageSize)
	if err != nil {
		return out, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		item, e := scanCodeClaim(rows.Scan)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, item)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	_ = rows.Close()
	return out, tx.Commit()
}

func (s *Service) allocateCampaignCode(ctx context.Context, c Config, j Job, now time.Time) (CodeClaim, error) {
	var out CodeClaim
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := s.lockConfig(ctx, tx)
	if err != nil {
		return out, err
	}
	if current.AppID != c.AppID {
		return out, CampaignRuleError("机器人配置已变化，请重试")
	}
	c = current
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(71427320)`); err != nil {
		return out, err
	}
	if err = ensureResumedBinding(ctx, tx, c, j); err != nil {
		return out, err
	}
	uid, err := boundUser(ctx, tx, c.AppID, j.MemberID)
	if err != nil {
		return out, CampaignRuleError("请先绑定可用的站点账号")
	}
	var raw, id string
	err = tx.QueryRowContext(ctx, `SELECT c.id,c.config_json FROM qqc_code_campaign_groups g JOIN qqc_code_campaigns c ON c.id=g.campaign_id AND c.app_id=g.app_id WHERE g.app_id=$1 AND g.group_id=$2`, c.AppID, j.GroupID).Scan(&id, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, CampaignRuleError("本群尚未配置领码活动")
	}
	if err != nil {
		return out, err
	}
	var campaign CodeCampaign
	if err = json.Unmarshal([]byte(raw), &campaign); err != nil {
		return out, err
	}
	period := "all"
	if campaign.ClaimMode == "daily" {
		period = day(now)
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT id FROM qqc_code_claims WHERE campaign_id=$1 AND period_key=$2 AND (user_id=$3 OR member_id=$4) ORDER BY created_at LIMIT 1`, id, period, uid, j.MemberID).Scan(&existing)
	if err == nil {
		out, err = scanCodeClaim(tx.QueryRowContext(ctx, claimSelect+` WHERE q.id=$1`, existing).Scan)
		if err != nil {
			return out, err
		}
		if out.UserID != uid || out.MemberID != j.MemberID {
			return out, CampaignRuleError("该账号或QQ本期已经领取，不能换绑重复领取")
		}
		return out, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if !c.Enabled || !c.allows(j.GroupID) || !campaign.Enabled || (campaign.StartsAt != nil && now.Before(*campaign.StartsAt)) || (campaign.EndsAt != nil && !now.Before(*campaign.EndsAt)) {
		return out, CampaignRuleError("当前领码活动未开放或已结束")
	}
	if err = validateCodeCampaign(campaign); err != nil {
		return out, err
	}
	if campaign.Type == "subscription" {
		var valid bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM groups WHERE id=$1 AND subscription_type='subscription' AND status='active' AND deleted_at IS NULL)`, *campaign.SubscriptionGroupID).Scan(&valid); err != nil {
			return out, err
		}
		if !valid {
			return out, CampaignRuleError("活动订阅分组已停用，请联系管理员")
		}
	}
	var claimed int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM qqc_code_claims WHERE campaign_id=$1`, id).Scan(&claimed); err != nil {
		return out, err
	}
	if claimed >= campaign.StockLimit {
		return out, CampaignRuleError("本次活动兑换码已领完")
	}
	code := (randomID() + randomID())[:32]
	expires := now.Add(time.Duration(campaign.CodeValidHours) * time.Hour)
	var codeID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO redeem_codes(code,type,value,status,group_id,validity_days,expires_at,notes,created_at) VALUES($1,$2,$3,'unused',$4,$5,$6,$7,$8) RETURNING id`, code, campaign.Type, campaign.Value, campaign.SubscriptionGroupID, campaign.ValidityDays, expires, "QQ campaign "+id, now).Scan(&codeID)
	if err != nil {
		return out, err
	}
	claimID := randomID()
	_, err = tx.ExecContext(ctx, `INSERT INTO qqc_code_claims(id,campaign_id,app_id,group_id,member_id,user_id,period_key,redeem_code_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, claimID, id, c.AppID, j.GroupID, j.MemberID, uid, period, codeID)
	if err != nil {
		return out, err
	}
	out, err = scanCodeClaim(tx.QueryRowContext(ctx, claimSelect+` WHERE q.id=$1`, claimID).Scan)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) sendCampaignClaim(ctx context.Context, c Config, claimID string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := s.lockConfig(ctx, tx)
	if err != nil {
		return "", err
	}
	if current.AppID != c.AppID {
		return "", CampaignRuleError("机器人配置已变化，请重试")
	}
	c = current
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(71427320)`); err != nil {
		return "", err
	}
	claim, err := scanCodeClaim(tx.QueryRowContext(ctx, claimSelect+` WHERE q.id=$1 AND q.app_id=$2`, claimID, c.AppID).Scan)
	if err != nil {
		return "", err
	}
	uid, err := boundUser(ctx, tx, c.AppID, claim.MemberID)
	if err != nil || uid != claim.UserID {
		return "", CampaignRuleError("绑定账号已变化，不能补发")
	}
	if !c.Enabled || !c.allows(claim.GroupID) {
		return "", CampaignRuleError("机器人或来源群未开放")
	}
	if claim.Status != "unused" {
		return "该活动本期已领取；兑换码已使用、过期或停用，不再分配新码。", nil
	}
	var last *time.Time
	if err = tx.QueryRowContext(ctx, `SELECT last_attempt_at FROM qqc_code_claims WHERE id=$1`, claimID).Scan(&last); err != nil {
		return "", err
	}
	if last != nil && time.Since(*last) < 30*time.Second {
		return "兑换码正在发送或刚刚发送，请查看私聊；未收到可在30秒后再次发送 /领码。", nil
	}
	token := randomID()
	_, err = tx.ExecContext(ctx, `UPDATE qqc_code_claims SET delivery_status='sending',delivery_token=$2,attempts=attempts+1,last_attempt_at=NOW() WHERE id=$1`, claimID, token)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	content := fmt.Sprintf("%s\n兑换码：%s\n", replyPlainLabel(claim.CampaignName), claim.Code)
	switch claim.Type {
	case "balance":
		content += fmt.Sprintf("内容：USD %.8g 余额\n", claim.Value)
	case "lottery_chances":
		content += fmt.Sprintf("内容：%.0f次抽奖\n", claim.Value)
	case "concurrency":
		content += fmt.Sprintf("内容：%.0f并发数\n", claim.Value)
	case "subscription":
		content += fmt.Sprintf("内容：%d天订阅\n", claim.ValidityDays)
	}
	if claim.ExpiresAt != nil {
		content += "有效期至：" + claim.ExpiresAt.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02 15:04") + "（北京时间）\n"
	}
	content += "请使用领取时绑定的站点账号，到兑换码页面自行兑换。"
	if link := s.publicSiteURL(ctx, "/redeem"); link != "" {
		content += "\n兑换入口：" + link
	} else {
		content += "\n请从你常用的站点入口进入「兑换码」页面。"
	}
	deliveryErr := s.sendOneBotPrivate(ctx, c, claim.MemberID, claim.GroupID, content)
	status := "sent"
	var sentAt *time.Time
	if deliveryErr != nil {
		status = "uncertain"
	} else {
		now := time.Now()
		sentAt = &now
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err = s.db.ExecContext(finishCtx, `UPDATE qqc_code_claims SET delivery_status=$3,sent_at=COALESCE($4,sent_at) WHERE id=$1 AND delivery_token=$2`, claimID, token, status, sentAt)
	if err != nil {
		return "兑换码发送结果待确认，请稍后再次发送 /领码，系统只会补发原码。", nil
	}
	if deliveryErr != nil {
		return "兑换码已保留，但私聊未确认发送。请添加机器人好友或开启临时会话，30秒后再发 /领码 补发原码。", nil
	}
	return "兑换码已私聊发送，请使用绑定账号到站点自行兑换。", nil
}

func (s *Service) ClaimCampaignCode(ctx context.Context, c Config, j Job) (string, error) {
	claim, err := s.allocateCampaignCode(ctx, c, j, time.Now())
	if err != nil {
		return "", err
	}
	return s.sendCampaignClaim(ctx, c, claim.ID)
}
func (s *Service) ResendCampaignCode(ctx context.Context, id string) (string, error) {
	c, err := s.Config(ctx)
	if err != nil {
		return "", err
	}
	return s.sendCampaignClaim(ctx, c, id)
}
