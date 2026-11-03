package herdr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUsesAppSelectsHerdrAppOnlyOutsideHerdrPanes(t *testing.T) {
	cases := []struct {
		name, backend, herdrSocket, hcSocket string
		want                                 bool
	}{
		{name: "herdr-app terminal", hcSocket: "/hc.sock", want: true},
		{name: "herdr pane inside hc", herdrSocket: "/herdr.sock", hcSocket: "/hc.sock"},
		{name: "plain shell"},
		{name: "forced app", backend: "app", want: true},
		{name: "forced herdr", backend: "herdr", hcSocket: "/hc.sock"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HERDR_SESH_BACKEND", tc.backend)
			t.Setenv("HERDR_SOCKET_PATH", tc.herdrSocket)
			t.Setenv("HC_TERMINAL_SOCKET", tc.hcSocket)
			assert.Equal(t, tc.want, UsesApp())
		})
	}
}
