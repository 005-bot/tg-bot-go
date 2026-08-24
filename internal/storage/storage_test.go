package storage_test

import (
	"context"
	"testing"

	"github.com/005-bot/tg-bot-go/internal/storage"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const testPrefix = "bot-005"

func newTestService(t *testing.T) (*storage.Service, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { _ = rdb.Close() })

	svc := storage.NewService(rdb, storage.Config{Prefix: testPrefix}, zap.NewNop())

	return svc, mr
}

func strPtr(s string) *string {
	return &s
}

func TestSubscribe_ExactBytes(t *testing.T) {
	svc, mr := newTestService(t)
	ctx := context.Background()

	if err := svc.Subscribe(ctx, "user-1", nil); err != nil {
		t.Fatalf("Subscribe(nil) error: %v", err)
	}
	got := mr.HGet(testPrefix+":filters", "user-1")
	if got != `{"street":null}` {
		t.Errorf("Subscribe(nil) stored %q, want %q", got, `{"street":null}`)
	}

	if err := svc.Subscribe(ctx, "user-2", strPtr("Ленина")); err != nil {
		t.Fatalf("Subscribe(street) error: %v", err)
	}
	got = mr.HGet(testPrefix+":filters", "user-2")
	if got != `{"street":"Ленина"}` {
		t.Errorf("Subscribe(street) stored %q, want %q", got, `{"street":"Ленина"}`)
	}
}

func TestUnsubscribe(t *testing.T) {
	svc, mr := newTestService(t)
	ctx := context.Background()

	if err := svc.Subscribe(ctx, "user-1", strPtr("Ленина")); err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}
	if err := svc.Unsubscribe(ctx, "user-1"); err != nil {
		t.Fatalf("Unsubscribe error: %v", err)
	}

	if got := mr.HGet(testPrefix+":filters", "user-1"); got != "" {
		t.Errorf("Unsubscribe left the hash member in place: %q", got)
	}

	f, err := svc.GetFilter(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetFilter after unsubscribe error: %v", err)
	}
	if f.Street != nil {
		t.Errorf("GetFilter after unsubscribe = %+v, want nil street", f)
	}

	subs, err := svc.GetSubscribed(ctx)
	if err != nil {
		t.Fatalf("GetSubscribed error: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("GetSubscribed after unsubscribe = %v, want empty", subs)
	}
}

func TestGetFilter_Missing(t *testing.T) {
	svc, _ := newTestService(t)

	f, err := svc.GetFilter(context.Background(), "no-such-user")
	if err != nil {
		t.Fatalf("GetFilter(missing) error: %v", err)
	}
	if f.Street != nil {
		t.Errorf("GetFilter(missing) = %+v, want nil street", f)
	}
}

func TestGetFilter_Value(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	if err := svc.Subscribe(ctx, "user-1", strPtr("Советская")); err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}

	f, err := svc.GetFilter(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetFilter error: %v", err)
	}
	if f.Street == nil || *f.Street != "Советская" {
		t.Errorf("GetFilter = %+v, want street %q", f, "Советская")
	}
}

func TestService_DoesNotTouchVersionKey(t *testing.T) {
	svc, mr := newTestService(t)
	ctx := context.Background()

	if err := svc.Subscribe(ctx, "user-1", strPtr("Ленина")); err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}
	if _, err := svc.GetFilter(ctx, "user-1"); err != nil {
		t.Fatalf("GetFilter error: %v", err)
	}
	if _, err := svc.GetSubscribed(ctx); err != nil {
		t.Fatalf("GetSubscribed error: %v", err)
	}
	if err := svc.Unsubscribe(ctx, "user-1"); err != nil {
		t.Fatalf("Unsubscribe error: %v", err)
	}

	if mr.Exists(testPrefix + ":version") {
		t.Error("service touched the legacy {prefix}:version key")
	}
}

func TestGetSubscribed_HappyPath(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	if err := svc.Subscribe(ctx, "user-1", nil); err != nil {
		t.Fatalf("Subscribe(nil) error: %v", err)
	}
	if err := svc.Subscribe(ctx, "user-2", strPtr("Ленина")); err != nil {
		t.Fatalf("Subscribe(street) error: %v", err)
	}

	subs, err := svc.GetSubscribed(ctx)
	if err != nil {
		t.Fatalf("GetSubscribed error: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("GetSubscribed length = %d, want 2", len(subs))
	}
	if f, ok := subs["user-1"]; !ok || f.Street != nil {
		t.Errorf("GetSubscribed[user-1] = %+v, want nil street", f)
	}
	if f, ok := subs["user-2"]; !ok || f.Street == nil || *f.Street != "Ленина" {
		t.Errorf("GetSubscribed[user-2] = %+v, want street %q", f, "Ленина")
	}
}

func TestGetSubscribed_SkipsMalformed(t *testing.T) {
	svc, mr := newTestService(t)
	ctx := context.Background()

	if err := svc.Subscribe(ctx, "good-1", nil); err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}
	if err := svc.Subscribe(ctx, "good-2", strPtr("Мира")); err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}
	mr.HSet(testPrefix+":filters", "bad-1", "not-json")
	mr.HSet(testPrefix+":filters", "bad-2", `{"street":`)

	subs, err := svc.GetSubscribed(ctx)
	if err != nil {
		t.Fatalf("GetSubscribed error: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("GetSubscribed length = %d, want 2 (malformed skipped)", len(subs))
	}
	if _, ok := subs["bad-1"]; ok {
		t.Error("GetSubscribed returned malformed member bad-1")
	}
	if _, ok := subs["bad-2"]; ok {
		t.Error("GetSubscribed returned malformed member bad-2")
	}
	if _, ok := subs["good-1"]; !ok {
		t.Error("GetSubscribed missing valid member good-1")
	}
	if f, ok := subs["good-2"]; !ok || f.Street == nil || *f.Street != "Мира" {
		t.Errorf("GetSubscribed[good-2] = %+v, want street %q", f, "Мира")
	}
}

func TestGetSubscribed_Empty(t *testing.T) {
	svc, _ := newTestService(t)

	subs, err := svc.GetSubscribed(context.Background())
	if err != nil {
		t.Fatalf("GetSubscribed error: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("GetSubscribed = %v, want empty map", subs)
	}
}
