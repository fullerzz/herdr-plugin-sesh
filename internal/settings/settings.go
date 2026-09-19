// Package settings provides a standalone or embedded Bubble Tea config editor.
package settings

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/uitheme"
)

type Result struct {
	Path  string
	Saved bool
}
type DoneMsg struct{ Result Result }
type savedMsg struct {
	doc     *config.SettingsDocument
	err     error
	warning string
}
type reloadedMsg struct {
	model Model
	err   error
}
type mode int

const (
	form mode = iota
	editing
	array
	arrayEditing
	review
	saving
	discard
	migration
	reloadConfirm
)

type Model struct {
	doc                           *config.SettingsDocument
	migration                     *config.Migration
	opts                          config.LoadOptions
	onSaved                       func(string) error
	fields                        []field
	cursor, width, height, offset int
	input                         textinput.Model
	area                          textarea.Model
	list                          []string
	listCursor                    int
	listAdding                    bool
	mode, back                    mode
	problem                       string
	conflict                      bool
	status                        string
	result                        Result
	palette                       uitheme.Palette
}

// Open does not write; legacy conversion remains a separate confirmed operation.
func Open(opts config.LoadOptions, onSaved func(string) error) (Model, error) {
	doc, err := config.OpenSettings(opts)
	if err == nil {
		m := newModel(doc)
		m.opts = opts
		m.onSaved = onSaved
		return m, nil
	}
	if !errors.Is(err, config.ErrSettingsLegacy) {
		return Model{}, err
	}
	conversion, err := config.PrepareMigration(opts, filepath.Dir(config.SettingsDestination(config.LoadOptions{Env: opts.Env, Home: opts.Home})))
	if err != nil {
		return Model{}, err
	}
	m := newModel(nil)
	m.mode = migration
	m.migration = conversion
	m.opts = opts
	m.onSaved = onSaved
	return m, nil
}

func Run(ctx context.Context, out io.Writer, doc *config.SettingsDocument) error {
	_, err := RunModel(ctx, out, newModel(doc))
	return err
}

func RunModel(ctx context.Context, out io.Writer, m Model) (Result, error) {
	final, err := tea.NewProgram(standalone{m}, tea.WithContext(ctx), tea.WithOutput(out)).Run()
	if err != nil {
		return Result{}, err
	}
	return final.(standalone).result, nil
}

type standalone struct{ Model }

func (s standalone) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn // Bubble Tea model contract.
	if done, ok := msg.(DoneMsg); ok {
		s.result = done.Result
		return s, tea.Quit
	}
	next, cmd := s.Model.Update(msg)
	s.Model = next.(Model)
	return s, cmd
}

func newModel(doc *config.SettingsDocument) Model {
	cfg := config.Default()
	if doc != nil {
		cfg = doc.Config
	}
	m := Model{doc: doc, fields: fieldsFromConfig(cfg), width: 90, height: 28, palette: uitheme.Resolve(cfg.TUI.HerdrThemeInherit)}
	m.input = textinput.New()
	m.input.CharLimit = 0
	m.area = textarea.New()
	m.area.CharLimit = 0
	m.area.ShowLineNumbers = false
	m.area.Prompt = "│ "
	if doc != nil {
		m.result.Path = doc.SelectedPath
	}
	m = m.styleInputs()
	return m
}

func (m Model) styleInputs() Model {
	styles := m.input.Styles()
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(m.palette.Accent).Bold(true)
	styles.Focused.Text = lipgloss.NewStyle().Foreground(m.palette.Text)
	styles.Cursor.Color = m.palette.Accent
	m.input.SetStyles(styles)
	stylesArea := m.area.Styles()
	stylesArea.Focused.Text = lipgloss.NewStyle().Foreground(m.palette.Text)
	stylesArea.Focused.Prompt = lipgloss.NewStyle().Foreground(m.palette.Accent)
	stylesArea.Focused.CursorLine = lipgloss.NewStyle()
	stylesArea.Cursor.Color = m.palette.Accent
	m.area.SetStyles(stylesArea)
	return m
}

