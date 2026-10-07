package app

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoryWatcherPIDsFromLSOF(t *testing.T) {
	for _, tt := range []struct {
		name   string
		script string
		want   []int
		fail   bool
	}{
		{name: "holders", script: "printf '123\n456\n'", want: []int{123, 456}},
		{name: "no holders", script: "exit 1"},
		{name: "fatal empty output", script: "exit 2", fail: true},
		{name: "failed with output", script: "printf '123\n'; exit 1", fail: true},
		{name: "invalid pid", script: "printf 'nope\n'", fail: true},
		{name: "process group pid", script: "printf '0\n'", fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			//nolint:gosec // scripts are fixed test cases, not external input.
			got, err := historyWatcherPIDsFromLSOF(exec.Command("/bin/sh", "-c", tt.script))
			if tt.fail {
				require.Error(t, err)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
