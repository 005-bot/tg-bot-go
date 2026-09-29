package state_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/005-bot/tg-bot-go/internal/state"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

const testPrefix = "bot-005"

func newTestStorage(t *testing.T) (*state.Storage, *redis.Client) {
	t.Helper()

	mr := miniredis.RunT(t)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	store := state.NewStorage(rdb, state.Config{Prefix: testPrefix})

	return store, rdb
}

func stateKey(chatID, userID int64) string {
	return fmt.Sprintf("%s:fsm:%d:%d:state", testPrefix, chatID, userID)
}

func dataKey(chatID, userID int64) string {
	return fmt.Sprintf("%s:fsm:%d:%d:data", testPrefix, chatID, userID)
}

func TestSet_WritesExactKeyValueNoTTL(t *testing.T) {
	store, rdb := newTestStorage(t)
	ctx := context.Background()

	const chatID, userID = int64(123), int64(456)

	if err := store.Set(ctx, chatID, userID, "Filter:filter"); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	key := stateKey(chatID, userID)
	if got, err := rdb.Get(ctx, key).Result(); err != nil || got != "Filter:filter" {
		t.Errorf("state key %s = %q, err %v; want %q", key, got, err, "Filter:filter")
	}

	// Redis TTL must be -1: no expiry (aiogram RedisStorage parity).
	if got, err := rdb.TTL(ctx, key).Result(); err != nil || got != -1 {
		t.Errorf("TTL(%s) = %v, err %v; want -1 (no expiry)", key, got, err)
	}

	// The data key must not exist: the store is state-only.
	if n, err := rdb.Exists(ctx, dataKey(chatID, userID)).Result(); err != nil || n != 0 {
		t.Errorf("data key exists after Set: n=%d, err %v; want 0", n, err)
	}
}

func TestSet_OverwritesPreviousState(t *testing.T) {
	store, _ := newTestStorage(t)
	ctx := context.Background()

	const chatID, userID = int64(1), int64(2)

	if err := store.Set(ctx, chatID, userID, "Filter:filter"); err != nil {
		t.Fatalf("first Set error: %v", err)
	}
	if err := store.Set(ctx, chatID, userID, "FeedbackState:feedback"); err != nil {
		t.Fatalf("second Set error: %v", err)
	}

	if got, err := store.Get(ctx, chatID, userID); err != nil || got != "FeedbackState:feedback" {
		t.Errorf("Get = %q, err %v; want %q", got, err, "FeedbackState:feedback")
	}
}

func TestGet_MissingReturnsEmptyNoError(t *testing.T) {
	store, _ := newTestStorage(t)
	ctx := context.Background()

	got, err := store.Get(ctx, 999, 888)
	if err != nil {
		t.Fatalf("Get missing key error: %v; want nil", err)
	}
	if got != "" {
		t.Errorf("Get missing key = %q; want empty string", got)
	}
}

func TestClear_RemovesStateAndData(t *testing.T) {
	store, rdb := newTestStorage(t)
	ctx := context.Background()

	const chatID, userID = int64(7), int64(8)

	if err := store.Set(ctx, chatID, userID, "FeedbackState:feedback"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	// Seed a legacy data key: Clear must still remove it (aiogram parity).
	if err := rdb.Set(ctx, dataKey(chatID, userID), []byte(`{"text":"ok"}`), 0).Err(); err != nil {
		t.Fatalf("seed data key: %v", err)
	}

	if err := store.Clear(ctx, chatID, userID); err != nil {
		t.Fatalf("Clear error: %v", err)
	}

	if n, err := rdb.Exists(ctx, stateKey(chatID, userID), dataKey(chatID, userID)).Result(); err != nil || n != 0 {
		t.Errorf("keys remain after Clear: n=%d, err %v; want 0", n, err)
	}

	if got, err := store.Get(ctx, chatID, userID); err != nil || got != "" {
		t.Errorf("Get after Clear = %q, err %v; want empty", got, err)
	}
}

func TestClear_MissingIsNoError(t *testing.T) {
	store, _ := newTestStorage(t)
	ctx := context.Background()

	if err := store.Clear(ctx, 11, 22); err != nil {
		t.Errorf("Clear on missing state error: %v; want nil", err)
	}
}
