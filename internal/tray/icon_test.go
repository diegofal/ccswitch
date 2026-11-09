package tray

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func decode(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	return img
}

func TestIconPNGIsAValid32x32PNG(t *testing.T) {
	img := decode(t, IconPNG(42, LevelOK))
	if b := img.Bounds(); b.Dx() != IconSize || b.Dy() != IconSize || IconSize != 32 {
		t.Fatalf("icon is %v, want 32×32", b)
	}
}

func TestIconPNGDiffersPerLevel(t *testing.T) {
	seen := map[string]string{}
	for _, level := range []string{LevelOK, LevelWarn, LevelAlert, LevelUnreadable, LevelStale} {
		key := string(IconPNG(90, level))
		if other, dup := seen[key]; dup {
			t.Errorf("%s and %s draw the same icon", level, other)
		}
		seen[key] = level
	}
}

// pixel is the colour at x, y as 8-bit RGBA.
func pixel(img image.Image, x, y int) (r, g, b, a uint8) {
	cr, cg, cb, ca := img.At(x, y).RGBA()
	return uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), uint8(ca >> 8)
}

func TestIconPNGRingFollowsThePercent(t *testing.T) {
	const ringY = 2 // a pixel row inside the ring
	top, right, bottom := [2]int{IconSize / 2, ringY}, [2]int{IconSize - 1 - ringY, IconSize / 2}, [2]int{IconSize / 2, IconSize - 1 - ringY}
	img := decode(t, IconPNG(30, LevelOK))
	// 30 %: the arc runs clockwise from 12 o'clock past 3 o'clock, not to 6.
	rT, gT, _, _ := pixel(img, top[0]+1, top[1])
	rR, gR, _, _ := pixel(img, right[0], right[1])
	rB, gB, _, _ := pixel(img, bottom[0], bottom[1])
	if gT <= rT || gR <= rR {
		t.Errorf("30 %%: the arc at 12 and 3 o'clock should be green: top %d,%d right %d,%d", rT, gT, rR, gR)
	}
	if gB > rB+40 {
		t.Errorf("30 %%: 6 o'clock should be the grey track, got r=%d g=%d", rB, gB)
	}
}

func TestIconPNGColours(t *testing.T) {
	cases := []struct {
		level string
		want  func(r, g, b uint8) bool
	}{
		{LevelOK, func(r, g, _ uint8) bool { return g > r+60 }},
		{LevelWarn, func(r, g, b uint8) bool { return r > 180 && g > 120 && b < 80 }},
		{LevelAlert, func(r, g, _ uint8) bool { return r > 180 && g < 80 }},
		{LevelUnreadable, func(r, g, _ uint8) bool { return r > 180 && g < 80 }},
		{LevelStale, func(r, g, b uint8) bool { return absDiff(r, g) < 20 && absDiff(g, b) < 20 }},
	}
	for _, c := range cases {
		img := decode(t, IconPNG(100, c.level))
		if r, g, b, a := pixel(img, IconSize/2, 2); a < 200 || !c.want(r, g, b) {
			t.Errorf("%s: ring pixel r=%d g=%d b=%d a=%d", c.level, r, g, b, a)
		}
	}
}

func TestIconPNGUnreadableHasADot(t *testing.T) {
	_, _, _, a := pixel(decode(t, IconPNG(0, LevelOK)), IconSize/2, IconSize/2)
	if a != 0 {
		t.Errorf("ok icon centre alpha = %d, want transparent", a)
	}
	r, g, _, a := pixel(decode(t, IconPNG(0, LevelUnreadable)), IconSize/2, IconSize/2)
	if a < 200 || r < 180 || g > 80 {
		t.Errorf("unreadable icon centre r=%d g=%d a=%d, want a red dot", r, g, a)
	}
}

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

func TestIconPNGSmallUseStillShows(t *testing.T) {
	r, g, _, a := pixel(decode(t, IconPNG(0.5, LevelOK)), IconSize/2+1, 2)
	if a < 200 || g <= r {
		t.Errorf("0.5 %%: no arc at 12 o'clock (r=%d g=%d a=%d)", r, g, a)
	}
}

func TestIconPNGUnknownLevelIsRed(t *testing.T) {
	if string(IconPNG(50, "bogus")) != string(IconPNG(50, LevelAlert)) {
		t.Error("an unknown level must draw as alert red, never invisible")
	}
}

func TestMarkPNGNoShapeIsNoPicture(t *testing.T) {
	if b := MarkPNG(Mark{}); b != nil {
		t.Errorf("MarkPNG(no shape) = %d bytes, want none", len(b))
	}
}

func TestMarkPNGDotIsTheLevelColourInTheMiddleOnly(t *testing.T) {
	img := decode(t, MarkPNG(Mark{Shape: MarkDot, Level: LevelWarn}))
	if b := img.Bounds(); b.Dx() != MarkSize || b.Dy() != MarkSize {
		t.Fatalf("mark is %v, want %d×%d", b, MarkSize, MarkSize)
	}
	if r, g, b, a := pixel(img, MarkSize/2, MarkSize/2); r != 230 || g != 160 || b != 20 || a != 255 {
		t.Errorf("middle = %d,%d,%d,%d, want amber", r, g, b, a)
	}
	// The edge stays clear: Windows clips the picture to 16 px at 100 %.
	for _, p := range [][2]int{{1, MarkSize / 2}, {MarkSize - 2, MarkSize / 2}, {MarkSize / 2, 1}, {MarkSize / 2, MarkSize - 2}} {
		if _, _, _, a := pixel(img, p[0], p[1]); a != 0 {
			t.Errorf("pixel %v is drawn (alpha %d); the margin should be clear", p, a)
		}
	}
}

func TestMarkPNGRingFollowsThePercent(t *testing.T) {
	img := decode(t, MarkPNG(Mark{Shape: MarkRing, Level: LevelAlert, Percent: 30}))
	top, bottom := [2]int{MarkSize/2 + 1, 4}, [2]int{MarkSize / 2, MarkSize - 5}
	if r, g, _, _ := pixel(img, top[0], top[1]); r < 200 || g > 60 {
		t.Errorf("30 %%: 12 o'clock should be red, got r=%d g=%d", r, g)
	}
	if r, g, _, _ := pixel(img, bottom[0], bottom[1]); r > 200 || r-g > 40 {
		t.Errorf("30 %%: 6 o'clock should be the grey track, got r=%d g=%d", r, g)
	}
	if _, _, _, a := pixel(img, MarkSize/2, MarkSize/2); a != 0 {
		t.Errorf("the ring's middle is drawn (alpha %d)", a)
	}
}
