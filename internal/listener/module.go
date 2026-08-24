package listener

import (
	"github.com/go-core-fx/fxutil"
	"github.com/go-core-fx/logger"
	"go.uber.org/fx"
)

// Module wires the outage listener into the fx graph. The Service is
// private to this module and registered as a background runnable, so the
// subscribe loop starts at serve boot and stops on application shutdown.
func Module() fx.Option {
	return fx.Module(
		"listener",
		logger.WithNamedLogger("listener"),
		fx.Provide(
			NewService, fx.Private,
		),
		fx.Invoke(
			fxutil.RegisterRunnable[*Service](),
		),
	)
}
