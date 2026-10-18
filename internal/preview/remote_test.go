package preview

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemotePreviewOnlyMetadata(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "/does-not-exist")
	s := model.Session{Source: model.SourceRemote, Name: "remote", PreviewCommand: "exit 42", Remote: &model.RemoteRef{Machine: "peer", Session: "main", WorkspaceID: "w1", CWD: "/does-not-exist", Offline: true}}
	for _, render := range []func() (string, error){func() (string, error) { return Render(context.Background(), s, "exit 43") }, func() (string, error) { return RenderPane(context.Background(), s) }} {
		text, err := render()
		require.NoError(t, err)
		assert.Contains(t, text, "peer")
		assert.Contains(t, text, "offline (cached)")
		assert.NotContains(t, text, "/does-not-exist")
	}
}