func (m Model) Init() tea.Cmd { return nil }
func (m Model) finish() (tea.Model, tea.Cmd) { //nolint:ireturn // Bubble Tea model contract.
	result := m.result
	return m, func() tea.Msg { return DoneMsg{Result: result} }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn // Bubble Tea model contract.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.offset = 0
		m.input.SetWidth(max(1, m.width-8))
		m.area.SetWidth(max(1, m.width-6))
		m.area.SetHeight(max(3, min(8, m.height-10)))
		return m, nil
	case savedMsg:
		return m.saved(msg)
	case reloadedMsg:
		if msg.err != nil {
			m.mode = m.back
			m.problem = msg.err.Error()
			return m, nil
		}
		next := msg.model
		next.width, next.height = m.width, m.height
		next.result.Saved = m.result.Saved
		return next.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	case tea.KeyPressMsg:
		if m.mode == saving {
			return m, nil
		}
		if m.problem != "" {
			return m.problemKey(msg.String())
		}
		if m.mode == discard || m.mode == reloadConfirm {
			return m.confirmKey(msg.String())
		}
		if msg.String() == "ctrl+c" {
			return m.requestExit()
		}
		if m.width < 60 || m.height < 18 {
			if msg.String() == "esc" || msg.String() == "q" {
				return m.requestExit()
			}
			return m, nil
		}
		return m.key(msg)
	default:
		var cmd tea.Cmd
		if m.mode == editing || m.mode == arrayEditing {
			if _, numeric := m.fields[m.cursor].value.(int); numeric && m.mode == editing {
				m.input, cmd = m.input.Update(msg)
			} else {
				if paste, ok := msg.(tea.PasteMsg); ok {
					msg = tea.PasteMsg{Content: encodeEditorText(paste.Content)}
				}
				m.area, cmd = m.area.Update(msg)
			}
		}
		return m, cmd
	}
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) { //nolint:ireturn // Bubble Tea model contract.
	key := msg.String()
	switch m.mode {
	case editing, arrayEditing:
		return m.editKey(msg)
	case array:
		return m.arrayKey(key)
	case review:
		switch key {
		case "esc", "n":
			m.mode = form
		case "y":
			return m.save(false)
		case "up", "k":
			m.offset = max(0, m.offset-1)
		case "down", "j":
			m.offset++
		}
	case migration:
		switch key {
		case "y":
			return m.save(true)
		case "esc", "n", "q":
			return m.finish()
		case "up":
			m.offset = max(0, m.offset-1)
		case "down":
			m.offset++
		}
	case form:
		switch key {
		case "esc", "q":
			return m.requestExit()
		case "up", "k", "shift+tab":
			m.cursor = (m.cursor + len(m.fields) - 1) % len(m.fields)
		case "down", "j", "tab":
			m.cursor = (m.cursor + 1) % len(m.fields)
		case "enter", "space", "right", "left":
			return m.editField(key)
		case "ctrl+r":
			m.fields[m.cursor].value = m.fields[m.cursor].original
			m.status = "Reverted selected setting."
		case "ctrl+s":
			if len(m.changes()) == 0 && !m.doc.Missing {
				m.status = "No changes to save."
				break
			}
			if _, err := m.doc.Preview(m.changes()); err != nil {
				m.problem = err.Error()
				break
			}
			m.mode = review
			m.offset = 0
			m.status = ""
		}
	}
	return m, nil
}

func (m Model) requestExit() (tea.Model, tea.Cmd) { //nolint:ireturn // Bubble Tea model contract.
	if len(m.changes()) > 0 || m.mode == editing || m.mode == array || m.mode == arrayEditing {
		m.back = m.mode
		m.mode = discard
		return m, nil
	}
	return m.finish()
}

