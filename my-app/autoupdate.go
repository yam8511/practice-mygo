package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

const (
	// updateInterval is how often the app checks for a new version, after
	// a first check updateFirstWait after it starts.
	updateInterval  = time.Second * 10
	updateFirstWait = 10 * time.Second
	// restartDelay is how many seconds the window counts down after an
	// update is installed before the app relaunches into it.
	restartDelay = 10
)

// updated is a version an update installed, kept in updated.json in the
// user data directory until the app runs it, to tell the user.
type updated struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
}

func updatedFile() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "updated.json"), nil
}

// loadUpdated returns the update installed before this launch, when the
// app now runs its version, and forgets it.
func loadUpdated() *updated {
	file, err := updatedFile()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	os.Remove(file)
	var u updated
	if json.Unmarshal(data, &u) != nil || u.Version != mygo.App.Version() {
		return nil
	}
	return &u
}

// autoUpdate checks for a new version every updateInterval, installs it in
// the background and relaunches the app into it after a countdown. It runs
// in a goroutine of its own, until it installed one.
func (a *app) autoUpdate() {
	if !mygo.Updater.Enabled() {
		return
	}
	time.Sleep(updateFirstWait)
	for !a.update() {
		time.Sleep(updateInterval)
	}
}

// update installs the newer version when there is one and relaunches the
// app into it, reporting whether it did.
func (a *app) update() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	up, err := mygo.Updater.Check(ctx)
	if err != nil {
		log.Printf("checking for updates: %v", err)
		return false
	}
	if up == nil {
		return false
	}
	if err := up.Install(ctx, nil); err != nil {
		log.Printf("installing %s: %v", up.Version, err)
		return false
	}
	if file, err := updatedFile(); err == nil {
		data, _ := json.Marshal(updated{Version: up.Version, Notes: up.Notes})
		if err := os.WriteFile(file, data, 0o644); err != nil {
			log.Printf("keeping the update: %v", err)
		}
	}
	for n := restartDelay; n > 0; n-- {
		a.win.Update(func() { a.restartVersion, a.restartIn = up.Version, n })
		select {
		case <-a.restartNow:
			n = 0
		case <-time.After(time.Second):
		}
	}
	mygo.App.Relaunch()
	return true
}

// updateBanner tells, at the bottom of the window, that an update is about
// to relaunch the app, or that the app was just updated.
func (a *app) updateBanner(c *ui.Context) {
	if a.restartIn == 0 && a.updated == nil {
		return
	}
	t := c.Theme()
	ui.Row(c).Absolute().Left(0).Right(0).Bottom(24).Justify(ui.Center).PassThrough().Children(func() {
		ui.Row(c).Gap(14).Padding(12, 14, 12, 20).Radius(22).AlignItems(ui.Center).MaxWidth(560).Material(glass.Glass{}).Children(func() {
			ui.Column(c).Gap(4).Shrink(1).Children(func() {
				if a.restartIn > 0 {
					ui.Textf(c, "已下載 v%s，%d 秒後重新啟動以完成更新", a.restartVersion, a.restartIn).Bold()
					return
				}
				ui.Textf(c, "已更新到 v%s", a.updated.Version).Bold()
				if a.updated.Notes != "" {
					ui.Text(c, a.updated.Notes).TextColor(t.TextMuted)
				}
			})
			label := "知道了"
			if a.restartIn > 0 {
				label = "立即重新啟動"
			}
			b := ui.Row(c).Padding(8, 16).Radius(18).Material(glass.Glass{Tint: t.Accent, Interactive: true}).Children(func() {
				ui.Text(c, label).TextColor(t.AccentText)
			})
			if b.Clicked() {
				if a.restartIn > 0 {
					select {
					case a.restartNow <- struct{}{}:
					default:
					}
				} else {
					a.updated = nil
				}
			}
		})
	})
}
