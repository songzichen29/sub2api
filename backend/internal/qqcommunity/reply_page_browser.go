package qqcommunity

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

// Every capture owns a fresh browser profile with no user/admin cookies. Only
// one capture runs at once; timeout and cancel also reap its Chromium children.
func (s *Service) renderReplyPage(parent context.Context, token string) ([]byte, error) {
	if s.pageOrigin == "" || s.pageGate == nil {
		return nil, errors.New("renderer not configured")
	}
	ctx, cancel := context.WithTimeout(parent, 35*time.Second)
	defer cancel()
	select {
	case s.pageGate <- struct{}{}:
		defer func() { <-s.pageGate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.WindowSize(1200, 900),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("host-resolver-rules", "MAP * ~NOTFOUND, EXCLUDE 127.0.0.1"),
	)
	if bin := os.Getenv("QQBOT_CHROMIUM_PATH"); bin != "" {
		options = append(options, chromedp.ExecPath(bin))
	}
	allocator, closeAllocator := chromedp.NewExecAllocator(ctx, options...)
	defer closeAllocator()
	browser, closeBrowser := chromedp.NewContext(allocator)
	defer closeBrowser()
	if err := chromedp.Do(browser,
		chromedp.Navigate(s.pageOrigin+"/qq/report?capture=1#"+token),
		chromedp.WaitReady(chromedp.CSS(`[data-report-ready="true"]`)),
	); err != nil {
		return nil, err
	}
	height, err := chromedp.Run(browser, chromedp.Evaluate[float64](`document.documentElement.scrollHeight`))
	if err != nil || height > 12000 {
		return nil, errors.New("report exceeds screenshot height")
	}
	img, err := chromedp.Run(browser, chromedp.FullScreenshot(90))
	if err == nil && len(img) > 10<<20 {
		return nil, errors.New("report exceeds image size")
	}
	return img, err
}
