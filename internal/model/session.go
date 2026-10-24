package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type WorktreeRelation struct {
	Linked              bool
	ParentWorkspaceID   string
	ParentWorkspaceName string
}

const SourceRemote = "remote"

// RemoteRef identifies a workspace in another machine's Herdr session. Remote
// rows keep WorkspaceID and Path empty so local actions never see foreign values.
type RemoteRef struct {
	Machine     string `json:"machine"`
	Session     string `json:"session"`
	WorkspaceID string `json:"workspace_id"`
	CWD         string `json:"cwd,omitempty"`
	Offline     bool   `json:"offline,omitempty"`
}

type Session struct {
	Remote                *RemoteRef       `json:"remote,omitempty"`
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
	// Panes is native-only; the legacy Sesh window schema has no equivalent.
	Panes []PaneConfig `json:"panes,omitempty" toml:"-"`
}

// PaneConfig describes one pane of a tab layout. The first pane reuses the
// tab's root pane; later panes split the earlier pane named by SplitFrom.
type PaneConfig struct {
	Name      string            `json:"name"`
	SplitFrom string            `json:"split_from,omitempty"`
	Split     string            `json:"split,omitempty"`
	Ratio     float64           `json:"ratio,omitempty"` // New pane's share of the split; 0 uses Herdr's default.
	Path      string            `json:"path,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Startup   string            `json:"startup,omitempty"`
	// WaitFor, when set, holds later layout operations until Startup prints Match.
	WaitFor *PaneWait `json:"wait_for,omitempty"`
	// Focus selects the pane focused after a new workspace's layout is built.
	Focus bool `json:"focus,omitempty"`
}

// PaneWait is a one-time startup barrier, not a health check.
type PaneWait struct {
	Match   string        `json:"match"`
	Timeout time.Duration `json:"timeout"`
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

// Only the canonical key namespace is reserved; names like remote:prod stay local.
func IsRemoteTarget(target string) bool {
	suffix, ok := strings.CutPrefix(target, SourceRemote+":")
	return ok && len(suffix) == 16 && strings.Trim(suffix, "0123456789abcdef") == ""
}

func Key(s Session) string {
	base := fmt.Sprintf("%s\x00%s\x00%s\x00%s", s.Source, s.Name, s.Path, s.WorkspaceID)
	if s.Remote != nil {
		base = fmt.Sprintf("%s\x00%s\x00%s", s.Remote.Machine, s.Remote.Session, s.Remote.WorkspaceID)
	}
	sum := sha256.Sum256([]byte(base))
	return s.Source + ":" + hex.EncodeToString(sum[:8])
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

func (s Session) DisplayName() string {
	if s.Remote == nil {
		return s.Name
	}
	status := ""
	if s.Remote.Offline {
		status = " (offline)"
	}
	return s.Name + " @ " + s.Remote.Machine + status
}
