package main

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/egoist/mygo/ui"
)

// square is a white picture with a dark square from (100, 80) to (300, 220).
func square() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(100, 80, 300, 220), image.NewUniform(color.RGBA{40, 40, 60, 255}), image.Point{}, draw.Src)
	return img
}

// nearSquareEdge is how far p is from the square's outline.
func nearSquareEdge(p point) float32 {
	dx := min(abs(p.X-100), abs(p.X-300))
	dy := min(abs(p.Y-80), abs(p.Y-220))
	switch {
	case p.X >= 100 && p.X <= 300:
		return min(dy, max(dx, 0)) // on a horizontal side, or in the square
	case p.Y >= 80 && p.Y <= 220:
		return dx
	}
	return dist(p, point{min(max(p.X, 100), 300), min(max(p.Y, 80), 220)})
}

func syncLasso() *lasso {
	return &lasso{run: func(work func() func()) { work()() }}
}

// Anchors clicked a few pixels off the square's corners snap to them, and
// the outline closed through them follows the square's sides.
func TestLassoFollowsEdges(t *testing.T) {
	l := syncLasso()
	l.prepare(square())
	if l.status != lassoReady {
		t.Fatalf("status %v", l.status)
	}
	for _, p := range []point{{103, 84}, {296, 77}, {303, 224}, {97, 217}} {
		l.move(p, 6)
		l.addAnchor(p, 6)
	}
	// The line to the pointer follows the edges too.
	l.move(point{102, 150}, 6)
	if l.kind != previewPath {
		t.Errorf("preview %v, want along a path", l.kind)
	}
	out := l.close()
	if len(out) < 4 {
		t.Fatalf("outline %v", out)
	}
	for _, p := range out {
		if d := nearSquareEdge(p); d > 3 {
			t.Errorf("point %v is %.1f px off the square's edge; outline %v", p, d, out)
		}
	}
	// Simplified: the square's sides are straight, so few points remain.
	if len(out) > 12 {
		t.Errorf("%d points after simplifying: %v", len(out), out)
	}
}

func TestLassoRemoveAnchor(t *testing.T) {
	l := syncLasso()
	l.prepare(square())
	for _, p := range []point{{100, 80}, {300, 80}, {300, 220}} {
		l.addAnchor(p, 6)
	}
	if n := l.removeLastAnchor(); n != 2 || len(l.segments) != 1 {
		t.Errorf("%d anchors, %d segments after removing one", n, len(l.segments))
	}
	if l.close() != nil {
		t.Error("closed with 2 anchors")
	}
}

// Over the page: clicks add anchors, Backspace takes one back, and a click
// on the first anchor closes the outline into a polygon.
func TestLassoPage(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1000, 700)
	cx, cy, _ := openPaint(t, a, tt, "套索")
	if a.paint.lasso.status != lassoReady {
		t.Fatalf("lasso %v", a.paint.lasso.status)
	}
	corners := [][2]float32{{cx, cy}, {cx + 120, cy}, {cx + 60, cy + 90}, {cx - 50, cy + 60}}
	for _, p := range corners {
		tt.Move(p[0], p[1])
		tt.ClickAt(p[0], p[1])
		tt.Move(p[0]+1, p[1]+40) // apart, so the clicks are not a double click
	}
	if n := len(a.paint.lasso.anchors); n != 4 {
		t.Fatalf("%d anchors after four clicks", n)
	}
	tt.Key(0, ui.KeyBackspace)
	if n := len(a.paint.lasso.anchors); n != 3 {
		t.Fatalf("%d anchors after Backspace", n)
	}
	tt.Move(cx+3, cy+2)
	tt.ClickAt(cx+3, cy+2) // on the first anchor
	if len(a.paint.strokes) != 1 || a.paint.strokes[0].kind != toolPolygon || len(a.paint.strokes[0].points) < 3 {
		t.Fatalf("strokes %+v after closing", a.paint.strokes)
	}
	if len(a.paint.lasso.anchors) != 0 {
		t.Errorf("%d anchors left", len(a.paint.lasso.anchors))
	}
}
