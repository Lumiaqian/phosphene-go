package main

import (
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// Liquid Glass controls: a switch and a segmented control whose knob or
// lens swells into clear glass while pressed, dragged or sliding, follows
// the pointer, and, let go, settles on the nearest place.

// dragThreshold is how far the pointer moves before a press is a drag.
const dragThreshold = 3

// dragFrame is how often a control pressed asks for a frame: a release
// that ends a drag is no click and asks for none, so without it the knob
// or lens would wait, half way, for whatever draws the window next.
const dragFrame = 16 * time.Millisecond

// knobDrag is what a glassSwitch or glassSegmented keeps while its knob or
// lens is dragged.
type knobDrag struct {
	active bool
	pos    float32 // the knob's or lens's left edge, following the pointer
	moved  float32 // how far the pointer moved since the press
	start  bool    // the switch's value when the press began
	origin float32 // where the pointer pressed, from the first segment
}

// lensGlass is the glass of a knob or lens swollen by l, from 0 at rest,
// tinted white by rest, to 1, clear.
func lensGlass(rest, l float32) glass.Glass {
	g := glass.Glass{Tint: ui.RGBA(255, 255, 255, rest*(1-l))}
	if l > 0.3 {
		g.Style = glass.Clear
	}
	return g
}

// lensRim is the hairline that lights a lens's edge as it swells.
func lensRim(l float32) ui.Color { return ui.RGBA(255, 255, 255, 0.45*l) }

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// glassSegmented is a segmented control in the Liquid Glass style: a faint
// capsule track and a lens of glass over the chosen segment. A press swells
// the lens into clear glass, which follows the pointer across the segments
// and, let go, settles on the nearest; a click chooses the segment under
// it, and Enter or Space the next one. The track is one button, so no
// segment takes the press from it. It reports whether *selected changed.
func glassSegmented(c *ui.Context, selected *int, label string, disabled bool, labels ...string) bool {
	t := c.Theme()
	const w, h, pad = 96, 30, 3
	n := len(labels)
	hi := float32((n - 1) * w)
	tint := float32(0.35)
	if t.Dark {
		tint = 0.14
	}
	track := ui.ButtonBase(c.Key("seg-" + label)).Padding(pad).Radius(999).Background(t.Text.Alpha(0.06)).
		Label(label + "：" + labels[*selected]).Disabled(disabled)
	d := ui.Local(track, "drag", func() knobDrag { return knobDrag{} })
	px, _, _ := track.PointerPosition()
	from := *selected
	if dx, _, pressed := track.Dragged(); pressed && !disabled {
		if !d.active {
			*d = knobDrag{active: true, pos: float32(*selected * w), origin: px - pad}
		}
		d.pos = min(hi, max(0, d.pos+dx))
		d.moved += absf(dx)
	} else if d.active {
		// Let go: a drag settles on the segment under the lens, a click
		// chooses the one it pressed.
		if d.moved > dragThreshold {
			*selected = lensIndex(d, *selected, w)
		} else {
			*selected = min(n-1, max(0, int(d.origin/w)))
		}
		d.active = false
	} else if track.Clicked() {
		*selected = (*selected + 1) % n // Enter or Space
	}
	target := float32(*selected * w)
	if d.active {
		target = d.pos
		c.After(dragFrame)
	}
	x := track.AnimateTo("lens", target, ui.Smooth)
	if d.active {
		x = d.pos // under the pointer, without lag
	}
	var swell float32
	if d.active || absf(x-target) > 0.5 {
		swell = 1
	}
	l := track.AnimateTo("swell", swell, ui.Smooth)
	if !current.glass {
		l = 0 // a solid lens does not swell
	}
	track.Children(func() {
		lw, lh := w+l*12, h+l*10
		lensLeft, lensTop := pad+x+(w-lw)/2, pad+(h-lh)/2
		ui.Box(c).Size(float32(n*w), h)
		// Three layers, ordered by ZIndex: the labels as they are, the lens
		// frosting them, and over it, clipped to it, the labels magnified
		// about its middle by up to a fifth as it swells, as through a lens.
		for i, lb := range labels {
			ui.Box(c).Absolute().Top(pad).Left(pad+float32(i*w)).Size(w, h).Center().ZIndex(0).Children(func() {
				ui.Text(c, lb).FontWeight(600).TextColor(t.TextMuted)
			})
		}
		lens := ui.Box(c).Absolute().Top(lensTop).Left(lensLeft).Size(lw, lh).Radius(999).ZIndex(1)
		g := lensGlass(tint, l)
		g.Style = glass.Regular // frosted, so the labels under it don't show twice
		current.fillControl(c, lens, g).InsetShadow(0, 0, 0, 1, lensRim(l))
		mag := 1 + 0.22*l
		middle := pad + x + w/2
		ui.Box(c).Absolute().Top(lensTop).Left(lensLeft).Size(lw, lh).Radius(999).Clip().PassThrough().ZIndex(2).Children(func() {
			for i, lb := range labels {
				at := middle + (pad+float32(i*w)+w/2-middle)*mag
				bw, bh := w*mag, h*mag
				ui.Box(c).Absolute().Left(at-bw/2-lensLeft).Top(pad+h/2-bh/2-lensTop).Size(bw, bh).Center().Children(func() {
					ui.Text(c, lb).FontSize(t.FontSize * mag).FontWeight(600).TextColor(t.Text)
				})
			}
		})
	})
	return *selected != from
}

// lensIndex is the segment a segmented control's lens is over.
func lensIndex(d *knobDrag, selected, w int) int {
	if !d.active {
		return selected
	}
	return int(d.pos/float32(w) + 0.5)
}

// glassSwitch is a switch in the Liquid Glass style: a capsule, the accent
// when on, and a white knob that, while pressed, dragged or sliding, swells
// into a clear lens of glass larger than the track, bending the color under
// it, then settles back. The knob follows the pointer; let go, it snaps to
// the nearer side. It reports whether *on changed.
func glassSwitch(c *ui.Context, on *bool, label string, disabled bool) bool {
	t := c.Theme()
	const w, h, knobW, knobH, lensW, lensH, pad = 50, 28, 30, 22, 42, 34, 3
	const lo, hi = float32(pad), float32(w - knobW - pad)
	side := func(on bool) float32 {
		if on {
			return hi
		}
		return lo
	}
	sw := ui.SwitchBase(c, on).Size(w, h).Radius(999).Label(label).Disabled(disabled)
	changed := sw.Changed()
	d := ui.Local(sw, "drag", func() knobDrag { return knobDrag{} })
	target := side(*on)
	if dx, _, pressed := sw.Dragged(); pressed && !disabled {
		if !d.active {
			*d = knobDrag{active: true, pos: target, start: *on}
		}
		d.pos = min(hi, max(lo, d.pos+dx))
		d.moved += absf(dx)
		target = d.pos
	} else if d.active {
		// Let go: a drag decides by where the knob is, overriding the toggle
		// the release also makes; a tap has toggled.
		d.active = false
		if d.moved > dragThreshold {
			*on = d.pos > (lo+hi)/2
			changed = *on != d.start
			target = side(*on)
		}
	}
	x := sw.AnimateTo("knob", target, ui.Smooth)
	if d.active {
		x = d.pos // under the pointer, without lag
		c.After(dragFrame)
	}
	// The track's color follows the knob, so the lens has it to bend.
	sw.Background(t.Text.Alpha(0.12).MixOklab(t.Accent, sw.AnimateTo("on", (target-lo)/(hi-lo), ui.Smooth)))
	var swell float32
	if d.active || sw.Pressed() || absf(x-target) > 0.5 {
		swell = 1
	}
	l := sw.AnimateTo("lens", swell, ui.Smooth)
	if !current.glass {
		l = 0 // a solid knob does not swell
	}
	kw, kh := knobW+l*(lensW-knobW), knobH+l*(lensH-knobH)
	sw.Children(func() {
		knob := ui.Box(c).Absolute().Top((h-kh)/2).Left(x+(knobW-kw)/2).Size(kw, kh).Radius(999).PassThrough()
		if current.glass {
			knob.Material(lensGlass(0.85, l)).InsetShadow(0, 0, 0, 1, lensRim(l))
		} else {
			knob.Background(ui.RGB(255, 255, 255)) // a switch's knob is white in either theme
		}
		knob.Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.18*(1-l)))
	})
	return changed
}
