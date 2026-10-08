package main

// The magnetic lasso (Intelligent Scissors, or Livewire), after gravity's
// magic pen (web/src/hooks/useMagicPen.ts and workers/MagicPenWorker.ts):
// each anchor clicked runs Dijkstra over a cost map of the picture's edges,
// and the line from the last anchor to the pointer follows the cheapest
// path, along the edges.
//
// Points are in the picture's pixels outside this file. Inside it, in
// "compute pixels": the picture's, or half of them for very large
// pictures, which edgeMap.k converts.

import (
	"image"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
)

const (
	// lassoMaxPixels is the size above which the edges are computed at half
	// size: the cost map takes 4 bytes a pixel.
	lassoMaxPixels = 16_000_000
	// lassoSnapMinGrad is the gradient (0-255) below which there is no edge
	// near the pointer, which then stays where it is.
	lassoSnapMinGrad = 16
	// lassoSimplify is how far, in pixels of the picture, the closed outline
	// may move when simplified.
	lassoSimplify = 1
	// lassoTreeRadius is how far from an anchor, in compute pixels, its
	// paths reach: 1000×1000 takes a few hundred milliseconds.
	lassoTreeRadius = 500
	// lassoSnapDIP is how far from the pointer, in DIPs, an anchor snaps to
	// the strongest edge, and lassoCloseDIP how near the first anchor a
	// click closes the outline.
	lassoSnapDIP  = 4
	lassoCloseDIP = 10
)

// edgeMap is the cost of walking along each pixel: low on edges.
type edgeMap struct {
	w, h int
	// k is compute pixels per pixel of the picture.
	k    float32
	cost []float32
	// grad is the strength of the edge at each pixel, 0-255, which anchors
	// snap to.
	grad []uint8
}

// newEdgeMap blurs each channel, takes its Sobel gradient, keeps the
// largest of the three, so that edges between colors of the same
// lightness count, and turns it into a cost.
func newEdgeMap(img image.Image) *edgeMap {
	b := img.Bounds()
	w0, h0 := b.Dx(), b.Dy()
	k := float32(1)
	if w0*h0 > lassoMaxPixels {
		k = 0.5
	}
	w, h := max(1, int(math.Round(float64(float32(w0)*k)))), max(1, int(math.Round(float64(float32(h0)*k))))
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	if k == 1 {
		draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	} else {
		xdraw.ApproxBiLinear.Scale(rgba, rgba.Bounds(), img, b, draw.Src, nil)
	}

	n := w * h
	grad := make([]float32, n)
	tmp := make([]float32, n)
	blur := make([]float32, n)
	kernel := [5]float32{1, 4, 6, 4, 1}
	clamp := func(v, hi int) int { return min(max(v, 0), hi) }
	for ch := range 3 {
		for y := range h {
			row := y * w
			for x := range w {
				var s float32
				for t := -2; t <= 2; t++ {
					s += kernel[t+2] * float32(rgba.Pix[(row+clamp(x+t, w-1))*4+ch])
				}
				tmp[row+x] = s / 16
			}
		}
		for y := range h {
			for x := range w {
				var s float32
				for t := -2; t <= 2; t++ {
					s += kernel[t+2] * tmp[clamp(y+t, h-1)*w+x]
				}
				blur[y*w+x] = s / 16
			}
		}
		for y := 1; y < h-1; y++ {
			for x := 1; x < w-1; x++ {
				i := y*w + x
				gx := -blur[i-w-1] - 2*blur[i-1] - blur[i+w-1] + blur[i-w+1] + 2*blur[i+1] + blur[i+w+1]
				gy := -blur[i-w-1] - 2*blur[i-w] - blur[i-w+1] + blur[i+w-1] + 2*blur[i+w] + blur[i+w+1]
				if m := float32(math.Sqrt(float64(gx*gx + gy*gy))); m > grad[i] {
					grad[i] = m
				}
			}
		}
	}

	// Normalize by the 99th percentile rather than the largest, which a
	// few very strong edges would make every other edge look weak beside.
	var maxG float32
	for _, g := range grad {
		maxG = max(maxG, g)
	}
	const bins = 1024
	var hist [bins]uint32
	if maxG > 0 {
		for _, g := range grad {
			hist[min(bins-1, int(g/maxG*bins))]++
		}
	}
	p99 := maxG
	acc := 0
	for i, c := range hist {
		acc += int(c)
		if float64(acc) >= float64(n)*0.99 {
			p99 = float32(i+1) / bins * maxG
			break
		}
	}
	if p99 <= 0 {
		p99 = 1
	}

	// A logarithm rather than min(1, g/p99): cut off, the whole blurred
	// band of an edge would cost the same and the path wander in it; the
	// logarithm still lifts weak edges but keeps the band's middle the
	// cheapest.
	norm := math.Log1p(float64(maxG / p99))
	m := &edgeMap{w: w, h: h, k: k, cost: make([]float32, n), grad: make([]uint8, n)}
	for i, g := range grad {
		v := 0.0
		if norm > 0 {
			v = math.Log1p(float64(g/p99)) / norm
		}
		// A floor of 0.02 makes paths across flat areas short rather than
		// wandering after faint noise.
		m.cost[i] = float32(1 - v + 0.02)
		m.grad[i] = uint8(math.Round(v * 255))
	}
	return m
}

