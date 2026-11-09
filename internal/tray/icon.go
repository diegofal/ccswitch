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

// MarkSize is a menu row picture's width and height in pixels. Windows
// draws it in the check-mark column, 16 px at 100 % scaling; the shapes
// keep a margin so a clipped edge cuts only transparent pixels.
const MarkSize = 20

const (
	samples  = 4 // per axis, for anti-aliased edges
	minShown = 4 // percent: any use draws at least this much arc
)

// shape is a picture's geometry: a ring between innerR and outerR, a dot
// of dotR in the middle, in a square of size pixels.
type shape struct {
	size                         int
	center, outerR, innerR, dotR float64
}

var (
	iconShape = shape{size: IconSize, center: IconSize / 2.0, outerR: IconSize/2.0 - 0.5, innerR: IconSize/2.0 - 5, dotR: 5}
	ringShape = shape{size: MarkSize, center: MarkSize / 2.0, outerR: 7, innerR: 4}
	dotShape  = shape{size: MarkSize, center: MarkSize / 2.0, dotR: 5.5}
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
	return draw(iconShape, sweep, fill, level == LevelUnreadable)
}

// MarkPNG draws a menu row's picture: a dot in the level's colour, or a
// ring like the tray icon's filled to the percent. No shape is no picture.
func MarkPNG(m Mark) []byte {
	fill, ok := colors[m.Level]
	if !ok {
		fill = colors[LevelUnreadable]
	}
	switch m.Shape {
	case MarkDot:
		return draw(dotShape, 0, fill, true)
	case MarkRing:
		return draw(ringShape, arcFraction(m.Percent), fill, false)
	}
	return nil
}

func draw(sh shape, sweep float64, fill color.NRGBA, dot bool) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, sh.size, sh.size))
	for y := 0; y < sh.size; y++ {
		for x := 0; x < sh.size; x++ {
			img.SetNRGBA(x, y, pixelColor(sh, x, y, sweep, fill, dot))
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
func pixelColor(sh shape, x, y int, sweep float64, fill color.NRGBA, dot bool) color.NRGBA {
	var r, g, b, a float64
	for i := 0; i < samples; i++ {
		for j := 0; j < samples; j++ {
			c := sampleColor(sh, float64(x)+(float64(i)+0.5)/samples, float64(y)+(float64(j)+0.5)/samples, sweep, fill, dot)
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

func sampleColor(sh shape, px, py, sweep float64, fill color.NRGBA, dot bool) color.NRGBA {
	dx, dy := px-sh.center, py-sh.center
	d := math.Hypot(dx, dy)
	switch {
	case dot && d <= sh.dotR:
		return fill
	case d < sh.innerR || d > sh.outerR:
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
