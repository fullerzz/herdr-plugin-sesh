package picker

import (
	"image/color"

	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/fx"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Palette colors, such as a preview command's ANSI blue, are drawn in the
// terminal's theme, but bloom blends them as xterm's defaults: ANSI blue as
// #000080. bloom and the renderer compare colors by value, so a blend that
// lands on #000080 matches ANSI blue: the cell is never redrawn once the
// effect ends, and an ANSI blue glyph drawn next to the blend is never given
// its own color. Either way the terminal shows dark blue. The effects here
// never draw a blend with a palette color's value.

// previewFade fades preview text in from the terminal's background as t
// falls from 1 to 0, the way fx.Dim does, except for cells in palette colors:
// those are hidden over the first half and faint over the second, so they
// keep the theme's colors.
type previewFade struct{}

// Apply implements fx.Effect.
func (previewFade) Apply(s *canvas.Surface, t float64) {
	if t <= 0 {
		return
	}
	for at, c := range s.Cells() {
		if isPalette(c.Style.Fg) || isPalette(c.Style.Bg) {
			if t > 0.5 {
				s.Hide(at.X, at.Y)
			} else {
				c.Style.Attrs |= uv.AttrFaint
			}
			continue
		}
		c.Style.Fg = offPalette(s.Mix(s.Fg(c), s.Env.Bg, t))
		if c.Style.Bg != nil {
			c.Style.Bg = offPalette(s.Mix(s.Bg(c), s.Env.Bg, t))
		}
	}
}

// paletteDistinct runs Effect and then moves any blended color that matches a
// palette color's xterm value off it by one step of blue, for effects that
// blend the cells beneath them, as fx.Fade does under a toast. A node fully
// shown, at t = 1, keeps its own colors.
type paletteDistinct struct{ Effect fx.Effect }

// Apply implements fx.Effect.
func (e paletteDistinct) Apply(s *canvas.Surface, t float64) {
	e.Effect.Apply(s, t)
	if t >= 1 {
		return
	}
	for _, c := range s.Cells() {
		c.Style.Fg = offPalette(c.Style.Fg)
		c.Style.Bg = offPalette(c.Style.Bg)
	}
}

func isPalette(c color.Color) bool {
	switch c.(type) {
	case ansi.BasicColor, ansi.IndexedColor:
		return true
	}
	return false
}

// paletteValues holds the xterm value of every palette color.
var paletteValues = func() map[color.RGBA]bool {
	values := make(map[color.RGBA]bool, 256)
	for i := range 256 {
		values[rgba(ansi.IndexedColor(i))] = true
	}
	return values
}()

func offPalette(c color.Color) color.Color {
	if c == nil || isPalette(c) || !paletteValues[rgba(c)] {
		return c
	}
	v := rgba(c)
	step := uint8(1)
	if v.B == 0xff {
		step = 0xff // wraps to minus one
	}
	for paletteValues[v] {
		v.B += step
	}
	return v
}

func rgba(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)} //nolint:gosec // RGBA returns 16-bit values.
}
