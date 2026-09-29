package server_test

import (
	"context"
	"testing"

	"github.com/005-bot/tg-bot-go/internal/server"
	"github.com/go-core-fx/fiberfx"
	"github.com/go-core-fx/fiberfx/openapi"
	"github.com/go-core-fx/healthfx"
	"github.com/go-core-fx/logger"
	"github.com/go-core-fx/telegofx"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/fx"
)

// TestModuleRegistersWebhookRoute guards the fx group tag on
// webhook.NewHandler: an untagged result is invisible to the
// group:"handlers" invoke, so the route silently disappears from the app.
func TestModuleRegistersWebhookRoute(t *testing.T) {
	var routes []fiber.Route

	app := fx.New(
		logger.Module(),
		logger.WithFxDefaultLogger(),
		fiberfx.Module(),
		healthfx.Module(),
		fx.Supply(
			fiberfx.Config{Address: "127.0.0.1:0"},
			healthfx.Version{Version: "test"},
			openapi.Config{},
			telegofx.WebhookHandler(func(context.Context, []byte) error { return nil }),
		),
		server.Module(),
		fx.Invoke(func(a *fiber.App) { routes = a.GetRoutes() }),
	)
	if err := app.Err(); err != nil {
		t.Fatalf("fx graph: %v", err)
	}

	for _, r := range routes {
		if r.Method == fiber.MethodPost && r.Path == "/api/v1/webhook" {
			return
		}
	}

	t.Errorf("POST /api/v1/webhook not registered; routes: %v", routePaths(routes))
}

func routePaths(routes []fiber.Route) []string {
	paths := make([]string, 0, len(routes))
	for _, r := range routes {
		paths = append(paths, r.Method+" "+r.Path)
	}

	return paths
}
