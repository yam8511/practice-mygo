package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The banner counts down to relaunching into an installed update, which
// its button hurries, and after the relaunch tells what changed until it is
// dismissed.
func TestUpdateBanner(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1000, 700)
	if tt.HasText("知道了") || tt.HasText("立即重新啟動") {
		t.Fatalf("a banner without an update: %q", tt.Texts())
	}

	a.restartVersion, a.restartIn = "0.1.3", 7
	tt.Frame()
	if !tt.HasText("已下載 v0.1.3，7 秒後重新啟動以完成更新") {
		t.Fatalf("texts %q while counting down", tt.Texts())
	}
	if err := tt.Click("立即重新啟動"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.restartNow:
	default:
		t.Error("the button did not ask to relaunch")
	}

	a.restartIn = 0
	a.updated = &updated{Version: "0.1.3", Notes: "- 自動更新後重新啟動"}
	tt.Frame()
	if !tt.HasText("已更新到 v0.1.3") || !tt.HasText("- 自動更新後重新啟動") {
		t.Fatalf("texts %q after an update", tt.Texts())
	}
	if err := tt.Click("知道了"); err != nil {
		t.Fatal(err)
	}
	if a.updated != nil || tt.HasText("已更新到 v0.1.3") {
		t.Errorf("the banner stays after it was dismissed: %q", tt.Texts())
	}
}
