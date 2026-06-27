package sources

import (
	"context"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

type Source interface {
	Name() string
	List(context.Context) (model.Sessions, error)
}
