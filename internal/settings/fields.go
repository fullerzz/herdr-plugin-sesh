package settings

import (
	"reflect"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
)

type field struct {
	key, label, help string
	original, value  any
	choices          []string
}

func fieldsFromConfig(c config.Config) []field {
	fields := []field{
		{key: "picker.show_icons", label: "Show icons", help: "Display source icons in the workspace list.", value: c.TUI.ShowIcons},
		{key: "picker.show_path", label: "Show paths", help: "Show directory paths beside workspace names.", value: c.TUI.ShowPath},
		{key: "picker.show_preview", label: "Show preview", help: "Display the picker preview panel.", value: c.TUI.ShowPreview},
		{key: "picker.preview_mode", label: "Preview mode", help: "Command output or the current workspace pane.", value: c.TUI.PreviewMode, choices: []string{"command", "pane"}},
		{key: "picker.workspace_sort", label: "Workspace order", help: "Default ordering when the picker opens.", value: c.TUI.DefaultSort, choices: []string{"workspace", "recent", "agent"}},
		{key: "picker.prioritize_home", label: "Prioritize home", help: "Prioritize the home directory in the picker.", value: c.TUI.PrioritizeHome},
		{key: "picker.herdr_theme_inherit", label: "Inherit Herdr theme", help: "Use the host theme in the native picker.", value: c.TUI.HerdrThemeInherit},
		{key: "picker.replace_worktree_icon", label: "Worktree icons", help: "Replace source icons for Git worktrees.", value: c.TUI.ReplaceWorktreeIcon},
		{key: "picker.show_last_workspace", label: "Last workspace", help: "Show the previous workspace in the picker header.", value: c.TUI.ShowLastWorkspace},
		{key: "picker.show_last_workspace_path", label: "Last workspace path", help: "Show the previous workspace directory.", value: c.TUI.ShowLastWorkspacePath},
		{key: "picker.separator_aware", label: "Separator-aware search", help: "Treat path separators specially during matching.", value: c.SeparatorAware},
		{key: "picker.prompt", label: "Search prompt", help: "Empty uses the built-in picker prompt.", value: c.TUI.Prompt},
		{key: "picker.placeholder", label: "Search placeholder", help: "Empty uses the built-in search hint.", value: c.TUI.Placeholder},
		{key: "list.source_order", label: "Source order", help: "Sources in priority order; empty uses the built-in order.", value: c.SortOrder},
		{key: "list.blacklist", label: "Blacklist patterns", help: "Regular expressions used to exclude matching sessions.", value: c.Blacklist},
		{key: "list.cache", label: "Cache session lists", help: "Cache eligible list results for five seconds.", value: c.Cache},
		{key: "naming.path_components", label: "Path components", help: "Number of trailing path components used for names (at least 1).", value: c.DirLength},
		{key: "keys.cycle_preview_mode", label: "Cycle preview key", help: "Bubble Tea key name, e.g. ctrl+o; empty disables the binding.", value: c.Keys.CyclePreviewMode},
		{key: "workspace_defaults.startup", label: "Default startup", help: "Command for new workspaces. Saving does not execute it.", value: c.DefaultSessionConfig.StartupCommand},
		{key: "workspace_defaults.preview", label: "Default preview", help: "Preview command; {} is the selected path. Empty uses the built-in default.", value: c.DefaultSessionConfig.PreviewCommand},
	}
	for i := range fields {
		fields[i].original = fields[i].value
	}
	return fields
}
func (m Model) changes() map[string]any {
	out := map[string]any{}
	for _, f := range m.fields {
		if !reflect.DeepEqual(f.value, f.original) {
			out[f.key] = f.value
		}
	}
	return out
}
