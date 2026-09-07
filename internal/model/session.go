package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type WorktreeRelation struct {
	Linked              bool
	ParentWorkspaceID   string
	ParentWorkspaceName string
}

type Session struct {
	SSH                   *SSHMachine      `json:"ssh,omitempty"`
	Source                string           `json:"source"`
	Name                  string           `json:"name"`
	Path                  string           `json:"path,omitempty"`
	WorkspaceID           string           `json:"workspace_id,omitempty"`
	TabID                 string           `json:"tab_id,omitempty"`
	StartupCommand        string           `json:"startup_command,omitempty"`
	PreviewCommand        string           `json:"preview_command,omitempty"`
	DisableStartupCommand bool             `json:"disable_startup_command,omitempty"`
	DisableStartupSet     bool             `json:"-"`
	WindowNames           []string         `json:"window_names,omitempty"`
	WindowConfigs         []WindowConfig   `json:"-"`
	AgentStatus           string           `json:"-"`
	Worktree              WorktreeRelation `json:"-"`
	Score                 float64          `json:"score,omitempty"`
	Attached              bool             `json:"attached,omitempty"`
}

type WindowConfig struct {
	Name          string `json:"name"                     toml:"name"`
	StartupScript string `json:"startup_script,omitempty" toml:"startup_script"`
	Path          string `json:"path,omitempty"           toml:"path"`
}

type Sessions struct {
	Directory    map[string]Session `json:"directory"`
	OrderedIndex []string           `json:"ordered_index"`
}

type ListOptions struct {
	Workspaces     bool
	Config         bool
	Zoxide         bool
	Directories    bool
	Panes          bool
	Icons          bool
	JSON           bool
	HideDuplicates bool
	Blacklisted    bool
}

func NewSessions() Sessions {
	return Sessions{Directory: map[string]Session{}, OrderedIndex: []string{}}
}

func Key(s Session) string {
	if s.SSH != nil {
		return "ssh-machine:" + s.SSH.ID
	}
	base := fmt.Sprintf("%s\x00%s\x00%s\x00%s", s.Source, s.Name, s.Path, s.WorkspaceID)
	sum := sha256.Sum256([]byte(base))
	return s.Source + ":" + hex.EncodeToString(sum[:8])
}

const SSHDisplayOnly = "SSH machines are display-only; switch using Herdr's machine sidebar"

type SSHMachine struct {
	ID            string `json:"id"`
	Target        string `json:"target"`
	RemoteSession string `json:"remote_session"`
	Enabled       bool   `json:"enabled"`
}

func (s Session) IsSSH() bool { return s.Source == "ssh" || s.SSH != nil }

func (s SSHMachine) Status() string {
	if s.Enabled {
		return "enabled"
	}
	return "disabled"
}

func (s SSHMachine) Summary() string {
	return s.Target + " · " + s.RemoteSession + " · " + s.Status() + " (display-only)"
}

func (ss *Sessions) Add(s Session) string {
	if ss.Directory == nil {
		ss.Directory = map[string]Session{}
	}
	key := Key(s)
	if _, ok := ss.Directory[key]; !ok {
		ss.OrderedIndex = append(ss.OrderedIndex, key)
	}
	ss.Directory[key] = s
	return key
}

func (ss *Sessions) Ordered() []Session {
	out := make([]Session, 0, len(ss.OrderedIndex))
	for _, key := range ss.OrderedIndex {
		if s, ok := ss.Directory[key]; ok {
			out = append(out, s)
		}
	}
	return out
}
