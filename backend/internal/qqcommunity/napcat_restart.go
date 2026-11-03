package qqcommunity

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNapCatRestartConfirmation = errors.New("请确认重新登录，并核对当前机器人 QQ")
	ErrNapCatRestartCooldown     = errors.New("重新登录请求间隔至少 60 秒，请等待状态更新")
	ErrNapCatLoginInProgress     = errors.New("QQ 正在扫码确认、初始化或重连，请等待当前登录流程结束")
	ErrNapCatRestartUnconfirmed  = errors.New("未确认重新登录请求是否被接受，请先等待状态更新，至少 60 秒后再重试")
)

type NapCatRestartResult struct {
	Accepted           bool      `json:"accepted"`
	RestartAvailableAt time.Time `json:"restart_available_at"`
}

// Only the explicit administrator action calls this method. Neither health
// probes nor background QR recovery may restart QQ. Use NapCat's worker restart
// endpoint, never Docker access or a caller-supplied process/URL/command.
func (s *Service) RestartNapCatLogin(ctx context.Context, expectedQQ string, confirmed bool) (*NapCatRestartResult, error) {
	if !confirmed || expectedQQ == "" {
		return nil, ErrNapCatRestartConfirmation
	}
	cfg, err := s.Config(ctx)
	if err != nil {
		return nil, errors.New("读取机器人配置失败")
	}
	if cfg.AppID != expectedQQ {
		return nil, ErrNapCatRestartConfirmation
	}
	n := s.napcatLogin
	if n == nil {
		return nil, errNapCatUnavailable
	}
	q, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	n.mu.Lock()
	defer n.mu.Unlock()
	if time.Since(n.lastRestart) < time.Minute {
		return nil, ErrNapCatRestartCooldown
	}
	var status struct {
		Phase        string `json:"loginPhase"`
		ScanAccepted bool   `json:"qrLoginAccepted"`
	}
	if err := n.request(q, "/api/QQLogin/CheckLoginStatus", &status); err != nil {
		return nil, err
	}
	if napcatLoginInProgress(status.ScanAccepted, status.Phase) {
		return nil, ErrNapCatLoginInProgress
	}
	// Reserve before sending: a lost response is uncertain delivery, not
	// permission to send the restart again. Concurrent requests share this lock.
	n.lastRestart = time.Now()
	err = n.request(q, "/api/QQLogin/RestartNapCat", nil)
	n.credential, n.validUntil = "", time.Time{}
	if err != nil {
		return nil, ErrNapCatRestartUnconfirmed
	}
	n.lastQRCode, n.qrObserved, n.wasOnline, n.recoverQR = "", time.Time{}, false, false
	return &NapCatRestartResult{Accepted: true, RestartAvailableAt: n.lastRestart.Add(time.Minute)}, nil
}
