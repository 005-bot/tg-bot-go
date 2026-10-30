package storage

import (
	"github.com/go-core-fx/logger"
	"go.uber.org/fx"
)

// Module wires the storage service into the fx graph.
func Module() fx.Option {
	return fx.Module(
		"storage",
		logger.WithNamedLogger("storage"),
		fx.Provide(
			NewService,
		),
	)
}
