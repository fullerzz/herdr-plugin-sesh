package picker

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/fullerzz/herdr-plugin-sesh/internal/uitheme"
)

var herdrTokenRoles = map[string]*color.Color{
	"text":     &textColor,
	"subtext0": &mutedColor,
	"green":    &greenColor,
	"yellow":   &amberColor,
	"red":      &redColor,
	"accent":   &skyColor,
	"mauve":    &violetColor,
	"overlay1": &ghostColor,
}

func rebuildPickerStyles() {
	titleStyle = lipgloss.NewStyle().
		Foreground(violetColor).
		Bold(true)

	countStyle = lipgloss.NewStyle().
		Foreground(mutedColor)

	sectionStyle = lipgloss.NewStyle().
		Foreground(violetColor).
		Bold(true)

	ruleStyle = lipgloss.NewStyle().
		Foreground(mutedColor)

	rowLabelStyle = lipgloss.NewStyle().
		Foreground(textColor)

	selectedLabelStyle = rowLabelStyle.Bold(true)

	matchStyle = lipgloss.NewStyle().
		Foreground(violetColor).
		Bold(true)

	selectionRailStyle = lipgloss.NewStyle().
		Foreground(skyColor).
		Bold(true)

	smearTrailStyle = lipgloss.NewStyle().
		Foreground(violetColor)

	pathStyle = lipgloss.NewStyle().
		Foreground(mutedColor)

	emptyStyle = lipgloss.NewStyle().
		Foreground(amberColor)

	moreStyle = lipgloss.NewStyle().
		Foreground(mutedColor)

	helpStyle = lipgloss.NewStyle().
		Foreground(mutedColor)
}

func applyHerdrTheme(tokens map[string]string) {
	if len(tokens) == 0 {
		return
	}
	for token, value := range tokens {
		target, ok := herdrTokenRoles[token]
		if !ok || !uitheme.ValidHexColor(value) {
			continue
		}
		*target = lipgloss.Color(value)
	}
	rebuildPickerStyles()
}

func configureHerdrTheme(inherit bool) {
	p := uitheme.Resolve(inherit)
	skyColor = p.Accent
	violetColor = p.Violet
	greenColor = p.Green
	amberColor = p.Warning
	redColor = p.Error
	textColor = p.Text
	mutedColor = p.Muted
	ghostColor = p.Ghost
	rebuildPickerStyles()
}
