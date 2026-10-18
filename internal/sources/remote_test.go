package sources

import (
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestFilterBlacklistMatchesMergeRule(t *testing.T) {
	sessions := []model.Session{{Name: "hidden"}, {Name: "visible"}}
	blacklist := []string{"^hidden$"}
	assert.Equal(t, sessions, FilterBlacklist(sessions, nil, false))
	assert.Equal(t, []model.Session{{Name: "visible"}}, FilterBlacklist(sessions, blacklist, false))
	assert.Equal(t, []model.Session{{Name: "hidden"}}, FilterBlacklist(sessions, blacklist, true))
	assert.Empty(t, FilterBlacklist(sessions, nil, true))
}
