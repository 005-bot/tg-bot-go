package fsm

import (
	"github.com/go-core-fx/logger"
	"go.uber.org/fx"
)

// Module wires the FSM store into the fx graph. The store is consumed by
// the bot handlers via constructor injection.
func Module() fx.Option {
	return fx.Module(
		"fsm",
		logger.WithNamedLogger("fsm"),
		fx.Provide(
			NewStore,
		),
	)
}
