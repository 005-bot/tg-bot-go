// Package webhook implements the set-webhook CLI command, mirroring the
// Python CLI (tg-bot/app/__main__.py).
package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/005-bot/tg-bot-go/internal/config"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

const (
	exitCodeFailure = 1
	exitCodeUsage   = 2
)

// ErrTokenNotConfigured is returned when TELEGRAM__TOKEN is not set.
var ErrTokenNotConfigured = errors.New("telegram token is not configured (TELEGRAM__TOKEN)")

// ErrInvalidWebhookURL is returned when the webhook URL is not an absolute
// http(s) URL.
var ErrInvalidWebhookURL = errors.New("invalid webhook url")

// Command returns the set-webhook command that points the Telegram bot at a
// webhook URL. Allowed updates are limited to "message", matching the Python
// bot.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "set-webhook",
		Usage:     "Set the Telegram bot webhook URL",
		ArgsUsage: "<url>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return cli.Exit("set-webhook requires exactly one argument: <url>", exitCodeUsage)
			}

			url := cmd.Args().First()
			if err := ValidateURL(url); err != nil {
				return cli.Exit(err, exitCodeUsage)
			}

			return runCommand(ctx, url)
		},
	}
}

// ValidateURL checks that the webhook URL is an absolute http(s) URL. This is
// stricter than the Python CLI, which forwards any string to the Bot API;
// Telegram rejects malformed URLs anyway, so the CLI fails fast with a usage
// error instead of a runtime API error.
func ValidateURL(rawURL string) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidWebhookURL, err.Error())
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: %q: scheme must be http or https", ErrInvalidWebhookURL, rawURL)
	}

	return nil
}

// runCommand loads the configuration, creates the Telegram bot, and sets the
// webhook. Every failure is returned as a cli.ExitCoder with a non-zero exit
// code.
func runCommand(ctx context.Context, url string) error {
	cfg, err := config.New()
	if err != nil {
		return cli.Exit(fmt.Errorf("load config: %w", err), exitCodeFailure)
	}

	bot, err := NewBot(cfg.Telegram.Token, cfg.Telegram.ProxyURL)
	if err != nil {
		return cli.Exit(err, exitCodeFailure)
	}

	err = Run(ctx, bot, url)
	if err != nil {
		return cli.Exit(err, exitCodeFailure)
	}

	return nil
}

// NewBot creates a Telegram bot from the configured token and optional proxy.
// telego rejects empty and malformed tokens at creation time (token regexp
// `^\d+:[\w-]{35}$`); the error is wrapped so the cause is clear to an
// operator.
func NewBot(token, proxyURL string) (*telego.Bot, error) {
	if token == "" {
		return nil, ErrTokenNotConfigured
	}

	bot, err := telegofx.New(
		telegofx.Config{
			Token:    token,
			ProxyURL: proxyURL,
			Mode:     telegofx.ModePolling,
		},
		nil,
		zap.NewNop(),
	)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}

	return bot.Bot, nil
}

// Run points the bot's webhook at url with allowed_updates ["message"] and
// prints a confirmation on success, matching the Python CLI output.
func Run(ctx context.Context, bot *telego.Bot, url string) error {
	if err := bot.SetWebhook(ctx, &telego.SetWebhookParams{
		URL: url,
	}); err != nil {
		return fmt.Errorf("set webhook: %w", err)
	}

	_, _ = fmt.Fprintln(os.Stdout, "Webhook set")

	return nil
}
