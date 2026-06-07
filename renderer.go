package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	quotaWidth      = 270
	quotaHeight     = 84
	collapsedWidth  = 42
	collapsedHeight = 104
	toastWidth      = 72
	toastHeight     = 24
	tipWidth        = 176
	tipHeight       = 26
	settingsWidth   = 336
	settingsHeight  = 170
)

var (
	rendererOnce sync.Once
	rendererInst *uiRenderer
)

type uiRenderer struct {
	title       font.Face
	widgetTitle font.Face
	tiny        font.Face
	body        font.Face
	small       font.Face
	number      font.Face
	percent     font.Face
	button      font.Face
	loaded      bool
}

func renderer() *uiRenderer {
	rendererOnce.Do(func() {
		rendererInst = newUIRenderer()
	})
	return rendererInst
}

func newUIRenderer() *uiRenderer {
	r := &uiRenderer{}
	regular, _ := loadWindowsFont(`C:\Windows\Fonts\msyh.ttc`, 0)
	bold, _ := loadWindowsFont(`C:\Windows\Fonts\msyhbd.ttc`, 0)
	if regular != nil && bold != nil {
		r.title = newFontFace(bold, 15)
		r.widgetTitle = newFontFace(regular, 8)
		r.tiny = newFontFace(regular, 6.4)
		r.body = newFontFace(regular, 10)
		r.small = newFontFace(regular, 9)
		r.number = newFontFace(regular, 10)
		r.percent = newFontFace(bold, 11)
		r.button = newFontFace(bold, 12)
		r.loaded = true
	}
	if r.title == nil {
		r.title = basicfont.Face7x13
		r.widgetTitle = basicfont.Face7x13
		r.tiny = basicfont.Face7x13
		r.body = basicfont.Face7x13
		r.small = basicfont.Face7x13
		r.number = basicfont.Face7x13
		r.percent = basicfont.Face7x13
		r.button = basicfont.Face7x13
	}
	return r
}

func loadWindowsFont(path string, index int) (*opentype.Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	collection, err := opentype.ParseCollection(data)
	if err == nil {
		return collection.Font(index)
	}
	return opentype.Parse(data)
}

func newFontFace(f *opentype.Font, size float64) font.Face {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     96,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	return face
}

func (r *uiRenderer) RenderQuota(state DisplayState, phase float64, fetching bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, quotaWidth, quotaHeight))
	accent := quotaAccent(state)
	low := state.LowBalance

	fillRoundedRect(img, 2, 2, quotaWidth-2, quotaHeight-2, 20, rgba(8, 13, 24, 142))
	drawGlow(img, 30, 22, 42, accent, 0.25)
	drawGlow(img, quotaWidth-38, quotaHeight+8, 82, rgba(68, 183, 255, 255), 0.31)
	drawGlow(img, 116, quotaHeight+12, 88, rgba(77, 112, 255, 255), 0.18)
	drawRoundedStroke(img, 2, 2, quotaWidth-2, quotaHeight-2, 20, rgba(190, 220, 255, 56), 1)

	drawTrafficLamp(img, 17, 17, 6, state, phase)
	r.text(img, "MeapiSpace", 30, 17, r.widgetTitle, rgba(255, 255, 255, 255), 88)
	r.text(img, compactUpdateText(state), 120, 17, r.tiny, rgba(171, 188, 211, 205), quotaWidth-158)
	drawRefreshButton(img, quotaWidth-30, 9, 20, accent, phase, fetching)

	drawQuotaOrb(img, 45, 49, 22, clamp01(state.Progress), accent, phase)
	r.centerText(img, percentText(state), 45, 50, r.percent, withAlpha(accent, 255), 0)
	r.centerText(img, "剩余", 45, 63, r.tiny, rgba(219, 230, 245, 190), 38)

	amount := amountText(state)
	cardColor := rgba(255, 255, 255, 18)
	if low {
		cardColor = rgba(255, 78, 101, 32)
	}
	fillRoundedRect(img, 78, 26, quotaWidth-13, quotaHeight-8, 14, withAlpha(cardColor, maxAlpha(cardColor.A, 15)))
	drawRoundedStroke(img, 78, 26, quotaWidth-13, quotaHeight-8, 14, rgba(220, 236, 255, 26), 1)

	labelColor := rgba(177, 194, 217, 225)
	valueColor := withAlpha(accent, 255)
	r.text(img, "余额", 88, 40, r.small, labelColor, 28)
	r.text(img, amount, 118, 40, r.number, valueColor, quotaWidth-132)
	r.text(img, "今日", 88, 56, r.small, labelColor, 28)
	r.text(img, metricValue(state.TodayCostText, "今日消耗"), 118, 56, r.body, rgba(233, 241, 250, 242), quotaWidth-132)
	r.text(img, "令牌", 88, 71, r.small, labelColor, 28)
	r.text(img, metricValue(state.TodayTokenText, "今日令牌"), 118, 71, r.body, rgba(233, 241, 250, 242), quotaWidth-132)
	return img
}

