package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"gocv.io/x/gocv"
)

// cameraIndex is the device the page opens, the system's default camera.
const cameraIndex = 0

// camera is the page that opens a camera with GoCV and shows its frames
// live.
type camera struct {
	// frame is the latest frame on the GPU; only the main thread touches it.
	frame   *ui.Bitmap
	running bool
	status  string

	// stop ends the capture goroutine and done is closed when it has
	// released the device.
	stop chan struct{}
	done chan struct{}
	once *sync.Once
	// last is the done of the latest capture, kept after it was asked to
	// stop, for shutdown to wait on.
	last chan struct{}
}

func newCamera() *camera { return &camera{} }

func (cam *camera) view(c *ui.Context, win *mygo.Window) {
	t := c.Theme()
	ui.Column(c).Fill().Gap(12).Children(func() {
		ui.Row(c).Padding(8, 12).Gap(10).Radius(24).AlignItems(ui.Center).Material(glass.Glass{}).Children(func() {
			if cam.running {
				if ui.Button(c, "關閉相機").Clicked() {
					cam.close()
				}
			} else if ui.PrimaryButton(c, "開啟相機").Clicked() {
				cam.open(win)
			}
			if cam.status != "" {
				ui.Text(c, cam.status).TextColor(t.TextMuted)
			}
		})

		canvas := ui.Box(c).Grow(1).Radius(16).Clip().Background(t.Surface).Label("camera")
		canvas.Draw(func(pt *ui.Painter, r ui.Rect) {
			if cam.frame == nil {
				return
			}
			iw, ih := cam.frame.Size()
			scale := min(r.W/float32(iw), r.H/float32(ih))
			w, h := float32(iw)*scale, float32(ih)*scale
			pt.Image(cam.frame, ui.Rect{X: r.X + (r.W-w)/2, Y: r.Y + (r.H-h)/2, W: w, H: h}, ui.FillBox)
		})
	})
}

// open starts capturing in the background. Each frame is converted off the
// main thread and handed to the window with Update.
func (cam *camera) open(win *mygo.Window) {
	if win == nil || cam.running {
		return
	}
	stop, done := make(chan struct{}), make(chan struct{})
	cam.stop, cam.done, cam.once, cam.last = stop, done, &sync.Once{}, done
	cam.running, cam.status = true, "開啟中…"

	go func() {
		defer close(done)
		cap, err := gocv.OpenVideoCapture(cameraIndex)
		if err != nil {
			win.Update(func() { cam.fail(done, fmt.Sprintf("無法開啟相機：%v", err)) })
			return
		}
		defer cap.Close()

		mat := gocv.NewMat()
		defer mat.Close()
		first := true
		for {
			select {
			case <-stop:
				return
			default:
			}
			if !cap.Read(&mat) {
				win.Update(func() { cam.fail(done, "讀取影像失敗") })
				return
			}
			if mat.Empty() {
				continue
			}
			img, err := mat.ToImage()
			if err != nil {
				win.Update(func() { cam.fail(done, "影像轉換失敗："+err.Error()) })
				return
			}
			bmp := ui.NewBitmap(img)
			announce := first
			first = false
			win.Update(func() {
				// A frame that raced with closing is dropped.
				if cam.done != done {
					return
				}
				cam.frame = bmp
				if announce {
					cam.status = "執行中"
				}
			})
		}
	}()
}

// fail stops showing the capture that ended by itself, unless it was
// already replaced.
func (cam *camera) fail(done chan struct{}, msg string) {
	if cam.done != done {
		return
	}
	cam.running, cam.status = false, msg
	cam.stop, cam.done = nil, nil
}

// shutdown stops the capture and waits, up to timeout, until it has
// released the device, also when it was already asked to stop.
func (cam *camera) shutdown(timeout time.Duration) {
	cam.close()
	if cam.last == nil {
		return
	}
	select {
	case <-cam.last:
	case <-time.After(timeout):
		log.Printf("the camera was not released in %v", timeout)
	}
}

// close asks the capture to stop and clears the picture.
func (cam *camera) close() {
	if !cam.running {
		return
	}
	cam.once.Do(func() { close(cam.stop) })
	cam.running, cam.status, cam.frame = false, "", nil
	cam.stop, cam.done = nil, nil
}
