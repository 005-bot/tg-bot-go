// Package reply sends Markdown-formatted replies to chats, mirroring the
// Python bot's DefaultBotProperties(parse_mode=ParseMode.MARKDOWN).
package reply

import (
	"context"
	"fmt"

	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// Service is the base reply helper. Every command reply is sent with Markdown
// parse mode, mirroring the Python bot's DefaultBotProperties(parse_mode =
// ParseMode.MARKDOWN). Send failures are logged and returned so the global
// error middleware can inform the user (aiogram error-handler parity).
type Service struct {
	bot    *telego.Bot
	logger *zap.Logger
}

// NewService creates the reply helper bound to the Telegram bot.
func NewService(bot *telegofx.Bot, logger *zap.Logger) *Service {
	return &Service{bot: bot.Bot, logger: logger}
}

// Send replies with a plain Markdown text message.
func (s *Service) Send(ctx context.Context, chatID int64, text string) error {
	return s.SendWithKeyboard(ctx, chatID, text, nil)
}

// SendWithKeyboard replies with Markdown text and an optional reply markup.
func (s *Service) SendWithKeyboard(
	ctx context.Context,
	chatID int64,
	text string,
	markup telego.ReplyMarkup,
) error {
	if _, err := s.bot.SendMessage(ctx, &telego.SendMessageParams{
		ChatID:      telego.ChatID{ID: chatID, Username: ""},
		Text:        text,
		ParseMode:   telego.ModeMarkdown,
		ReplyMarkup: markup,
	}); err != nil {
		s.logger.Error("send telegram message failed",
			zap.Int64("chat_id", chatID), zap.Error(err))
		return fmt.Errorf("send telegram message: %w", err)
	}
	return nil
}
