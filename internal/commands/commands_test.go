package commands_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/005-bot/tg-bot-go/internal/commands"
	"github.com/go-core-fx/healthfx"
	"github.com/urfave/cli/v3"
)

const (
	testCommandWebhook = "set-webhook"
	testWebhookURL     = "https://example.com/hook"
	testTelegramToken  = "123456:abcdefghijklmnopqrstuvwxyzABCDEFGHI"
)

func testVersion() healthfx.Version {
	return healthfx.Version{
		Version:   "test",
		ReleaseID: 1,
		BuildDate: "2026-01-01",
		GitCommit: "test",
		GoVersion: "go1.25",
	}
}

func findCommand(t *testing.T, cmds []*cli.Command, name string) *cli.Command {
	t.Helper()

	for _, cmd := range cmds {
		if cmd.Name == name {
			return cmd
		}
	}

	t.Fatalf("command %q not found in %v", name, cmds)
	return nil
}

func TestCommandsServeHasRunAlias(t *testing.T) {
	serve := findCommand(t, commands.Commands(testVersion()), "serve")
	if !slices.Contains(serve.Aliases, "run") {
		t.Fatalf("serve aliases = %v, want alias %q", serve.Aliases, "run")
	}
}

func TestCommandsHasSetWebhook(t *testing.T) {
	findCommand(t, commands.Commands(testVersion()), testCommandWebhook)
}

func TestCommandsNoMigrate(t *testing.T) {
	// Structural check that no migrate command exists. The wave-5 gate also
	// verified via grep that no migrate command, migration flag, or goose
	// wiring exists anywhere in the module (internal/commands, serve.go,
	// go.mod) - grep stays the authoritative check for out-of-tree drift.
	for _, cmd := range commands.Commands(testVersion()) {
		if cmd.Name == "migrate" {
			t.Fatal("migrate command must not exist (user override: no legacy data formats)")
		}
	}
}

// runRoot runs the CLI root command with the same shape as internal/app.go
// and returns the dispatched error without exiting the test process.
func runRoot(t *testing.T, args ...string) error {
	t.Helper()

	root := &cli.Command{
		Name:           "tg-bot-go",
		Usage:          "Telegram bot service",
		DefaultCommand: "serve",
		Commands:       commands.Commands(testVersion()),
		ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {
			// Keep cli from calling os.Exit; the test asserts on the error.
		},
	}

	return root.Run(context.Background(), args)
}

func TestRootSetWebhookMissingArg(t *testing.T) {
	err := runRoot(t, "tg-bot-go", testCommandWebhook)

	var exitErr cli.ExitCoder
	if !errors.As(err, &exitErr) {
		t.Fatalf("want cli.ExitCoder, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2", exitErr.ExitCode())
	}
	if !strings.Contains(exitErr.Error(), "exactly one argument") {
		t.Fatalf("error = %q, want missing-argument message", exitErr.Error())
	}
}

func TestRootSetWebhookInvalidToken(t *testing.T) {
	t.Setenv("TELEGRAM__TOKEN", "not-a-token")

	err := runRoot(t, "tg-bot-go", testCommandWebhook, testWebhookURL)

	var exitErr cli.ExitCoder
	if !errors.As(err, &exitErr) {
		t.Fatalf("want cli.ExitCoder, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want 1", exitErr.ExitCode())
	}
	if !strings.Contains(exitErr.Error(), "token") {
		t.Fatalf("error = %q, want token context", exitErr.Error())
	}
}

func TestRootSetWebhookInvalidProxy(t *testing.T) {
	t.Setenv("TELEGRAM__TOKEN", testTelegramToken)
	t.Setenv("TELEGRAM__PROXY_URL", "http://127.0.0.1:8080")

	err := runRoot(t, "tg-bot-go", testCommandWebhook, testWebhookURL)

	var exitErr cli.ExitCoder
	if !errors.As(err, &exitErr) {
		t.Fatalf("want cli.ExitCoder, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want 1", exitErr.ExitCode())
	}
	if !strings.Contains(exitErr.Error(), "proxy") {
		t.Fatalf("error = %q, want proxy context", exitErr.Error())
	}
}

func TestRootSetWebhookEmptyToken(t *testing.T) {
	t.Setenv("TELEGRAM__TOKEN", "")

	err := runRoot(t, "tg-bot-go", testCommandWebhook, testWebhookURL)

	var exitErr cli.ExitCoder
	if !errors.As(err, &exitErr) {
		t.Fatalf("want cli.ExitCoder, got %T: %v", err, err)
	}
	if !strings.Contains(exitErr.Error(), "not configured") {
		t.Fatalf("error = %q, want not-configured message", exitErr.Error())
	}
}

func TestRootSetWebhookInvalidURL(t *testing.T) {
	// URL validation happens before config/token loading, so no token env is
	// needed: the usage error must win regardless of the runtime config.
	err := runRoot(t, "tg-bot-go", testCommandWebhook, "example.com/hook")

	var exitErr cli.ExitCoder
	if !errors.As(err, &exitErr) {
		t.Fatalf("want cli.ExitCoder, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2 (usage error)", exitErr.ExitCode())
	}
	if !strings.Contains(exitErr.Error(), "invalid webhook url") {
		t.Fatalf("error = %q, want URL-validation message", exitErr.Error())
	}
}