func (r *uiRenderer) RenderCollapsed(state DisplayState) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, collapsedWidth, collapsedHeight))
	fillRoundedRect(img, 2, 2, collapsedWidth-2, collapsedHeight-2, 18, rgba(8, 13, 24, 146))
	drawGlow(img, collapsedWidth/2, collapsedHeight/2, 46, quotaAccent(state), 0.18)
	drawRoundedStroke(img, 2, 2, collapsedWidth-2, collapsedHeight-2, 18, rgba(190, 220, 255, 54), 1)
	drawStackLamp(img, 21, 22, StatusGreen, state)
	drawStackLamp(img, 21, 52, StatusYellow, state)
	drawStackLamp(img, 21, 82, StatusRed, state)
	return img
}

func (r *uiRenderer) RenderToast(message string, state DisplayState) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, toastWidth, toastHeight))
	accent := quotaAccent(state)
	fillRoundedRect(img, 1, 1, toastWidth-1, toastHeight-1, 12, rgba(8, 13, 24, 172))
	drawGlow(img, toastWidth-12, toastHeight+2, 34, accent, 0.22)
	drawRoundedStroke(img, 1, 1, toastWidth-1, toastHeight-1, 12, withAlpha(accent, 78), 1)
	r.centerText(img, message, toastWidth/2, 16, r.small, rgba(255, 255, 255, 245), toastWidth-14)
	return img
}

func (r *uiRenderer) RenderTip(text string) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, tipWidth, tipHeight))
	fillRoundedRect(img, 1, 1, tipWidth-1, tipHeight-1, 8, rgba(0, 0, 0, 186))
	drawRoundedStroke(img, 1, 1, tipWidth-1, tipHeight-1, 8, rgba(255, 255, 255, 36), 1)
	r.centerText(img, text, tipWidth/2, 17, r.small, rgba(255, 255, 255, 245), tipWidth-12)
	return img
}

func (r *uiRenderer) RenderSettings(input, message string, focused, selected bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, settingsWidth, settingsHeight))
	fillRoundedRect(img, 2, 2, settingsWidth-2, settingsHeight-2, 26, rgba(8, 13, 24, 176))
	drawGlow(img, 38, 32, 42, rgba(70, 154, 255, 255), 0.40)
	drawGlow(img, settingsWidth-40, settingsHeight+8, 120, rgba(69, 238, 255, 255), 0.42)
	drawGlow(img, settingsWidth/2, settingsHeight+16, 130, rgba(70, 104, 255, 255), 0.32)
	drawRoundedStroke(img, 2, 2, settingsWidth-2, settingsHeight-2, 26, rgba(188, 219, 255, 68), 1.2)

	r.text(img, "访问密钥", 24, 31, r.title, rgba(250, 252, 255, 255), 180)
	r.text(img, "输入后保存，自动刷新额度", 24, 49, r.small, rgba(191, 205, 224, 230), 190)
	drawIconButton(img, settingsWidth-36, 18, "×", r.button)

	inputStroke := rgba(147, 176, 217, 100)
	if focused {
		inputStroke = rgba(102, 199, 255, 190)
	}
	fillRoundedRect(img, 22, 62, settingsWidth-22, 102, 15, rgba(255, 255, 255, 25))
	drawRoundedStroke(img, 22, 62, settingsWidth-22, 102, 15, inputStroke, 1.2)
	shown := maskAPIKey(input)
	textColor := rgba(245, 249, 255, 255)
	if shown == "" {
		shown = "输入访问密钥"
		textColor = rgba(162, 178, 199, 220)
	}
	if focused && selected && strings.TrimSpace(input) != "" {
		w := clampInt(textWidth(r.body, shown)+12, 30, settingsWidth-62)
		fillRoundedRect(img, 29, 69, 29+w, 96, 9, rgba(60, 156, 255, 92))
		drawRoundedStroke(img, 29, 69, 29+w, 96, 9, rgba(123, 218, 255, 135), 1)
	}
	r.text(img, shown, 34, 88, r.body, textColor, settingsWidth-64)
	if focused && !selected && time.Now().UnixMilli()%1000 < 520 {
		cursorX := 34 + int(math.Min(float64(textWidth(r.body, shown)+4), float64(settingsWidth-70)))
		fillRect(img, cursorX, 74, cursorX+1, 93, rgba(229, 246, 255, 230))
	}

	if strings.TrimSpace(message) != "" {
		r.text(img, message, 24, 122, r.small, rgba(255, 206, 128, 245), settingsWidth-48)
	} else {
		r.text(img, "默认服务：https://meapi.space", 24, 122, r.small, rgba(178, 193, 214, 220), settingsWidth-48)
	}

	drawTextButton(img, 96, 136, 190, 158, "获取API秘钥", r.body, false)
	drawTextButton(img, 200, 136, 258, 158, "取消", r.body, false)
	drawTextButton(img, 266, 136, 316, 158, "保存", r.body, true)
	return img
}

