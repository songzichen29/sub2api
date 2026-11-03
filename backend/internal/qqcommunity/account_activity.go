package qqcommunity

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// AccountActivityItem represents a committed redemption or QQ balance transfer.
type AccountActivityItem struct {
	ID           int64     `json:"id"`
	Code         string    `json:"code"`
	Type         string    `json:"type"`
	Value        float64   `json:"value"`
	Status       string    `json:"status"`
	UsedAt       time.Time `json:"used_at"`
	CreatedAt    time.Time `json:"created_at"`
	Notes        string    `json:"notes"`
	GroupID      *int64    `json:"group_id,omitempty"`
	ValidityDays int       `json:"validity_days,omitempty"`
	GroupName    string    `json:"group_name,omitempty"`
	Counterparty string    `json:"counterparty,omitempty"`
	Amount       string    `json:"amount,omitempty"`
	Fee          string    `json:"fee,omitempty"`
	NetAmount    string    `json:"net_amount,omitempty"`
}

// AccountActivity merges completed ledgers before paging and limits both to userID.
func (s *Service) AccountActivity(ctx context.Context, userID int64, page, pageSize int) ([]AccountActivityItem, int64, error) {
	if userID <= 0 || page < 1 || pageSize < 1 || pageSize > 100 || page-1 > 1000000/pageSize {
		return nil, 0, errors.New("查询参数不正确")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var total int64
	err = tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM redeem_codes WHERE used_by=$1) + (SELECT COUNT(*) FROM qqc_transfer_history WHERE from_user_id=$1 OR to_user_id=$1)`, userID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	query := `SELECT id,code,type,value,status,used_at,created_at,notes,group_id,validity_days,group_name,counterparty,amount,fee,net_amount
 FROM (
  SELECT r.id,r.code,r.type,r.value,r.status,COALESCE(r.used_at,r.created_at) AS used_at,r.created_at,
   COALESCE(r.notes,'') AS notes,r.group_id,r.validity_days,COALESCE(g.name,'') AS group_name,
   '' AS counterparty,0 AS amount,0 AS fee,0 AS net_amount
  FROM redeem_codes r LEFT JOIN groups g ON g.id=r.group_id WHERE r.used_by=$1
  UNION ALL
  SELECT h.id,'' AS code,CASE WHEN h.from_user_id=$1 THEN 'transfer_out' ELSE 'transfer_in' END AS type,
   CASE WHEN h.from_user_id=$1 THEN -h.amount ELSE h.net_amount END AS value,
   'completed' AS status,h.created_at AS used_at,h.created_at,
   '' AS notes,NULL AS group_id,0 AS validity_days,'' AS group_name,
   CASE WHEN h.from_user_id=$1 THEN h.to_email ELSE h.from_email END AS counterparty,
   h.amount,h.fee,h.net_amount
  FROM qqc_transfer_history h WHERE h.from_user_id=$1 OR h.to_user_id=$1
 ) activity ORDER BY used_at DESC,id DESC,type DESC LIMIT $2 OFFSET $3`
	if s.db.mysql {
		// GROUPS is reserved in MySQL 8, but PostgreSQL accepts the plain table name.
		query = strings.ReplaceAll(query, "LEFT JOIN groups g", "LEFT JOIN `groups` g")
	}
	rows, err := tx.QueryContext(ctx, query, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]AccountActivityItem, 0, pageSize)
	for rows.Next() {
		var item AccountActivityItem
		var groupID sql.NullInt64
		if err = rows.Scan(&item.ID, &item.Code, &item.Type, &item.Value, &item.Status, &item.UsedAt, &item.CreatedAt, &item.Notes, &groupID, &item.ValidityDays, &item.GroupName, &item.Counterparty, &item.Amount, &item.Fee, &item.NetAmount); err != nil {
			return nil, 0, err
		}
		if groupID.Valid {
			item.GroupID = &groupID.Int64
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
