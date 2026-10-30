package notifier

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	apidev "github.com/005-bot/apis-go"
	"github.com/005-bot/tg-bot-go/internal/listener"
	"github.com/005-bot/tg-bot-go/internal/storage"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoapi"
	"go.uber.org/zap"
)

// Worker broadcasts outage events to the subscribed users, mirroring the
// Python listen() loop (tg-bot/app/bot.py). Each outage is delivered to
// every subscriber whose street filter matches one of the affected streets;
// a nil or empty filter matches everything.
type Worker struct {
	bot        *telego.Bot
	storageSvc *storage.Service
	events     <-chan apidev.Outage

	logger *zap.Logger
}

// NewWorker creates the broadcast worker. events delivers outages as they
// arrive; in the fx graph it is wired to listener.Service.Events.
func NewWorker(
	bot *telegofx.Bot,
	storageSvc *storage.Service,
	listenerSvc *listener.Service,
	logger *zap.Logger,
) *Worker {
	return &Worker{
		bot:        bot.Bot,
		storageSvc: storageSvc,
		events:     listenerSvc.Events,

		logger: logger,
	}
}

// Run consumes outage events until ctx is cancelled. Failures are logged
// and the loop keeps running: the outage listener already retries its own
// connectivity errors, so a per-user send failure must not stop the worker.
func (w *Worker) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case outage, ok := <-w.events:
			if !ok {
				w.logger.Warn("outage events channel closed")
				return nil
			}
			w.broadcast(ctx, outage)
		}
	}
}

// broadcast delivers one outage to every matching subscriber. A 403
// (forbidden) response means the user blocked the bot: the filter is
// removed (Python TelegramForbiddenError parity). Other send errors are
// logged and the batch continues with the next user.
func (w *Worker) broadcast(ctx context.Context, outage apidev.Outage) {
	message := FormatOutage(outage)

	streets := make(map[string]struct{}, len(outage.Details.Streets))
	for _, street := range outage.Details.Streets {
		streets[strings.ToLower(street.Name)] = struct{}{}
	}

	subscribers, err := w.storageSvc.GetSubscribed(ctx)
	if err != nil {
		w.logger.Error("get subscribed users failed", zap.Error(err))
		return
	}

	for userID, filter := range subscribers {
		if !streetMatches(streets, filter) {
			continue
		}

		chatID, perr := strconv.ParseInt(userID, 10, 64)
		if perr != nil {
			w.logger.Warn("skipping subscriber with non-numeric user id",
				zap.String("user_id", userID))
			continue
		}

		if serr := w.sendMessage(ctx, chatID, message); serr != nil {
			var apiErr *telegoapi.Error
			if errors.As(serr, &apiErr) && apiErr.ErrorCode == http.StatusForbidden {
				if uerr := w.storageSvc.Unsubscribe(ctx, userID); uerr != nil {
					w.logger.Error("unsubscribe blocked user failed",
						zap.String("user_id", userID), zap.Error(uerr))
				} else {
					w.logger.Info("user unsubscribed from updates",
						zap.String("user_id", userID))
				}
				continue
			}

			w.logger.Error("send outage message failed",
				zap.String("user_id", userID), zap.Error(serr))
		}
	}
}

// sendMessage delivers the HTML outage message to the chat.
func (w *Worker) sendMessage(ctx context.Context, chatID int64, text string) error {
	if _, err := w.bot.SendMessage(ctx, &telego.SendMessageParams{
		ChatID:    telego.ChatID{ID: chatID, Username: ""},
		Text:      text,
		ParseMode: telego.ModeHTML,
	}); err != nil {
		return fmt.Errorf("send outage message: %w", err)
	}
	return nil
}

// streetMatches reports whether the subscriber's filter covers the outage's
// streets: a nil or empty filter street matches everything (Python falsy
// check); otherwise membership is exact and case-insensitive (Python
// f.street.lower() not in streets), never a substring match.
func streetMatches(streets map[string]struct{}, filter storage.Filter) bool {
	if filter.Street == nil || *filter.Street == "" {
		return true
	}
	_, ok := streets[strings.ToLower(*filter.Street)]
	return ok
}