func quotaAccent(state DisplayState) color.RGBA {
	if state.LowBalance {
		return rgba(255, 78, 101, 255)
	}
	if state.HasData && state.Progress <= 0.25 && !state.Unlimited {
		return rgba(255, 190, 52, 255)
	}
	if !state.HasData {
		return rgba(92, 168, 255, 255)
	}
	return rgba(75, 241, 102, 255)
}

func compactUpdateText(state DisplayState) string {
	text := strings.Replace(state.UpdatedText, "更新时间：", "更新 ", 1)
	if text == "更新 --" {
		return "等待刷新"
	}
	return text
}

func maskAPIKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= 16 {
		return s
	}
	head := string(runes[:7])
	tail := string(runes[len(runes)-7:])
	return head + "******" + tail
}

func drawIconButton(img *image.RGBA, x, y int, label string, face font.Face) {
	fillRoundedRect(img, x, y, x+23, y+23, 11, rgba(255, 255, 255, 28))
	drawRoundedStroke(img, x, y, x+23, y+23, 11, rgba(220, 235, 255, 60), 1)
	renderer().centerText(img, label, x+11, y+16, face, rgba(232, 241, 255, 235), 18)
}

func drawRefreshButton(img *image.RGBA, x, y, size int, accent color.RGBA, phase float64, fetching bool) {
	fillRoundedRect(img, x, y, x+size, y+size, size/2, rgba(255, 255, 255, 20))
	drawRoundedStroke(img, x, y, x+size, y+size, size/2, rgba(225, 239, 255, 54), 1)
	cx := float64(x + size/2)
	cy := float64(y + size/2)
	radius := float64(size) * 0.26
	iconColor := rgba(235, 244, 255, 235)
	if fetching || phase-math.Floor(phase) < 0.16 {
		iconColor = mixRGBA(iconColor, accent, 0.24)
	}
	spin := 0.0
	if fetching {
		spin = phase * math.Pi * 2
	}
	start := -0.88*math.Pi + spin
	end := start + 1.63*math.Pi
	steps := 26
	prevX := cx + math.Cos(start)*radius
	prevY := cy + math.Sin(start)*radius
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		a := start + (end-start)*t
		px := cx + math.Cos(a)*radius
		py := cy + math.Sin(a)*radius
		drawStrokeLine(img, prevX, prevY, px, py, 1.35, iconColor)
		prevX, prevY = px, py
	}
	tipX := cx + math.Cos(end)*radius
	tipY := cy + math.Sin(end)*radius
	drawStrokeLine(img, tipX, tipY, tipX-3.6, tipY-0.3, 1.25, iconColor)
	drawStrokeLine(img, tipX, tipY, tipX-1.1, tipY-3.4, 1.25, iconColor)
}

func drawStatusToast(img *image.RGBA, x, y, width int, message string, accent color.RGBA, face font.Face) {
	if message == "" {
		return
	}
	fillRoundedRect(img, x, y, x+width, y+18, 9, rgba(8, 13, 24, 116))
	drawRoundedStroke(img, x, y, x+width, y+18, 9, withAlpha(accent, 94), 1)
	renderer().centerText(img, message, x+width/2, y+13, face, mixRGBA(accent, rgba(255, 255, 255, 255), 0.40), width-10)
}

