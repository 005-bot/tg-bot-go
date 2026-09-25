package webhook

import (
	"context"

	"github.com/go-core-fx/fiberfx/handler"
	"github.com/go-core-fx/telegofx"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// errorKey is the JSON key of the error description in webhook responses.
const errorKey = "error"

// controller serves the Telegram webhook endpoint. The route is
// registered only in webhook mode; in polling mode Telegram never sends
// updates to this service.
type controller struct {
	feeder telegofx.WebhookHandler

	logger *zap.Logger
}

// NewHandler creates the webhook controller for the configured
// ingestion mode.
func NewHandler(feeder telegofx.WebhookHandler, logger *zap.Logger) handler.Handler {
	return &controller{feeder: feeder, logger: logger}
}

// Register adds the POST {path} route when webhook mode is enabled.
func (w *controller) Register(r fiber.Router) {
	r.Post("/webhook", w.handle)
}

// handle decodes the Telegram update and pushes it into the ingestion
// pipeline. The update is processed after the request returns, so the request
// context is detached (telego webhook guidance). A full ingestion buffer
// answers 503 so Telegram retries the delivery.
func (w *controller) handle(c *fiber.Ctx) error {
	if err := w.feeder(context.WithoutCancel(c.Context()), c.Body()); err != nil {
		w.logger.Warn("webhook update decode failed", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{errorKey: "failed to decode update"})
	}

	return c.JSON(fiber.Map{"ok": true})
}
