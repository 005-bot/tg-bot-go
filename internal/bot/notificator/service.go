// Package notificator sends admin-facing notifications to the configured
// admin chat (new subscriptions and user feedback).
package notificator

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// Config configures the admin notificator. An AdminID of zero disables all
// admin notifications (safe no-op).
type Config struct {
	AdminID int64
}

// Service sends admin-facing notifications to the configured admin chat.
type Service struct {
	cfg Config

	bot *telego.Bot

	logger *zap.Logger
}

// NewService creates the admin notificator bound to the Telegram bot.
func NewService(cfg Config, bot *telegofx.Bot, logger *zap.Logger) *Service {
	return &Service{cfg: cfg, bot: bot.Bot, logger: logger}
}

// NewUser notifies the admin about a new subscription with a Markdown link
// to the user. The link title falls back username -> first_name -> id
// (aiogram notificator parity), with markdown quoting of the dynamic title.
func (s *Service) NewUser(ctx context.Context, user *telego.User) error {
	title := user.Username
	if title == "" {
		title = user.FirstName
	}
	if title == "" {
		title = strconv.FormatInt(user.ID, 10)
	}
	id := strconv.FormatInt(user.ID, 10)
	message := "👤 *Новая подписка:* [" + quoteMarkdown(title) + "](tg://user?id=" + id + ")"
	return s.Notify(ctx, message)
}

// Feedback forwards the user's message to the admin chat.
func (s *Service) Feedback(ctx context.Context, message *telego.Message) error {
	if s.cfg.AdminID == 0 {
		return nil
	}
	if _, err := s.bot.ForwardMessage(ctx, &telego.ForwardMessageParams{
		ChatID:     telego.ChatID{ID: s.cfg.AdminID, Username: ""},
		FromChatID: telego.ChatID{ID: message.Chat.ID, Username: ""},
		MessageID:  message.MessageID,
	}); err != nil {
		s.logger.Error("forward feedback failed", zap.Error(err))
		return fmt.Errorf("forward feedback: %w", err)
	}
	return nil
}

// Notify sends a Markdown message to the admin chat. It is a no-op when no
// admin is configured.
func (s *Service) Notify(ctx context.Context, text string) error {
	if s.cfg.AdminID == 0 {
		return nil
	}
	if _, err := s.bot.SendMessage(ctx, &telego.SendMessageParams{
		ChatID:    telego.ChatID{ID: s.cfg.AdminID, Username: ""},
		Text:      text,
		ParseMode: telego.ModeMarkdown,
	}); err != nil {
		s.logger.Error("notify admin failed", zap.Error(err))
		return fmt.Errorf("notify admin: %w", err)
	}
	return nil
}

// markdownReplacer escapes exactly the characters of aiogram's MARKDOWN_CHARS
// (_*[]()~`>#+-=|{}.!) with a backslash, in a single pass.
//
//nolint:gochecknoglobals // static lookup table, not mutable state
var markdownReplacer = strings.NewReplacer(
	"_", "\\_",
	"*", "\\*",
	"[", "\\[",
	"]", "\\]",
	"(", "\\(",
	")", "\\)",
	"~", "\\~",
	"`", "\\`",
	">", "\\>",
	"#", "\\#",
	"+", "\\+",
	"-", "\\-",
	"=", "\\=",
	"|", "\\|",
	"{", "\\{",
	"}", "\\}",
	".", "\\.",
	"!", "\\!",
)

// quoteMarkdown backslash-escapes dynamic text for Telegram Markdown
// (aiogram utils.markdown.quote parity).
func quoteMarkdown(s string) string {
	return markdownReplacer.Replace(s)
}
