package main

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// look is how much Liquid Glass the window shows (prefs.Glass): what the
// backdrop shows of the wallpaper, and what fills panes and controls.
type look struct {
	key, label string
	about      string
	glass      bool        // false: a plain background and solid surfaces
	blur       float32     // how much the backdrop blurs the wallpaper
	tone       float32     // how far the backdrop fades toward the theme's background
	pane       glass.Glass // the glass of large panes: the sidebar, groups, cards
}

// looks are the choices of prefs.Glass, in the order the settings show them.
// The more glass, the more of the wallpaper the glass has to bend: its
// refraction shows only over detail.
var looks = []look{
	{key: "none", label: "无", about: "纯色背景和实色面板，最清晰，也最省电。"},
	{key: "", label: "标准", about: "透出模糊的壁纸，玻璃面板略带磨砂。", glass: true, blur: 32, tone: 0.38, pane: glass.Glass{Clarity: 0.3}},
	{key: "max", label: "极致", about: "壁纸几乎清晰可见，面板接近透明，折射和高光最明显。", glass: true, blur: 16, tone: 0.26, pane: glass.Glass{Clarity: 0.7}},
}

// current is the look of the frame being built; view sets it first. The
// app has one window, built on the main thread.
var current = &looks[1]

func lookFor(key string) *look {
	for i := range looks {
		if looks[i].key == key {
			return &looks[i]
		}
	}
	return &looks[1]
}

// Solid surfaces of the look without glass, as grouped settings draw them:
// panes on a gray background, controls raised on panes.
func solidBackground(t *ui.Theme) ui.Color { return pick(t, ui.RGB(242, 242, 247), ui.RGB(28, 28, 30)) }
func solidPane(t *ui.Theme) ui.Color       { return pick(t, ui.RGB(255, 255, 255), ui.RGB(44, 44, 46)) }
func solidRaised(t *ui.Theme) ui.Color     { return pick(t, ui.RGB(255, 255, 255), ui.RGB(72, 72, 76)) }

func pick(t *ui.Theme, light, dark ui.Color) ui.Color {
	if t.Dark {
		return dark
	}
	return light
}

// fillPane fills a large pane: the look's glass, or a solid card.
func (l *look) fillPane(c *ui.Context, e ui.Element) ui.Element {
	if l.glass {
		return e.Material(l.pane)
	}
	t := c.Theme()
	return e.Background(solidPane(t)).Border(1, t.Text.Alpha(0.06))
}

// fillControl fills a control with g, or without glass, with its tint or a
// raised surface.
func (l *look) fillControl(c *ui.Context, e ui.Element, g glass.Glass) ui.Element {
	switch {
	case l.glass:
		return e.Material(g)
	case g.Tint.A > 0 && !isWhite(g.Tint):
		return e.Background(g.Tint)
	}
	t := c.Theme()
	return e.Background(solidRaised(t)).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
}

// onBackdrop styles text set straight on the backdrop, which glass leaves
// light in places and dark in others: a firmer color, and a soft halo of
// the theme's background that keeps it apart from the wallpaper. muted is
// for secondary text.
func (l *look) onBackdrop(c *ui.Context, e ui.Element, muted bool) ui.Element {
	t := c.Theme()
	switch {
	case !l.glass && muted:
		return e.TextColor(t.TextMuted)
	case !l.glass:
		return e
	case muted:
		e.TextColor(t.Text.Alpha(0.72))
	}
	return e.TextShadow(0, 0, 6, t.Background.Alpha(0.75))
}

func isWhite(c ui.Color) bool { return c.R == 255 && c.G == 255 && c.B == 255 }
