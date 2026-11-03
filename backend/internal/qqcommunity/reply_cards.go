package qqcommunity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"math"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Cards are deterministic views of the same safe command output, independent
// of paid image generation, its feature switch and its quotas.
type replyDocument struct {
	Title     string
	Bars      []replyBar
	CostBars  []replyBar
	Matrix    []replyMatrixRow
	Trend     []ChannelStatusPoint
	ChartNote string
}
type replyBar struct {
	Label   string
	Value   float64
	Display string
}
type replyMatrixRow struct {
	Name   string
	Points []ChannelStatusPoint
}
type replyCardPage struct {
	Lines  []string
	Bars   []replyBar
	Matrix []replyMatrixRow
	Trend  []ChannelStatusPoint
	Note   string
}

var cardFont struct {
	sync.Once
	Font   *opentype.Font
	Source string
	Err    error
}

func loadCardFont() (*opentype.Font, error) {
	cardFont.Do(func() {
		cardFont.Font, cardFont.Source, cardFont.Err = resolveCardFont(os.Getenv("QQBOT_CARD_FONT"), []string{
			"/usr/share/fonts/wqy-zenhei/wqy-zenhei.ttf",
			"/usr/share/fonts/wqy-zenhei/wqy-zenhei.ttc",
			"/usr/share/fonts/wenquanyi/wqy-zenhei/wqy-zenhei.ttc",
			"C:/Windows/Fonts/msyh.ttc",
			"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
		})
	})
	return cardFont.Font, cardFont.Err
}

func resolveCardFont(override string, defaults []string) (*opentype.Font, string, error) {
	paths := defaults
	// An explicit font is also used by release checks. Falling back here would
	// conceal an unusable image font behind a developer's system font.
	if override != "" {
		paths = []string{override}
	}
	var failures []error
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		collection, err := opentype.ParseCollection(data)
		if err != nil {
			failures = append(failures, fmt.Errorf("parse reply font %s: %w", path, err))
			continue
		}
		for i := 0; i < collection.NumFonts(); i++ {
			f, err := collection.Font(i)
			if err == nil {
				return f, path, nil
			}
			failures = append(failures, fmt.Errorf("parse reply font %s face %d: %w", path, i, err))
		}
	}
	return nil, "", fmt.Errorf("Chinese reply font unavailable: %w", errors.Join(failures...))
}

// CommandReplyImagePreview exercises the production font loader and renderer
// without configuration, database access, model calls or QQ messages.
func CommandReplyImagePreview() ([]byte, string, error) {
	f, err := loadCardFont()
	if err != nil {
		return nil, "", err
	}
	for _, r := range "帮助监控用量余额中文" {
		glyph, err := f.GlyphIndex(nil, r)
		if err != nil || glyph == 0 {
			return nil, cardFont.Source, fmt.Errorf("reply font missing Chinese glyph %U", r)
		}
	}
	data, err := renderReplyCard(f, "QQ 指令图片检查", replyCardPage{Lines: []string{"帮助 · 监控 · 用量 · 余额", "中文字体与 PNG 渲染正常", "此检查不连接数据库，不发送 QQ 消息。"}}, 0, 1)
	return data, cardFont.Source, err
}

var cardLink = regexp.MustCompile(`https?://[^\s<>]+`)

func replyLinks(text string) string {
	var links []string
	seen := map[string]bool{}
	for _, link := range cardLink.FindAllString(text, -1) {
		link = strings.TrimRight(link, "),，。；】")
		if !seen[link] {
			seen[link] = true
			links = append(links, link)
		}
	}
	return strings.Join(links, "\n")
}

func cardPlainText(text string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "**", ""), "```", "")
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if (unicode.IsControl(r) && r != '\n') || r == 0xfe0f || (r >= 0x1f000 && r <= 0x1faff) {
			return -1
		}
		return r
	}, text)
}

// Use glyph advances rather than byte or rune counts: CJK and Latin labels
// wrap correctly, including long URLs, without dropping a suffix.
func wrapCardText(text string, face font.Face, width int) []string {
	var lines []string
	for _, paragraph := range strings.Split(cardPlainText(text), "\n") {
		var line []rune
		advance := fixed.Int26_6(0)
		for _, r := range paragraph {
			a, _ := face.GlyphAdvance(r)
			if len(line) > 0 && advance+a > fixed.I(width) {
				lines = append(lines, string(line))
				line = nil
				advance = 0
			}
			line = append(line, r)
			advance += a
		}
		lines = append(lines, string(line))
	}
	return lines
}

