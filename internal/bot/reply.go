package bot

import (
	"context"
	"fmt"

	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// Reply is the base reply helper. Every command reply is sent with Markdown
// parse mode, mirroring the Python bot's DefaultBotProperties(parse_mode =
// ParseMode.MARKDOWN). Send failures are logged and returned so the global
// error middleware can inform the user (aiogram error-handler parity).
type Reply struct {
	bot    *telego.Bot
	logger *zap.Logger
}

// NewReply creates the reply helper bound to the Telegram bot.
func NewReply(bot *telegofx.Bot, logger *zap.Logger) *Reply {
	return &Reply{bot: bot.Bot, logger: logger}
}

// Send replies with a plain Markdown text message.
func (r *Reply) Send(ctx context.Context, chatID int64, text string) error {
	return r.SendWithKeyboard(ctx, chatID, text, nil)
}

// SendWithKeyboard replies with Markdown text and an optional reply markup.
func (r *Reply) SendWithKeyboard(
	ctx context.Context,
	chatID int64,
	text string,
	markup telego.ReplyMarkup,
) error {
	if _, err := r.bot.SendMessage(ctx, &telego.SendMessageParams{
		ChatID:      telego.ChatID{ID: chatID, Username: ""},
		Text:        text,
		ParseMode:   telego.ModeMarkdown,
		ReplyMarkup: markup,
	}); err != nil {
		r.logger.Error("send telegram message failed",
			zap.Int64("chat_id", chatID), zap.Error(err))
		return fmt.Errorf("send telegram message: %w", err)
	}
	return nil
}
