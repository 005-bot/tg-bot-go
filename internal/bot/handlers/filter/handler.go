// Package filter implements the /filter command and its FSM conversation:
// the user types a street name, the address parser normalizes it, and a
// high-confidence match subscribes the user while a low-confidence match
// offers a confirmation keyboard (aiogram subscription.py parity).
package filter

import (
	"fmt"
	"strconv"

	"github.com/005-bot/tg-bot-go/internal/bot/handler"
	"github.com/005-bot/tg-bot-go/internal/bot/reply"
	"github.com/005-bot/tg-bot-go/internal/bot/subscribe"
	"github.com/005-bot/tg-bot-go/internal/state"
	"github.com/005-bot/tg-bot-go/internal/storage"
	"github.com/go-core-fx/telegofx"
	"github.com/go-core-fx/telegofx/fsm"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"go.uber.org/zap"
)

const (
	// promptBase is the /filter command reply; a current street value is
	// appended as "\n\n*Текущее значение:* {street}".
	promptBase = "📍 Введите название улицы для фильтрации уведомлений"
)

// Handler handles the /filter command and its FSM conversation.
type Handler struct {
	logger    *zap.Logger
	storage   *storage.Service
	fsm       *fsm.Store
	subscribe *subscribe.Service
	reply     *reply.Service
}

// New creates the /filter handler.
func New(
	logger *zap.Logger,
	svc *storage.Service,
	store *fsm.Store,
	sub *subscribe.Service,
	reply *reply.Service,
) handler.Handler {
	return &Handler{
		logger:    logger,
		storage:   svc,
		fsm:       store,
		subscribe: sub,
		reply:     reply,
	}
}

// Register registers the /filter command route and the FSM cancel/value
// routes. The value handlers exclude '/'-prefixed text, so commands always
// reach the command handlers regardless of the active state; the cancel
// route must precede the value route ("Отмена" is also plain text).
func (h *Handler) Register(router *telegofx.Router) {
	router.Handle(h.handleCommand, th.CommandEqual("filter"), th.AnyMessageWithFrom())
	router.Handle(h.handleCancel, fsm.StateEqual(h.fsm, state.FilterState), th.TextEqual("Отмена"))
	router.Handle(h.handleValue, fsm.StateEqual(h.fsm, state.FilterState), th.Not(th.AnyCommand()))
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

	if stateErr := h.fsm.Set(ctx, msg.Chat.ID, msg.From.ID, state.FilterState); stateErr != nil {
		return fmt.Errorf("set fsm state: %w", stateErr)
	}

	markup := tu.Keyboard(
		tu.KeyboardRow(tu.KeyboardButton("Отмена")),
	)
	markup.ResizeKeyboard = true
	markup.OneTimeKeyboard = true
	if sendErr := h.reply.SendWithKeyboard(ctx, msg.Chat.ID, promptBase+filterText, markup); sendErr != nil {
		return fmt.Errorf("send filter prompt: %w", sendErr)
	}
	return nil
}

// handleCancel clears the FSM state and reports the current subscription.
func (h *Handler) handleCancel(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}

	if err := h.fsm.Clear(ctx, msg.Chat.ID, msg.From.ID); err != nil {
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
	if sendErr := h.reply.SendWithKeyboard(ctx, msg.Chat.ID, text, tu.ReplyKeyboardRemove()); sendErr != nil {
		return fmt.Errorf("send filter cancellation: %w", sendErr)
	}
	return nil
}

// handleValue normalizes the street input and either subscribes, asks for
// confirmation, or reports a no-match. A handled result means the shared
// subscription flow already replied and the flow must stop (Python parity).
func (h *Handler) handleValue(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}

	parsed, handled, err := h.subscribe.ParseAndSubscribe(ctx, msg.Chat.ID, msg.From.ID, msg.Text)
	if err != nil {
		return fmt.Errorf("parse and subscribe: %w", err)
	}
	if handled {
		return nil
	}

	if clearErr := h.fsm.Clear(ctx, msg.Chat.ID, msg.From.ID); clearErr != nil {
		return fmt.Errorf("clear fsm state: %w", clearErr)
	}
	h.logger.Info("user subscribed via filter", zap.Int64("user_id", msg.From.ID), zap.String("street", parsed.Name))
	if sendErr := h.reply.SendWithKeyboard(
		ctx,
		msg.Chat.ID,
		"Создана подписка: "+parsed.Name,
		tu.ReplyKeyboardRemove(),
	); sendErr != nil {
		return fmt.Errorf("send filter subscription: %w", sendErr)
	}
	return nil
}
