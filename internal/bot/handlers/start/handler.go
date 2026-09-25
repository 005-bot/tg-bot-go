// Package start implements the /start command with aiogram deep-link
// payload parity (base64url-encoded street name) and the subscription flow.
package start

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	address "github.com/005-bot/address-parser-go"
	"github.com/005-bot/tg-bot-go/internal/bot/handler"
	"github.com/005-bot/tg-bot-go/internal/fsm"
	"github.com/005-bot/tg-bot-go/internal/storage"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"
)

const minSubscribeConfidence = 0.85

var errInvalidUTF8 = errors.New("invalid utf-8")

// Handler handles the /start command.
type Handler struct {
	logger      *zap.Logger
	storage     *storage.Service
	fsm         *fsm.Store
	parser      *address.Parser
	reply       handler.Sender
	notificator handler.Notificator
}

// New creates the /start handler.
func New(
	logger *zap.Logger,
	svc *storage.Service,
	store *fsm.Store,
	parser *address.Parser,
	reply handler.Sender,
	notificator handler.Notificator,
) handler.Handler {
	return &Handler{
		logger:      logger,
		storage:     svc,
		fsm:         store,
		parser:      parser,
		reply:       reply,
		notificator: notificator,
	}
}

// Register registers the /start route on the router.
func (h *Handler) Register(router *telegofx.Router) {
	router.Handle(h.handleStart, th.CommandEqual("start"), th.AnyMessageWithFrom())
}

func (h *Handler) handleStart(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}
	userID := strconv.FormatInt(msg.From.ID, 10)

	payload, err := decodeStartPayload(commandArgs(msg.Text))
	if err != nil {
		if sendErr := h.reply.Send(ctx, msg.Chat.ID, "🚨 Системная ошибка - наша команда уведомлена"); sendErr != nil {
			return fmt.Errorf("send start error: %w", sendErr)
		}
		return nil
	}

	if err = h.storage.Subscribe(ctx, userID, nil); err != nil {
		return fmt.Errorf("subscribe user %s: %w", userID, err)
	}

	if payload == "" {
		return h.finishStart(ctx, msg, nil)
	}

	match, handled, err := h.parseAndSubscribe(ctx, msg, payload)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	return h.finishStart(ctx, msg, match)
}

// parseAndSubscribe normalizes a street name and either subscribes with its
// original name or asks for clarification. A handled result means the handler
// already replied and the flow must stop (Python parity).
func (h *Handler) parseAndSubscribe(
	ctx context.Context,
	msg *telego.Message,
	value string,
) (*address.Match, bool, error) {
	parsed, err := h.parser.Normalize(ctx, strings.TrimSpace(value))
	if errors.Is(err, address.ErrNoMatch) {
		if stateErr := h.fsm.SetState(ctx, msg.Chat.ID, msg.From.ID, fsm.FilterState); stateErr != nil {
			return nil, false, fmt.Errorf("set fsm state: %w", stateErr)
		}
		if sendErr := h.reply.Send(
			ctx,
			msg.Chat.ID,
			"⚠️ Не удалось определить улицу!\n\nПожалуйста, укажите *только название улицы*, например:\n- Ленина\n- Мира",
		); sendErr != nil {
			return nil, false, fmt.Errorf("send no-match prompt: %w", sendErr)
		}
		return nil, true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("normalize street %q: %w", value, err)
	}

	if parsed.Confidence < minSubscribeConfidence {
		if stateErr := h.fsm.SetState(ctx, msg.Chat.ID, msg.From.ID, fsm.FilterState); stateErr != nil {
			return nil, false, fmt.Errorf("set fsm state: %w", stateErr)
		}
		markup := &telego.ReplyKeyboardMarkup{
			Keyboard: [][]telego.KeyboardButton{
				{
					{
						Text:              parsed.Name,
						IconCustomEmojiID: "",
						Style:             "",
						RequestUsers:      nil,
						RequestChat:       nil,
						RequestManagedBot: nil,
						RequestContact:    false,
						RequestLocation:   false,
						RequestPoll:       nil,
						WebApp:            nil,
					},
				},
				{
					{
						Text:              "Отмена",
						IconCustomEmojiID: "",
						Style:             "",
						RequestUsers:      nil,
						RequestChat:       nil,
						RequestManagedBot: nil,
						RequestContact:    false,
						RequestLocation:   false,
						RequestPoll:       nil,
						WebApp:            nil,
					},
				},
			},
			IsPersistent:          false,
			ResizeKeyboard:        true,
			OneTimeKeyboard:       true,
			InputFieldPlaceholder: "",
			Selective:             false,
			ForceReply:            false,
		}
		if sendErr := h.reply.SendWithKeyboard(
			ctx,
			msg.Chat.ID,
			fmt.Sprintf(
				"🔍 Вы имели в виду *%s*?\n\n✅ Используйте кнопку для подтверждения\n🔄 Или введите другой вариант.",
				parsed.Name,
			),
			markup,
		); sendErr != nil {
			return nil, false, fmt.Errorf("send confirmation prompt: %w", sendErr)
		}
		return nil, true, nil
	}

	userID := strconv.FormatInt(msg.From.ID, 10)
	if err = h.storage.Subscribe(ctx, userID, &parsed.Name); err != nil {
		return nil, false, fmt.Errorf("subscribe street for user %s: %w", userID, err)
	}
	return parsed, false, nil
}

// finishStart sends the welcome message and notifies the admin about the
// new subscription.
func (h *Handler) finishStart(ctx context.Context, msg *telego.Message, match *address.Match) error {
	base := "✅ Вы подписаны на уведомления об отключениях"
	var details string
	if match != nil {
		details = fmt.Sprintf(" по адресу:\n*%s*", match.Name)
	} else {
		details = "\n\n🔍 Чтобы получать уведомления только по конкретной улице, используйте /filter"
	}
	text := fmt.Sprintf("%s%s\n\nℹ️ Источник информации об отключениях: https://005красноярск.рф", base, details)

	if err := h.reply.Send(ctx, msg.Chat.ID, text); err != nil {
		return fmt.Errorf("send welcome: %w", err)
	}
	h.logger.Info("user subscribed", zap.Int64("user_id", msg.From.ID))
	if err := h.notificator.NewUser(ctx, msg.From); err != nil {
		return fmt.Errorf("notify admin: %w", err)
	}
	return nil
}

// commandArgs extracts the arguments group of a command message.
func commandArgs(text string) string {
	matches := th.CommandRegexp.FindStringSubmatch(text)
	if len(matches) != th.CommandMatchGroupsLen {
		return ""
	}
	return matches[th.CommandMatchArgsGroup]
}

// decodeStartPayload decodes an aiogram deep-link payload (base64url without
// padding, tolerant of trailing '=' padding). An empty payload decodes to an
// empty string without error; a decode failure is returned as an error.
func decodeStartPayload(args string) (string, error) {
	if args == "" {
		return "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(args, "="))
	if err != nil {
		return "", fmt.Errorf("decode start payload: %w", err)
	}
	if !utf8.Valid(raw) {
		return "", fmt.Errorf("decode start payload: %w", errInvalidUTF8)
	}
	return string(raw), nil
}
