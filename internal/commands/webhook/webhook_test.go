package webhook_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/005-bot/tg-bot-go/internal/commands/webhook"
	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoapi"
)

const (
	testToken = "123456:abcdefghijklmnopqrstuvwxyzABCDEFGHI"
	testURL   = "https://example.com/hook"
)

// setWebhookRequest is the JSON body sent to the Bot API setWebhook method.
type setWebhookRequest struct {
	URL            string   `json:"url"`
	AllowedUpdates []string `json:"allowed_updates"`
}

// fakeAPI mimics the Telegram Bot API for setWebhook requests.
type fakeAPI struct {
	server *httptest.Server

	mu       sync.Mutex
	requests []setWebhookRequest
}

func newFakeAPI(t *testing.T, status int, body string) *fakeAPI {
	t.Helper()

	f := &fakeAPI{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want %s", r.Method, http.MethodPost)
		}
		if r.URL.Path != "/bot"+testToken+"/setWebhook" {
			t.Errorf("path = %q, want /bot<token>/setWebhook", r.URL.Path)
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		var req setWebhookRequest
		err = json.Unmarshal(raw, &req)
		if err != nil {
			t.Errorf("unmarshal body %q: %v", raw, err)
		}

		f.mu.Lock()
		f.requests = append(f.requests, req)
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.server.Close)

	return f
}

func (f *fakeAPI) calls() []setWebhookRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]setWebhookRequest(nil), f.requests...)
}

func newTestBot(t *testing.T, apiURL string) *telego.Bot {
	t.Helper()

	bot, err := telego.NewBot(testToken, telego.WithAPIServer(apiURL), telego.WithDiscardLogger())
	if err != nil {
		t.Fatalf("telego.NewBot: %v", err)
	}

	return bot
}

// captureStdout runs f with [os.Stdout] piped and returns what was written.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()

	oldStdout := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = write                        //nolint:reassign // test-only: redirect stdout to capture CLI output
	defer func() { os.Stdout = oldStdout }() //nolint:reassign // test-only: restore stdout

	f()
	_ = write.Close()

	var buf bytes.Buffer
	if _, copyErr := io.Copy(&buf, read); copyErr != nil {
		t.Fatalf("read stdout: %v", copyErr)
	}

	return buf.String()
}

func TestRunSetWebhookSuccess(t *testing.T) {
	fake := newFakeAPI(t, http.StatusOK, `{"ok":true,"result":true}`)

	out := captureStdout(t, func() {
		err := webhook.Run(context.Background(), newTestBot(t, fake.server.URL), testURL)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if out != "Webhook set\n" {
		t.Fatalf("stdout = %q, want %q", out, "Webhook set\n")
	}

	calls := fake.calls()
	if len(calls) != 1 {
		t.Fatalf("setWebhook calls = %d, want 1", len(calls))
	}
	if calls[0].URL != testURL {
		t.Errorf("url = %q, want %q", calls[0].URL, testURL)
	}
	wantUpdates := []string{"message"}
	if !slices.Equal(calls[0].AllowedUpdates, wantUpdates) {
		t.Errorf("allowed_updates = %v, want %v", calls[0].AllowedUpdates, wantUpdates)
	}
}

func TestRunSetWebhookAPIError(t *testing.T) {
	fake := newFakeAPI(t, http.StatusUnauthorized, `{"ok":false,"error_code":401,"description":"Unauthorized"}`)

	err := webhook.Run(context.Background(), newTestBot(t, fake.server.URL), testURL)
	if err == nil {
		t.Fatal("Run: want error, got nil")
	}

	var apiErr *telegoapi.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *telegoapi.Error in error chain, got %T: %v", err, err)
	}
	if apiErr.ErrorCode != http.StatusUnauthorized {
		t.Errorf("error code = %d, want %d", apiErr.ErrorCode, http.StatusUnauthorized)
	}
	if !strings.Contains(err.Error(), "set webhook") {
		t.Errorf("error = %q, want %q context", err.Error(), "set webhook")
	}
}

func TestRunSetWebhookEmptyURL(t *testing.T) {
	fake := newFakeAPI(t, http.StatusOK, `{"ok":true,"result":true}`)

	if err := webhook.Run(context.Background(), newTestBot(t, fake.server.URL), ""); err != nil {
		t.Fatalf("Run: %v", err)
	}

	calls := fake.calls()
	if len(calls) != 1 || calls[0].URL != "" {
		t.Fatalf("calls = %+v, want single request with empty url forwarded as-is", calls)
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid https", url: "https://example.com/hook", wantErr: false},
		{name: "valid http with port", url: "http://127.0.0.1:8080/hook", wantErr: false},
		{name: "missing scheme", url: "example.com/hook", wantErr: true},
		{name: "malformed", url: "://bad", wantErr: true},
		{name: "invalid url escape", url: "https://example.com/%zz", wantErr: true},
		{name: "unsupported scheme", url: "ftp://example.com/hook", wantErr: true},
		{name: "empty", url: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := webhook.ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), "webhook url") {
				t.Fatalf("error = %q, want webhook-url context", err.Error())
			}
		})
	}
}

func TestNewBotTokenValidation(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "empty", token: ""},
		{name: "not a token", token: "not-a-token"},
		{name: "short secret", token: "123456:abc"},
		{name: "missing numeric id", token: ":abcdefghijklmnopqrstuvwxyzABCDEFGHI"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := webhook.NewBot(tt.token)
			if err == nil {
				t.Fatal("NewBot: want error, got nil")
			}
			if !strings.Contains(err.Error(), "token") {
				t.Fatalf("error = %q, want token context", err.Error())
			}
		})
	}
}

func TestNewBotValidToken(t *testing.T) {
	bot, err := webhook.NewBot(testToken)
	if err != nil {
		t.Fatalf("NewBot: %v", err)
	}
	if bot == nil {
		t.Fatal("NewBot: nil bot")
	}
}