// pathTree holds, for each pixel of a window around an anchor, the pixel
// before it on the cheapest path from the anchor: an index in the window,
// -1 at the anchor.
type pathTree struct {
	x0, y0, w, h int
	prev         []int32
}

// tree runs Dijkstra from (sx, sy) over the 8 neighbors of each pixel in a
// window around it.
func (m *edgeMap) tree(sx, sy int) *pathTree {
	sx, sy = min(max(sx, 0), m.w-1), min(max(sy, 0), m.h-1)
	x0, y0 := max(0, sx-lassoTreeRadius), max(0, sy-lassoTreeRadius)
	x1, y1 := min(m.w-1, sx+lassoTreeRadius), min(m.h-1, sy+lassoTreeRadius)
	w, h := x1-x0+1, y1-y0+1
	n := w * h

	// float64 distances: rounded to float32, a distance stored would differ
	// from the same one computed again, and flat areas relax forever.
	dist := make([]float64, n)
	for i := range dist {
		dist[i] = math.Inf(1)
	}
	done := make([]bool, n)
	prev := make([]int32, n)
	for i := range prev {
		prev[i] = -1
	}
	var hp heap
	s := (sy-y0)*w + (sx - x0)
	dist[s] = 0
	hp.push(int32(s), 0)
	for len(hp.idx) > 0 {
		i, d0 := hp.pop()
		if done[i] || d0 > dist[i] {
			continue // handled, or a stale entry
		}
		done[i] = true
		lx, ly := int(i)%w, int(i)/w
		for dy := -1; dy <= 1; dy++ {
			ny := ly + dy
			if ny < 0 || ny >= h {
				continue
			}
			for dx := -1; dx <= 1; dx++ {
				nx := lx + dx
				if (dx == 0 && dy == 0) || nx < 0 || nx >= w {
					continue
				}
				j := ny*w + nx
				if done[j] {
					continue
				}
				step := float64(m.cost[(ny+y0)*m.w+nx+x0])
				if dx != 0 && dy != 0 {
					step *= math.Sqrt2
				}
				if nd := d0 + step; nd < dist[j] {
					dist[j], prev[j] = nd, i
					hp.push(int32(j), nd)
				}
			}
		}
	}
	return &pathTree{x0, y0, w, h, prev}
}

// heap is a binary min-heap of pixels by distance, with stale entries left
// in rather than updated.
type heap struct {
	idx  []int32
	dist []float64
}

func (q *heap) push(i int32, d float64) {
	q.idx, q.dist = append(q.idx, 0), append(q.dist, 0)
	k := len(q.idx) - 1
	for k > 0 {
		p := (k - 1) / 2
		if q.dist[p] <= d {
			break
		}
		q.idx[k], q.dist[k] = q.idx[p], q.dist[p]
		k = p
	}
	q.idx[k], q.dist[k] = i, d
}

func (q *heap) pop() (int32, float64) {
	top, topD := q.idx[0], q.dist[0]
	n := len(q.idx) - 1
	li, ld := q.idx[n], q.dist[n]
	q.idx, q.dist = q.idx[:n], q.dist[:n]
	if n == 0 {
		return top, topD
	}
	k := 0
	for {
		c := 2*k + 1
		if c >= n {
			break
		}
		if c+1 < n && q.dist[c+1] < q.dist[c] {
			c++
		}
		if q.dist[c] >= ld {
			break
		}
		q.idx[k], q.dist[k] = q.idx[c], q.dist[c]
		k = c
	}
	q.idx[k], q.dist[k] = li, ld
	return top, topD
}

