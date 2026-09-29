package fsm_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/005-bot/tg-bot-go/internal/fsm"
	"github.com/alicebob/miniredis/v2"
	"github.com/mymmrac/telego"
	"github.com/redis/go-redis/v9"
)

const testPrefix = "bot-005"

func newTestStore(t *testing.T) (*fsm.Store, *miniredis.Miniredis, *redis.Client) {
	t.Helper()

	mr := miniredis.RunT(t)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	store := fsm.NewStore(rdb, fsm.Config{Prefix: testPrefix})

	return store, mr, rdb
}

func stateKey(chatID, userID int64) string {
	return fmt.Sprintf("%s:fsm:%d:%d:state", testPrefix, chatID, userID)
}

func dataKey(chatID, userID int64) string {
	return fmt.Sprintf("%s:fsm:%d:%d:data", testPrefix, chatID, userID)
}

func TestSetState_WritesExactKeyValueNoTTL(t *testing.T) {
	store, _, rdb := newTestStore(t)
	ctx := context.Background()

	const chatID, userID = int64(123), int64(456)

	if err := store.SetState(ctx, chatID, userID, "Filter:filter"); err != nil {
		t.Fatalf("SetState error: %v", err)
	}

	key := stateKey(chatID, userID)
	if got, err := rdb.Get(ctx, key).Result(); err != nil || got != "Filter:filter" {
		t.Errorf("state key %s = %q, err %v; want %q", key, got, err, "Filter:filter")
	}

	// Redis TTL must be -1: no expiry (aiogram RedisStorage parity).
	if got, err := rdb.TTL(ctx, key).Result(); err != nil || got != -1 {
		t.Errorf("TTL(%s) = %v, err %v; want -1 (no expiry)", key, got, err)
	}

	// The data key must not exist yet.
	if n, err := rdb.Exists(ctx, dataKey(chatID, userID)).Result(); err != nil || n != 0 {
		t.Errorf("data key exists before SetData: n=%d, err %v; want 0", n, err)
	}
}

func TestSetState_OverwritesPreviousState(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	const chatID, userID = int64(1), int64(2)

	if err := store.SetState(ctx, chatID, userID, "Filter:filter"); err != nil {
		t.Fatalf("first SetState error: %v", err)
	}
	if err := store.SetState(ctx, chatID, userID, "FeedbackState:feedback"); err != nil {
		t.Fatalf("second SetState error: %v", err)
	}

	if got, err := store.GetState(ctx, chatID, userID); err != nil || got != "FeedbackState:feedback" {
		t.Errorf("GetState = %q, err %v; want %q", got, err, "FeedbackState:feedback")
	}
}

func TestGetState_MissingReturnsEmptyNoError(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	got, err := store.GetState(ctx, 999, 888)
	if err != nil {
		t.Fatalf("GetState missing key error: %v; want nil", err)
	}
	if got != "" {
		t.Errorf("GetState missing key = %q; want empty string", got)
	}
}

func TestClearState_RemovesStateAndData(t *testing.T) {
	store, _, rdb := newTestStore(t)
	ctx := context.Background()

	const chatID, userID = int64(7), int64(8)

	if err := store.SetState(ctx, chatID, userID, "FeedbackState:feedback"); err != nil {
		t.Fatalf("SetState error: %v", err)
	}
	if err := store.SetData(ctx, chatID, userID, map[string]string{"text": "ok"}); err != nil {
		t.Fatalf("SetData error: %v", err)
	}

	if err := store.ClearState(ctx, chatID, userID); err != nil {
		t.Fatalf("ClearState error: %v", err)
	}

	if n, err := rdb.Exists(ctx, stateKey(chatID, userID), dataKey(chatID, userID)).Result(); err != nil || n != 0 {
		t.Errorf("keys remain after ClearState: n=%d, err %v; want 0", n, err)
	}

	if got, err := store.GetState(ctx, chatID, userID); err != nil || got != "" {
		t.Errorf("GetState after ClearState = %q, err %v; want empty", got, err)
	}
}

func TestClearState_MissingIsNoError(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.ClearState(ctx, 11, 22); err != nil {
		t.Errorf("ClearState on missing state error: %v; want nil", err)
	}
}