func drawTrafficLamp(img *image.RGBA, cx, cy, r int, state DisplayState, _ float64) {
	c, pulse := trafficLampColor(state)
	pulse = clamp01(pulse)
	fillCircle(img, cx, cy, r+4, rgba(6, 10, 17, 136))
	drawGlow(img, cx, cy, 18, c, 0.03+0.43*pulse)
	outer := c
	outer.A = uint8(32 + 146*pulse)
	fillCircle(img, cx, cy, r+2, outer)
	inner := c
	inner.A = uint8(45 + 200*pulse)
	fillCircle(img, cx, cy, r, inner)
	fillCircle(img, cx-2, cy-2, maxInt(1, r/3), rgba(255, 255, 255, uint8(18+108*pulse)))
	drawRoundedStroke(img, cx-r-3, cy-r-3, cx+r+4, cy+r+4, r+3, rgba(255, 255, 255, 34), 1)
}

func drawStackLamp(img *image.RGBA, cx, cy int, lamp StatusColor, state DisplayState) {
	active := activeStatus(state)
	c := rgba(76, 84, 96, 255)
	pulse := 0.12
	if lamp == active {
		c = statusColor(lamp)
		_, pulse = trafficLampColor(state)
	}
	fillCircle(img, cx, cy, 10, rgba(4, 8, 14, 170))
	if lamp == active {
		drawGlow(img, cx, cy, 25, c, 0.08+0.38*pulse)
	}
	outer := c
	outer.A = uint8(24 + 128*pulse)
	fillCircle(img, cx, cy, 9, outer)
	inner := c
	inner.A = uint8(42 + 198*pulse)
	fillCircle(img, cx, cy, 7, inner)
	fillCircle(img, cx-3, cy-3, 2, rgba(255, 255, 255, uint8(14+100*pulse)))
	drawRoundedStroke(img, cx-10, cy-10, cx+11, cy+11, 10, rgba(255, 255, 255, 28), 1)
}

func activeStatus(state DisplayState) StatusColor {
	if state.LowBalance || strings.TrimSpace(state.Error) != "" {
		return StatusRed
	}
	if state.HasData && state.Progress <= 0.25 && !state.Unlimited {
		return StatusYellow
	}
	if state.HasData {
		return StatusGreen
	}
	return StatusYellow
}

func statusColor(status StatusColor) color.RGBA {
	switch status {
	case StatusGreen:
		return rgba(64, 238, 112, 255)
	case StatusYellow:
		return rgba(255, 196, 46, 255)
	case StatusRed:
		return rgba(255, 64, 82, 255)
	default:
		return rgba(94, 104, 119, 255)
	}
}

func trafficLampColor(state DisplayState) (color.RGBA, float64) {
	now := float64(time.Now().UnixMilli()) / 1000
	if state.LowBalance || strings.TrimSpace(state.Error) != "" {
		return rgba(255, 64, 82, 255), blinkPulse(now, 0.62, 0.32)
	}
	if state.HasData && state.Progress <= 0.25 && !state.Unlimited {
		return rgba(255, 196, 46, 255), blinkPulse(now, 2.50, 2.00)
	}
	if state.HasData {
		return rgba(64, 238, 112, 255), blinkPulse(now, 2.50, 2.00)
	}
	return rgba(255, 196, 46, 255), blinkPulse(now, 2.50, 2.00)
}

func blinkPulse(seconds, period, onDuration float64) float64 {
	if period <= 0 || onDuration <= 0 {
		return 0.08
	}
	if math.Mod(seconds, period) < onDuration {
		return 1
	}
	return 0.08
}

func drawTextButton(img *image.RGBA, x1, y1, x2, y2 int, label string, face font.Face, primary bool) {
	bg := rgba(255, 255, 255, 25)
	stroke := rgba(220, 235, 255, 70)
	if primary {
		bg = rgba(58, 157, 255, 84)
		stroke = rgba(112, 218, 255, 145)
	}
	fillRoundedRect(img, x1, y1, x2, y2, 10, bg)
	drawRoundedStroke(img, x1, y1, x2, y2, 10, stroke, 1)
	renderer().centerText(img, label, (x1+x2)/2, y1+16, face, rgba(245, 250, 255, 245), x2-x1-8)
}

