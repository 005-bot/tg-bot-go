<a id="readme-top"></a>

<!-- PROJECT SHIELDS -->
[![Contributors][contributors-shield]][contributors-url]
[![Forks][forks-shield]][forks-url]
[![Stargazers][stars-shield]][stars-url]
[![Issues][issues-shield]][issues-url]
[![Apache License][license-shield]][license-url]
[![Go][go-shield]][go-url]

<!-- PROJECT LOGO -->
<br />
<div align="center">
  <a href="https://github.com/005-bot/tg-bot-go">
    <img src="https://raw.githubusercontent.com/golang-samples/gopher-vector/master/gopher.png" alt="Logo" width="120" height="120">
  </a>

<h3 align="center">tg-bot-go</h3>

  <p align="center">
    Telegram bot that subscribes residents to utility outage notifications for their street.
    <br />
    <a href="https://github.com/005-bot/tg-bot-go"><strong>Explore the docs »</strong></a>
    <br />
    <br />
    <a href="https://github.com/005-bot/tg-bot-go/issues">Report Bug</a>
    ·
    <a href="https://github.com/005-bot/tg-bot-go/issues">Request Feature</a>
  </p>
</div>



<!-- TABLE OF CONTENTS -->
- [About The Project](#about-the-project)
  - [Built With](#built-with)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
- [Usage](#usage)
- [Configuration](#configuration)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)
- [Contact](#contact)
- [Acknowledgments](#acknowledgments)



<!-- ABOUT THE PROJECT -->
## About The Project

The bot subscribes a resident to the utility outages of one street and sends an
outage broadcast when the outages API publishes one.

* The resident picks a street with `/start` or `/filter`; the address parser
  matches the typed text against the street database.
* The subscription and the filter are stored in Redis.
* Outages arrive over a Redis pub/sub channel. The listener decodes them, the
  notifier formats the message and sends it to every subscriber whose street
  filter matches.
* `/feedback` forwards a message to the bot admin.

The service is a Go port of the Python bot (`005-bot/tg-bot`) and keeps its
behaviour command by command.

<p align="right">(<a href="#readme-top">back to top</a>)</p>


### Built With

* [![Go][go-shield]][go-url]
* [![Fiber][fiber-shield]][fiber-url]
* [![Fx][fx-shield]][fx-url]
* [![CLI][cli-shield]][cli-url]
* [![Swagger][swagger-shield]][swagger-url]
* [![Telegram][telegram-shield]][telegram-url]
* [![Redis][redis-shield]][redis-url]

<p align="right">(<a href="#readme-top">back to top</a>)</p>



<!-- GETTING STARTED -->
## Getting Started

Follow these steps to run the bot locally.

### Prerequisites

* Go 1.26+
  ```sh
  go version
  ```
* A Redis instance for the subscriptions, the conversation state and the
  outage channel
  ```sh
  docker run -d --name redis -p 6379:6379 redis:7-alpine
  ```
* A Telegram bot token from [@BotFather](https://core.telegram.org/bots)
* `golangci-lint` (optional but recommended)
  ```sh
  golangci-lint version
  ```
* `swag` CLI for docs generation
  ```sh
  go install github.com/swaggo/swag/cmd/swag@latest
  ```

### Installation

1. Clone the repo.
   ```sh
   git clone https://github.com/005-bot/tg-bot-go.git
   cd tg-bot-go
   ```
2. Download dependencies.
   ```sh
   make deps
   ```
3. (Optional) Generate OpenAPI docs.
   ```sh
   make gen
   ```
4. Build the binary.
   ```sh
   make build
   ```

<p align="right">(<a href="#readme-top">back to top</a>)</p>



<!-- USAGE EXAMPLES -->
## Usage

Run the service (default command):

```sh
go run .
```

Or with live reload:

```sh
make air
```

CLI commands:

```sh
go run . --help                     # list commands
go run . serve                      # explicit serve (alias: run)
go run . set-webhook <url>          # register the webhook URL with Telegram
go run . --version                  # print version
```

The default server address is `127.0.0.1:3000`.

Endpoints:

| Endpoint | Purpose |
|---|---|
| `GET /health` | health check |
| `GET /health/live`, `/health/ready`, `/health/startup` | health probes |
| `GET /metrics` | Prometheus metrics |
| `GET /api/v1/docs` | OpenAPI UI |
| `POST /api/v1/webhook` | Telegram webhook receiver |

The webhook route is registered in every mode, but only the webhook mode has a
handler ready to accept a payload; in polling mode the route answers 500.

Quality checks:

```sh
make fmt
make lint
make test
make coverage
```

<p align="right">(<a href="#readme-top">back to top</a>)</p>



<!-- CONFIGURATION -->
## Configuration

Configuration is loaded from environment variables, or from a YAML file when
`CONFIG_PATH` points to one. The `__` separator nests the keys.

| Variable | Default | Purpose |
|---|---|---|
| `TELEGRAM__TOKEN` | — | **Required.** Bot API token; the service fails to start without it |
| `HTTP__ADDRESS` | `127.0.0.1:3000` | HTTP listen address |
| `HTTP__PROXY_HEADER` | `X-Forwarded-For` | client IP header behind a proxy |
| `HTTP__PROXIES` | — | comma-separated trusted proxy addresses |
| `HTTP__OPENAPI__ENABLED` | `false` | serve the OpenAPI UI |
| `HTTP__OPENAPI__PUBLIC_HOST` | — | public host used in the generated docs |
| `HTTP__OPENAPI__PUBLIC_PATH` | — | public path prefix for the docs route |
| `REDIS__URL` | `redis://localhost:6379` | Redis connection URL |
| `REDIS__PREFIX` | `bot-005` | key prefix for the subscriptions, the state and the outage channel |
| `TELEGRAM__PROXY_URL` | — | proxy for the outgoing Telegram API calls |
| `TELEGRAM__MODE` | `polling` | update delivery mode: `polling` or `webhook` |
| `ADMIN__TELEGRAM_ID` | `0` | admin chat for the new subscriptions and the feedback; `0` disables them |

See `.env.example` for the annotated list.

> **Webhook security.** The webhook route performs **no request authentication**:
> the `X-Telegram-Bot-Api-Secret-Token` check is left to the application and is
> not implemented. A caller that reaches the route can submit forged updates.
> Keep the HTTP server behind a reverse proxy that only forwards Telegram
> traffic, and prefer the default polling mode.

<p align="right">(<a href="#readme-top">back to top</a>)</p>



<!-- ROADMAP -->
## Roadmap

- [x] Telegram bot commands and the street subscription flow
- [x] Redis-backed subscriptions and conversation state
- [x] Outage broadcasts over the Redis pub/sub channel
- [x] Webhook delivery mode
- [ ] Telegram webhook secret token validation
- [ ] Outage details in the notification

See the [open issues](https://github.com/005-bot/tg-bot-go/issues) for a full list of proposed features (and known issues).

<p align="right">(<a href="#readme-top">back to top</a>)</p>



<!-- CONTRIBUTING -->
## Contributing

Contributions are what make the open source community such an amazing place to learn, inspire, and create. Any contributions you make are **greatly appreciated**.

If you have a suggestion that would make this better, please fork the repo and create a pull request. You can also open an issue with the tag "enhancement".
Don't forget to give the project a star! Thanks again!

1. Fork the Project
2. Create your Feature Branch (`git checkout -b feature/AmazingFeature`)
3. Commit your Changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the Branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

<p align="right">(<a href="#readme-top">back to top</a>)</p>



<!-- LICENSE -->
## License

Distributed under the Apache 2.0 License. See `LICENSE` for more information.

<p align="right">(<a href="#readme-top">back to top</a>)</p>



<!-- CONTACT -->
## Contact

Maintainer: [@capcom6](https://github.com/capcom6)

Project Link: [https://github.com/005-bot/tg-bot-go](https://github.com/005-bot/tg-bot-go)

<p align="right">(<a href="#readme-top">back to top</a>)</p>



<!-- ACKNOWLEDGMENTS -->
## Acknowledgments

* [Best README Template](https://github.com/othneildrew/Best-README-Template)
* [Go Fiber](https://github.com/gofiber/fiber)
* [Uber Fx](https://github.com/uber-go/fx)
* [Telego](https://github.com/mymmrac/telego)
* [Shields.io](https://shields.io)

<p align="right">(<a href="#readme-top">back to top</a>)</p>



<!-- MARKDOWN LINKS & IMAGES -->
[contributors-shield]: https://img.shields.io/github/contributors/005-bot/tg-bot-go.svg?style=for-the-badge
[contributors-url]: https://github.com/005-bot/tg-bot-go/graphs/contributors
[forks-shield]: https://img.shields.io/github/forks/005-bot/tg-bot-go.svg?style=for-the-badge
[forks-url]: https://github.com/005-bot/tg-bot-go/network/members
[stars-shield]: https://img.shields.io/github/stars/005-bot/tg-bot-go.svg?style=for-the-badge
[stars-url]: https://github.com/005-bot/tg-bot-go/stargazers
[issues-shield]: https://img.shields.io/github/issues/005-bot/tg-bot-go.svg?style=for-the-badge
[issues-url]: https://github.com/005-bot/tg-bot-go/issues
[license-shield]: https://img.shields.io/github/license/005-bot/tg-bot-go.svg?style=for-the-badge
[license-url]: https://github.com/005-bot/tg-bot-go/blob/master/LICENSE
[go-shield]: https://img.shields.io/badge/go-1.26%2B-00ADD8?style=for-the-badge&logo=go
[go-url]: https://go.dev/
[fiber-shield]: https://img.shields.io/badge/Fiber-v2-00b894?style=for-the-badge
[fiber-url]: https://github.com/gofiber/fiber
[fx-shield]: https://img.shields.io/badge/Uber%20Fx-DI-6f42c1?style=for-the-badge
[fx-url]: https://github.com/uber-go/fx
[cli-shield]: https://img.shields.io/badge/urfave%2Fcli-v3-00b4d8?style=for-the-badge
[cli-url]: https://github.com/urfave/cli
[swagger-shield]: https://img.shields.io/badge/OpenAPI-Swagger-85EA2D?style=for-the-badge
[swagger-url]: https://github.com/swaggo/swag
[telegram-shield]: https://img.shields.io/badge/Telegram-Bot-26A5E4?style=for-the-badge&logo=telegram
[telegram-url]: https://core.telegram.org/bots
[redis-shield]: https://img.shields.io/badge/Redis-cache-FF4438?style=for-the-badge&logo=redis
[redis-url]: https://redis.io
