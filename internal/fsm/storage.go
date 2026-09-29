// Package fsm provides a Redis-backed conversation state layer compatible
// with aiogram's RedisStorage key scheme: states and data persist
// indefinitely under {prefix}:fsm:{chat_id}:{user_id}:{state|data}.
package fsm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Config configures the FSM storage key prefix.
type Config struct {
	Prefix string
}

// Store persists conversation state in Redis. Keys carry no TTL, matching
// aiogram RedisStorage defaults.
type Store struct {
	rdb    *redis.Client
	prefix string
}

// NewStore creates a Store using the given go-redis client.
func NewStore(rdb *redis.Client, cfg Config) *Store {
	return &Store{rdb: rdb, prefix: cfg.Prefix}
}

func (s *Store) stateKey(chatID, userID int64) string {
	return fmt.Sprintf("%s:fsm:%d:%d:state", s.prefix, chatID, userID)
}

func (s *Store) dataKey(chatID, userID int64) string {
	return fmt.Sprintf("%s:fsm:%d:%d:data", s.prefix, chatID, userID)
}

// SetState stores the conversation state for the chat/user pair.
func (s *Store) SetState(ctx context.Context, chatID, userID int64, state string) error {
	if err := s.rdb.Set(ctx, s.stateKey(chatID, userID), state, 0).Err(); err != nil {
		return fmt.Errorf("set fsm state: %w", err)
	}
	return nil
}

// GetState returns the stored state for the chat/user pair. A missing key
// yields an empty string and no error.
func (s *Store) GetState(ctx context.Context, chatID, userID int64) (string, error) {
	state, err := s.rdb.Get(ctx, s.stateKey(chatID, userID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get fsm state: %w", err)
	}
	return state, nil
}

// ClearState removes the stored state and its data for the chat/user pair.
// Clearing a missing state is not an error.
func (s *Store) ClearState(ctx context.Context, chatID, userID int64) error {
	if err := s.rdb.Del(ctx, s.stateKey(chatID, userID), s.dataKey(chatID, userID)).Err(); err != nil {
		return fmt.Errorf("clear fsm state: %w", err)
	}
	return nil
}

// SetData stores JSON-encoded conversation state data for the chat/user
// pair. A nil data stores the JSON null literal.
func (s *Store) SetData(ctx context.Context, chatID, userID int64, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal fsm data: %w", err)
	}
	if err = s.rdb.Set(ctx, s.dataKey(chatID, userID), raw, 0).Err(); err != nil {
		return fmt.Errorf("set fsm data: %w", err)
	}
	return nil
}

// GetData decodes the stored state data for the chat/user pair into dst.
// A missing key leaves dst untouched and returns no error.
func (s *Store) GetData(ctx context.Context, chatID, userID int64, dst any) error {
	raw, err := s.rdb.Get(ctx, s.dataKey(chatID, userID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get fsm data: %w", err)
	}
	if err = json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("unmarshal fsm data: %w", err)
	}
	return nil
}