func drawQuotaOrb(img *image.RGBA, cx, cy, r int, progress float64, accent color.RGBA, phase float64) {
	drawGlow(img, cx, cy, r+7, rgba(210, 229, 255, 255), 0.13)
	fillCircle(img, cx, cy, r, rgba(255, 255, 255, 18))
	drawGlow(img, cx-6, cy-8, maxInt(7, r-9), rgba(255, 255, 255, 255), 0.10)

	outer := float64(r)
	inner := outer - math.Max(2.4, outer*0.11)
	start := -math.Pi/2 + phase*math.Pi*2
	end := start + clamp01(progress)*math.Pi*2
	for y := cy - r - 4; y <= cy+r+4; y++ {
		for x := cx - r - 4; x <= cx+r+4; x++ {
			dx := float64(x) - float64(cx)
			dy := float64(y) - float64(cy)
			dist := math.Hypot(dx, dy)
			if dist < inner-1 || dist > outer+1 {
				continue
			}
			alpha := ringSmoothAlpha(dist, inner, outer)
			if alpha <= 0 {
				continue
			}
			a := math.Atan2(dy, dx)
			if a < start {
				a += math.Pi * 2
			}
			c := rgba(255, 255, 255, 70)
			if progress >= 0.995 || (a >= start && a <= end) {
				c = accent
			}
			c.A = uint8(float64(c.A) * alpha)
			blendPixel(img, x, y, c)
		}
	}
	fillCircle(img, cx-r/3, cy-r/2, maxInt(3, r/5), rgba(255, 255, 255, 38))
}

func drawStrokeLine(img *image.RGBA, x1, y1, x2, y2, width float64, c color.RGBA) {
	steps := int(math.Ceil(math.Hypot(x2-x1, y2-y1) * 2))
	if steps < 1 {
		steps = 1
	}
	r := int(math.Max(1, math.Round(width)))
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(x1 + (x2-x1)*t))
		y := int(math.Round(y1 + (y2-y1)*t))
		fillCircle(img, x, y, r, c)
	}
}

func ringSmoothAlpha(dist, inner, outer float64) float64 {
	if dist >= inner && dist <= outer {
		return 1
	}
	if dist < inner {
		return clamp01(dist - (inner - 1))
	}
	return clamp01((outer + 1) - dist)
}

func (r *uiRenderer) text(img *image.RGBA, s string, x, baseline int, face font.Face, c color.RGBA, maxWidth int) {
	if s == "" || face == nil {
		return
	}
	s = ellipsize(face, s, maxWidth)
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(x, baseline),
	}
	d.DrawString(s)
}

func (r *uiRenderer) centerText(img *image.RGBA, s string, cx, baseline int, face font.Face, c color.RGBA, maxWidth int) {
	s = ellipsize(face, s, maxWidth)
	w := textWidth(face, s)
	r.text(img, s, cx-w/2, baseline, face, c, maxWidth)
}

func ellipsize(face font.Face, s string, maxWidth int) string {
	if maxWidth <= 0 || textWidth(face, s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		candidate := string(runes) + "..."
		if textWidth(face, candidate) <= maxWidth {
			return candidate
		}
	}
	return "..."
}

func textWidth(face font.Face, s string) int {
	if face == nil || s == "" {
		return 0
	}
	return font.MeasureString(face, s).Round()
}

func rgba(r, g, b, a uint8) color.RGBA {
	return color.RGBA{R: r, G: g, B: b, A: a}
}

func withAlpha(c color.RGBA, a uint8) color.RGBA {
	c.A = a
	return c
}

func maxAlpha(a, min uint8) uint8 {
	if a < min {
		return min
	}
	return a
}

func mixRGBA(a, b color.RGBA, amount float64) color.RGBA {
	amount = clamp01(amount)
	keep := 1 - amount
	return rgba(
		uint8(float64(a.R)*keep+float64(b.R)*amount),
		uint8(float64(a.G)*keep+float64(b.G)*amount),
		uint8(float64(a.B)*keep+float64(b.B)*amount),
		a.A,
	)
}

func fillRect(img *image.RGBA, x1, y1, x2, y2 int, c color.RGBA) {
	draw.Draw(img, image.Rect(x1, y1, x2, y2), image.NewUniform(c), image.Point{}, draw.Over)
}

