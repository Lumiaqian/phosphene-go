package main

import (
	"os/exec"
	"time"

	"github.com/Lumiaqian/phosphene-go/internal/catalog"
)

// WallpaperAgent sometimes stops serving the extension: after the extension
// restarts it never asks it for surfaces again, or it spins on connections
// that call nothing. The sandboxed extension cannot recover from either; a
// restart of WallpaperAgent, which the app can make, rebuilds every
// display's wallpaper.

const (
	// stallAfter is how long the displays wait for the system, or the
	// extension stays silent, before the app offers a repair. Waiting is
	// also what a locked screen looks like, so the app only offers it.
	stallAfter = 20 * time.Second
	// silentAfter is how old the extension's last report is when it counts
	// as silent.
	silentAfter = 30 * time.Second
	// repairSettle is how long a repair takes to show, during which the app
	// neither offers nor makes another.
	repairSettle = 30 * time.Second
)

// restartAgent restarts WallpaperAgent, which launchd relaunches; tests
// replace it.
var restartAgent = func() error { return exec.Command("/usr/bin/killall", "-TERM", "WallpaperAgent").Run() }

// observeWallpaper follows the extension's report and its calls for help,
// on the main thread: a call for help repairs at once, a stall is offered.
func (a *application) observeWallpaper(r catalog.Runtime, stuckSignals int64, now time.Time) {
	if stuckSignals > a.stuckSignals {
		a.stuckSignals = stuckSignals
		a.repairWallpaper("壁纸扩展报告墙纸服务卡住，已自动重建", now)
	}
	silent := r.UpdatedAtUnix > 0 && now.Sub(time.Unix(int64(r.UpdatedAtUnix), 0)) > silentAfter
	waiting := len(r.Displays) > 0 && len(r.ActiveDisplayKeys) == 0
	switch {
	case !silent && !waiting:
		a.stallSince = time.Time{}
	case a.stallSince.IsZero():
		a.stallSince = now
	}
}

// wallpaperStalled reports that the wallpaper has waited or been silent long
// enough for a repair to be worth offering.
func (a *application) wallpaperStalled(now time.Time) bool {
	return !a.stallSince.IsZero() && now.Sub(a.stallSince) > stallAfter && now.Sub(a.repairedAt) > repairSettle
}

// repairWallpaper restarts WallpaperAgent, at most once while a repair
// settles, and says so with message.
func (a *application) repairWallpaper(message string, now time.Time) {
	if now.Sub(a.repairedAt) < repairSettle {
		return
	}
	a.repairedAt, a.stallSince = now, time.Time{}
	if err := restartAgent(); err != nil {
		a.status = "重建墙纸服务失败：" + err.Error()
		return
	}
	a.status = message
}
