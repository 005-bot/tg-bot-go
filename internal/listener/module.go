package listener

import (
	"github.com/go-core-fx/fxutil"
	"github.com/go-core-fx/logger"
	"go.uber.org/fx"
)

// Module wires the outage listener into the fx graph. The Service is
// registered as a background runnable, so the subscribe loop starts at
// serve boot and stops on application shutdown. It is deliberately not
// fx.Private: the notifier module consumes Service.Events for the
// broadcast worker (fx module scopes hide private results from sibling
// scopes).
func Module() fx.Option {
	return fx.Module(
		"listener",
		logger.WithNamedLogger("listener"),
		fx.Provide(
			NewService,
		),
		fx.Invoke(
			fxutil.RegisterRunnable[*Service](),
		),
	)
}
