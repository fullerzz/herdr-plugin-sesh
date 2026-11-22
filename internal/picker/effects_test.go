package picker

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/fx"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

// frameCells parses a drawn frame back into cells, as the terminal holds it.
func frameCells(content string, width, height int) uv.ScreenBuffer {
	buf := uv.NewScreenBuffer(width, height)
	uv.NewStyledString(content).Draw(buf, uv.Rect(0, 0, width, height))
	return buf
}

// wordCells returns the cells of each place word is drawn in buf.
func wordCells(buf uv.ScreenBuffer, word string) [][]*uv.Cell {
	var found [][]*uv.Cell
	for y := range buf.Height() {
		var row strings.Builder
		var cells []*uv.Cell
		for x := range buf.Width() {
			c := buf.CellAt(x, y)
			if c == nil || c.Width == 0 {
				continue
			}
			row.WriteString(c.Content)
			for range len(c.Content) {
				cells = append(cells, c)
			}
		}
		if i := strings.Index(row.String(), word); i >= 0 {
			found = append(found, cells[i:i+len(word)])
		}
	}
	return found
}

// A preview command's palette colors stay its palette colors while the
// preview fades in. A blend stored as true color, such as ANSI blue as
// #000080, equals the palette color by value, so the terminal kept it after
// the fade.
func TestPreviewFadeKeepsPaletteColors(t *testing.T) {
	want := []struct {
		word   string
		fg, bg color.Color
	}{
		{word: "basic-blue", fg: ansi.BasicColor(4)},
		{word: "indexed-blue", fg: ansi.IndexedColor(4)},
		{word: "blue-background", bg: ansi.BasicColor(4)},
	}
	text := "\x1b[34m> \x1b[1mbasic-blue\x1b[0m\n\x1b[38;5;4mindexed-blue\x1b[0m\n\x1b[44mblue-background\x1b[0m\ntrue-color"
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 20, false
	start := time.Now()
	updated, _ := m.Update(previewMsg{key: m.previewKey, requestID: m.previewRequestID, text: text})
	m = updated.(teaModel)
	require.True(t, m.previewIn.Animating())
	check := func(frame uv.ScreenBuffer, when string) (seen, faint map[string]bool) {
		seen, faint = map[string]bool{}, map[string]bool{}
		for _, w := range want {
			for _, cells := range wordCells(frame, w.word) {
				seen[w.word] = true
				for _, c := range cells {
					assert.Equalf(t, w.fg, c.Style.Fg, "%s %s: foreground", w.word, when)
					assert.Equalf(t, w.bg, c.Style.Bg, "%s %s: background", w.word, when)
					if c.Style.Attrs&uv.AttrFaint != 0 {
						faint[w.word] = true
					}
				}
			}
		}
		return seen, faint
	}

	seen, faint := map[string]bool{}, map[string]bool{}
	steps := make([]time.Duration, 0, 222)
	for i := range 220 {
		steps = append(steps, time.Duration(i)*previewInDuration/220)
	}
	steps = append(steps, previewInDuration-time.Millisecond, previewInDuration-10*time.Microsecond)
	for i, d := range steps {
		m.previewIn.Advance(start.Add(d))
		require.Truef(t, m.previewIn.Animating(), "still fading at %v", d)
		frame := frameCells(m.View().Content, m.width, m.height)
		if i == 0 {
			assert.Len(t, wordCells(frame, "true-color"), 1, "true-color text is drawn, blended, from the start")
		}
		frameSeen, frameFaint := check(frame, "at "+d.String())
		if i == 0 {
			assert.Empty(t, frameSeen, "palette text is hidden as the fade starts")
		}
		for word := range frameSeen {
			seen[word] = true
		}
		for word := range frameFaint {
			faint[word] = true
		}
	}
	assert.Len(t, seen, len(want), "each line shows partway through the fade")
	assert.Len(t, faint, len(want), "each line is faint partway through the fade")

	m.previewIn.Advance(start.Add(time.Hour))
	require.False(t, m.previewIn.Animating())
	frame := frameCells(m.View().Content, m.width, m.height)
	settledSeen, settledFaint := check(frame, "settled")
	assert.Len(t, settledSeen, len(want), "each line shows once settled")
	assert.Empty(t, settledFaint, "no line is left faint")
	assert.NotZero(t, wordCells(frame, "basic-blue")[0][0].Style.Attrs&uv.AttrBold, "bold survives the fade")
}

