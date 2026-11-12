package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// heldFrames checks that a control held at (x, y) keeps drawing, so the
// frame of its release comes at once even where a release asks for none,
// and that let go it draws nothing more. frames counts the view's builds.
func heldFrames(t *testing.T, tt *ui.Tester, frames *int, x, y float32) {
	t.Helper()
	count := func() int {
		tt.Advance(3 * time.Second) // let animations settle; a lens eases home in about 1.3s
		*frames = 0
		tt.Advance(200 * time.Millisecond)
		return *frames
	}
	tt.Press(x, y)
	tt.Move(x+20, y)
	if n := count(); n < 8 {
		t.Fatalf("held: %d frames in 200ms, want a frame every %v", n, dragFrame)
	}
	tt.Release(x+20, y)
	if n := count(); n != 0 {
		t.Fatalf("let go: %d frames in 200ms, want none", n)
	}
}

// drag presses at from, moves to to in steps and lets go, then settles.
func drag(tt *ui.Tester, from, to, y float32) {
	tt.Press(from, y)
	for i := 1; i <= 8; i++ {
		tt.Move(from+(to-from)*float32(i)/8, y)
	}
	tt.Release(to, y)
	tt.Advance(time.Second)
}

func TestGlassSwitch(t *testing.T) {
	var on bool
	changes, frames := 0, 0
	tt := ui.NewTester(func(c *ui.Context) {
		frames++
		ui.Column(c).Padding(40).Children(func() {
			if glassSwitch(c, &on, "开关", false) {
				changes++
			}
		})
	}, 200, 120)
	r, _ := tt.Find("开关")
	y, left, right := r.Y+r.H/2, r.X+12, r.X+r.W-12
	steps := []struct {
		name    string
		do      func()
		on      bool
		changes int
	}{
		{"tap", func() { tt.Click("开关"); tt.Advance(time.Second) }, true, 1},
		{"tap back", func() { tt.Click("开关"); tt.Advance(time.Second) }, false, 2},
		{"drag on", func() { drag(tt, left, right, y) }, true, 3},
		{"drag off", func() { drag(tt, right, left, y) }, false, 4},
		{"short drag", func() { drag(tt, left, r.X+r.W/2-6, y) }, false, 4},
	}
	for _, s := range steps {
		s.do()
		if on != s.on || changes != s.changes {
			t.Fatalf("%s: on=%v changes=%d, want %v %d", s.name, on, changes, s.on, s.changes)
		}
	}
	heldFrames(t, tt, &frames, left, y)
}

func TestGlassSegmented(t *testing.T) {
	sel, changes, frames := 0, 0, 0
	tt := ui.NewTester(func(c *ui.Context) {
		frames++
		ui.Column(c).Padding(40).Children(func() {
			if glassSegmented(c, &sel, "模式", false, "播完一遍", "按时长") {
				changes++
			}
		})
	}, 320, 120)
	a, _ := tt.Find("播完一遍")
	b, _ := tt.Find("按时长")
	ax, bx, y := a.X+a.W/2, b.X+b.W/2, a.Y+a.H/2
	steps := []struct {
		name         string
		do           func()
		sel, changes int
	}{
		{"click second", func() { tt.ClickAt(bx, y); tt.Advance(time.Second) }, 1, 1},
		{"click first", func() { tt.ClickAt(ax, y); tt.Advance(time.Second) }, 0, 2},
		{"drag right", func() { drag(tt, ax, bx, y) }, 1, 3},
		{"drag left", func() { drag(tt, bx, ax, y) }, 0, 4},
		{"short drag", func() { drag(tt, ax, ax+30, y) }, 0, 4},
	}
	for _, s := range steps {
		s.do()
		if sel != s.sel || changes != s.changes {
			t.Fatalf("%s: sel=%d changes=%d, want %d %d", s.name, sel, changes, s.sel, s.changes)
		}
	}
	heldFrames(t, tt, &frames, ax, y)
}
