// Package stop implements the /stop command: unsubscribes the user from
// outage notifications without touching the FSM state (Python parity).
package stop

import (
	"fmt"
	"strconv"

	"github.com/005-bot/tg-bot-go/internal/bot/handler"
	"github.com/005-bot/tg-bot-go/internal/storage"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"
)

// Handler handles the /stop command.
type Handler struct {
	logger  *zap.Logger
	storage *storage.Service
	reply   handler.Sender
}

// New creates the /stop handler.
func New(logger *zap.Logger, svc *storage.Service, reply handler.Sender) handler.Handler {
	return &Handler{logger: logger, storage: svc, reply: reply}
}

// Register registers the /stop route on the router.
func (h *Handler) Register(router *telegofx.Router) {
	router.Handle(h.handleStop, th.CommandEqual("stop"), th.AnyMessageWithFrom())
}

func (h *Handler) handleStop(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}
	userID := strconv.FormatInt(msg.From.ID, 10)
	if err := h.storage.Unsubscribe(ctx, userID); err != nil {
		return fmt.Errorf("unsubscribe user %s: %w", userID, err)
	}
	return h.reply.Send(ctx, msg.Chat.ID, "🔕 Вы *отписались* от уведомлений об отключениях")
}
