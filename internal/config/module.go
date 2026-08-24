package config

import (
	"github.com/005-bot/tg-bot-go/internal/listener"
	"github.com/005-bot/tg-bot-go/internal/storage"
	"github.com/go-core-fx/fiberfx"
	"github.com/go-core-fx/fiberfx/openapi"
	"github.com/go-core-fx/redisfx"
	"github.com/go-core-fx/telegofx"
	"go.uber.org/fx"
)

func Module() fx.Option {
	return fx.Module(
		"config",
		fx.Provide(New, fx.Private),
		fx.Provide(
			func(cfg Config) fiberfx.Config {
				return fiberfx.Config{
					Address:     cfg.HTTP.Address,
					ProxyHeader: cfg.HTTP.ProxyHeader,
					Proxies:     cfg.HTTP.Proxies,
				}
			},
			func(cfg Config) openapi.Config {
				return openapi.Config{
					Enabled:    cfg.HTTP.OpenAPI.Enabled,
					PublicHost: cfg.HTTP.OpenAPI.PublicHost,
					PublicPath: cfg.HTTP.OpenAPI.PublicPath,
				}
			},
			func(cfg Config) telegofx.Config {
				return telegofx.Config{
					Token:    cfg.Telegram.Token,
					ProxyURL: cfg.Telegram.ProxyURL,
				}
			},
			func(cfg Config) redisfx.Config {
				return redisfx.Config{
					URL: cfg.Redis.URL,
				}
			},
			func(cfg Config) storage.Config {
				return storage.Config{
					Prefix: cfg.Redis.Prefix,
				}
			},
			func(cfg Config) listener.Config {
				return listener.Config{
					Prefix: cfg.Redis.Prefix,
				}
			},
		),
	)
}
