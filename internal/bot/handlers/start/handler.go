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
	"github.com/005-bot/tg-bot-go/internal/bot/notificator"
	"github.com/005-bot/tg-bot-go/internal/bot/reply"
	"github.com/005-bot/tg-bot-go/internal/bot/subscribe"
	"github.com/005-bot/tg-bot-go/internal/storage"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"go.uber.org/zap"
)

var errInvalidUTF8 = errors.New("invalid utf-8")

// Handler handles the /start command.
type Handler struct {
	storageSvc     *storage.Service
	subscribeSvc   *subscribe.Service
	replySvc       *reply.Service
	notificatorSvc *notificator.Service

	logger *zap.Logger
}

// New creates the /start handler.
func New(
	storageSvc *storage.Service,
	subscribeSvc *subscribe.Service,
	replySvc *reply.Service,
	notificatorSvc *notificator.Service,
	logger *zap.Logger,
) handler.Handler {
	return &Handler{
		storageSvc:     storageSvc,
		subscribeSvc:   subscribeSvc,
		replySvc:       replySvc,
		notificatorSvc: notificatorSvc,

		logger: logger,
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

	_, _, args := tu.ParseCommandPayload(msg.Text)
	payload, err := decodeStartPayload(args)
	if err != nil {
		if sendErr := h.replySvc.Send(
			ctx,
			msg.Chat.ID,
			"🚨 Системная ошибка - наша команда уведомлена",
		); sendErr != nil {
			return fmt.Errorf("send start error: %w", sendErr)
		}
		return nil
	}

	if err = h.storageSvc.Subscribe(ctx, userID, nil); err != nil {
		return fmt.Errorf("subscribe user %s: %w", userID, err)
	}

	if payload == "" {
		return h.finishStart(ctx, msg, nil)
	}

	match, handled, err := h.subscribeSvc.ParseAndSubscribe(ctx, msg.Chat.ID, msg.From.ID, payload)
	if err != nil {
		return fmt.Errorf("parse and subscribe: %w", err)
	}
	if handled {
		return nil
	}
	return h.finishStart(ctx, msg, match)
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

	if err := h.replySvc.Send(ctx, msg.Chat.ID, text); err != nil {
		return fmt.Errorf("send welcome: %w", err)
	}
	h.logger.Info("user subscribed", zap.Int64("user_id", msg.From.ID))
	if err := h.notificatorSvc.NewUser(ctx, msg.From); err != nil {
		return fmt.Errorf("notify admin: %w", err)
	}
	return nil
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
