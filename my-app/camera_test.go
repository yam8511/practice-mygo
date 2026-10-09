package main

import (
	"sync"
	"testing"
	"time"
)

// shutdown stops a capture and waits until it has released the device,
// also when the page already asked it to stop.
func TestCameraShutdown(t *testing.T) {
	for _, closedFirst := range []bool{false, true} {
		cam := newCamera()
		stop, done := make(chan struct{}), make(chan struct{})
		cam.stop, cam.done, cam.once, cam.last, cam.running = stop, done, &sync.Once{}, done, true
		go func() {
			<-stop
			time.Sleep(50 * time.Millisecond) // releasing the device
			close(done)
		}()
		if closedFirst {
			cam.close()
		}
		cam.shutdown(5 * time.Second)
		select {
		case <-done:
		default:
			t.Errorf("closed first %v: shutdown returned before the capture ended", closedFirst)
		}
		if cam.running {
			t.Errorf("closed first %v: still running", closedFirst)
		}
	}
}
