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

	sessionmodel "github.com/fullerzz/herdr-plugin-sesh/internal/model"
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
	args := []string{
		"--layout=reverse",
		"--border",
		"--ansi",
		"--prompt=" + prompt,
		"--delimiter=\t",
		"--with-nth=3,4,5",
		"--nth=3..6",
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
			fzfSourceBadge(s.Source),
			fzfField(fzfLabel(s)),
			fzfField(s.Path),
			fzfSearchField(s, separatorAware),
		)
	}
	return b.String()
}

func fzfPreviewCommand() string {
	return strings.Join([]string{
		`PATH="${PATH:+$PATH:}/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$HOME/.local/bin:$HOME/.local/share/mise/shims"`,
		`export PATH`,
		`source={2}`,
		`label={4}`,
		`item_path={5}`,
		`if [ -z "$item_path" ] || [ ! -d "$item_path" ]; then`,
		`  printf 'No item path available\n'`,
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
		`printf 'session: %s\nsource: %s\npath: %s\n\n' "$label" "$source" "$item_path"`,
		`for file in "$item_path/README.md" "$item_path/README" "$item_path/AGENTS.md" "$item_path/go.mod" "$item_path/package.json" "$item_path/pyproject.toml"; do`,
		`  if [ -f "$file" ]; then`,
		`    "$bat_bin" --color=always --style=numbers --line-range=:160 "$file"`,
		`    exit 0`,
		`  fi`,
		`done`,
		`find "$item_path" -maxdepth 2 -type f ! -path '*/.git/*' | sort | head -80 | "$bat_bin" --color=always --style=plain --language=txt --file-name "$item_path"`,
	}, "\n")
}

func fzfLabel(s sessionmodel.Session) string {
	if s.Name != "" {
		return s.Name
	}
	return s.Path
}

func fzfSourceBadge(source string) string {
	return "\x1b[1;38;5;" + sourceBadgeColor(source) + "m" + fzfField(sourceBadge(source, true)) + "\x1b[0m"
}

func fzfSearchField(s sessionmodel.Session, separatorAware bool) string {
	var terms []string
	if separatorAware {
		repl := strings.NewReplacer("-", " ", "_", " ", "/", " ", ".", " ")
		terms = append(terms, repl.Replace(s.Name+" "+s.Path))
	}
	if isHomeSession(s) {
		terms = append(terms, "home")
	}
	return fzfField(strings.Join(terms, " "))
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
