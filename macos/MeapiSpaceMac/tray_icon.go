package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"time"
)

func trayIconPNG(state DisplayState, phase float64) []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	status := activeStatus(state)
	accent := statusColour(status)
	pulse := trayPulse(status)
	progress := 0.0
	if state.Unlimited {
		progress = 1
	} else if state.HasData {
		progress = clamp01(state.Progress)
	}

	drawGlow(img, 16, 16, 15, accent, 0.08+0.30*pulse)
	fillCircle(img, 16, 16, 15, color.RGBA{R: 3, G: 6, B: 11, A: 220})
	fillCircle(img, 16, 16, 13, color.RGBA{R: 15, G: 20, B: 29, A: 255})
	drawLiquidLevel(img, 16, 16, 12, progress, accent, phase, pulse)
	fillCircle(img, 11, 10, 3, color.RGBA{R: 255, G: 255, B: 255, A: uint8(28 + 58*pulse)})
	drawCircleStroke(img, 16, 16, 14, color.RGBA{R: 230, G: 242, B: 255, A: 86})

	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}

func statusColour(status string) color.RGBA {
	switch status {
	case "green":
		return color.RGBA{R: 64, G: 238, B: 112, A: 255}
	case "yellow":
		return color.RGBA{R: 255, G: 196, B: 46, A: 255}
	case "red":
		return color.RGBA{R: 255, G: 64, B: 82, A: 255}
	default:
		return color.RGBA{R: 94, G: 104, B: 119, A: 255}
	}
}

func trayPulse(status string) float64 {
	now := float64(time.Now().UnixMilli()) / 1000
	period := 2.65
	amount := 0.20
	if status == "red" {
		period = 0.78
		amount = 0.34
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
			c := mixColour(accent, color.RGBA{R: 255, G: 255, B: 255, A: 255}, 0.18*(1-depth))
			c.A = uint8((142 + 83*pulse) * edge)
			blendPixel(img, x, y, c)
		}
	}

	line := mixColour(accent, color.RGBA{R: 255, G: 255, B: 255, A: 255}, 0.52)
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
}

func liquidSurfaceY(x, baseLevel, amp, wavePhase float64) float64 {
	return baseLevel +
		math.Sin(x*0.58+wavePhase*1.45)*amp +
		math.Sin(x*0.31-wavePhase*1.05)*amp*0.45
}

func drawCircleStroke(img *image.RGBA, cx, cy, radius int, c color.RGBA) {
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

func ringSmoothAlpha(dist, inner, outer float64) float64 {
	if dist >= inner && dist <= outer {
		return 1
	}
	if dist < inner {
		return clamp01(dist - (inner - 1))
	}
	return clamp01((outer + 1) - dist)
}

func circleAlphaAt(x, y, cx, cy, radius int) float64 {
	dist := math.Hypot(float64(x-cx)+0.5, float64(y-cy)+0.5)
	return clamp01(float64(radius) + 0.5 - dist)
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

func mixColour(a, b color.RGBA, amount float64) color.RGBA {
	amount = clamp01(amount)
	keep := 1 - amount
	return color.RGBA{
		R: uint8(float64(a.R)*keep + float64(b.R)*amount),
		G: uint8(float64(a.G)*keep + float64(b.G)*amount),
		B: uint8(float64(a.B)*keep + float64(b.B)*amount),
		A: a.A,
	}
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

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
