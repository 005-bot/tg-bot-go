package streetsdb

import (
	"context"
	"fmt"

	"github.com/005-bot/address-parser-go"
	"github.com/go-core-fx/logger"
	"go.uber.org/fx"
)

func Module() fx.Option {
	return fx.Module(
		"streetsdb",
		logger.WithNamedLogger("streetsdb"),
		fx.Provide(
			func(cfg Config) (*address.Parser, error) {
				parser, err := address.NewParser(address.Config{DBPath: cfg.Path})
				if err != nil {
					return nil, fmt.Errorf("create address parser: %w", err)
				}
				return parser, nil
			},
		),
		fx.Invoke(
			func(lc fx.Lifecycle, parser *address.Parser) {
				lc.Append(fx.StopHook(func(context.Context) error {
					parser.Stop()
					return nil
				}))
			},
		),
	)
}
