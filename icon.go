package main

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/lxn/walk"
)

type StatusColor int

const (
	StatusGray StatusColor = iota
	StatusGreen
	StatusYellow
	StatusRed
)

func iconForDisplay(state DisplayState, dpi int, phase float64) (*walk.Icon, error) {
	return buildLiquidIcon(state, activeStatus(state), dpi, phase)
}

func buildLiquidIcon(state DisplayState, status StatusColor, dpi int, phase float64) (*walk.Icon, error) {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	accent := statusColor(status)
	pulse := trayLiquidPulse(status)
	progress := trayFillLevel(state)

	drawGlow(img, 16, 16, 15, accent, 0.07+0.30*pulse)
	fillCircle(img, 16, 16, 15, color.RGBA{R: 3, G: 6, B: 11, A: 210})
	fillCircle(img, 16, 16, 13, color.RGBA{R: 15, G: 20, B: 29, A: 255})
	drawLiquidLevel(img, 16, 16, 12, progress, accent, phase, pulse)
	fillCircle(img, 11, 10, 3, color.RGBA{R: 255, G: 255, B: 255, A: uint8(28 + 58*pulse)})
	drawIconCircleStroke(img, 16, 16, 14, color.RGBA{R: 230, G: 242, B: 255, A: 86})
	return walk.NewIconFromImageForDPI(img, dpi)
}

func trayFillLevel(state DisplayState) float64 {
	if state.Unlimited {
		return 1
	}
	if !state.HasData {
		return 0
	}
	return clamp01(state.Progress)
}

func trayLiquidPulse(status StatusColor) float64 {
	now := float64(time.Now().UnixMilli()) / 1000
	period := 2.65
	amount := 0.20
	switch status {
	case StatusRed:
		period = 0.78
		amount = 0.34
	case StatusYellow:
		period = 2.35
		amount = 0.24
	}
	if period <= 0 {
		return 0.64
	}
	return 1 - amount + amount*(0.5+0.5*math.Sin(now*math.Pi*2/period))
}

func drawLiquidLevel(img *image.RGBA, cx, cy, radius int, progress float64, accent color.RGBA, phase float64, pulse float64) {
	progress = clamp01(progress)
	top := float64(cy - radius + 3)
	bottom := float64(cy + radius - 2)
	baseLevel := bottom - (bottom-top)*progress
	wavePhase := phase * math.Pi * 2
	amp := 1.15 + (1-progress)*0.55

	for y := cy - radius - 1; y <= cy+radius+1; y++ {
		for x := cx - radius - 1; x <= cx+radius+1; x++ {
			edge := circleAlphaAt(x, y, cx, cy, radius)
			if edge <= 0 {
				continue
			}
			surface := liquidSurfaceY(float64(x), baseLevel, amp, wavePhase)
			py := float64(y) + 0.5
			if py < surface {
				continue
			}
			depth := clamp((py-surface)/math.Max(1, bottom-surface), 0, 1)
			c := mixRGBA(accent, color.RGBA{R: 255, G: 255, B: 255, A: 255}, 0.18*(1-depth))
			c.A = uint8((142 + 83*pulse) * edge)
			blendPixel(img, x, y, c)
		}
	}

	line := mixRGBA(accent, color.RGBA{R: 255, G: 255, B: 255, A: 255}, 0.52)
	for x := cx - radius + 1; x <= cx+radius-1; x++ {
		surface := liquidSurfaceY(float64(x), baseLevel, amp, wavePhase)
		y := int(math.Round(surface))
		for dy := -1; dy <= 1; dy++ {
			edge := circleAlphaAt(x, y+dy, cx, cy, radius)
			if edge <= 0 {
				continue
			}
			cc := line
			cc.A = uint8(float64(112-absInt(dy)*38) * edge)
			blendPixel(img, x, y+dy, cc)
		}
	}

	if progress > 0.03 {
		sparkX := cx - radius + 4 + int(math.Mod(phase*1.9, 1)*float64(radius*2-8))
		sparkY := int(math.Round(liquidSurfaceY(float64(sparkX), baseLevel, amp, wavePhase))) - 2
		if circleAlphaAt(sparkX, sparkY, cx, cy, radius) > 0 {
			fillCircle(img, sparkX, sparkY, 1, color.RGBA{R: 255, G: 255, B: 255, A: 94})
		}
	}
}

func liquidSurfaceY(x, baseLevel, amp, wavePhase float64) float64 {
	return baseLevel +
		math.Sin(x*0.58+wavePhase*1.45)*amp +
		math.Sin(x*0.31-wavePhase*1.05)*amp*0.45
}

func circleAlphaAt(x, y, cx, cy, radius int) float64 {
	dist := math.Hypot(float64(x-cx)+0.5, float64(y-cy)+0.5)
	return clamp01(float64(radius) + 0.5 - dist)
}

func drawIconCircleStroke(img *image.RGBA, cx, cy, radius int, c color.RGBA) {
	outer := float64(radius)
	inner := outer - 1.4
	for y := cy - radius - 1; y <= cy+radius+1; y++ {
		for x := cx - radius - 1; x <= cx+radius+1; x++ {
			d := math.Hypot(float64(x-cx)+0.5, float64(y-cy)+0.5)
			if d < inner || d > outer+0.8 {
				continue
			}
			a := ringSmoothAlpha(d, inner, outer)
			cc := c
			cc.A = uint8(float64(cc.A) * a)
			blendPixel(img, x, y, cc)
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
