package preview

import (
	"os"
	"testing"
)

// Tests fake the Herdr TUI CLI unless they select herdr-app themselves; a run
// from a herdr-app terminal must never read the live app.
func TestMain(m *testing.M) {
	if err := os.Setenv("HERDR_SESH_BACKEND", "herdr"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