func layoutReplyCards(text string, doc *replyDocument, face font.Face) []replyCardPage {
	var pages []replyCardPage
	if doc != nil {
		if len(doc.Bars) > 0 || len(doc.Trend) > 0 || len(doc.Matrix) > 0 {
			// Bound chart height, retaining every visible group on further pages.
			for i := 0; i < max(1, len(doc.Matrix)); i += 12 {
				pages = append(pages, replyCardPage{Bars: doc.Bars, Trend: doc.Trend, Matrix: doc.Matrix[min(i, len(doc.Matrix)):min(i+12, len(doc.Matrix))], Note: doc.ChartNote})
			}
		}
		if len(doc.CostBars) > 0 {
			pages = append(pages, replyCardPage{Bars: doc.CostBars, Note: "费用前 5 模型 · USD（独立比例）"})
		}
	}
	lines := wrapCardText(text, face, 864)
	for i := 0; i < len(lines); i += 40 {
		pages = append(pages, replyCardPage{Lines: lines[i:min(i+40, len(lines))]})
	}
	return pages
}

var cardInk = color.RGBA{R: 31, G: 35, B: 40, A: 255}
var cardMuted = color.RGBA{R: 101, G: 109, B: 118, A: 255}
var cardGreen = color.RGBA{R: 45, G: 149, B: 94, A: 255}
var cardRed = color.RGBA{R: 205, G: 64, B: 64, A: 255}
var cardGray = color.RGBA{R: 225, G: 229, B: 234, A: 255}

