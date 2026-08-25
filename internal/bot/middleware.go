package bot

import (
	"errors"

	boterr "github.com/005-bot/tg-bot-go/internal/bot/errors"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"
)

// ErrorMiddleware is a telegohandler middleware that maps handler errors to
// verbatim Russian user messages and replies to the update's chat. It always
// returns nil so a handled error is consumed exactly once.
func ErrorMiddleware(reply *Reply, logger *zap.Logger) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		if err := ctx.Next(update); err != nil {
			logger.Error("bot update handling failed",
				zap.Int("update_id", update.UpdateID), zap.Error(err))
			if update.Message != nil {
				if sendErr := reply.Send(ctx.Context(), update.Message.Chat.ID, MapError(err)); sendErr != nil {
					logger.Error("failed to send error reply", zap.Error(sendErr))
				}
			}
		}
		return nil
	}
}

// MapError maps an error to the user-facing Russian message:
//   - UserInputError: "⚠️ Ошибка ввода: {err}"
//   - APIError: "🔧 Временная проблема с сервисом, пожалуйста, попробуйте позже"
//   - any other error: "🚨 Системная ошибка - наша команда уведомлена"
func MapError(err error) string {
	var userInput boterr.UserInputError
	if errors.As(err, &userInput) {
		return "⚠️ Ошибка ввода: " + err.Error()
	}
	var api boterr.APIError
	if errors.As(err, &api) {
		return "🔧 Временная проблема с сервисом, пожалуйста, попробуйте позже"
	}
	return "🚨 Системная ошибка - наша команда уведомлена"
}
