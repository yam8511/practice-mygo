package main

import (
	"fmt"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"
)

// app is the state the window shows. Its view builds the interface from
// it, on the main thread, whenever the window needs a frame: after input,
// after Window.Update, and while something animates.
type app struct {
	win   *mygo.Window
	page  int
	name  string
	count int
	paint *paint
	cam   *camera

	// An installed update counts restartIn seconds down to relaunching the
	// app into restartVersion, sooner when restartNow receives; updated is
	// the update that was installed before this launch.
	restartVersion string
	restartIn      int
	restartNow     chan struct{}
	updated        *updated
}

func newApp() *app {
	return &app{paint: newPaint(), cam: newCamera(), restartNow: make(chan struct{}, 1)}
}

func (a *app) view(c *ui.Context) {
	// Glass shows what is painted under it, so the window gets a colorful
	// backdrop: a gradient with a few soft blobs of color.
	ui.Box(c).Fill().Clip().Gradient(ui.Hex("#6366f1"), ui.Hex("#06b6d4"), 135).Children(func() {
		ui.Box(c).Absolute().Top(-80).Left(-60).Size(320, 320).Radius(160).PassThrough().Background(ui.Hex("#ec4899"))
		ui.Box(c).Absolute().Top(180).Right(-40).Size(260, 260).Radius(130).PassThrough().Background(ui.Hex("#f59e0b"))
		ui.Box(c).Absolute().Bottom(-100).Left(120).Size(300, 300).Radius(150).PassThrough().Background(ui.Hex("#10b981"))

		// The pages are a layer of their own after the blobs, so they draw
		// over them.
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Padding(16).Gap(12).Children(func() {
			ui.Row(c).Justify(ui.Center).Children(func() {
				ui.Row(c).Padding(4).Radius(20).Material(glass.Glass{}).Children(func() {
					ui.Segmented(c, &a.page, "1.首頁", "2.畫筆", "3.相機")
				})
			})
			switch a.page {
			case 1:
				ui.Box(c).Grow(1).Children(func() { a.paint.view(c, a.win) })
			case 2:
				ui.Box(c).Grow(1).Children(func() { a.cam.view(c, a.win) })
			default:
				ui.Box(c).Grow(1).Center().Children(func() { a.card(c) })
			}
		})
		a.updateBanner(c)
	})
}

// card is the pane of glass holding the app's controls.
func (a *app) card(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(16).Padding(28).Radius(28).AlignItems(ui.Center).Material(glass.Glass{}).Children(func() {
		ui.Text(c, "My APP").FontSize(26).Bold()
		ui.TextInput(c, &a.name).Placeholder("Your name").Label("Name").Width(240)
		greeting := "Hello!"
		if a.name != "" {
			greeting = fmt.Sprintf("Hello, %s!", a.name)
		}
		ui.Text(c, greeting).TextColor(t.TextMuted)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if glassButton(c, "−", glass.Glass{Interactive: true}, t.Text).Clicked() {
				a.count--
			}
			ui.Textf(c, "%d", a.count).FontSize(20).Width(48).TextAlign(ui.Center)
			if glassButton(c, "+", glass.Glass{Tint: t.Accent, Interactive: true}, t.AccentText).Clicked() {
				a.count++
			}
		})
		ui.Textf(c, "v%s", mygo.App.Version()).TextColor(t.TextMuted)
		check := ui.Row(c).Padding(8, 16).Radius(20).Material(glass.Glass{Interactive: true}).Children(func() {
			ui.Text(c, "Check for Updates…")
		})
		if check.Clicked() {
			updater.CheckForUpdates()
		}
	})
}

// glassButton is a round button of glass showing label.
func glassButton(c *ui.Context, label string, g glass.Glass, color ui.Color) ui.Element {
	return ui.Box(c).Size(44, 44).Radius(22).Center().Material(g).Children(func() {
		ui.Text(c, label).FontSize(20).Bold().TextColor(color)
	})
}

func main() {
	a := newApp()
	// The update window of native UI (the app shows no web page), for
	// "Check for Updates…": autoUpdate checks in the background instead,
	// installs without asking and relaunches the app.
	mygo.Use(native.New(updater.Options{DisableAutomaticChecks: true}))
	mygo.App.WhenReady(func() {
		log.Println("Ready 😊")
		a.updated = loadUpdated()
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title: "my-app",
			// Width:     480,
			// Height:    360,
			// MinWidth:  320,
			// MinHeight: 280,
			// Opens where the user left it last time.
			StateKey: "main",
			// The window shows the interface MyGo draws, not a web page.
			Content: ui.View(a.view),
			// Maximized: true,
			FullScreen: true,
		})
		go a.autoUpdate()
		log.Println("Ready Done😊")
	})
	log.Println("Run 🏃‍♂️")
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	log.Println("Bye👋")
}
