// Package commands defines CLI commands for the application.
//
// Each command lives in its own sub-package and exposes a Command constructor
// that returns a [*cli.Command]. The commands package aggregates them for the
// root CLI shell in internal/app.go.
package commands

import (
	"github.com/005-bot/tg-bot-go/internal/commands/serve"
	"github.com/005-bot/tg-bot-go/internal/commands/webhook"
	"github.com/go-core-fx/healthfx"
	"github.com/urfave/cli/v3"
)

// Commands returns all available CLI commands. The serve command is aliased
// to run for parity with the Python CLI (tg-bot/app/__main__.py).
func Commands(version healthfx.Version) []*cli.Command {
	return []*cli.Command{
		serve.Command(version),
		webhook.Command(),
	}
}
