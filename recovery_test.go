package main

import (
	"testing"
	"time"

	"github.com/Lumiaqian/phosphene-go/internal/catalog"
)

func TestWallpaperRecovery(t *testing.T) {
	restarts, real := 0, restartAgent
	restartAgent = func() error { restarts++; return nil }
	t.Cleanup(func() { restartAgent = real })

	t0 := time.Unix(1_000_000, 0)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	report := func(s int, active ...string) catalog.Runtime {
		return catalog.Runtime{UpdatedAtUnix: float64(at(s).Unix()), Displays: []catalog.DisplayProgress{{DisplayKey: "A"}, {DisplayKey: "B"}}, ActiveDisplayKeys: active}
	}
	a := &application{}

	// Playing: nothing offered.
	a.observeWallpaper(report(0, "A", "B"), 0, at(0))
	if a.wallpaperStalled(at(0)) {
		t.Fatal("playing wallpaper offered a repair")
	}
	// Waiting: offered only once it lasts, never repaired by itself.
	a.observeWallpaper(report(1), 0, at(1))
	a.observeWallpaper(report(15), 0, at(15))
	if a.wallpaperStalled(at(15)) {
		t.Fatal("offered a repair before stallAfter")
	}
	a.observeWallpaper(report(25), 0, at(25))
	if !a.wallpaperStalled(at(25)) || restarts != 0 {
		t.Fatalf("waiting 24s: stalled=%v restarts=%d", a.wallpaperStalled(at(25)), restarts)
	}
	// Playing again clears it.
	a.observeWallpaper(report(26, "A"), 0, at(26))
	if a.wallpaperStalled(at(26)) {
		t.Fatal("still offered once a display shows")
	}
	// A silent extension counts too.
	a.observeWallpaper(report(26, "A", "B"), 0, at(60))
	a.observeWallpaper(report(26, "A", "B"), 0, at(90))
	if !a.wallpaperStalled(at(90)) {
		t.Fatal("silent extension not offered a repair")
	}
	// The user repairs: one restart, not offered again while it settles.
	a.repairWallpaper("", at(90))
	a.repairWallpaper("", at(95))
	a.observeWallpaper(report(26), 0, at(96))
	a.observeWallpaper(report(26), 0, at(118))
	if restarts != 1 || a.wallpaperStalled(at(118)) {
		t.Fatalf("after a repair: restarts=%d stalled=%v", restarts, a.wallpaperStalled(at(118)))
	}
	// The extension calls for help: repaired at once, once per call.
	a.observeWallpaper(report(200, "A", "B"), 1, at(200))
	a.observeWallpaper(report(202, "A", "B"), 1, at(202))
	if restarts != 2 {
		t.Fatalf("call for help: restarts=%d, want 2", restarts)
	}
}
