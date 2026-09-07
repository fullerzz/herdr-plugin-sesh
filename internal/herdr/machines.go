package herdr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Machine is a saved profile on the executing host, not a live client view.
// The catalog's persisted selected field is deliberately not exposed.
type Machine struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Target  string `json:"target"`
	Session string `json:"session"`
	Enabled bool   `json:"enabled"`
}

func (c *CLIClient) MachineList(ctx context.Context) ([]Machine, error) {
	out, err := c.run(ctx, "machine", "list", "--json")
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Machine

		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("decode herdr machine list JSON: %w", err)
	}
	if rows == nil {
		return nil, fmt.Errorf("decode herdr machine list JSON: expected an array")
	}
	machines := make([]Machine, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	for i, row := range rows {
		if !validMachineText(row.ID) || strings.ContainsFunc(row.ID, unicode.IsSpace) ||
			!validMachineText(row.Label) || !validMachineText(row.Target) || !validMachineText(row.Session) || row.Enabled == nil {
			return nil, fmt.Errorf("invalid saved machine at row %d: expected printable identity, label, target, session and enabled flag", i+1)
		}
		if seen[row.ID] {
			return nil, fmt.Errorf("duplicate saved machine ID at row %d", i+1)
		}
		seen[row.ID] = true
		row.Machine.Enabled = *row.Enabled
		machines = append(machines, row.Machine)
	}
	return machines, nil
}

func validMachineText(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 1024 && utf8.ValidString(value) &&
		!strings.ContainsFunc(value, func(r rune) bool { return !unicode.IsPrint(r) })
}
