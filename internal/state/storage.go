// Package state provides the Redis-backed conversation state storage and
// bot predicates used by the handlers. The state store itself is the
// telegofx/fsm Store over this Storage. Keys follow aiogram's RedisStorage
// scheme: state and data persist indefinitely under
// {prefix}:fsm:{chat_id}:{user_id}:{state|data}.
package state

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Config configures the FSM storage key prefix.
type Config struct {
	Prefix string
}

// Storage persists conversation state in Redis. Keys carry no TTL, matching
// aiogram RedisStorage defaults. It implements the telegofx/fsm.Storage
// contract.
type Storage struct {
	rdb    *redis.Client
	prefix string
}

// NewStorage creates a Storage using the given go-redis client.
func NewStorage(rdb *redis.Client, cfg Config) *Storage {
	return &Storage{rdb: rdb, prefix: cfg.Prefix}
}

func (s *Storage) stateKey(chatID, userID int64) string {
	return fmt.Sprintf("%s:fsm:%d:%d:state", s.prefix, chatID, userID)
}

func (s *Storage) dataKey(chatID, userID int64) string {
	return fmt.Sprintf("%s:fsm:%d:%d:data", s.prefix, chatID, userID)
}

// Set stores the conversation state for the chat/user pair.
func (s *Storage) Set(ctx context.Context, chatID, userID int64, state string) error {
	if err := s.rdb.Set(ctx, s.stateKey(chatID, userID), state, 0).Err(); err != nil {
		return fmt.Errorf("set fsm state: %w", err)
	}
	return nil
}

// Get returns the stored state for the chat/user pair. A missing key yields
// an empty string and no error.
func (s *Storage) Get(ctx context.Context, chatID, userID int64) (string, error) {
	state, err := s.rdb.Get(ctx, s.stateKey(chatID, userID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get fsm state: %w", err)
	}
	return state, nil
}

// Clear removes the stored state and its data key for the chat/user pair.
// Clearing a missing state is not an error.
func (s *Storage) Clear(ctx context.Context, chatID, userID int64) error {
	if err := s.rdb.Del(ctx, s.stateKey(chatID, userID), s.dataKey(chatID, userID)).Err(); err != nil {
		return fmt.Errorf("clear fsm state: %w", err)
	}
	return nil
}
