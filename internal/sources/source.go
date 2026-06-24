package sources

import (
	"context"

	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
)

type Source interface {
	Name() string
	List(context.Context) (model.Sessions, error)
}
