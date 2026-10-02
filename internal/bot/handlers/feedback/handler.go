// Package feedback implements the /feedback command and its FSM
// conversation: the user writes a message that is forwarded to the admin
// chat, or cancels with the "Ничего" keyboard button (aiogram feedback.py
// parity).
package feedback

import (
	"fmt"

	"github.com/005-bot/tg-bot-go/internal/bot/handler"
	"github.com/005-bot/tg-bot-go/internal/fsm"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"
)

// Handler handles the /feedback command and its FSM conversation.
type Handler struct {
	logger      *zap.Logger
	fsm         *fsm.Store
	reply       handler.Sender
	notificator handler.Notificator
}

// New creates the /feedback handler.
func New(
	logger *zap.Logger,
	store *fsm.Store,
	reply handler.Sender,
	notificator handler.Notificator,
) handler.Handler {
	return &Handler{
		logger:      logger,
		fsm:         store,
		reply:       reply,
		notificator: notificator,
	}
}

// Register registers the /feedback command route and the FSM cancel/value
// routes. The value handler excludes '/'-prefixed text, so commands always
// reach the command handlers regardless of the active state; the cancel
// route must precede the value route ("Ничего" is also plain text).
func (h *Handler) Register(router *telegofx.Router) {
	router.Handle(h.handleCommand, th.CommandEqual("feedback"), th.AnyMessageWithFrom())
	router.Handle(h.handleCancel, fsm.StateFilter(h.fsm, fsm.FeedbackState), th.TextEqual("Ничего"))
	router.Handle(h.handleValue, fsm.StateFilter(h.fsm, fsm.FeedbackState), fsm.TextNotCommand())
}

func (h *Handler) handleCommand(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}

	if err := h.fsm.SetState(ctx, msg.Chat.ID, msg.From.ID, fsm.FeedbackState); err != nil {
		return fmt.Errorf("set fsm state: %w", err)
	}

	markup := &telego.ReplyKeyboardMarkup{
		Keyboard: [][]telego.KeyboardButton{
			{
				{
					Text:              "Ничего",
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
	h.logger.Info("user started feedback", zap.Int64("user_id", msg.From.ID))
	if err := h.reply.SendWithKeyboard(ctx, msg.Chat.ID, "Что бы Вы хотели нам сказать?", markup); err != nil {
		return fmt.Errorf("send feedback prompt: %w", err)
	}
	return nil
}

// handleCancel clears the FSM state without forwarding anything.
func (h *Handler) handleCancel(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}

	if err := h.fsm.ClearState(ctx, msg.Chat.ID, msg.From.ID); err != nil {
		return fmt.Errorf("clear fsm state: %w", err)
	}
	h.logger.Info("user canceled feedback", zap.Int64("user_id", msg.From.ID))
	if err := h.reply.SendWithKeyboard(ctx, msg.Chat.ID, "Отзыв не отправлен", &telego.ReplyKeyboardRemove{
		RemoveKeyboard: true,
		Selective:      false,
	}); err != nil {
		return fmt.Errorf("send feedback cancellation: %w", err)
	}
	return nil
}

// handleValue forwards the message to the admin chat and closes the
// conversation. Forwarding is a no-op when no admin is configured.
func (h *Handler) handleValue(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}

	if err := h.notificator.Feedback(ctx, msg); err != nil {
		return fmt.Errorf("forward feedback: %w", err)
	}

	if err := h.fsm.ClearState(ctx, msg.Chat.ID, msg.From.ID); err != nil {
		return fmt.Errorf("clear fsm state: %w", err)
	}
	h.logger.Info("user sent feedback", zap.Int64("user_id", msg.From.ID))
	if err := h.reply.SendWithKeyboard(ctx, msg.Chat.ID, "Спасибо за отзыв!", &telego.ReplyKeyboardRemove{
		RemoveKeyboard: true,
		Selective:      false,
	}); err != nil {
		return fmt.Errorf("send feedback confirmation: %w", err)
	}
	return nil
}
