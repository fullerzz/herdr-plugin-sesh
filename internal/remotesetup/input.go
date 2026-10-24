//nolint:ireturn // Huh's Field contract returns Model and Field interfaces.
package remotesetup

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

type themedInput struct {
	*huh.Input

	inactive *huh.Text
	value    *string
	focused  bool
}

func newInput(title, description string, value *string, validate func(string) error) *themedInput {
	input := huh.NewInput().Title(title).Description(description).Value(value).Validate(validate)
	return &themedInput{
		Input: input,
		inactive: huh.NewText().Title(title).Description(description).Lines(1).CharLimit(0).
			Validate(func(string) error { return input.Error() }),
		value: value,
	}
}

func (i *themedInput) Focus() tea.Cmd {
	i.focused = true
	return i.Input.Focus()
}

func (i *themedInput) Blur() tea.Cmd {
	i.focused = false
	return i.Input.Blur()
}

func (i *themedInput) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	_, cmd := i.Input.Update(msg)
	if _, ok := msg.(tea.BackgroundColorMsg); ok {
		_, _ = i.inactive.Update(msg)
	}
	return i, cmd
}

func (i *themedInput) View() string {
	if i.focused {
		return i.Input.View()
	}
	// Huh Input ignores blurred text styles; Text applies them correctly.
	value := *i.value
	i.inactive.Value(&value)
	i.inactive.Blur()
	return i.inactive.View()
}

func (i *themedInput) WithTheme(theme huh.Theme) huh.Field {
	i.Input.WithTheme(theme)
	i.inactive.WithTheme(theme)
	return i
}

func (i *themedInput) WithKeyMap(keys *huh.KeyMap) huh.Field {
	i.Input.WithKeyMap(keys)
	return i
}

func (i *themedInput) WithPosition(position huh.FieldPosition) huh.Field {
	i.Input.WithPosition(position)
	return i
}

func (i *themedInput) WithWidth(width int) huh.Field {
	i.Input.WithWidth(width)
	i.inactive.WithWidth(width)
	return i
}

func (i *themedInput) WithHeight(height int) huh.Field {
	i.Input.WithHeight(height)
	i.inactive.WithHeight(height)
	return i
}
