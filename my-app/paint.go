package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"golang.org/x/image/vector"
)

// point is a place on the picture, in its pixels.
type point struct{ X, Y float32 }

// stroke is one shape drawn on the picture, in its pixels, so that it stays
// where it was drawn whatever size the picture shows at: a line through its
// points, back to the first when closed, as a rectangle or a polygon.
type stroke struct {
	points []point
	closed bool
	color  ui.Color
	width  float32
}

// The tools that draw.
const (
	toolPen = iota
	toolRect
	toolPolygon
)

var toolNames = []string{"畫筆", "矩形", "多邊形"}

// closeDistance is how near the first corner of a polygon, in DIPs, a click
// closes it.
const closeDistance = 10

// penColors are the colors of the pen to choose from.
var penColors = []ui.Color{
	ui.Hex("#ef4444"), ui.Hex("#f59e0b"), ui.Hex("#10b981"),
	ui.Hex("#3b82f6"), ui.Hex("#8b5cf6"), ui.Hex("#111827"), ui.Hex("#ffffff"),
}

// paint is the page that shows a picture and draws on it.
type paint struct {
	// img is the picture as decoded, which saving draws the strokes on,
	// and photo the same picture on the GPU.
	img   image.Image
	photo *ui.Bitmap
	name  string

	tool    int
	strokes []stroke
	// drawing is whether a press is drawing the last stroke, a line of the
	// pen or a rectangle from anchor.
	drawing bool
	anchor  point
	// poly are the corners of the polygon being drawn, and hover where the
	// pointer is over the picture, which its next side goes to.
	poly    []point
	hover   point
	hovered bool

	color  int
	width  float64
	status string
}

func newPaint() *paint {
	// A blank page to draw on until a picture is opened.
	blank := image.NewRGBA(image.Rect(0, 0, 1280, 800))
	draw.Draw(blank, blank.Bounds(), image.White, image.Point{}, draw.Src)
	return &paint{img: blank, photo: ui.NewBitmap(blank), name: "drawing", width: 6}
}

// fit returns where the picture shows in a box of w×h DIPs, keeping its
// proportions: its offset in the box and DIPs per pixel.
func (p *paint) fit(w, h float32) (ox, oy, scale float32) {
	iw, ih := p.photo.Size()
	scale = min(w/float32(iw), h/float32(ih))
	return (w - float32(iw)*scale) / 2, (h - float32(ih)*scale) / 2, scale
}

// newStroke starts a stroke in the pen's color and the width it shows at
// as it is drawn.
func (p *paint) newStroke(closed bool, scale float32) stroke {
	return stroke{closed: closed, color: penColors[p.color], width: float32(p.width) / scale}
}

// finishPolygon adds the polygon being drawn, when it has a surface.
func (p *paint) finishPolygon(scale float32) {
	if len(p.poly) >= 3 && scale > 0 {
		s := p.newStroke(true, scale)
		s.points = p.poly
		p.strokes = append(p.strokes, s)
	}
	p.poly = nil
}

// undo takes back the last corner of the polygon being drawn, else the
// last stroke.
func (p *paint) undo() {
	if n := len(p.poly); n > 0 {
		p.poly = p.poly[:n-1]
	} else if n := len(p.strokes); n > 0 {
		p.strokes = p.strokes[:n-1]
	}
}

func dist(a, b point) float32 {
	return float32(math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)))
}

