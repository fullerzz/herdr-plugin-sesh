package picker

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	sessionmodel "github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/settings"
)

type settingsOpenedMsg struct {
	model settings.Model
	err   error
}
type settingsReloadedMsg struct {
	options Options
	result  ReloadResult
	err     error
}

func (m teaModel) settingsKey() string {
	if m.cyclePreviewModeKey == "f2" {
		return "ctrl+,"
	}
	return "f2"
}

func (m teaModel) updateSettings(msg tea.Msg) (teaModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case settingsOpenedMsg:
		m.settingsBusy = false
		if msg.err != nil {
			m.closeError = fmt.Sprintf("Settings: %v", msg.err)
			m, cmd := m.resumePicker()
			return m, cmd, true
		}
		child, _ := msg.model.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		editor := child.(settings.Model)
		m.settings = &editor
		return m, editor.Init(), true
	case settings.DoneMsg:
		if m.settings == nil {
			return m, nil, true
		}
		if msg.Result.Saved && m.reloadSettings != nil {
			m.settingsBusy = true
			ctx, cancel := context.WithCancel(m.previewParentContext)
			m.settingsCancel = cancel
			reload := m.reloadSettings
			result := msg.Result
			return m, func() tea.Msg {
				opts, state, err := reload(ctx, result)
				return settingsReloadedMsg{options: opts, result: state, err: err}
			}, true
		}
		m.settings = nil
		m, cmd := m.resumePicker()
		return m, cmd, true
	case settingsReloadedMsg:
		if m.settingsCancel != nil {
			m.settingsCancel()
			m.settingsCancel = nil
		}
		m.settingsBusy = false
		m.settings = nil
		if m.quitAfterSettings {
			return m, tea.Quit, true
		}
		if msg.err != nil {
			m.closeError = fmt.Sprintf("Settings saved; picker reload failed: %v. Reopen the picker to retry.", msg.err)
			m, cmd := m.resumePicker()
			return m, cmd, true
		}
		key := ""
		if current, ok := m.list.Current(); ok {
			key = sessionmodel.Key(current)
		}
		configureHerdrTheme(msg.options.HerdrThemeInherit)
		next := newTeaModel(msg.result.Sessions, msg.options).cancelActivePreview()
		next.width, next.height = m.width, m.height
		next.previewRequestID = m.previewRequestID + 1
		next.refreshGeneration = m.refreshGeneration
		next.input.SetValue(m.input.Value())
		next.list.Filter(m.list.Query)
		next.list.Selected = min(m.list.Selected, max(0, len(next.list.Filtered)-1))
		for i, item := range next.list.Filtered {
			if sessionmodel.Key(item) == key {
				next.list.Selected = i
				break
			}
		}
		next.listFocused = m.listFocused
		if next.listFocused {
			next.input.Blur()
		}
		next, cmd := next.resumePicker()
		return next, cmd, true
	}
	if m.settings != nil || m.settingsBusy {
		if size, ok := msg.(tea.WindowSizeMsg); ok {
			m.width, m.height = size.Width, size.Height
		}
		if m.settingsBusy {
			if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
				if m.settingsCancel != nil {
					m.settingsCancel()
					// Wait for the canceled callback before app state is read on exit.
					m.quitAfterSettings = true
					return m, nil, true
				}
				return m, tea.Quit, true
			}
			return m, nil, true
		}
		// Ignore old picker jobs; forwarding their ticks could restart work or move focus.
		switch msg.(type) {
		case previewMsg, previewLoadingMsg, panePreviewTickMsg, statusRefreshTickMsg, agentStatusesMsg, spinner.TickMsg, smearTickMsg:
			return m, nil, true
		}
		next, cmd := m.settings.Update(msg)
		editor := next.(settings.Model)
		m.settings = &editor
		return m, cmd, true
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || key.String() != m.settingsKey() || m.openSettings == nil {
		return m, nil, false
	}
	if m.closingWorkspaceID != "" {
		return m, nil, true
	}
	m = m.cancelActivePreview()
	m.previewRequestID++
	m.previewKey = ""
	m.refreshGeneration++
	m.agentSpinner = spinner.New(spinner.WithSpinner(agentStatusSpinner))
	m.smearActive = false
	m.focusSmearActive = false
	m.draggingPreview = false
	m.settingsBusy = true
	open := m.openSettings
	return m, func() tea.Msg { editor, err := open(); return settingsOpenedMsg{model: editor, err: err} }, true
}

func (m teaModel) resumePicker() (teaModel, tea.Cmd) {
	m.previewKey = ""
	next, preview := m.refreshPreview()
	cmds := []tea.Cmd{preview}
	if !next.listFocused {
		cmds = append(cmds, next.input.Focus())
	}
	if next.refreshAgentStatuses != nil {
		cmds = append(cmds, scheduleStatusRefreshFor(next.refreshGeneration), next.agentSpinner.Tick)
	}
	return next, tea.Batch(cmds...)
}