// lassoStatus is how far the edges of the picture are.
type lassoStatus int

const (
	lassoIdle lassoStatus = iota
	lassoComputing
	lassoReady
	lassoFailed
)

// previewKind is what the line from the last anchor to the pointer is.
type previewKind int

const (
	previewPath    previewKind = iota // along the edges
	previewFar                        // straight: beyond the anchor's paths
	previewPending                    // straight: its paths are being computed
)

// lasso is the outline being drawn, and the edges of the picture it snaps
// to. Its points are in compute pixels.
type lasso struct {
	// run does work off the main thread and then what it returns on it.
	run func(work func() func())

	img    image.Image
	edges  *edgeMap
	status lassoStatus
	// gen counts the pictures, and seq the trees asked for, so that late
	// results of an earlier one are dropped.
	gen, seq int

	anchors []point
	// segments[i] goes from anchors[i] to anchors[i+1], both included.
	segments [][]point
	tree     *pathTree
	preview  []point
	kind     previewKind
}

// prepare computes the edges of img, unless they are computed or being
// computed. A failure lets the next call try again.
func (l *lasso) prepare(img image.Image) {
	if l.img == img && l.status != lassoFailed {
		return
	}
	l.img, l.edges = img, nil
	l.gen++
	gen := l.gen
	l.reset()
	l.status = lassoComputing
	l.run(func() func() {
		m := func() (m *edgeMap) {
			defer func() {
				if recover() != nil {
					m = nil
				}
			}()
			return newEdgeMap(img)
		}()
		return func() {
			if gen != l.gen {
				return
			}
			if m == nil {
				l.status = lassoFailed
				return
			}
			l.edges, l.status = m, lassoReady
		}
	})
}

func (l *lasso) reset() {
	l.anchors, l.segments, l.tree, l.preview = nil, nil, nil, nil
	l.seq++
	l.kind = previewPending
}

// requestTree computes the paths from anchor a.
func (l *lasso) requestTree(a point) {
	l.tree = nil
	l.seq++
	seq, gen, edges := l.seq, l.gen, l.edges
	if edges == nil {
		return
	}
	l.run(func() func() {
		t := edges.tree(int(a.X), int(a.Y))
		return func() {
			if seq == l.seq && gen == l.gen {
				l.tree = t
			}
		}
	})
}

// snap returns the pixel of the strongest edge within radius pixels of the
// picture around p, or p when no edge is near.
func (l *lasso) snap(p point, radius float32) point {
	m := l.edges
	cx, cy := int(math.Round(float64(p.X*m.k))), int(math.Round(float64(p.Y*m.k)))
	r := min(30, max(1, int(math.Round(float64(radius*m.k)))))
	best, bx, by, bestD := -1, cx, cy, math.MaxInt
	for y := max(0, cy-r); y <= min(m.h-1, cy+r); y++ {
		for x := max(0, cx-r); x <= min(m.w-1, cx+r); x++ {
			d := (x-cx)*(x-cx) + (y-cy)*(y-cy)
			if d > r*r {
				continue
			}
			// Of equal edges the nearest, so that the pointer does not jump
			// about on a thick one.
			if v := int(m.grad[y*m.w+x]); v > best || (v == best && d < bestD) {
				best, bx, by, bestD = v, x, y, d
			}
		}
	}
	if best < lassoSnapMinGrad {
		return point{float32(min(max(cx, 0), m.w-1)), float32(min(max(cy, 0), m.h-1))}
	}
	return point{float32(bx), float32(by)}
}

// pathTo walks the tree back from p to the last anchor, or returns nil
// when p is beyond it.
func (l *lasso) pathTo(p point) []point {
	t := l.tree
	if t == nil {
		return nil
	}
	lx, ly := int(p.X)-t.x0, int(p.Y)-t.y0
	if lx < 0 || ly < 0 || lx >= t.w || ly >= t.h {
		return nil
	}
	var rev []int32
	for i, guard := int32(ly*t.w+lx), t.w*t.h; i != -1 && guard > 0; i, guard = t.prev[i], guard-1 {
		rev = append(rev, i)
	}
	out := make([]point, len(rev))
	for j, i := range rev {
		out[len(rev)-1-j] = point{float32(int(i)%t.w + t.x0), float32(int(i)/t.w + t.y0)}
	}
	return out
}

