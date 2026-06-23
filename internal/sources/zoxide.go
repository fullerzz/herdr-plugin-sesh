package sources

import (
	"bufio"
	"context"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Zoxide struct {
	Runner func(context.Context) (string, error)
}

func (Zoxide) Name() string { return "zoxide" }
func (z Zoxide) List(ctx context.Context) (model.Sessions, error) {
	out := model.NewSessions()
	var raw string
	var err error
	if z.Runner != nil {
		raw, err = z.Runner(ctx)
	} else {
		b, e := exec.CommandContext(ctx, "zoxide", "query", "-l", "-s").Output()
		raw = string(b)
		err = e
	}
	if err != nil {
		return out, nil
	}
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		if s, ok := ParseZoxideLine(scanner.Text()); ok {
			out.Add(s)
		}
	}
	return out, nil
}
func ParseZoxideLine(line string) (model.Session, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return model.Session{}, false
	}
	fields := strings.Fields(line)
	score := 0.0
	path := line
	if len(fields) >= 2 {
		if f, err := strconv.ParseFloat(fields[0], 64); err == nil {
			score = f
			path = strings.Join(fields[1:], " ")
		}
	}
	return model.Session{Source: "zoxide", Name: filepath.Base(path), Path: path, Score: score}, true
}
func AddPath(ctx context.Context, path string) {
	_ = exec.CommandContext(ctx, "zoxide", "add", path).Run()
}