func (m Model) confirmKey(key string) (tea.Model, tea.Cmd) { //nolint:ireturn // Bubble Tea model contract.
	switch key {
	case "y":
		if m.mode == discard {
			return m.finish()
		}
		opts := m.opts
		if m.doc != nil {
			opts.Path = m.doc.SelectedPath
		}
		onSaved := m.onSaved
		m.mode = saving
		return m, func() tea.Msg { next, err := Open(opts, onSaved); return reloadedMsg{model: next, err: err} }
	case "n", "esc":
		m.mode = m.back
	}
	return m, nil
}

func (m Model) problemKey(key string) (tea.Model, tea.Cmd) { //nolint:ireturn // Bubble Tea model contract.
	switch key {
	case "esc", "enter":
		m.problem = ""
		m.conflict = false
		m.offset = 0
	case "up", "k":
		m.offset = max(0, m.offset-1)
	case "down", "j":
		m.offset++
	case "r":
		if m.conflict {
			m.problem = ""
			m.conflict = false
			m.back = m.mode
			m.mode = reloadConfirm
		}
	}
	return m, nil
}

func (m Model) editField(key string) (tea.Model, tea.Cmd) { //nolint:ireturn // Bubble Tea model contract.
	f := &m.fields[m.cursor]
	m.status = ""
	m.offset = 0
	if b, ok := f.value.(bool); ok {
		f.value = !b
		return m, nil
	}
	if len(f.choices) > 0 {
		delta := 1
		if key == "left" {
			delta = -1
		}
		for i, v := range f.choices {
			if v == f.value {
				f.value = f.choices[(i+len(f.choices)+delta)%len(f.choices)]
				break
			}
		}
		return m, nil
	}
	if values, ok := f.value.([]string); ok {
		m.list = slices.Clone(values)
		m.listCursor = 0
		m.mode = array
		return m, nil
	}
	m.mode = editing
	if _, ok := f.value.(int); ok {
		m.input.SetValue(fmt.Sprint(f.value))
		return m, m.input.Focus()
	}
	m.area.SetValue(encodeEditorText(fmt.Sprint(f.value)))
	return m, m.area.Focus()
}