func TestSetDataGetData_RoundTrip(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	const chatID, userID = int64(3), int64(4)

	want := struct {
		Street  string `json:"street"`
		Numbers []int  `json:"numbers"`
	}{Street: "ул. Ленина", Numbers: []int{1, 2, 3}}

	if err := store.SetData(ctx, chatID, userID, want); err != nil {
		t.Fatalf("SetData error: %v", err)
	}

	var got struct {
		Street  string `json:"street"`
		Numbers []int  `json:"numbers"`
	}
	if err := store.GetData(ctx, chatID, userID, &got); err != nil {
		t.Fatalf("GetData error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetData = %+v; want %+v", got, want)
	}
}

func TestGetData_MissingLeavesDstUntouched(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	dst := "sentinel"
	if err := store.GetData(ctx, 5, 6, &dst); err != nil {
		t.Fatalf("GetData missing key error: %v; want nil", err)
	}
	if dst != "sentinel" {
		t.Errorf("GetData missing key mutated dst = %q; want %q", dst, "sentinel")
	}
}

func TestGetData_MalformedJSONReturnsError(t *testing.T) {
	store, _, rdb := newTestStore(t)
	ctx := context.Background()

	const chatID, userID = int64(5), int64(6)

	if err := rdb.Set(ctx, dataKey(chatID, userID), "not-json", 0).Err(); err != nil {
		t.Fatalf("seed malformed data: %v", err)
	}

	var dst map[string]string
	if err := store.GetData(ctx, chatID, userID, &dst); err == nil {
		t.Error("GetData malformed JSON error = nil; want error")
	}
}

func update(chatID, userID int64, text string) telego.Update {
	return telego.Update{
		Message: &telego.Message{
			Chat: telego.Chat{ID: chatID},
			From: &telego.User{ID: userID},
			Text: text,
		},
	}
}

func TestStateFilter(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	const chatID, userID = int64(42), int64(43)

	if err := store.SetState(ctx, chatID, userID, "Filter:filter"); err != nil {
		t.Fatalf("SetState error: %v", err)
	}

	tests := []struct {
		name  string
		upd   telego.Update
		state string
		want  bool
	}{
		{
			name:  "matching state",
			upd:   update(chatID, userID, "hello"),
			state: "Filter:filter",
			want:  true,
		},
		{
			name:  "different state",
			upd:   update(chatID, userID, "hello"),
			state: "FeedbackState:feedback",
			want:  false,
		},
		{
			name:  "no stored state",
			upd:   update(777, 778, "hello"),
			state: "Filter:filter",
			want:  false,
		},
		{
			name:  "nil message",
			upd:   telego.Update{},
			state: "Filter:filter",
			want:  false,
		},
		{
			name: "nil from",
			upd: telego.Update{
				Message: &telego.Message{Chat: telego.Chat{ID: chatID}},
			},
			state: "Filter:filter",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pred := fsm.StateFilter(store, tt.state)
			if got := pred(ctx, tt.upd); got != tt.want {
				t.Errorf("StateFilter(%q) = %v; want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestStateFilter_StorageErrorDoesNotMatch(t *testing.T) {
	store, mr, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.SetState(ctx, 42, 43, "Filter:filter"); err != nil {
		t.Fatalf("SetState error: %v", err)
	}

	// Redis outage: the storage read fails and the predicate must not match.
	mr.Close()

	if got := fsm.StateFilter(store, "Filter:filter")(ctx, update(42, 43, "hello")); got {
		t.Error("StateFilter matched while storage is down; want false")
	}
}

func TestIsCommand(t *testing.T) {
	tests := []struct {
		name string
		upd  telego.Update
		want bool
	}{
		{name: "plain command", upd: update(1, 2, "/start"), want: true},
		{name: "command with bot mention", upd: update(1, 2, "/start@my_bot"), want: true},
		{name: "bare slash", upd: update(1, 2, "/"), want: true},
		{name: "plain text", upd: update(1, 2, "hello"), want: false},
		{name: "empty text", upd: update(1, 2, ""), want: false},
		{name: "nil message", upd: telego.Update{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fsm.IsCommand()(context.Background(), tt.upd); got != tt.want {
				t.Errorf("IsCommand() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestTextNotCommand(t *testing.T) {
	tests := []struct {
		name string
		upd  telego.Update
		want bool
	}{
		{name: "plain text", upd: update(1, 2, "hello"), want: true},
		{name: "multiline text", upd: update(1, 2, "line one\nline two"), want: true},
		{name: "command", upd: update(1, 2, "/start"), want: false},
		{name: "empty text", upd: update(1, 2, ""), want: false},
		{name: "nil message", upd: telego.Update{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fsm.TextNotCommand()(context.Background(), tt.upd); got != tt.want {
				t.Errorf("TextNotCommand() = %v; want %v", got, tt.want)
			}
		})
	}
}
