// Package listener subscribes to the Redis pub/sub outages channel and
// yields apis-go domain.Outage events, mirroring the Python bot's
// listener (tg-bot/app/services/listener.py).
package listener

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apidev "github.com/005-bot/apis-go"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// retryDelay is the pause between subscribe-loop attempts, matching the
// Python retry loop's 1-second sleep.
const retryDelay = time.Second

// eventsBufferSize bounds the buffered outage queue while no consumer is
// attached yet (e.g. before the broadcast worker is wired).
const eventsBufferSize = 16

// Config holds the outage listener configuration.
type Config struct {
	Prefix string
}

// Service subscribes to the {prefix}:outages Redis channel and delivers
// unmarshaled outages on Events.
type Service struct {
	rdb     *redis.Client
	channel string
	logger  *zap.Logger

	// Events delivers outages as they arrive; it stays open until the
	// service stops.
	Events <-chan apidev.Outage
	events chan apidev.Outage
}

// NewService creates a Service subscribed to {prefix}:outages.
func NewService(rdb *redis.Client, cfg Config, logger *zap.Logger) *Service {
	events := make(chan apidev.Outage, eventsBufferSize)
	return &Service{
		rdb:     rdb,
		channel: cfg.Prefix + ":outages",
		logger:  logger,
		Events:  events,
		events:  events,
	}
}

// Run subscribes to the outages channel and forwards messages until ctx is
// cancelled. Any failure (subscribe error, connection drop) is logged and
// retried after retryDelay; malformed payloads are skipped with a warning.
// Run returns nil when ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	for {
		err := s.runOnce(ctx)
		if err != nil {
			s.logger.Error("outage listener error, retrying",
				zap.Error(err), zap.Duration("delay", retryDelay))
		}

		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
			return nil
		}
	}
}

// runOnce subscribes to the outages channel and forwards messages until the
// subscription drops or ctx is cancelled.
func (s *Service) runOnce(ctx context.Context) error {
	pubsub := s.rdb.Subscribe(ctx, s.channel)

	done := make(chan struct{})
	defer close(done)
	defer func() { _ = pubsub.Close() }()

	// ReceiveMessage blocks on a raw connection read that cannot be
	// interrupted by ctx, so it runs in its own goroutine. The goroutine
	// exits when ctx is done or runOnce returns (done closes).
	type recvResult struct {
		msg *redis.Message
		err error
	}

	recvCh := make(chan recvResult, 1)
	go func() {
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			select {
			case recvCh <- recvResult{msg: msg, err: err}:
			case <-ctx.Done():
				return
			case <-done:
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case r := <-recvCh:
			if r.err != nil {
				return fmt.Errorf("receive from %s: %w", s.channel, r.err)
			}

			var outage apidev.Outage
			if err := json.Unmarshal([]byte(r.msg.Payload), &outage); err != nil {
				s.logger.Warn("skipping malformed outage message",
					zap.String("channel", s.channel), zap.Error(err))
				continue
			}

			s.logger.Info("received outage",
				zap.String("area", outage.Area),
				zap.Int("streets", len(outage.Details.Streets)))

			select {
			case s.events <- outage:
			case <-ctx.Done():
				return nil
			}
		}
	}
}