func (m Model) editKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) { //nolint:ireturn // Bubble Tea model contract.
	key := msg.String()
	if key == "esc" {
		if m.mode == arrayEditing {
			m.mode = array
		} else {
			m.mode = form
		}
		m.input.Blur()
		m.area.Blur()
		return m, nil
	}
	_, numeric := m.fields[m.cursor].value.(int)
	if key == "ctrl+s" || (numeric && key == "enter") {
		if m.mode == arrayEditing {
			value := decodeEditorText(m.area.Value())
			if m.listAdding {
				m.list = append(m.list, value)
				m.listCursor = len(m.list) - 1
			} else {
				m.list[m.listCursor] = value
			}
			m.mode = array
			m.area.Blur()
			return m, nil
		}
		var value any = decodeEditorText(m.area.Value())
		if numeric {
			number, err := strconv.Atoi(m.input.Value())
			if err != nil || number < 1 {
				m.problem = "Enter a whole number of at least 1."
				return m, nil
			}
			value = number
		}
		if err := m.validateField(value); err != nil {
			m.problem = err.Error()
			return m, nil
		}
		m.fields[m.cursor].value = value
		m.mode = form
		m.area.Blur()
		m.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	if numeric && m.mode == editing {
		m.input, cmd = m.input.Update(msg)
	} else {
		m.area, cmd = m.area.Update(msg)
	}
	return m, cmd
}

func (m Model) validateField(value any) error {
	changes := m.changes()
	changes[m.fields[m.cursor].key] = value
	_, err := m.doc.Preview(changes)
	return err
}

func (m Model) arrayKey(key string) (tea.Model, tea.Cmd) { //nolint:ireturn // Bubble Tea model contract.
	switch key {
	case "esc":
		m.mode = form
	case "up":
		m.listCursor = max(0, m.listCursor-1)
	case "down":
		m.listCursor = min(max(0, len(m.list)-1), m.listCursor+1)
	case "ctrl+up":
		if m.listCursor > 0 {
			m.list[m.listCursor], m.list[m.listCursor-1] = m.list[m.listCursor-1], m.list[m.listCursor]
			m.listCursor--
		}
	case "ctrl+down":
		if m.listCursor+1 < len(m.list) {
			m.list[m.listCursor], m.list[m.listCursor+1] = m.list[m.listCursor+1], m.list[m.listCursor]
			m.listCursor++
		}
	case "d", "delete":
		if len(m.list) > 0 {
			m.list = slices.Delete(m.list, m.listCursor, m.listCursor+1)
			m.listCursor = min(m.listCursor, max(0, len(m.list)-1))
		}
	case "a", "enter", "e":
		if key != "a" && len(m.list) == 0 {
			return m, nil
		}
		m.listAdding = key == "a"
		value := ""
		if !m.listAdding {
			value = m.list[m.listCursor]
		}
		m.area.SetValue(encodeEditorText(value))
		m.mode = arrayEditing
		return m, m.area.Focus()
	case "ctrl+s":
		// A non-nil empty slice compares consistently with effective empty config arrays.
		value := slices.Clone(m.list)
		if len(value) == 0 {
			value = []string{}
		}
		if err := m.validateField(value); err != nil {
			m.problem = err.Error()
			return m, nil
		}
		m.fields[m.cursor].value = value
		m.mode = form
	}
	return m, nil
}

func (m Model) save(convert bool) (tea.Model, tea.Cmd) { //nolint:ireturn // Bubble Tea model contract.
	m.mode = saving
	m.status = ""
	m.problem = ""
	changes := m.changes()
	onSaved := m.onSaved
	if convert {
		conversion := m.migration
		return m, func() tea.Msg {
			if err := conversion.Save(); err != nil {
				return savedMsg{err: err}
			}
			doc, err := config.OpenSettings(config.LoadOptions{Path: conversion.NativePath})
			result := savedMsg{doc: doc, err: err}
			if err == nil && onSaved != nil {
				if err := onSaved(doc.Path); err != nil {
					result.warning = err.Error()
				}
			}
			return result
		}
	}
	// Save a copy: the render goroutine continues to read the old baseline.
	doc := *m.doc
	return m, func() tea.Msg {
		err := doc.Save(changes)
		result := savedMsg{doc: &doc, err: err}
		if err == nil && onSaved != nil {
			if err := onSaved(doc.Path); err != nil {
				result.warning = err.Error()
			}
		}
		return result
	}
}

func (m Model) saved(msg savedMsg) (tea.Model, tea.Cmd) { //nolint:ireturn // Bubble Tea model contract.
	if msg.err != nil {
		m.mode = review
		if m.migration != nil {
			m.mode = migration
		}
		m.problem = msg.err.Error()
		m.conflict = errors.Is(msg.err, config.ErrSettingsConflict)
		m.offset = 0
		return m, nil
	}
	oldMigration := m.migration
	m.doc = msg.doc
	m.migration = nil
	m.fields = fieldsFromConfig(msg.doc.Config)
	m.mode = form
	m.result = Result{Path: msg.doc.SelectedPath, Saved: true}
	m.opts.Path = msg.doc.SelectedPath
	m.palette = uitheme.Resolve(msg.doc.Config.TUI.HerdrThemeInherit)
	m = m.styleInputs()
	m.status = "Saved. Settings apply when you return to the picker."
	if oldMigration != nil {
		m.problem = "Migration complete. Legacy files were preserved.\nFor future launches, use --config " + msg.doc.SelectedPath + " or set HERDR_SESH_CONFIG=" + strconv.Quote(msg.doc.SelectedPath) + " if your current override still selects " + oldMigration.LegacyPath + "."
	}
	if msg.warning != "" {
		m.problem = "Config saved, but cache invalidation failed:\n" + msg.warning
	}
	return m, nil
}

// Escape control characters from paths, errors and labels before styling them.
func safe(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' {
			fmt.Fprintf(&b, "\\u%04x", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
func display(v any) string {
	if b, ok := v.(bool); ok {
		if b {
			return "on"
		}
		return "off"
	}
	if s, ok := v.(string); ok {
		return strconv.Quote(s)
	}
	if list, ok := v.([]string); ok {
		values := make([]string, len(list))
		for i, s := range list {
			values[i] = strconv.Quote(s)
		}
		return "[" + strings.Join(values, ", ") + "]"
	}
	return fmt.Sprint(v)
}

// Textarea sanitizes literal tabs. Keep them editable without losing their
// distinction from spaces or a literal backslash followed by t.
func encodeEditorText(s string) string {
	return strings.NewReplacer("\\", "\\\\", "\t", "\\t").Replace(s)
}

func decodeEditorText(s string) string {
	return strings.NewReplacer("\\\\", "\\", "\\t", "\t").Replace(s)
}
func group(key string) string {
	prefix, _, _ := strings.Cut(key, ".")
	return map[string]string{"picker": "Picker", "list": "Lists", "naming": "Naming", "keys": "Keys", "workspace_defaults": "Workspace Defaults"}[prefix]
}
func (m Model) wrap(s string) []string {
	return strings.Split(ansi.Hardwrap(s, max(1, m.width-4), true), "\n")
}

func (m Model) body() (string, []string, string, int) {
	f := m.fields[m.cursor]
	title := "SETTINGS"
	footer := "↑/↓ select · enter edit · ctrl+r revert · ctrl+s review · esc back"
	selected := -1
	var lines []string
	if m.problem != "" {
		footer = "↑/↓ scroll · enter/esc return"
		if m.conflict {
			footer += " · r reload"
		}
		return "SETTINGS — MESSAGE", m.wrap(safe(m.problem)), footer, -1
	}
	if m.mode == discard || m.mode == reloadConfirm {
		text := "Discard unsaved edits and return?"
		if m.mode == reloadConfirm {
			text = "Reload the file and discard this draft?"
		}
		return "SETTINGS — CONFIRM", m.wrap(text), "y confirm · n/esc keep editing", -1
	}
	if m.width < 60 || m.height < 18 {
		return title, []string{"Resize to at least 60×18.", "Your draft is preserved."}, "esc back", -1
	}
	switch m.mode {
	case saving:
		return title, []string{"Saving…"}, "Please wait", -1
	case migration:
		text := "Convert legacy config?\n\nSource: " + safe(m.migration.LegacyPath) + "\nDestination: " + safe(m.migration.NativePath) + "\n\nImports will be flattened. Formatting and defaults may be normalized. The source and imported files remain unchanged. An existing destination will not be overwritten."
		return "SETTINGS — MIGRATION", m.wrap(text), "↑/↓ scroll · y migrate · n/esc cancel", -1
	case review:
		title = "SETTINGS — REVIEW"
		if m.doc.Missing {
			lines = append(lines, "Create "+safe(m.doc.SelectedPath), "")
		}
		for _, field := range m.fields {
			if !reflect.DeepEqual(field.value, field.original) {
				lines = append(lines, m.wrap(field.label+" ("+field.key+")\n  "+display(field.original)+" → "+display(field.value))...)
			}
		}
		lines = append(lines, "", "Only edited settings will be written.")
		for key := range m.changes() {
			if key == "list.source_order" || key == "list.blacklist" {
				lines = append(lines, m.wrap("Edited arrays are reformatted; their comments are retained but may move above the elements.")...)
				break
			}
		}
		footer = "↑/↓ scroll · y save · n/esc keep editing"
	case editing, arrayEditing:
		title = "SETTINGS — " + f.label
		lines = append(lines, m.wrap(safe(f.help))...)
		if _, ok := f.value.(int); ok && m.mode == editing {
			lines = append(lines, m.input.View())
			footer = "enter apply to draft · esc cancel"
		} else {
			lines = append(lines, strings.Split(m.area.View(), "\n")...)
			footer = "\\t tab · \\\\ backslash\nenter newline · ctrl+s apply to draft · esc cancel"
		}
	case array:
		title = "SETTINGS — " + f.label
		if len(m.list) == 0 {
			lines = append(lines, "Empty list. Press a to add an item.")
		}
		for i, value := range m.list {
			mark := "  "
			if i == m.listCursor {
				mark = "› "
				selected = len(lines)
			}
			lines = append(lines, mark+display(value))
		}
		footer = "a add · enter edit · d delete · ctrl+↑/↓ move\nctrl+s apply to draft · esc cancel"
	default:
		previous := ""
		for i, field := range m.fields {
			section := group(field.key)
			if section != previous {
				lines = append(lines, lipgloss.NewStyle().Foreground(m.palette.Violet).Bold(true).Render(section))
				previous = section
			}
			mark := " "
			if !reflect.DeepEqual(field.value, field.original) {
				mark = "*"
			}
			label := fmt.Sprintf("%s %-25s %s", mark, field.label, display(field.value))
			if i == m.cursor {
				selected = len(lines)
				label = lipgloss.NewStyle().Foreground(m.palette.Accent).Bold(true).Render("› " + label)
			} else if mark == "*" {
				label = lipgloss.NewStyle().Foreground(m.palette.Warning).Render("  " + label)
			} else {
				label = lipgloss.NewStyle().Foreground(m.palette.Text).Render("  " + label)
			}
			lines = append(lines, label)
		}
	}
	return title, lines, footer, selected
}

func (m Model) View() tea.View {
	width := max(1, m.width-4)
	title, body, footer, selected := m.body()
	muted := lipgloss.NewStyle().Foreground(m.palette.Muted)
	heading := lipgloss.NewStyle().Foreground(m.palette.Violet).Bold(true)
	path := m.result.Path
	if path == "" && m.migration != nil {
		path = m.migration.LegacyPath
	}
	header := make([]string, 0, max(4, m.height))
	header = append(header, heading.Render("HERDR SESH / "+title), muted.Render(strings.ReplaceAll(safe(path), "\n", "\\n")), muted.Render(fmt.Sprintf("%d pending changes", len(m.changes()))), muted.Render(strings.Repeat("─", width)))
	foot := m.wrap(footer)
	// Description is shown in the form only; long text remains available in edit view.
	if m.mode == form && m.problem == "" && m.width >= 60 && m.height >= 18 {
		f := m.fields[m.cursor]
		help := m.wrap(safe(f.key + " — " + f.help))
		foot = append(append(help[:min(2, len(help))], ""), foot...)
	}
	if m.status != "" && m.problem == "" {
		foot = append(m.wrap(safe(m.status)), foot...)
	}
	available := max(1, m.height-len(header)-len(foot)-1)
	start := min(m.offset, max(0, len(body)-available))
	if selected >= 0 {
		start = max(0, min(selected-available/2, len(body)-available))
	}
	end := min(len(body), start+available)
	if m.problem != "" {
		for i := range body {
			body[i] = lipgloss.NewStyle().Foreground(m.palette.Warning).Render(body[i])
		}
	}
	lines := append(header, body[start:end]...)
	for len(lines) < m.height-len(foot)-1 {
		lines = append(lines, "")
	}
	lines = append(lines, muted.Render(strings.Repeat("─", width)))
	for _, line := range foot {
		lines = append(lines, muted.Render(line))
	}
	// Even the resize/confirmation view must fit very small terminals.
	if len(lines) > m.height && m.height > 0 {
		lines = append(lines[:max(0, m.height-1)], lines[len(lines)-1])
	}
	for i := range lines {
		lines[i] = "  " + ansi.Truncate(lines[i], width, "…")
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}