// Halfway through, true-color blue blends to #000080 on black. An ANSI blue
// glyph drawn next to it still gets its own color, not the blend's.
func TestPreviewFadeSeparatesAdjacentRuns(t *testing.T) {
	viewport := bloom.Viewport(bloom.NewBuffer("\x1b[38;2;0;0;255mA\x1b[34mB\x1b[0m"))
	frame := frameCells(bloom.Render(4, 1, bloom.Transition(0.5, viewport, fx.Reverse{Effect: previewFade{}})), 4, 1)

	blend, palette := frame.CellAt(0, 0), frame.CellAt(1, 0)
	require.Equal(t, "A", blend.Content)
	require.Equal(t, "B", palette.Content)
	assert.IsType(t, color.RGBA{}, blend.Style.Fg)
	assert.False(t, paletteValues[rgba(blend.Style.Fg)], "the blend %#v is not a palette color's value", blend.Style.Fg)
	assert.Equal(t, ansi.BasicColor(4), palette.Style.Fg)
	assert.NotZero(t, palette.Style.Attrs&uv.AttrFaint)
}

func TestPaletteDistinctMovesOnlyBlends(t *testing.T) {
	white := lipgloss.NewStyle().Foreground(color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	toNavy := fx.Func(func(s *canvas.Surface, _ float64) {
		for _, c := range s.Cells() {
			c.Style.Fg = color.RGBA{B: 0x80, A: 0xff}
		}
	})

	mid := frameCells(bloom.Render(1, 1, bloom.Transition(0.5, bloom.Text("x"), paletteDistinct{Effect: toNavy})), 1, 1)
	assert.Equal(t, color.RGBA{B: 0x81, A: 0xff}, mid.CellAt(0, 0).Style.Fg)

	shown := frameCells(bloom.Render(1, 1, bloom.Transition(1, bloom.Text("x").Style(white), paletteDistinct{Effect: fx.Fade{}})), 1, 1)
	assert.Equal(t, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, shown.CellAt(0, 0).Style.Fg, "a fully shown node keeps its colors")
}

// The toast fades over the preview, blending the cells beneath it. Just
// before halfway out, their glyphs blend into the black default background,
// #000000, which is ANSI black's value; the terminal would keep that black
// once the toast is gone. One frame there shows the toast keeps its blends
// off palette values.
func TestToastFadeLeavesNoPaletteLookalike(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 16, false
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "\x1b[34m" + strings.Repeat("x", 200) + "\x1b[0m"
	}
	updated, _ := m.Update(previewMsg{key: m.previewKey, requestID: m.previewRequestID, text: strings.Join(lines, "\n")})
	m = updated.(teaModel)
	m.previewIn.Advance(time.Now().Add(time.Hour))
	m.toastText = "Failed to close workspace: boom"
	start := time.Now()
	m.toast.ShowAt(start)
	m.toast.Advance(start.Add(time.Hour))
	hide := start.Add(2 * time.Hour)
	m.toast.HideAt(hide)
	for d := time.Duration(0); m.toast.Float() >= 0.4995; d += 10 * time.Microsecond {
		require.Less(t, d, 5*time.Second, "the toast hides")
		m.toast.Advance(hide.Add(d))
	}
	require.Greater(t, m.toast.Float(), 0.498)

	frame := frameCells(m.View().Content, m.width, m.height)
	blends := 0
	for y := range m.height {
		for x := range m.width {
			c := frame.CellAt(x, y)
			if c == nil || c.Content != "x" || c.Style.Fg == nil || isPalette(c.Style.Fg) {
				continue
			}
			if v := rgba(c.Style.Fg); v.R == 0 && v.G == 0 && v.B <= 1 {
				blends++
			}
			assert.Falsef(t, paletteValues[rgba(c.Style.Fg)], "(%d,%d) has %#v", x, y, c.Style.Fg)
		}
	}
	assert.Positive(t, blends, "the toast blends preview text into the background")
}

func TestOffPaletteMovesOnlyLookalikes(t *testing.T) {
	navy := color.RGBA{B: 0x80, A: 0xff}
	assert.Equal(t, color.RGBA{B: 0x81, A: 0xff}, offPalette(navy))
	white := color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	assert.Equal(t, color.RGBA{R: 0xff, G: 0xff, B: 0xfe, A: 0xff}, offPalette(white))
	other := color.RGBA{R: 1, G: 2, B: 3, A: 0xff}
	assert.Equal(t, other, offPalette(other))
	assert.Nil(t, offPalette(nil))
	for _, c := range []color.Color{navy, white} {
		assert.False(t, paletteValues[rgba(offPalette(c))])
	}
	assert.Equal(t, ansi.BasicColor(4), offPalette(ansi.BasicColor(4)), "palette colors themselves stay")
}
