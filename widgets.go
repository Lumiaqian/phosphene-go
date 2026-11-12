package main

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// Building blocks the pages share.

// overlay makes e cover its parent, over what comes before it.
func overlay(e ui.Element) ui.Element { return e.Absolute().Top(0).Left(0).Right(0).Bottom(0) }

func sectionTitle(c *ui.Context, s string) {
	current.onBackdrop(c, ui.Text(c, s).FontSize(13).Bold(), true)
}

// group is a rounded panel of rows on glass, as System Settings draws them.
func group(c *ui.Context, fn func()) {
	current.fillPane(c, ui.Column(c).FillWidth().Radius(14)).Children(fn)
}

func groupRow(c *ui.Context, title, detail string, control func()) {
	t := c.Theme()
	ui.Row(c).Gap(16).AlignItems(ui.Center).Padding(12, 14).FillWidth().Children(func() {
		ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, title)
			if detail != "" {
				ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted)
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(control)
	})
}

// glassButton is a capsule of interactive glass; prominent tints it with
// the accent, for the page's main action.
func glassButton(c *ui.Context, svg *ui.SVG, label string, prominent bool) ui.Element {
	t := c.Theme()
	g, fg := glass.Glass{Interactive: true}, t.Text
	if prominent {
		g.Tint, fg = t.Accent, t.AccentText
	}
	b := current.fillControl(c, ui.ButtonBase(c.Key("glass-"+label)).Padding(8, 16).Gap(6).Radius(999), g).TextColor(fg).Label(label)
	b.Children(func() {
		if svg != nil {
			ui.Icon(c, svg)
		}
		ui.Text(c, label).FontWeight(600).SingleLine()
	})
	return b
}

// glassPill is a small label on glass, laid over a thumbnail.
func glassPill(c *ui.Context, text string, g glass.Glass, fg ui.Color) ui.Element {
	return current.fillControl(c, ui.Box(c).PassThrough().Padding(3, 9).Radius(999), g).Children(func() {
		ui.Text(c, text).FontSize(11).FontWeight(600).TextColor(fg)
	})
}

func iconButton(c *ui.Context, svg *ui.SVG, label string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(label)).Size(28, 28).Radius(6).Center().TextColor(t.TextMuted).Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(t.SurfaceHover).TextColor(t.Text)
	}
	b.Children(func() { ui.Icon(c, svg).FontSize(15) })
	return b
}

func thumbBox(c *ui.Context, bm *ui.Bitmap, w, h, r float32) {
	t := c.Theme()
	ui.Box(c).Size(w, h).Shrink(0).Radius(r).Clip().Background(t.SurfaceHover).Center().TextColor(t.TextMuted).Children(func() {
		if bm != nil {
			ui.Image(c, bm).Size(w, h).Fit(ui.Cover)
		} else {
			ui.Icon(c, iconFilm)
		}
	})
}

func emptyState(c *ui.Context, svg *ui.SVG, title, detail string) {
	t := c.Theme()
	ui.Column(c).Grow(1).FillWidth().Center().Gap(10).Padding(40).Children(func() {
		ui.Box(c).Size(64, 64).Radius(18).Center().Background(t.Accent.Alpha(0.12)).TextColor(t.Accent).Children(func() {
			ui.Icon(c, svg).FontSize(28)
		})
		ui.Text(c, title).FontSize(16).Bold()
		ui.Text(c, detail).FontSize(12).TextColor(t.TextMuted)
	})
}
