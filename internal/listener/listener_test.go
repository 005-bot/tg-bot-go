package listener_test

import (
	"context"
	"net"
	"reflect"
	"slices"
	"testing"
	"time"

	apidev "github.com/005-bot/apis-go"
	"github.com/005-bot/tg-bot-go/internal/listener"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const testPrefix = "bot-005"

// sampleOutageJSON is the wire-format outage fixture (matches the apis-go
// domain.Outage schema and monitor-go's publisher).
func sampleOutageJSON() string {
	return `{"area":"Тест","organization_info":{"resource_type":"Холодное водоснабжение","resource":"ХВС","organization":"Тестовая","phones":["+7 111"]},"details":{"streets":[{"name":"ул. Ленина","buildings":["1"]}],"reason":{"type":"Ремонт","description":"Тест"},"water_deliveries":[],"comments":"Тест"},"period":["2026-08-20T10:00:00Z","2026-08-20T14:00:00Z"]}`
}

// sampleOutage is the expected unmarshaled domain.Outage for
// sampleOutageJSON.
func sampleOutage() apidev.Outage {
	resourceType := apidev.ResourceTypeColdWater
	return apidev.Outage{
		Area: "Тест",
		OrganizationInfo: apidev.OrganizationInfo{
			ResourceType: &resourceType,
			Resource:     "ХВС",
			Organization: "Тестовая",
			Phones:       []string{"+7 111"},
		},
		Details: apidev.OutageDetails{
			Streets: []apidev.Street{
				{Name: "ул. Ленина", Buildings: []string{"1"}},
			},
			Reason:          &apidev.Reason{Type: "Ремонт", Description: "Тест"},
			WaterDeliveries: []apidev.WaterDelivery{},
			Comments:        "Тест",
		},
		Period: []time.Time{
			time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
			time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC),
		},
	}
}

func newTestService(t *testing.T) (*listener.Service, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	svc := listener.NewService(rdb, listener.Config{Prefix: testPrefix}, zap.NewNop())

	return svc, mr
}

// waitSubscribed polls the miniredis subscriber registry until the outages
// channel has an active subscription or the deadline passes.
func waitSubscribed(t *testing.T, mr *miniredis.Miniredis) {
	t.Helper()

	channel := testPrefix + ":outages"
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if slices.Contains(mr.PubSubChannels(""), channel) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("listener did not subscribe to %s", channel)
}

// expectEvent waits up to 2 seconds for an outage on the Events channel.
func expectEvent(t *testing.T, svc *listener.Service) apidev.Outage {
	t.Helper()

	select {
	case outage := <-svc.Events:
		return outage
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for outage event")
		return apidev.Outage{}
	}
}

// expectNoEvent asserts that no outage arrives within 300ms.
func expectNoEvent(t *testing.T, svc *listener.Service) {
	t.Helper()

	select {
	case outage := <-svc.Events:
		t.Fatalf("unexpected outage event: %+v", outage)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestRun_YieldsOutage(t *testing.T) {
	svc, mr := newTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() { runErr <- svc.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-runErr:
		case <-time.After(2 * time.Second):
			t.Error("Run did not stop after cancel")
		}
	})

	channel := testPrefix + ":outages"
	waitSubscribed(t, mr)

	if got := mr.Publish(channel, sampleOutageJSON()); got != 1 {
		t.Errorf("Publish receivers = %d, want 1", got)
	}

	if got, want := expectEvent(t, svc), sampleOutage(); !reflect.DeepEqual(got, want) {
		t.Errorf("received outage = %+v, want %+v", got, want)
	}
}

func TestRun_SkipsMalformedJSON(t *testing.T) {
	svc, mr := newTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() { runErr <- svc.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-runErr:
		case <-time.After(2 * time.Second):
			t.Error("Run did not stop after cancel")
		}
	})

	channel := testPrefix + ":outages"
	waitSubscribed(t, mr)

	mr.Publish(channel, "not-json")
	mr.Publish(channel, `{"area":`)
	mr.Publish(channel, sampleOutageJSON())

	if got, want := expectEvent(t, svc), sampleOutage(); !reflect.DeepEqual(got, want) {
		t.Errorf("received outage = %+v, want %+v", got, want)
	}

	// The loop must survive malformed messages: a second valid message
	// still arrives.
	mr.Publish(channel, sampleOutageJSON())
	if got, want := expectEvent(t, svc), sampleOutage(); !reflect.DeepEqual(got, want) {
		t.Errorf("received outage after malformed = %+v, want %+v", got, want)
	}
}

func TestRun_CancellationStopsLoop(t *testing.T) {
	svc, mr := newTestService(t)
	ctx, cancel := context.WithCancel(context.Background())

	runErr := make(chan error, 1)
	go func() { runErr <- svc.Run(ctx) }()

	waitSubscribed(t, mr)

	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Errorf("Run returned error on cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRun_ReconnectsAfterServerRestart(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	mr1 := miniredis.NewMiniRedis()
	if err = mr1.StartAddr(addr); err != nil {
		t.Fatalf("start first miniredis: %v", err)
	}
	t.Cleanup(mr1.Close)

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })

	svc := listener.NewService(rdb, listener.Config{Prefix: testPrefix}, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() { runErr <- svc.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-runErr:
		case <-time.After(2 * time.Second):
			t.Error("Run did not stop after cancel")
		}
	})

	channel := testPrefix + ":outages"
	waitSubscribed(t, mr1)

	if got := mr1.Publish(channel, sampleOutageJSON()); got != 1 {
		t.Errorf("Publish receivers = %d, want 1", got)
	}
	if got, want := expectEvent(t, svc), sampleOutage(); !reflect.DeepEqual(got, want) {
		t.Errorf("received outage = %+v, want %+v", got, want)
	}

	// Simulate a Redis outage: stop the server. The listener's subscription
	// drops and the run loop must re-subscribe after the retry delay.
	mr1.Close()

	mr2 := miniredis.NewMiniRedis()
	if err = mr2.StartAddr(addr); err != nil {
		t.Fatalf("restart miniredis on %s: %v", addr, err)
	}
	t.Cleanup(mr2.Close)

	waitSubscribed(t, mr2)

	if got := mr2.Publish(channel, sampleOutageJSON()); got != 1 {
		t.Errorf("Publish after reconnect receivers = %d, want 1", got)
	}
	if got, want := expectEvent(t, svc), sampleOutage(); !reflect.DeepEqual(got, want) {
		t.Errorf("received outage after reconnect = %+v, want %+v", got, want)
	}
}

func TestRun_PrefixScopesChannel(t *testing.T) {
	svc, mr := newTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() { runErr <- svc.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-runErr:
		case <-time.After(2 * time.Second):
			t.Error("Run did not stop after cancel")
		}
	})

	channel := testPrefix + ":outages"
	waitSubscribed(t, mr)

	// A message on a foreign channel must not be received.
	mr.Publish("other-005:outages", sampleOutageJSON())
	expectNoEvent(t, svc)

	if got := mr.Publish(channel, sampleOutageJSON()); got != 1 {
		t.Errorf("Publish receivers = %d, want 1", got)
	}
	if got, want := expectEvent(t, svc), sampleOutage(); !reflect.DeepEqual(got, want) {
		t.Errorf("received outage = %+v, want %+v", got, want)
	}
}
