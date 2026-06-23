package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"--version"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "herdr-sesh 0.1.0-dev" {
		t.Fatalf("got %q", out.String())
	}
}

func TestConfigPathCommand(t *testing.T) {
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"config", "path"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "sesh.toml") {
		t.Fatalf("got %q", out.String())
	}
}
