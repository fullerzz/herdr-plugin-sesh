package remotesetup

import (
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/fullerzz/herdr-plugin-sesh/internal/uitheme"
)

func formTheme(p uitheme.Palette) huh.ThemeFunc {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		t := huh.ThemeBase(isDark)
		t.Focused.Base = t.Focused.Base.BorderForeground(p.Accent)
		t.Focused.Title = t.Focused.Title.Foreground(p.Accent).Bold(true)
		t.Focused.Description = t.Focused.Description.Foreground(p.Muted)
		t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(p.Error)
		t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(p.Error)
		t.Focused.TextInput.Text = t.Focused.TextInput.Text.Foreground(p.Text)
		t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(p.Accent)
		t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(p.Accent)
		t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(p.Ghost)
		t.Focused.FocusedButton = t.Focused.FocusedButton.UnsetBackground().Foreground(p.Accent).Bold(true).Underline(true)
		t.Focused.BlurredButton = t.Focused.BlurredButton.UnsetBackground().Foreground(p.Muted)

		t.Blurred = t.Focused
		t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
		t.Blurred.Title = t.Blurred.Title.Foreground(p.Violet)
		t.Blurred.TextInput.Prompt = t.Blurred.TextInput.Prompt.Foreground(p.Ghost)

		t.Help.ShortKey = t.Help.ShortKey.Foreground(p.Accent)
		t.Help.ShortDesc = t.Help.ShortDesc.Foreground(p.Muted)
		t.Help.ShortSeparator = t.Help.ShortSeparator.Foreground(p.Ghost)
		return t
	})
}
