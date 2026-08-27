package bot

import (
	"context"
	"fmt"
	"time"

	address "github.com/005-bot/address-parser-go"
	"github.com/005-bot/tg-bot-go/internal/bot/handler"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/feedback"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/filter"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/help"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/start"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/stop"
	"github.com/go-core-fx/logger"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpproxy"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Module wires the bot: proxy client option, address parser, reply helper,
// admin notificator, error middleware, and the command handlers. The error
// middleware is registered before the handlers so it wraps every route.
func Module() fx.Option {
	return fx.Module(
		"bot",
		logger.WithNamedLogger("bot"),
		fx.Provide(func() []telego.BotOption {
			return []telego.BotOption{
				telego.WithFastHTTPClient(&fasthttp.Client{Dial: fasthttpproxy.FasthttpProxyHTTPDialer()}),
			}
		}),
		fx.Provide(
			newAddressParser,
			NewReply,
			// fx has no implicit interface binding: expose the concrete
			// implementations under the handler contract types.
			func(reply *Reply) handler.Sender { return reply },
			NewNotificator,
			func(notificator *Notificator) handler.Notificator { return notificator },
			ErrorMiddleware,
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
func registerHandlers(handlers []handler.Handler, router *telegofx.Router, middleware th.Handler) {
	router.Use(middleware)
	for _, h := range handlers {
		h.Register(router)
	}
}

// newAddressParser creates the street parser backed by the embedded streets
// database and releases its temporary files on shutdown.
func newAddressParser(lc fx.Lifecycle) (*address.Parser, error) {
	parser, err := address.NewParser(address.Config{})
	if err != nil {
		return nil, fmt.Errorf("create address parser: %w", err)
	}
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			parser.Stop()
			return nil
		},
	})
	return parser, nil
}

// setCommands installs the bot's command list at startup. The stop
// description keeps the combining breve (U+0306) from the Python contract.
// It runs asynchronously: Telegram API connectivity (or a dead proxy) must
// never block application startup. Failures are logged, not fatal.
func setCommands(bot *telegofx.Bot, logger *zap.Logger) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
