package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Service stores user street filters in Redis under {prefix}:filters.
type Service struct {
	rdb        *redis.Client
	logger     *zap.Logger
	keyFilters string
}

// NewService creates a Service using the given go-redis client.
func NewService(rdb *redis.Client, cfg Config, logger *zap.Logger) *Service {
	return &Service{
		rdb:        rdb,
		logger:     logger,
		keyFilters: cfg.Prefix + ":filters",
	}
}

// Subscribe stores the filter for the user, overwriting any previous value.
func (s *Service) Subscribe(ctx context.Context, userID string, street *string) error {
	data, err := marshalFilter(Filter{Street: street})
	if err != nil {
		return fmt.Errorf("marshal filter: %w", err)
	}
	if err = s.rdb.HSet(ctx, s.keyFilters, userID, string(data)).Err(); err != nil {
		return fmt.Errorf("hset %s: %w", s.keyFilters, err)
	}
	return nil
}

// Unsubscribe removes the user's filter, if present.
func (s *Service) Unsubscribe(ctx context.Context, userID string) error {
	if err := s.rdb.HDel(ctx, s.keyFilters, userID).Err(); err != nil {
		return fmt.Errorf("hdel %s: %w", s.keyFilters, err)
	}
	return nil
}

// GetSubscribed returns all user filters. Malformed hash members are skipped
// with a warning log; the rest are returned.
func (s *Service) GetSubscribed(ctx context.Context) (map[string]Filter, error) {
	raw, err := s.rdb.HGetAll(ctx, s.keyFilters).Result()
	if err != nil {
		return nil, fmt.Errorf("hgetall %s: %w", s.keyFilters, err)
	}

	filters := make(map[string]Filter, len(raw))
	for userID, data := range raw {
		var f Filter
		if err = json.Unmarshal([]byte(data), &f); err != nil {
			s.logger.Warn("skipping malformed filter hash member",
				zap.String("user_id", userID), zap.Error(err))
			continue
		}
		filters[userID] = f
	}

	return filters, nil
}

// GetFilter returns the filter for a single user. A missing member yields a
// Filter with a nil street and no error.
func (s *Service) GetFilter(ctx context.Context, userID string) (Filter, error) {
	data, err := s.rdb.HGet(ctx, s.keyFilters, userID).Result()
	if errors.Is(err, redis.Nil) {
		return Filter{}, nil
	}
	if err != nil {
		return Filter{}, fmt.Errorf("hget %s: %w", s.keyFilters, err)
	}

	var f Filter
	if err = json.Unmarshal([]byte(data), &f); err != nil {
		return Filter{}, fmt.Errorf("unmarshal filter for user %s: %w", userID, err)
	}

	return f, nil
}

// marshalFilter encodes the filter as exact pydantic-compatible JSON without
// HTML escaping (pydantic model_dump_json emits raw UTF-8 for &, <, >).
func marshalFilter(f Filter) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(f); err != nil {
		return nil, fmt.Errorf("encode filter: %w", err)
	}

	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), nil
}
