# go-project-template — AGENTS.md

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

## Architecture
- **Entrypoint**: main.go — swag //go:generate directive; version injected via ldflags (appVersion, appBuildDate, appReleaseID)
- **CLI**: urfave/cli/v3 in internal/app.go — CLI shell with DefaultCommand "serve"; commands in internal/commands/{serve,example}/
- **DI**: Fx graphs live in each command (internal/commands/serve/serve.go, internal/commands/example/example.go)
- **Config**: go-core-fx/config — env vars + optional YAML via CONFIG_PATH env var
- **HTTP**: Fiber at 127.0.0.1:3000, routes under /api/v1, validation middleware at group level
- **Bot**: telego + telegofx (Telegram), proxy set via fasthttpproxy.FasthttpProxyHTTPDialer()
- **DB**: MySQL/MariaDB via Bun (mysqldialect), Goose migrations in internal/db/migrations/ (//go:embed *.sql)
- **Metrics**: Prometheus via fiberfx (auto), per-module counters via promauto

## Module Conventions
- Each package exposes a Module(...) fx.Option (withRun bool for modules with background work)
- Handlers registered via group tags: Provide(..., fx.ResultTags(`group:"handlers"`))
- Internal-only deps use fx.Private
- Services with Run(ctx) use `fxutil.RegisterRunnable[*T]()` — conditionally via `withRun` bool
- Per-module named logger: logger.WithNamedLogger("name")
- Config module maps raw Config struct to sub-configs for fiberfx, telegofx, sqlfx, openapi, example

## Code Generation
- Swagger docs: go generate ./... (swag init --parseDependency --outputTypes go -g ./main.go -o ./internal/server/docs)
- Output: internal/server/docs/docs.go — DO NOT EDIT
- Live reload config: .air.toml

## Linting & CI
- golangci-lint v2, ~70 linters (exhaustruct, cyclop, gochecknoglobals, etc.)
- golines max length: 120
- Format + lint + test + coverage in that order
- CI: lint + test on push/PR to master in .github/workflows/go.yml
- CI: goreleaser snapshot on PR; release on v* tags
- E2E CI job is disabled (if: false)
- Stale issues/PRs closed after 14d inactivity

## Testing
- No tests exist yet — add _test.go as needed
- Test flags: -race -shuffle=on -count=1 (disables cache, randomizes order)

## Build & Release
- GoReleaser v2 for linux/windows/darwin, CGO_ENABLED=0
- Go 1.25+
- Docker images pushed to ghcr.io on PR and release

## Development Notes (Wave 1)
- telego validates bot token format at creation with regex `^\d+:[\w-]{35}$` (exactly 35 [\w-] chars after colon); boot/smoke checks must use a format-valid dummy token (e.g. 123456:abcdefghijklmnopqrstuvwxyzABCDEFGHI) or the fx graph fails before the server binds
- koanf env provider (go-core-fx/config) includes empty-valued env vars and unmarshals with WeaklyTypedInput=true, so ADMIN__TELEGRAM_ID="" decodes to 0 without error (Python crashed on int("") - Go is more robust)
- go-core-fx/healthfx auto-registers /health (200 JSON status) via healthfx.Module() + health.NewHandler - no route code needed
- DetectResourceType uses substring matching and fails on declined RU forms (e.g. "теплоснабжения" != "теплоснабжение"): consumers must match base forms or extend the matcher
- encoding/json emits empty []time.Time as null and non-omitempty fields as null; apis-go Outage schema pins streets:null and period:null when nil
- streets.db extraction rule: byte-copy + MD5 verification (10072cee7eb84361125cbdaf76559093) is the parity gate; regenerate never, copy always
- Empty/punctuation street input passes the Unicode cleanName step (regex yields empty) and surfaces as ErrNoMatch from Normalize, not at the clean step
- go mod tidy after dropping bun/goose/mysql wiring removes the unused module deps automatically; a zero-dependency module keeps an empty tracked go.sum
- Template Makefile BINARY_NAME=basename($PWD) and .goreleaser .ProjectName templating auto-adapt to a renamed repo - no edits needed when repurposing the template
- go-core-fx/redisfx resolves via go mod tidy to v0.0.0-20251029094515-c9e3d82dfaa2 (same pin as monitor-go)
- Wave-1 boundary: tg-bot-go consumes neither apis-go nor address-parser-go yet (no replace directives); library consumption and redisfx runtime wiring land in wave 2 (TASK-005/006)

## Development Notes (Wave 2)
- Filter JSON parity with pydantic requires SetEscapeHTML(false) - Go's default JSON encoding escapes <,>,&
- Workspace members (tagless libs) resolve via go.work WITHOUT require lines; a versioned require of a tagless member poisons the workspace build (go fetches member go.mod at that version; unknown revision)
- Dangling requires of removed-tag workspace libs in ANY member's go.mod poison the whole go.work graph for all modules
- Go toolchain fetches workspace-member go.mod/zip at the required version even in workspace mode; `use` only overrides package source - tagless shared libs need cache-planted .mod/.zip + go.work.sum entries (machine-local; CI portability is the TASK-014 open question)
- apis-go domain package is at the module ROOT: import path github.com/005-bot/apis-go (package name domain), NOT /domain
- go-redis v9 PubSub.ReceiveMessage/Receive block on raw conn reads; ctx passed to Subscribe is NOT bound to connection lifetime - cancellation needs a goroutine+select wrapper
- fxutil.RegisterRunnable[T]() returns a void callback for fx.Invoke; using it in fx.Provide fails with 'must provide at least one non-error type'
- miniredis v2.38 lacks Restart(); simulate a Redis restart by Close() + StartAddr(sameAddr)
- golangci-lint v2.13 fails in workspace mode (directory prefix ... does not contain modules listed in go.work); use per-package runs or GOWORK=off + CI-mode
- go work sync silently drops requires on workspace members while they are unresolvable
- go-redis Client.Subscribe never returns an error (discarded inside); subscription failures surface only via Receive/ReceiveMessage

## Development Notes (Wave 3)
- telego v1.11.2 telegohandler.Predicate is ctx-first: func(ctx context.Context, update telego.Update) bool
- telego v1.11.2 has no th.Command; command predicates are th.AnyCommand/th.CommandEqual using strict regexp requiring a command word after '/'
- fx v1.24 has no implicit interface binding: expose concrete types via explicit adapter providers (e.g. func(*Reply) handler.Sender)
- fx.Private constructor results are invisible to providers in other fx.Module scopes; cross-module consumers force removing fx.Private
- fx.New executes fx.Invoke during construction: blocking network calls in an invoke prevent the server from binding; startup Telegram calls (setMyCommands) must be async
- telegofx.Bot.Updates() is receive-only; tests create their own channel + th.NewBotHandler(bot.Bot, ch) wrapped as &telegofx.Router{BotHandler: bh}
- apis-go/format exposes DateRU/Dates (not FormatDateRU); verify actual shared API names before wrapping
- strings.NewReplacer replaces in one pass: escaped output (&lt;) is never re-processed - matches aiogram html.quote single-pass semantics incl. double-escaping literal entities
- aiogram RedisStorage parity: keys {prefix}:fsm:{chat}:{user}:state|data, redis TTL = -1 (no expiry); miniredis asserts both
- golangci-lint v2 stale parallel-run lock at $TMPDIR/golangci-lint.lock survives pkill; subsequent runs hang until the lock file is removed
- exhaustruct (repo config) requires full struct literals for telego non-Params types (ReplyKeyboardMarkup, KeyboardButton, BotCommand, ChatID); test files are excluded
- User added real tags v0.0.1 in apis-go and address-parser-go (replacing the tagless state) and updated monitor-go + tg-bot-go go.mod requires to v0.0.1 - v0.0.1 requires are now resolvable locally
