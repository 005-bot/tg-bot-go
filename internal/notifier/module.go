package notifier

import (
	"github.com/go-core-fx/fxutil"
	"github.com/go-core-fx/logger"
	"go.uber.org/fx"
)

// Module wires the outage broadcast worker into the fx graph. The Worker is
// private to this module and registered as a background runnable, so the
// broadcast loop starts at serve boot and stops on application shutdown.
func Module() fx.Option {
	return fx.Module(
		"notifier",
		logger.WithNamedLogger("notifier"),
		fx.Provide(NewWorker, fx.Private),
		fx.Invoke(
			fxutil.RegisterRunnable[*Worker](),
		),
	)
}
