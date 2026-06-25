package picker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	sessionmodel "forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
)

func RunFZF(ctx context.Context, items []sessionmodel.Session, opts Options) (sessionmodel.Session, bool, error) {
	if len(items) == 0 {
		return sessionmodel.Session{}, false, nil
	}
	command := opts.FZFCommand
	if command == "" {
		command = "fzf"
	}
	if _, err := exec.LookPath(command); err != nil {
		return sessionmodel.Session{}, false, fmt.Errorf("fzf picker requires %q in PATH: %w", command, err)
	}
	//nolint:gosec // fzf is an external picker executable by design.
	cmd := exec.CommandContext(ctx, command, fzfArgs(opts)...)
	cmd.Stdin = strings.NewReader(fzfInput(items, opts.SeparatorAware))
	var selected bytes.Buffer
	cmd.Stdout = &selected
	cmd.Stderr = opts.Output
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && (exitErr.ExitCode() == 1 || exitErr.ExitCode() == 130) {
			return sessionmodel.Session{}, false, nil
		}
		return sessionmodel.Session{}, false, err
	}
	idx, ok := fzfSelectionIndex(selected.String(), len(items))
	if !ok {
		return sessionmodel.Session{}, false, fmt.Errorf("fzf returned invalid selection %q", strings.TrimSpace(selected.String()))
	}
	return items[idx], true, nil
}

func fzfArgs(opts Options) []string {
	prompt := opts.Prompt
	if prompt == "" {
		prompt = defaultPrompt
	}
	nth := "3..5"
	if opts.SeparatorAware {
		nth = "3..6"
	}
	args := []string{
		"--layout=reverse",
		"--border",
		"--prompt=" + prompt,
		"--delimiter=\t",
		"--with-nth=3,4,5",
		"--nth=" + nth,
		"--preview=" + fzfPreviewCommand(),
		"--preview-window=right:60%,border-left,wrap",
		"--header=Enter select  Ctrl-U clear  Esc cancel",
		"--bind=ctrl-u:clear-query",
	}
	if opts.Placeholder != "" {
		args = append(args, "--ghost="+opts.Placeholder)
	}
	return args
}

func fzfInput(items []sessionmodel.Session, separatorAware bool) string {
	var b strings.Builder
	for i, s := range items {
		_, _ = fmt.Fprintf(
			&b,
			"%d\t%s\t%s\t%s\t%s\t%s\n",
			i,
			fzfField(s.Source),
			fzfField(sourceBadge(s.Source)),
			fzfField(fzfLabel(s)),
			fzfField(s.Path),
			fzfSearchField(s, separatorAware),
		)
	}
	return b.String()
}

func fzfPreviewCommand() string {
	return strings.Join([]string{
		`if [ {2} != herdr ]; then`,
		`  printf 'Preview is available for existing Herdr workspaces only\n'`,
		`  exit 0`,
		`fi`,
		`path={5}`,
		`if [ -z "$path" ] || [ ! -d "$path" ]; then`,
		`  printf 'No workspace path available\n'`,
		`  exit 0`,
		`fi`,
		`bat_bin=$(command -v bat 2>/dev/null || command -v batcat 2>/dev/null || true)`,
		`if [ -z "$bat_bin" ]; then`,
		`  for candidate in /opt/homebrew/bin/bat /usr/local/bin/bat /usr/bin/batcat "$HOME/.local/bin/bat" "$HOME/.local/share/mise/shims/bat"; do`,
		`    if [ -x "$candidate" ]; then`,
		`      bat_bin=$candidate`,
		`      break`,
		`    fi`,
		`  done`,
		`fi`,
		`if [ -z "$bat_bin" ]; then`,
		`  printf 'bat not found in PATH or common install locations\n'`,
		`  exit 0`,
		`fi`,
		`printf 'workspace: %s\npath: %s\n\n' {4} "$path"`,
		`for file in "$path/README.md" "$path/README" "$path/AGENTS.md" "$path/go.mod" "$path/package.json" "$path/pyproject.toml"; do`,
		`  if [ -f "$file" ]; then`,
		`    "$bat_bin" --color=always --style=numbers --line-range=:160 "$file"`,
		`    exit 0`,
		`  fi`,
		`done`,
		`find "$path" -maxdepth 2 -type f ! -path '*/.git/*' | sort | head -80 | "$bat_bin" --color=always --style=plain --language=txt --file-name "$path"`,
	}, "\n")
}

func fzfLabel(s sessionmodel.Session) string {
	if s.Name != "" {
		return s.Name
	}
	return s.Path
}

func fzfSearchField(s sessionmodel.Session, separatorAware bool) string {
	if !separatorAware {
		return ""
	}
	repl := strings.NewReplacer("-", " ", "_", " ", "/", " ", ".", " ")
	return fzfField(repl.Replace(s.Name + " " + s.Path))
}

func fzfField(s string) string {
	repl := strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")
	return repl.Replace(s)
}

func fzfSelectionIndex(out string, max int) (int, bool) {
	line := strings.TrimRight(out, "\x00\r\n")
	if line == "" {
		return 0, false
	}
	field, _, _ := strings.Cut(line, "\t")
	idx, err := strconv.Atoi(field)
	if err != nil || idx < 0 || idx >= max {
		return 0, false
	}
	return idx, true
}
