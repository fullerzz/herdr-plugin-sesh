package preview

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemotePreviewDoesNotExecuteLocalCommandsOrReadDirectories(t *testing.T) {
	s := model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: "/does-not-exist", WorkspaceID: "buntu26/sp1", PreviewCommand: "exit 97"}
	text, err := Render(context.Background(), s, "exit 98")
	require.NoError(t, err)
	assert.Contains(t, text, "machine: buntu26")
	assert.Contains(t, text, "id: buntu26/sp1")
	assert.Contains(t, text, "path: /does-not-exist")
	s.PreviewCommand = ""
	text, err = Render(context.Background(), s, "exit 98")
	require.NoError(t, err)
	assert.Contains(t, text, "machine: buntu26")
}
