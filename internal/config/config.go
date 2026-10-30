// Package config loads application configuration from environment variables
// and an optional YAML file (CONFIG_PATH), mirroring the Python bot's env
// contract: `__` delimited keys map into nested structs (e.g. HTTP__ADDRESS,
// REDIS__PREFIX, TELEGRAM__TOKEN, ADMIN__TELEGRAM_ID).
//
// Note: the parser's street database path is set via STREETS_DB__PATH, so a
// newer streets.db can be dropped in without rebuilding the app; when empty,
// the parser uses its embedded streets.db via a temporary directory.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/go-core-fx/config"
	"github.com/go-core-fx/telegofx"
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
	Token    string        `koanf:"token"`
	ProxyURL string        `koanf:"proxy_url"`
	Timeout  time.Duration `koanf:"timeout"`
	Mode     telegofx.Mode `koanf:"mode"`
}

type adminConfig struct {
	TelegramID int `koanf:"telegram_id"`
}

type streetsDBConfig struct {
	Path string `koanf:"path"`
}

type Config struct {
	HTTP      http            `koanf:"http"`
	Redis     redisConfig     `koanf:"redis"`
	Telegram  telegramConfig  `koanf:"telegram"`
	Admin     adminConfig     `koanf:"admin"`
	StreetsDB streetsDBConfig `koanf:"streets_db"`
}

func Default() Config {
	//nolint:mnd // default values
	return Config{
		HTTP: http{
			Address:     "127.0.0.1:3000",
			ProxyHeader: "X-Forwarded-For",
			Proxies:     []string{},
			OpenAPI: openAPIConfig{
				Enabled:    true,
				PublicHost: "",
				PublicPath: "",
			},
		},
		Redis: redisConfig{
			URL:    "redis://localhost:6379",
			Prefix: "bot-005",
		},
		Telegram: telegramConfig{
			Token:    "",
			ProxyURL: "",
			Timeout:  60 * time.Second,
			Mode:     telegofx.ModePolling,
		},
		Admin: adminConfig{
			TelegramID: 0,
		},
		StreetsDB: streetsDBConfig{
			Path: "",
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
