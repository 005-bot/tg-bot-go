// Package filter implements the /filter command and its FSM conversation:
// the user types a street name, the address parser normalizes it, and a
// high-confidence match subscribes the user while a low-confidence match
// offers a confirmation keyboard (aiogram subscription.py parity).
package filter

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	address "github.com/005-bot/address-parser-go"
	"github.com/005-bot/tg-bot-go/internal/bot/handler"
	"github.com/005-bot/tg-bot-go/internal/fsm"
	"github.com/005-bot/tg-bot-go/internal/storage"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"
)

const (
	// promptBase is the /filter command reply; a current street value is
	// appended as "\n\n*Текущее значение:* {street}".
	promptBase = "📍 Введите название улицы для фильтрации уведомлений"

	// minSubscribeConfidence is the parser confidence required to subscribe
	// without asking for confirmation (aiogram parity: 0.85).
	minSubscribeConfidence = 0.85

	noMatchPrompt = "⚠️ Не удалось определить улицу!\n\n" +
		"Пожалуйста, укажите *только название улицы*, например:\n" +
		"- Ленина\n- Мира"

	confirmFormat = "🔍 Вы имели в виду *%s*?\n\n✅ Используйте кнопку для подтверждения\n🔄 Или введите другой вариант."
)

// Handler handles the /filter command and its FSM conversation.
type Handler struct {
	logger  *zap.Logger
	storage *storage.Service
	fsm     *fsm.Store
	parser  *address.Parser
	reply   handler.Sender
}

// New creates the /filter handler.
func New(
	logger *zap.Logger,
	svc *storage.Service,
	store *fsm.Store,
	parser *address.Parser,
	reply handler.Sender,
) handler.Handler {
	return &Handler{
		logger:  logger,
		storage: svc,
		fsm:     store,
		parser:  parser,
		reply:   reply,
	}
}

// Register registers the /filter command route and the FSM cancel/value
// routes. The value handlers exclude '/'-prefixed text, so commands always
// reach the command handlers regardless of the active state; the cancel
// route must precede the value route ("Отмена" is also plain text).
func (h *Handler) Register(router *telegofx.Router) {
	router.Handle(h.handleCommand, th.CommandEqual("filter"), th.AnyMessageWithFrom())
	router.Handle(h.handleCancel, fsm.StateFilter(h.fsm, fsm.FilterState), th.TextEqual("Отмена"))
	router.Handle(h.handleValue, fsm.StateFilter(h.fsm, fsm.FilterState), fsm.TextNotCommand())
}

func (h *Handler) handleCommand(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}
	userID := strconv.FormatInt(msg.From.ID, 10)

	f, err := h.storage.GetFilter(ctx, userID)
	if err != nil {
		return fmt.Errorf("get filter for user %s: %w", userID, err)
	}

	filterText := ""
	if f.Street != nil {
		filterText = fmt.Sprintf("\n\n*Текущее значение:* %s", *f.Street)
	}

	if stateErr := h.fsm.SetState(ctx, msg.Chat.ID, msg.From.ID, fsm.FilterState); stateErr != nil {
		return fmt.Errorf("set fsm state: %w", stateErr)
	}

	markup := &telego.ReplyKeyboardMarkup{
		Keyboard: [][]telego.KeyboardButton{
			{{Text: "Отмена", IconCustomEmojiID: "", Style: ""}},
		},
		IsPersistent:          false,
		ResizeKeyboard:        true,
		OneTimeKeyboard:       true,
		InputFieldPlaceholder: "",
		Selective:             false,
	}
	return h.reply.SendWithKeyboard(ctx, msg.Chat.ID, promptBase+filterText, markup)
}

// handleCancel clears the FSM state and reports the current subscription.
func (h *Handler) handleCancel(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}

	if err := h.fsm.ClearState(ctx, msg.Chat.ID, msg.From.ID); err != nil {
		return fmt.Errorf("clear fsm state: %w", err)
	}

	f, err := h.storage.GetFilter(ctx, strconv.FormatInt(msg.From.ID, 10))
	if err != nil {
		return fmt.Errorf("get filter for user %d: %w", msg.From.ID, err)
	}

	text := "Вы подписаны на все уведомления"
	if f.Street != nil {
		text = fmt.Sprintf("Вы подписаны на уведомления для %s", *f.Street)
	}
	return h.reply.SendWithKeyboard(ctx, msg.Chat.ID, text, &telego.ReplyKeyboardRemove{
		RemoveKeyboard: true,
		Selective:      false,
	})
}

// handleValue normalizes the street input and either subscribes, asks for
// confirmation, or reports a no-match. A nil match without error means the
// handler already replied and the flow must stop (Python parity).
func (h *Handler) handleValue(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}

	parsed, err := h.parseAndSubscribe(ctx, msg, msg.Text)
	if err != nil {
		return err
	}
	if parsed == nil {
		return nil
	}

	if clearErr := h.fsm.ClearState(ctx, msg.Chat.ID, msg.From.ID); clearErr != nil {
		return fmt.Errorf("clear fsm state: %w", clearErr)
	}
	h.logger.Info("user subscribed via filter", zap.Int64("user_id", msg.From.ID), zap.String("street", parsed.Name))
	return h.reply.SendWithKeyboard(ctx, msg.Chat.ID, "Создана подписка: "+parsed.Name, &telego.ReplyKeyboardRemove{
		RemoveKeyboard: true,
		Selective:      false,
	})
}

// parseAndSubscribe normalizes value and subscribes the user with the
// ORIGINAL database street name when confidence is at least 0.85. Lower
// confidence offers a confirmation keyboard; no match re-asks with the
// state retained (aiogram parse_and_subscribe parity).
func (h *Handler) parseAndSubscribe(ctx context.Context, msg *telego.Message, value string) (*address.Match, error) {
	parsed, err := h.parser.Normalize(ctx, strings.TrimSpace(value))
	if errors.Is(err, address.ErrNoMatch) {
		if stateErr := h.fsm.SetState(ctx, msg.Chat.ID, msg.From.ID, fsm.FilterState); stateErr != nil {
			return nil, fmt.Errorf("set fsm state: %w", stateErr)
		}
		if sendErr := h.reply.Send(ctx, msg.Chat.ID, noMatchPrompt); sendErr != nil {
			return nil, sendErr
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("normalize street %q: %w", value, err)
	}

	if parsed.Confidence < minSubscribeConfidence {
		if stateErr := h.fsm.SetState(ctx, msg.Chat.ID, msg.From.ID, fsm.FilterState); stateErr != nil {
			return nil, fmt.Errorf("set fsm state: %w", stateErr)
		}
		markup := &telego.ReplyKeyboardMarkup{
			Keyboard: [][]telego.KeyboardButton{
				{{Text: parsed.Name, IconCustomEmojiID: "", Style: ""}},
				{{Text: "Отмена", IconCustomEmojiID: "", Style: ""}},
			},
			IsPersistent:          false,
			ResizeKeyboard:        true,
			OneTimeKeyboard:       true,
			InputFieldPlaceholder: "",
			Selective:             false,
		}
		if sendErr := h.reply.SendWithKeyboard(
			ctx,
			msg.Chat.ID,
			fmt.Sprintf(confirmFormat, parsed.Name),
			markup,
		); sendErr != nil {
			return nil, sendErr
		}
		return nil, nil
	}

	userID := strconv.FormatInt(msg.From.ID, 10)
	if subErr := h.storage.Subscribe(ctx, userID, &parsed.Name); subErr != nil {
		return nil, fmt.Errorf("subscribe street for user %s: %w", userID, subErr)
	}
	return parsed, nil
}
