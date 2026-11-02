package app

import (
	"os"
	"testing"
)

// Tests fake the Herdr TUI CLI. Running them from a herdr-app terminal must not
// select herdr-app and drive the live window.
func TestMain(m *testing.M) {
	for _, name := range []string{"HC_TERMINAL_SOCKET", "HC_TERMINAL_ID", "HERDR_SESH_BACKEND"} {
		_ = os.Unsetenv(name)
	}
	os.Exit(m.Run())
}
