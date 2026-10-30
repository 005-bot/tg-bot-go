# Changelog

All notable changes to this project are documented in this file.

## [Unreleased]

Initial implementation of the Go port of the Python bot (`005-bot/tg-bot`).

### New Features

#### Telegram bot

- **Street subscriptions** — `/start` subscribes the user and accepts an optional deep-link payload with a street; `/filter` picks a street filter through an interactive conversation that normalizes the typed name with the address parser and confirms low-confidence matches
- **Subscription management** — `/stop` unsubscribes, `/help` prints the command list, `/feedback` forwards a message to the bot admin (disabled while `ADMIN__TELEGRAM_ID` is `0`)
- **Conversation state** — the multi-step `/filter` and `/feedback` flows keep their state in Redis

#### Outage notifications

- **Outage broadcasts** — outage messages published on the Redis pub/sub channel are formatted and sent to every subscriber whose street filter matches

#### Service

- **Webhook delivery mode** — run with `TELEGRAM__MODE=webhook` and register the public URL:
  ```bash
  tg-bot-go set-webhook https://example.com/api/v1/webhook
  ```
  Allowed updates are limited to `message`. The webhook route performs no request authentication, so keep the server behind a reverse proxy; polling remains the default mode
- **HTTP endpoints** — `GET /health` plus the `/health/live`, `/health/ready` and `/health/startup` probes, `GET /metrics` (Prometheus), `GET /api/v1/docs` (OpenAPI UI) and `POST /api/v1/webhook`
- **Layered configuration** — environment variables or a `.env` file, with an optional `CONFIG_PATH` YAML file as the base layer; `STREETS_DB__PATH` drops in a newer streets database without rebuilding the app
