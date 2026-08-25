// Package help implements the /help command with the verbatim static help
// text from the Python bot.
package help

import (
	"github.com/005-bot/tg-bot-go/internal/bot/handler"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"
)

// helpText is the verbatim static help message (Python app/handlers/help.py).
const helpText = "👋 *Добро пожаловать!*\n\n" +
	"📋 **Доступные команды:**\n" +
	"- /start - Подписаться на уведомления об отключениях 🔔\n" +
	"- /stop - Отписаться от уведомлений 🔕\n" +
	"- /filter - Подписаться на уведомления по конкретной улице 🏘️\n" +
	"- /feedback - Оставить отзыв 💬\n" +
	"- /help - Показать эту справку ❓\n\n" +
	"📬 *По всем вопросам:*\n" +
	"Пишите нам: help@xn--005-ddd9dya.xn--p1ai\n\n" +
	"ℹ️ *Источник информации об отключениях:*\n" +
	"https://005красноярск.рф"

// Handler handles the /help command.
type Handler struct {
	logger *zap.Logger
	reply  handler.Sender
}

// New creates the /help handler.
func New(logger *zap.Logger, reply handler.Sender) handler.Handler {
	return &Handler{logger: logger, reply: reply}
}

// Register registers the /help route on the router.
func (h *Handler) Register(router *telegofx.Router) {
	router.Handle(h.handleHelp, th.CommandEqual("help"), th.AnyMessageWithFrom())
}

func (h *Handler) handleHelp(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}
	return h.reply.Send(ctx, msg.Chat.ID, helpText)
}