// segmentTo is the line from the last anchor to p: along the edges when
// it can be, else straight.
func (l *lasso) segmentTo(p point) ([]point, previewKind) {
	if path := l.pathTo(p); path != nil {
		return path, previewPath
	}
	kind := previewPending
	if l.tree != nil {
		kind = previewFar
	}
	return []point{l.anchors[len(l.anchors)-1], p}, kind
}

// addAnchor adds an anchor at p, in the picture's pixels, snapped to an
// edge within radius of it.
func (l *lasso) addAnchor(p point, radius float32) {
	s := l.snap(p, radius)
	if len(l.anchors) > 0 {
		seg, _ := l.segmentTo(s)
		l.segments = append(l.segments, seg)
	}
	l.anchors = append(l.anchors, s)
	l.preview = nil
	l.requestTree(s)
}

// move lays the line from the last anchor to the pointer at p.
func (l *lasso) move(p point, radius float32) {
	if len(l.anchors) == 0 || l.edges == nil {
		return
	}
	l.preview, l.kind = l.segmentTo(l.snap(p, radius))
}

// removeLastAnchor takes back the last anchor and returns how many are
// left.
func (l *lasso) removeLastAnchor() int {
	if n := len(l.anchors); n > 0 {
		l.anchors = l.anchors[:n-1]
	}
	if n := len(l.segments); n > 0 {
		l.segments = l.segments[:n-1]
	}
	l.preview = nil
	if n := len(l.anchors); n > 0 {
		l.requestTree(l.anchors[n-1])
	} else {
		l.tree = nil
	}
	return len(l.anchors)
}

// close follows the edges back to the first anchor, and returns the
// outline simplified, in the picture's pixels, or nil with fewer than 3
// anchors.
func (l *lasso) close() []point {
	if len(l.anchors) < 3 {
		return nil
	}
	back, _ := l.segmentTo(l.anchors[0])
	flat := join(append(append([][]point(nil), l.segments...), back))
	k := l.edges.k
	out := simplifyClosed(flat, lassoSimplify*k)
	for i := range out {
		out[i] = point{out[i].X / k, out[i].Y / k}
	}
	return out
}

// committed is the outline through the anchors so far.
func (l *lasso) committed() []point { return join(l.segments) }

// join chains segments that share their ends, keeping each end once.
func join(segs [][]point) []point {
	var out []point
	for _, s := range segs {
		if len(out) > 0 && len(s) > 0 {
			s = s[1:]
		}
		out = append(out, s...)
	}
	return out
}

// simplifyClosed simplifies a closed outline, whose last point is its
// first, with Douglas-Peucker, without recursion, and leaves out the
// repeated last point.
func simplifyClosed(pts []point, epsilon float32) []point {
	n := len(pts)
	if n < 4 {
		return append([]point(nil), pts...)
	}
	keep := make([]bool, n)
	keep[0], keep[n-1] = true, true
	// The first and last points are the same, which Douglas-Peucker cannot
	// split on: split on the point farthest from the first too.
	far, farD := 0, float32(-1)
	for i := 1; i < n-1; i++ {
		if d := dist(pts[i], pts[0]); d > farD {
			far, farD = i, d
		}
	}
	keep[far] = true
	stack := [][2]int{{0, far}, {far, n - 1}}
	for len(stack) > 0 {
		ab := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		a, b := pts[ab[0]], pts[ab[1]]
		dx, dy := b.X-a.X, b.Y-a.Y
		length := float32(math.Hypot(float64(dx), float64(dy)))
		maxD, idx := float32(-1), -1
		for i := ab[0] + 1; i < ab[1]; i++ {
			p := pts[i]
			var d float32
			if length == 0 {
				d = dist(p, a)
			} else {
				d = abs(dy*p.X-dx*p.Y+b.X*a.Y-b.Y*a.X) / length
			}
			if d > maxD {
				maxD, idx = d, i
			}
		}
		if idx != -1 && maxD > epsilon {
			keep[idx] = true
			stack = append(stack, [2]int{ab[0], idx}, [2]int{idx, ab[1]})
		}
	}
	var out []point
	for i := 0; i < n-1; i++ {
		if keep[i] {
			out = append(out, pts[i])
		}
	}
	return out
}
