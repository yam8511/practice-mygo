package main

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The view runs without a window in tests, which click and type as a user
// would.
func TestView(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 640, 520)
	if err := tt.Click("+"); err != nil {
		t.Fatal(err)
	}
	if a.count != 1 || !tt.HasText("1") {
		t.Errorf("count %d after a click, texts %q", a.count, tt.Texts())
	}
	if err := tt.Click("Name"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Ada")
	if !tt.HasText("Hello, Ada!") {
		t.Errorf("texts %q after typing a name", tt.Texts())
	}
}

// Dragging over the canvas draws a stroke, which undo takes back.
func TestPaint(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1000, 700)
	if err := tt.Click("畫筆"); err != nil {
		t.Fatal(err)
	}
	r, ok := tt.Find("canvas")
	if !ok {
		t.Fatalf("no canvas, texts %q", tt.Texts())
	}
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	tt.Move(cx, cy)
	tt.Press(cx, cy)
	for i := 1; i <= 10; i++ {
		tt.Move(cx+float32(i)*10, cy+float32(i)*5)
	}
	tt.Release(cx+100, cy+50)
	if len(a.paint.strokes) != 1 || len(a.paint.strokes[0].points) < 5 {
		t.Fatalf("strokes %+v after a drag", a.paint.strokes)
	}
	// The middle of the canvas is the middle of the 1280×800 page.
	if p := a.paint.strokes[0].points[0]; p.X < 600 || p.X > 680 || p.Y < 360 || p.Y > 440 {
		t.Errorf("first point %+v, want near (640, 400)", p)
	}
	if err := tt.Click("復原"); err != nil {
		t.Fatal(err)
	}
	if len(a.paint.strokes) != 0 {
		t.Errorf("%d strokes after undo", len(a.paint.strokes))
	}
}

// openPaint shows the paint page and returns the canvas's center, and how
// many DIPs a pixel of the picture shows at.
func openPaint(t *testing.T, a *app, tt *ui.Tester, tool string) (cx, cy, scale float32) {
	t.Helper()
	if err := tt.Click("畫筆"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(tool); err != nil {
		t.Fatal(err)
	}
	r, ok := tt.Find("canvas")
	if !ok {
		t.Fatalf("no canvas, texts %q", tt.Texts())
	}
	_, _, scale = a.paint.fit(r.W, r.H)
	return r.X + r.W/2, r.Y + r.H/2, scale
}

// Dragging with the rectangle tool draws a closed rectangle from where the
// drag started.
func TestRectangle(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1000, 700)
	cx, cy, _ := openPaint(t, a, tt, "矩形")
	tt.Move(cx, cy)
	tt.Press(cx, cy)
	for i := 1; i <= 5; i++ {
		tt.Move(cx+float32(i)*20, cy+float32(i)*10)
	}
	tt.Release(cx+100, cy+50)
	if len(a.paint.strokes) != 1 {
		t.Fatalf("%d strokes after a drag", len(a.paint.strokes))
	}
	s := a.paint.strokes[0]
	if !s.closed || len(s.points) != 4 || s.points[0].Y != s.points[1].Y || s.points[2].X != s.points[1].X {
		t.Errorf("rectangle %+v", s)
	}
	// A click without a drag draws nothing.
	tt.Click("canvas")
	if len(a.paint.strokes) != 1 {
		t.Errorf("%d strokes after a click", len(a.paint.strokes))
	}
}

// Clicks with the polygon tool add corners, and a click on the first
// corner closes the polygon; the button finishes one too.
func TestPolygon(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1000, 700)
	cx, cy, _ := openPaint(t, a, tt, "多邊形")
	corners := [][2]float32{{cx, cy}, {cx + 120, cy}, {cx + 60, cy + 90}}
	for _, p := range corners {
		tt.Move(p[0], p[1])
		tt.ClickAt(p[0], p[1])
		tt.Move(p[0]+1, p[1]+40) // apart, so the clicks are not a double click
	}
	if len(a.paint.poly) != 3 || len(a.paint.strokes) != 0 {
		t.Fatalf("%d corners, %d strokes after three clicks", len(a.paint.poly), len(a.paint.strokes))
	}
	tt.Move(cx+3, cy+2)
	tt.ClickAt(cx+3, cy+2) // near the first corner
	if len(a.paint.strokes) != 1 || !a.paint.strokes[0].closed || len(a.paint.strokes[0].points) != 3 || a.paint.poly != nil {
		t.Fatalf("strokes %+v, corners %v after closing", a.paint.strokes, a.paint.poly)
	}

	for _, p := range corners {
		tt.Move(p[0]-200, p[1]-100)
		tt.ClickAt(p[0]-200, p[1]-100)
		tt.Move(p[0]-150, p[1])
	}
	if err := tt.Click("復原"); err != nil { // takes back the last corner
		t.Fatal(err)
	}
	if len(a.paint.poly) != 2 {
		t.Fatalf("%d corners after undo", len(a.paint.poly))
	}
	tt.Move(cx-100, cy+150)
	tt.ClickAt(cx-100, cy+150)
	if err := tt.Click("完成多邊形"); err != nil {
		t.Fatal(err)
	}
	if len(a.paint.strokes) != 2 || len(a.paint.strokes[1].points) != 3 {
		t.Errorf("strokes %+v after finishing", a.paint.strokes)
	}
}

// Saving draws the strokes on the picture's own pixels.
func TestRender(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	out := render(img, []stroke{{points: []point{{10, 50}, {90, 50}}, color: ui.RGB(255, 0, 0), width: 10}})
	if got := out.RGBAAt(50, 50); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("on the line %v, want red", got)
	}
	if got := out.RGBAAt(50, 70); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("off the line %v, want white", got)
	}
	if got := out.RGBAAt(8, 50); got.G == 255 {
		t.Errorf("round cap missing at the start: %v", got)
	}

	// A closed shape draws its last side back to the first point.
	tri := render(img, []stroke{{points: []point{{10, 10}, {90, 10}, {10, 90}}, closed: true, color: ui.RGB(0, 0, 255), width: 6}})
	if got := tri.RGBAAt(50, 50); got != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("on the closing side %v, want blue", got)
	}
}
