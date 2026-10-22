# tg-bot-go — AGENTS.md

Telegram bot that subscribes residents to utility outage notifications for
their street. Go port of the Python bot (`005-bot/tg-bot`), it mirrors the
Python behaviour command by command.

## Commands

- `make deps` — go mod download
- `make gen` — go generate ./... (swag init)
- `make fmt` — golangci-lint fmt (goimports, golines 120, swaggo formatter)
- `make lint` — golangci-lint run --timeout=5m (very strict config)
- `make test` — go test -race -shuffle=on -count=1 -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...
- `make coverage` — test + go tool cover text + HTML
- `make build` — binary to bin/$(BINARY_NAME)
- `make air` — live reload (requires air, sets TZ=UTC DEBUG=1)
- `make swagger` — standalone swag fmt + swag init
- `make benchmark` — go test -run=^$ -bench=. -benchmem ./...
- `make release` — goreleaser snapshot

## Architecture

- **Entrypoint**: main.go — swag //go:generate directive; version injected via ldflags (appVersion, appBuildDate, appReleaseID)
- **CLI**: urfave/cli/v3 in internal/app.go — CLI shell with DefaultCommand "serve"; commands in internal/commands/{serve,webhook}/
  - `serve` (alias `run`) starts the HTTP server, the bot and the background workers
  - `set-webhook <url>` registers the webhook URL with Telegram (allowed updates limited to "message")
- **DI**: Fx graph in internal/commands/serve/serve.go — core modules (logger, fiberfx, healthfx, redisfx, telegofx, validatorfx) then the app modules (config, server, bot, storage, state, listener, notifier)
- **Config**: go-core-fx/config — env vars with `__` nesting (`HTTP__ADDRESS`, `TELEGRAM__TOKEN`, `ADMIN__TELEGRAM_ID`) plus optional YAML via `CONFIG_PATH`. `.env.example` documents the full surface and must stay in sync with `internal/config/config.go`
- **HTTP**: Fiber at 127.0.0.1:3000 — health endpoints at `/health*`, Prometheus at `/metrics`, OpenAPI UI at `/api/v1/docs`, API routes under `/api/v1`, validation middleware at group level
- **Bot**: telego + telegofx (Telegram), proxy set via fasthttpproxy.FasthttpProxyHTTPDialer()
- **Street subscription**: `internal/bot/subscribe` — the shared parse → subscribe-or-prompt flow used by `/start` (deep-link payload) and `/filter` (FSM conversation); it owns the no-match and confirmation prompts
- **Redis**: redisfx — three uses: subscriptions (`internal/storage`), conversation state (`internal/state`), outage pub/sub on `{prefix}:outages` (`internal/listener`)
- **Address parser**: address-parser-go with the embedded streets.db; the temporary copy is removed on shutdown
- **Outage delivery**: listener decodes the outage JSON → notifier worker formats it and sends it to every subscriber whose street filter matches

## Bot commands

| Command | Handler | Behaviour |
|---|---|---|
| `/start` | `internal/bot/handlers/start` | optional deep-link payload, then subscribes the user to a parsed street |
| `/stop` | `internal/bot/handlers/stop` | unsubscribes the user, leaves the FSM state alone (Python parity) |
| `/help` | `internal/bot/handlers/help` | prints the command list |
| `/filter` | `internal/bot/handlers/filter` | picks and confirms a street filter (FSM conversation) |
| `/feedback` | `internal/bot/handlers/feedback` | collects a message and forwards it to the admin (FSM conversation) |

Replies go out in Markdown (`internal/bot/reply`), outage broadcasts in HTML
(`internal/notifier`).

## Module Conventions

- Each package exposes a `Module()` fx.Option; modules with background work register a runnable via `fxutil.RegisterRunnable[*T]()`
- Handlers are registered through group tags: `fx.Annotate(new.New, fx.ResultTags(`group:"handlers"`))`
- The bot error middleware is registered before the command handlers, because telegohandler stops at the first matching route
- Internal-only deps use fx.Private; the listener service is deliberately not private, the notifier module consumes `Service.Events`
- Per-module named logger: `logger.WithNamedLogger("name")`
- The config module maps the raw Config struct to the sub-configs of fiberfx, telegofx, redisfx, openapi, storage, state and listener
- Telegram helpers: use `github.com/mymmrac/telego/telegoutil` (imported as `tu`, like `th` for telegohandler) instead of hand-written telego literals — `tu.ParseCommandPayload` for command arguments, `tu.Keyboard`/`tu.KeyboardRow`/`tu.KeyboardButton` for reply keyboards, `tu.ReplyKeyboardRemove` for keyboard removal, `tu.ID`/`tu.Message`/`tu.Webhook` for chat IDs and API params. telegoutil sets no keyboard flags, so `ResizeKeyboard`/`OneTimeKeyboard` are still set explicitly

## Code Generation

- Swagger docs: go generate ./... (swag init --parseDependency --outputTypes go -g ./main.go -o ./internal/server/docs)
- Output: internal/server/docs/docs.go — DO NOT EDIT
- Live reload config: .air.toml

## Linting & CI

- golangci-lint v2, ~70 linters (exhaustruct, cyclop, gochecknoglobals, mnd, etc.)
- golines max length: 120
- Format + lint + test + coverage in that order
- CI: lint + test on push/PR to master in .github/workflows/go.yml
- CI: goreleaser snapshot on PR; release on v* tags
- E2E CI job is disabled (if: false)
- Stale issues/PRs closed after 14d inactivity

## Testing

- Test packages live in `package foo_test`; the fakes (Telegram API server, miniredis) are wired in `internal/bot/handlers/harness_test.go`
- Test flags: -race -shuffle=on -count=1 (disables cache, randomizes order)
- The handler tests wait on conditions with a 60 s budget: the address parser matches a street by running a Levenshtein and an LCS comparison against every street in its database, and that scan takes seconds under the race detector and the coverage instrumentation

## Python parity

The Go service is a port of the Python bot (`005-bot/tg-bot`). Most behaviour is
kept byte for byte, and the differences are deliberate and documented in the
package doc of the code that differs:

- `internal/notifier` — the street block is truncated before it is html-quoted (Python quotes first and can cut an entity in half), the suffix is uppercased before it is quoted (Python uppercases the quoted text and emits `&LT;`, which Telegram rejects) and the reason is bounded to Telegram's 4096 character limit (Python applies no bound)
- `internal/storage` — the `Filter` JSON encoding must match the Python pydantic model byte for byte: `{"street": "..."}` or `{"street": null}`
- `internal/state` — the state names are the aiogram StatesGroup names as stored in Redis

## Deployment notes

- The HTTP server binds to 127.0.0.1 by default; put it behind a reverse proxy to expose it
- The webhook route `POST /api/v1/webhook` performs **no request authentication**. telegofx leaves the `X-Telegram-Bot-Api-Secret-Token` check to the application, and the check is not implemented yet, so a caller that reaches the route can submit forged updates. Keep the server behind a reverse proxy that only forwards Telegram traffic, and prefer the default polling mode
- In polling mode the webhook route answers 500 (telegofx `ErrHandlerNotReady`), because no webhook handler is registered
- The default update mode is polling; webhook mode needs `TELEGRAM__MODE=webhook` and a `set-webhook <url>` registration