func fillRoundedRect(img *image.RGBA, x1, y1, x2, y2 int, radius int, c color.RGBA) {
	for y := y1; y < y2; y++ {
		for x := x1; x < x2; x++ {
			a := roundedRectCoverage(float64(x)+0.5, float64(y)+0.5, float64(x1), float64(y1), float64(x2), float64(y2), float64(radius))
			if a <= 0 {
				continue
			}
			cc := c
			cc.A = uint8(float64(cc.A) * a)
			blendPixel(img, x, y, cc)
		}
	}
}

func drawRoundedStroke(img *image.RGBA, x1, y1, x2, y2 int, radius int, c color.RGBA, thickness float64) {
	for y := y1 - 2; y < y2+2; y++ {
		for x := x1 - 2; x < x2+2; x++ {
			outer := roundedRectCoverage(float64(x)+0.5, float64(y)+0.5, float64(x1), float64(y1), float64(x2), float64(y2), float64(radius))
			inner := roundedRectCoverage(float64(x)+0.5, float64(y)+0.5, float64(x1)+thickness, float64(y1)+thickness, float64(x2)-thickness, float64(y2)-thickness, float64(radius)-thickness)
			a := clamp01(outer - inner)
			if a <= 0 {
				continue
			}
			cc := c
			cc.A = uint8(float64(cc.A) * a)
			blendPixel(img, x, y, cc)
		}
	}
}

func roundedRectCoverage(px, py, x1, y1, x2, y2, r float64) float64 {
	cx := clamp(px, x1+r, x2-r)
	cy := clamp(py, y1+r, y2-r)
	dist := math.Hypot(px-cx, py-cy)
	return clamp01(r + 0.5 - dist)
}

func fillCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	rr := float64(r)
	for y := cy - r - 1; y <= cy+r+1; y++ {
		for x := cx - r - 1; x <= cx+r+1; x++ {
			dist := math.Hypot(float64(x-cx)+0.5, float64(y-cy)+0.5)
			a := clamp01(rr + 0.5 - dist)
			if a <= 0 {
				continue
			}
			cc := c
			cc.A = uint8(float64(cc.A) * a)
			blendPixel(img, x, y, cc)
		}
	}
}

func drawGlow(img *image.RGBA, cx, cy, radius int, c color.RGBA, strength float64) {
	if radius <= 0 || strength <= 0 {
		return
	}
	for y := cy - radius; y <= cy+radius; y++ {
		for x := cx - radius; x <= cx+radius; x++ {
			d := math.Hypot(float64(x-cx), float64(y-cy)) / float64(radius)
			if d >= 1 {
				continue
			}
			alpha := math.Pow(1-d, 2.2) * strength
			cc := c
			cc.A = uint8(float64(cc.A) * alpha)
			blendPixel(img, x, y, cc)
		}
	}
}

func blendPixel(img *image.RGBA, x, y int, c color.RGBA) {
	if !image.Pt(x, y).In(img.Bounds()) || c.A == 0 {
		return
	}
	i := img.PixOffset(x, y)
	dstA := float64(img.Pix[i+3]) / 255
	srcA := float64(c.A) / 255
	outA := srcA + dstA*(1-srcA)
	if outA <= 0 {
		return
	}
	dstR := float64(img.Pix[i+0]) / 255
	dstG := float64(img.Pix[i+1]) / 255
	dstB := float64(img.Pix[i+2]) / 255
	srcR := float64(c.R) / 255
	srcG := float64(c.G) / 255
	srcB := float64(c.B) / 255
	outR := (srcR*srcA + dstR*dstA*(1-srcA)) / outA
	outG := (srcG*srcA + dstG*dstA*(1-srcA)) / outA
	outB := (srcB*srcA + dstB*dstA*(1-srcA)) / outA
	img.Pix[i+0] = uint8(clamp(outR, 0, 1)*255 + 0.5)
	img.Pix[i+1] = uint8(clamp(outG, 0, 1)*255 + 0.5)
	img.Pix[i+2] = uint8(clamp(outB, 0, 1)*255 + 0.5)
	img.Pix[i+3] = uint8(outA*255 + 0.5)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func debugRenderInfo() string {
	r := renderer()
	return fmt.Sprintf("font=%t", r.loaded)
}
