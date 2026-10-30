package bot

import (
	"context"
	"time"

	"github.com/005-bot/tg-bot-go/internal/bot/handler"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/feedback"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/filter"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/help"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/start"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/stop"
	"github.com/005-bot/tg-bot-go/internal/bot/notificator"
	"github.com/005-bot/tg-bot-go/internal/bot/reply"
	"github.com/005-bot/tg-bot-go/internal/bot/subscribe"
	"github.com/go-core-fx/logger"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const setCommandsTimeout = 10 * time.Second

// Module wires the bot's business logic: the address parser, the reply
// helper, admin notificator, error middleware, and the command handlers. The
// error middleware is registered before the handlers so it wraps every route.
// The Telegram transport (HTTP client and telegofx options) lives in the
// telegram module.
func Module() fx.Option {
	return fx.Module(
		"bot",
		logger.WithNamedLogger("bot"),
		fx.Provide(
			reply.NewService,
			notificator.NewService,
			subscribe.NewService,
		),
		fx.Provide(
			fx.Annotate(start.New, fx.ResultTags(`group:"handlers"`)),
			fx.Annotate(stop.New, fx.ResultTags(`group:"handlers"`)),
			fx.Annotate(help.New, fx.ResultTags(`group:"handlers"`)),
			fx.Annotate(filter.New, fx.ResultTags(`group:"handlers"`)),
			fx.Annotate(feedback.New, fx.ResultTags(`group:"handlers"`)),
		),
		fx.Invoke(
			fx.Annotate(
				registerHandlers,
				fx.ParamTags(`group:"handlers"`, "", ""),
			),
			setCommands,
		),
	)
}

// registerHandlers registers the global error middleware before the command
// handlers: telegohandler processes routes in registration order and stops
// at the first match, so the middleware must come first.
func registerHandlers(handlers []handler.Handler, router *telegofx.Router) {
	for _, h := range handlers {
		h.Register(router)
	}
}

// setCommands installs the bot's command list at startup. The stop
// description keeps the combining breve (U+0306) from the Python contract.
// It runs asynchronously: Telegram API connectivity (or a dead proxy) must
// never block application startup. Failures are logged, not fatal.
func setCommands(bot *telegofx.Bot, logger *zap.Logger) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), setCommandsTimeout)
		defer cancel()

		commands := []telego.BotCommand{
			{Command: "start", Description: "Подписаться на уведомления", IsEphemeral: false},
			{Command: "stop", Description: "Отписаться от уведомлении\u0306", IsEphemeral: false},
			{
				Command:     "filter",
				Description: "Подписаться на уведомления только по определенной улице",
				IsEphemeral: false,
			},
			{Command: "feedback", Description: "Отправить отзыв", IsEphemeral: false},
			{Command: "help", Description: "Показать справку", IsEphemeral: false},
		}
		if err := bot.SetMyCommands(ctx, &telego.SetMyCommandsParams{Commands: commands}); err != nil {
			logger.Error("set my commands failed", zap.Error(err))
		}
	}()
}
