package state

import (
	"github.com/go-core-fx/logger"
	"github.com/go-core-fx/telegofx/fsm"
	"go.uber.org/fx"
)

// Module wires the Redis-backed fsm.Storage into the fx graph. The telegofx
// module (telegofx.WithFSM) provides the fsm.Store over it; handlers consume
// the store via constructor injection.
func Module() fx.Option {
	return fx.Module(
		"fsm",
		logger.WithNamedLogger("fsm"),
		fx.Provide(
			fx.Annotate(NewStorage, fx.As(new(fsm.Storage))),
		),
	)
}
