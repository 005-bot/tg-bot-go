// Package config loads application configuration from environment variables
// and an optional YAML file (CONFIG_PATH), mirroring the Python bot's env
// contract: `__` delimited keys map into nested structs (e.g. HTTP__PORT,
// REDIS__PREFIX, TELEGRAM__TOKEN, ADMIN__TELEGRAM_ID).
//
// Note: unlike address-parser-go, tg-bot-go does not expose a parser
// database-path env var. The parser uses its embedded streets.db default
// via a temporary directory.
package config

import (
	"fmt"
	"os"

	"github.com/go-core-fx/config"
)

type http struct {
	Address     string   `koanf:"address"`
	ProxyHeader string   `koanf:"proxy_header"`
	Proxies     []string `koanf:"proxies"`

	OpenAPI openAPIConfig `koanf:"openapi"`
}

type openAPIConfig struct {
	Enabled    bool   `koanf:"enabled"`
	PublicHost string `koanf:"public_host"`
	PublicPath string `koanf:"public_path"`
}

type redisConfig struct {
	URL    string `koanf:"url"`
	Prefix string `koanf:"prefix"`
}

type telegramConfig struct {
	Token      string `koanf:"token"`
	ProxyURL   string `koanf:"proxy_url"`
	WebhookURL string `koanf:"webhook_url"`
}

type adminConfig struct {
	TelegramID int `koanf:"telegram_id"`
}

type Config struct {
	HTTP     http           `koanf:"http"`
	Redis    redisConfig    `koanf:"redis"`
	Telegram telegramConfig `koanf:"telegram"`
	Admin    adminConfig    `koanf:"admin"`
}

func Default() Config {
	return Config{
		HTTP: http{
			Address:     "127.0.0.1:3000",
			ProxyHeader: "X-Forwarded-For",
			Proxies:     []string{},
		},
		Redis: redisConfig{
			URL:    "redis://localhost:6379",
			Prefix: "bot-005",
		},
		Telegram: telegramConfig{
			Token:      "",
			WebhookURL: "",
		},
		Admin: adminConfig{
			TelegramID: 0,
		},
	}
}

func New() (Config, error) {
	cfg := Default()

	options := []config.Option{}
	if yamlPath := os.Getenv("CONFIG_PATH"); yamlPath != "" {
		options = append(options, config.WithLocalYAML(yamlPath))
	}

	if err := config.Load(&cfg, options...); err != nil {
		return Config{}, fmt.Errorf("failed to load config: %w", err)
	}

	return cfg, nil
}