func (p *paint) view(c *ui.Context, win *mygo.Window) {
	t := c.Theme()
	ui.Column(c).Fill().Gap(12).Children(func() {
		// The tools, on a bar of glass.
		ui.Row(c).Padding(8, 12).Gap(10).Radius(24).AlignItems(ui.Center).Wrap().Material(glass.Glass{}).Children(func() {
			if ui.Button(c, "開啟圖片").Clicked() {
				p.open(win)
			}
			if ui.Button(c, "儲存 PNG").Clicked() {
				p.save(win)
			}
			ui.Box(c).Size(1, 24).Background(t.Border)
			ui.Segmented(c, &p.tool, toolNames...)
			if p.tool != toolPolygon {
				p.poly = nil
			}
			ui.Box(c).Size(1, 24).Background(t.Border)
			for i, col := range penColors {
				sw := ui.Box(c).Size(26, 26).Radius(13).Background(col).Border(1, t.Border).Label(fmt.Sprintf("color %d", i+1))
				if i == p.color {
					sw.Border(3, t.Accent)
				}
				if sw.Clicked() {
					p.color = i
				}
			}
			ui.Box(c).Size(1, 24).Background(t.Border)
			ui.Text(c, "筆寬")
			ui.Slider(c, &p.width, 1, 40).Width(120)
			ui.Textf(c, "%.0f", p.width).Width(24)
			ui.Box(c).Size(1, 24).Background(t.Border)
			busy := len(p.strokes) > 0 || len(p.poly) > 0
			if ui.Button(c, "復原").Disabled(!busy).Clicked() {
				p.undo()
			}
			if ui.Button(c, "清除").Disabled(!busy).Clicked() {
				p.strokes, p.poly = nil, nil
			}
			if p.status != "" {
				ui.Text(c, p.status).TextColor(t.TextMuted)
			}
		})

		// The picture, and the strokes over it. Its box is the previous
		// frame's, which the polygon's buttons below need as well.
		canvas := ui.Box(c).Grow(1).Radius(16).Clip().Background(t.Surface).Cursor(ui.CursorCrosshair).Label("canvas")
		b := canvas.Bounds()
		ox, oy, scale := p.fit(b.W, b.H)
		iw, ih := p.photo.Size()
		x, y, over := canvas.PointerPosition()
		if scale > 0 {
			// Where the pointer is on the picture, kept inside it.
			p.hover = point{min(max((x-ox)/scale, 0), float32(iw)), min(max((y-oy)/scale, 0), float32(ih))}
		}
		p.hovered = over && scale > 0
		canvas.Dragged() // the tools take the drag, not the window
		pressed := canvas.Pressed() && p.hovered
		clicked, double := canvas.Clicked(), canvas.DoubleClicked()

		switch p.tool {
		case toolPen:
			if pressed {
				if !p.drawing {
					p.strokes = append(p.strokes, p.newStroke(false, scale))
					p.drawing = true
				}
				s := &p.strokes[len(p.strokes)-1]
				if n := len(s.points); n == 0 || dist(p.hover, s.points[n-1]) >= 0.5 {
					s.points = append(s.points, p.hover)
				}
			} else {
				p.drawing = false
			}
		case toolRect:
			if pressed {
				if !p.drawing {
					p.anchor = p.hover
					p.strokes = append(p.strokes, p.newStroke(true, scale))
					p.drawing = true
				}
				a, z := p.anchor, p.hover
				p.strokes[len(p.strokes)-1].points = []point{a, {z.X, a.Y}, z, {a.X, z.Y}}
			} else if p.drawing {
				// A click without a drag draws no rectangle.
				if s := p.strokes[len(p.strokes)-1]; dist(s.points[0], s.points[2])*scale < 2 {
					p.strokes = p.strokes[:len(p.strokes)-1]
				}
				p.drawing = false
			}
		case toolPolygon:
			switch {
			case !clicked || !p.hovered:
			case double:
				// The first click of the two added the last corner.
				p.finishPolygon(scale)
			case len(p.poly) >= 3 && dist(p.hover, p.poly[0])*scale <= closeDistance:
				p.finishPolygon(scale)
			default:
				p.poly = append(p.poly, p.hover)
			}
		}

		canvas.Draw(func(pt *ui.Painter, r ui.Rect) {
			ox, oy, scale := p.fit(r.W, r.H)
			at := func(q point) (float32, float32) { return r.X + ox + q.X*scale, r.Y + oy + q.Y*scale }
			pt.Image(p.photo, ui.Rect{X: r.X + ox, Y: r.Y + oy, W: float32(iw) * scale, H: float32(ih) * scale}, ui.FillBox)
			for _, s := range p.strokes {
				pt.StrokePath(polyline(s.points, s.closed, at), s.width*scale, s.color)
			}
			if len(p.poly) == 0 {
				return
			}
			// The polygon being drawn: its sides so far and the next one to
			// the pointer, half transparent, and its corners, the first
			// larger when a click there would close it.
			col := penColors[p.color]
			path := polyline(p.poly, false, at)
			if p.hovered {
				path.LineTo(at(p.hover))
			}
			pt.StrokePath(path, float32(p.width), col.Alpha(0.6))
			closing := len(p.poly) >= 3 && p.hovered && dist(p.hover, p.poly[0])*scale <= closeDistance
			for i, q := range p.poly {
				px, py := at(q)
				rad := float32(4)
				if i == 0 && closing {
					rad = 8
				}
				var dot ui.Path
				dot.Circle(px, py, rad)
				pt.FillPath(&dot, ui.RGB(255, 255, 255))
				pt.StrokePath(&dot, 2, col)
			}
		})

		if p.tool == toolPolygon {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(6, 6, 6, 16).Radius(24).Material(glass.Glass{}).Children(func() {
				ui.Text(c, "點擊加入頂點，點回第一個頂點或雙擊完成").Grow(1)
				if ui.PrimaryButton(c, "完成多邊形").Disabled(len(p.poly) < 3).Clicked() {
					p.finishPolygon(scale)
				}
				if ui.Button(c, "取消").Disabled(len(p.poly) == 0).Clicked() {
					p.poly = nil
				}
			})
		}
	})
}