func cardText(img *image.RGBA, face font.Face, x, y int, text string, ink color.Color) {
	d := font.Drawer{Dst: img, Src: image.NewUniform(ink), Face: face, Dot: fixed.P(x, y)}
	d.DrawString(text)
}
func cardRect(img *image.RGBA, x, y, w, h int, ink color.Color) {
	draw.Draw(img, image.Rect(x, y, x+w, y+h), image.NewUniform(ink), image.Point{}, draw.Src)
}
func clippedCardLabel(text string, face font.Face, width int) string {
	text = cardPlainText(replyPlainLabel(text))
	if font.MeasureString(face, text) <= fixed.I(width) {
		return text
	}
	r := []rune(text)
	for len(r) > 0 && font.MeasureString(face, string(r)+"…") > fixed.I(width) {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
func healthColor(p ChannelStatusPoint) color.Color {
	if !p.Metrics.HasRequests || p.Metrics.InsufficientSamples {
		return cardGray
	}
	switch p.Health {
	case "critical":
		return cardRed
	case "warning":
		return color.RGBA{R: 210, G: 154, B: 42, A: 255}
	case "healthy":
		return cardGreen
	default:
		return cardGray
	}
}

func renderReplyCard(f *opentype.Font, title string, page replyCardPage, index, total int) ([]byte, error) {
	body, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 26, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	small, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 20, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, err
	}
	defer small.Close()
	heading, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 32, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, err
	}
	defer heading.Close()
	height := max(230, 170+len(page.Lines)*36)
	if len(page.Bars) > 0 || len(page.Matrix) > 0 || len(page.Trend) > 0 {
		height = max(height, 240+len(page.Matrix)*66+len(page.Bars)*75+len(wrapCardText(page.Note, small, 864))*28)
		if len(page.Trend) > 0 {
			height += 107
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, 960, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	cardText(img, heading, 48, 62, clippedCardLabel(title, heading, 864), cardInk)
	cardRect(img, 48, 85, 864, 2, cardGray)
	y := 126
	for _, line := range page.Lines {
		cardText(img, body, 48, y, line, cardInk)
		y += 36
	}
	if len(page.Trend) > 0 {
		cardText(img, small, 48, y, "近 90 分钟 · 整体趋势", cardMuted)
		y += 18
		drawHealthBuckets(img, 48, y, 864, 24, page.Trend)
		first, last := page.Trend[0].At, page.Trend[len(page.Trend)-1].At
		if !first.IsZero() && !last.IsZero() {
			zone := time.FixedZone("Asia/Shanghai", 8*60*60)
			cardText(img, small, 48, y+46, first.In(zone).Format("15:04"), cardMuted)
			cardText(img, small, 720, y+46, last.In(zone).Format("15:04")+" 北京时间", cardMuted)
		}
		y += 89
	}
	for _, row := range page.Matrix {
		cardText(img, body, 48, y, clippedCardLabel(row.Name, body, 280), cardInk)
		drawHealthBuckets(img, 340, y-23, 572, 28, row.Points)
		y += 66
	}
	maxValue := 0.0
	for _, b := range page.Bars {
		if !math.IsNaN(b.Value) && !math.IsInf(b.Value, 0) {
			maxValue = math.Max(maxValue, b.Value)
		}
	}
	for _, b := range page.Bars {
		cardText(img, body, 48, y, clippedCardLabel(b.Label, body, 600), cardInk)
		cardText(img, small, 730, y, clippedCardLabel(b.Display, small, 180), cardMuted)
		y += 13
		cardRect(img, 48, y, 864, 14, cardGray)
		if maxValue > 0 && b.Value > 0 && !math.IsNaN(b.Value) && !math.IsInf(b.Value, 0) {
			cardRect(img, 48, y, int(864*b.Value/maxValue), 14, cardGreen)
		}
		y += 62
	}
	if page.Note != "" {
		for _, line := range wrapCardText(page.Note, small, 864) {
			cardText(img, small, 48, y, line, cardMuted)
			y += 28
		}
	}
	cardText(img, small, 48, height-28, "Sub2API · QQ", cardMuted)
	cardText(img, small, 730, height-28, fmt.Sprintf("第 %d / %d 页", index+1, total), cardMuted)
	var buf bytes.Buffer
	err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img)
	return buf.Bytes(), err
}

func drawHealthBuckets(img *image.RGBA, x, y, width, height int, points []ChannelStatusPoint) {
	if len(points) == 0 {
		cardRect(img, x, y, width, height, cardGray)
		return
	}
	for i, p := range points {
		start, end := i*width/len(points), (i+1)*width/len(points)
		cardRect(img, x+start, y, max(1, end-start-2), height, healthColor(p))
	}
}

// Render one page at a time so a long reply cannot retain many full canvases.
// On a confirmed image rejection only that page falls back to text; earlier
// pages are never repeated. Uncertain delivery errors abort without retry.
func (s *Service) sendCommandCards(ctx context.Context, c Config, j Job, text string) ([]byte, bool, error) {
	f, err := loadCardFont()
	if err != nil {
		slog.WarnContext(ctx, "QQ command image fell back to text", "component", "qqbot.reply_card", "reason", "font_unavailable", "job_id", j.ID, "error", err)
	}
	if err != nil || len(text) > 256<<10 {
		raw, err := s.sendOneBot(ctx, c, j, text, nil)
		return raw, false, err
	}
	return s.sendCommandCardsWithFont(ctx, c, j, text, f)
}

func (s *Service) sendCommandCardsWithFont(ctx context.Context, c Config, j Job, text string, f *opentype.Font) ([]byte, bool, error) {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 26, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		raw, err := s.sendOneBot(ctx, c, j, text, nil)
		return raw, false, err
	}
	pages := layoutReplyCards(text, j.ReplyCard, face)
	_ = face.Close()
	title := "指令回复"
	if j.ReplyCard != nil && j.ReplyCard.Title != "" {
		title = j.ReplyCard.Title
	} else if first, _, _ := strings.Cut(cardPlainText(text), "\n"); strings.TrimSpace(first) != "" {
		title = first
	}
	var raw []byte
	usedImage := false
	for i, page := range pages {
		if err := ctx.Err(); err != nil {
			return nil, usedImage, err
		}
		picture, renderErr := renderReplyCard(f, title, page, i, len(pages))
		if renderErr == nil {
			raw, err = s.sendOneBot(ctx, c, j, "", picture)
		}
		var rejected *oneBotRejected
		if renderErr != nil || errors.As(err, &rejected) {
			if renderErr != nil {
				slog.WarnContext(ctx, "QQ command image fell back to text", "component", "qqbot.reply_card", "reason", "render_failed", "job_id", j.ID, "page", i+1, "error", renderErr)
			} else {
				slog.WarnContext(ctx, "QQ command image fell back to text", "component", "qqbot.reply_card", "reason", "image_rejected", "job_id", j.ID, "page", i+1, "retcode", rejected.Retcode)
			}
			fallback := strings.Join(page.Lines, "\n")
			// The detailed text pages already contain every chart value.
			if len(page.Lines) == 0 {
				fallback = "图表暂不可用，详细数据见后续回复。"
			}
			raw, err = s.sendOneBot(ctx, c, j, fallback, nil)
		} else if err == nil {
			usedImage = true
		}
		if err != nil {
			return nil, usedImage, err
		}
		j.SuppressMention = true
		j.ReplyReference = ""
	}
	if links := replyLinks(text); links != "" {
		raw, err = s.sendOneBot(ctx, c, j, "可点击链接：\n"+links, nil)
	}
	return raw, usedImage, err
}
