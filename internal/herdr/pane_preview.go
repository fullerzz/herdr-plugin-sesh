package herdr

import (
	"context"
	"encoding/json"
	"fmt"
)

// WorkspacePaneRead reads the selected pane of a workspace's active tab,
// including when that workspace is not globally focused.
func (c *CLIClient) WorkspacePaneRead(ctx context.Context, workspaceID string) (string, error) {
	if workspaceID == "" {
		return "", fmt.Errorf("no Herdr workspace selected")
	}
	out, err := c.run(ctx, "api", "snapshot")
	if err != nil {
		return "", err
	}
	raw, _, err := responseJSON(out, "api snapshot")
	if err != nil {
		return "", err
	}
	var response struct {
		Snapshot struct {
			Workspaces []Workspace `json:"workspaces"`
			Layouts    []struct {
				WorkspaceID   string `json:"workspace_id"`
				TabID         string `json:"tab_id"`
				FocusedPaneID string `json:"focused_pane_id"`
			} `json:"layouts"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return "", fmt.Errorf("decode herdr api snapshot JSON: %w", err)
	}
	activeTabID := ""
	for _, workspace := range response.Snapshot.Workspaces {
		if workspace.ID == workspaceID {
			activeTabID = workspace.ActiveTabID
			break
		}
	}
	for _, layout := range response.Snapshot.Layouts {
		if activeTabID != "" && layout.WorkspaceID == workspaceID && layout.TabID == activeTabID && layout.FocusedPaneID != "" {
			out, err := c.run(ctx, "pane", "read", layout.FocusedPaneID, "--source", "visible", "--format", "ansi")
			return string(out), err
		}
	}
	return "", fmt.Errorf("no active pane available for workspace %s", workspaceID)
}