// polyline is the path through points, placed in the window by at, back
// to the first when closed. A single point draws a dot.
func polyline(points []point, closed bool, at func(point) (float32, float32)) *ui.Path {
	var path ui.Path
	for i, q := range points {
		x, y := at(q)
		if i == 0 {
			path.MoveTo(x, y)
		}
		path.LineTo(x, y)
	}
	if closed {
		path.Close()
	}
	return &path
}

// open asks for a picture and shows it, without the strokes of the last.
func (p *paint) open(win *mygo.Window) {
	if win == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:  win,
			Title:   "開啟圖片",
			Filters: []mygo.FileFilter{{Name: "Images", Extensions: []string{"png", "jpg", "jpeg", "gif"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		data, err := os.ReadFile(paths[0])
		var img image.Image
		if err == nil {
			img, _, err = image.Decode(bytes.NewReader(data))
		}
		// The bitmap is made from the same decoded image as the one saving
		// draws on, so that the strokes land where they show.
		var photo *ui.Bitmap
		if err == nil {
			photo = ui.NewBitmap(img)
		}
		win.Update(func() {
			if err != nil {
				p.status = "無法開啟：" + err.Error()
				return
			}
			p.img, p.photo, p.strokes = img, photo, nil
			p.name = trimExt(filepath.Base(paths[0]))
			p.status = ""
		})
	}()
}

// save asks where to save the picture with its strokes, as a PNG.
func (p *paint) save(win *mygo.Window) {
	if win == nil {
		return
	}
	img, strokes, name := p.img, append([]stroke(nil), p.strokes...), p.name
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      win,
			Title:       "儲存 PNG",
			DefaultPath: name + "-drawn.png",
			Filters:     []mygo.FileFilter{{Name: "PNG", Extensions: []string{"png"}}},
		})
		if err != nil || path == "" {
			return
		}
		var buf bytes.Buffer
		err = png.Encode(&buf, render(img, strokes))
		if err == nil {
			err = os.WriteFile(path, buf.Bytes(), 0o644)
		}
		win.Update(func() {
			if err != nil {
				p.status = "儲存失敗：" + err.Error()
			} else {
				p.status = "已儲存 " + filepath.Base(path)
			}
		})
	}()
}

// render draws the strokes on a copy of img.
func render(img image.Image, strokes []stroke) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	for _, s := range strokes {
		col := image.NewUniform(color.NRGBA{s.color.R, s.color.G, s.color.B, s.color.A})
		r := s.width / 2
		for i := range s.points {
			a, z := s.points[max(i-1, 0)], s.points[i]
			capsule(out, a, z, r, col)
		}
		if n := len(s.points); s.closed && n > 2 {
			capsule(out, s.points[n-1], s.points[0], r, col)
		}
	}
	return out
}

// capsule draws a segment from a to z, r pixels on each side, with round
// ends, as StrokePath draws its round caps and joins.
func capsule(dst *image.RGBA, a, z point, r float32, src image.Image) {
	minX := int(math.Floor(float64(min(a.X, z.X) - r - 1)))
	minY := int(math.Floor(float64(min(a.Y, z.Y) - r - 1)))
	maxX := int(math.Ceil(float64(max(a.X, z.X) + r + 1)))
	maxY := int(math.Ceil(float64(max(a.Y, z.Y) + r + 1)))
	box := image.Rect(minX, minY, maxX, maxY).Intersect(dst.Bounds())
	if box.Empty() {
		return
	}
	// The outline: half a circle around z, then half a circle around a,
	// one convex shape winding one way.
	angle := math.Atan2(float64(z.Y-a.Y), float64(z.X-a.X))
	const steps = 16
	ras := vector.NewRasterizer(box.Dx(), box.Dy())
	first := true
	arc := func(c point, from float64) {
		for k := 0; k <= steps; k++ {
			th := from + math.Pi*float64(k)/steps
			x := c.X + r*float32(math.Cos(th)) - float32(box.Min.X)
			y := c.Y + r*float32(math.Sin(th)) - float32(box.Min.Y)
			if first {
				ras.MoveTo(x, y)
				first = false
			} else {
				ras.LineTo(x, y)
			}
		}
	}
	arc(z, angle-math.Pi/2)
	arc(a, angle+math.Pi/2)
	ras.ClosePath()
	ras.Draw(dst, box, src, image.Point{})
}

func trimExt(name string) string { return name[:len(name)-len(filepath.Ext(name))] }
