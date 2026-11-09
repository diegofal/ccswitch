package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// IconSize is the tray icon's width and height in pixels.
const IconSize = 32

const (
	center   = IconSize / 2.0
	outerR   = IconSize/2.0 - 0.5
	innerR   = outerR - 4.5
	dotR     = 5.0
	samples  = 4 // per axis, for anti-aliased edges
	minShown = 4 // percent: any use draws at least this much arc
)

var (
	track  = color.NRGBA{R: 160, G: 160, B: 160, A: 110}
	colors = map[string]color.NRGBA{
		LevelOK:         {R: 46, G: 160, B: 67, A: 255},
		LevelWarn:       {R: 230, G: 160, B: 20, A: 255},
		LevelAlert:      {R: 215, G: 40, B: 40, A: 255},
		LevelUnreadable: {R: 215, G: 40, B: 40, A: 255},
		LevelStale:      {R: 128, G: 128, B: 128, A: 255},
	}
)

// IconPNG draws the tray icon: a ring filled clockwise from 12 o'clock to
// percent, in the level's colour, over a faint track. An unreadable read
// is a full red ring with a red dot in the middle, so it never looks like
// a number.
func IconPNG(percent float64, level string) []byte {
	fill, ok := colors[level]
	if !ok {
		fill = colors[LevelUnreadable]
	}
	sweep := arcFraction(percent)
	if level == LevelUnreadable {
		sweep = 1
	}
	img := image.NewNRGBA(image.Rect(0, 0, IconSize, IconSize))
	for y := 0; y < IconSize; y++ {
		for x := 0; x < IconSize; x++ {
			img.SetNRGBA(x, y, pixelColor(x, y, sweep, fill, level == LevelUnreadable))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err) // encoding an in-memory NRGBA cannot fail
	}
	return buf.Bytes()
}

func arcFraction(percent float64) float64 {
	switch {
	case percent <= 0:
		return 0
	case percent >= 100:
		return 1
	case percent < minShown:
		return minShown / 100.0
	}
	return percent / 100
}

// pixelColor averages samples×samples points inside the pixel: each is the
// arc colour, the track, the dot or nothing.
func pixelColor(x, y int, sweep float64, fill color.NRGBA, dot bool) color.NRGBA {
	var r, g, b, a float64
	for i := 0; i < samples; i++ {
		for j := 0; j < samples; j++ {
			c := sampleColor(float64(x)+(float64(i)+0.5)/samples, float64(y)+(float64(j)+0.5)/samples, sweep, fill, dot)
			alpha := float64(c.A)
			r += float64(c.R) * alpha
			g += float64(c.G) * alpha
			b += float64(c.B) * alpha
			a += alpha
		}
	}
	if a == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{R: uint8(r / a), G: uint8(g / a), B: uint8(b / a), A: uint8(a / (samples * samples))}
}

func sampleColor(px, py, sweep float64, fill color.NRGBA, dot bool) color.NRGBA {
	dx, dy := px-center, py-center
	d := math.Hypot(dx, dy)
	switch {
	case dot && d <= dotR:
		return fill
	case d < innerR || d > outerR:
		return color.NRGBA{}
	}
	angle := math.Atan2(dx, -dy) // 0 at 12 o'clock, clockwise
	if angle < 0 {
		angle += 2 * math.Pi
	}
	if angle < sweep*2*math.Pi {
		return fill
	}
	return track
}
