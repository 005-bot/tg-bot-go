package handler

import (
	"context"

	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
)

// Handler registers bot routes in the shared telegofx router.
type Handler interface {
	Register(router *telegofx.Router)
}

// Sender replies to chats with Markdown-formatted text (aiogram
// DefaultBotProperties(parse_mode=Markdown) parity). Implemented by
// internal/bot.Reply.
type Sender interface {
	Send(ctx context.Context, chatID int64, text string) error
	SendWithKeyboard(ctx context.Context, chatID int64, text string, markup telego.ReplyMarkup) error
}

// Notificator notifies the admin chat about bot events. Implemented by
// internal/bot.Notificator.
type Notificator interface {
	NewUser(ctx context.Context, user *telego.User) error
	Feedback(ctx context.Context, message *telego.Message) error
	Notify(ctx context.Context, text string) error
}
