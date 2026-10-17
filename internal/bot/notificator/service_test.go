package notificator_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"sync"
	"testing"

	"github.com/005-bot/tg-bot-go/internal/bot/notificator"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

const testToken = "123456:abcdefghijklmnopqrstuvwxyzABCDEFGHI"

// apiCall is a Bot API request captured by the fake Telegram server.
type apiCall struct {
	method string
	body   []byte
}

// fakeTelegram mimics the Bot API: it captures every request and answers
// with a minimal success payload.
type fakeTelegram struct {
	mu       sync.Mutex
	requests []apiCall
	server   *httptest.Server
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	t.Helper()
	f := &fakeTelegram{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		method := path.Base(r.URL.Path)
		f.mu.Lock()
		f.requests = append(f.requests, apiCall{method: method, body: body})
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if method == "sendMessage" || method == "forwardMessage" {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeTelegram) calls(method string) []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []apiCall
	for _, c := range f.requests {
		if c.method == method {
			out = append(out, c)
		}
	}
	return out
}

// sendParams mirrors the sendMessage request body fields under test.
type sendParams struct {
	ChatID    any    `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

type forwardParams struct {
	ChatID     any `json:"chat_id"`
	FromChatID any `json:"from_chat_id"`
	MessageID  int `json:"message_id"`
}

func decodeSend(t *testing.T, call apiCall) sendParams {
	t.Helper()
	var p sendParams
	if err := json.Unmarshal(call.body, &p); err != nil {
		t.Fatalf("decode sendMessage body %q: %v", call.body, err)
	}
	return p
}

func decodeForward(t *testing.T, call apiCall) forwardParams {
	t.Helper()
	var p forwardParams
	if err := json.Unmarshal(call.body, &p); err != nil {
		t.Fatalf("decode forwardMessage body %q: %v", call.body, err)
	}
	return p
}

func wantChatID(t *testing.T, got any, want int64) {
	t.Helper()
	num, ok := got.(float64)
	if !ok || int64(num) != want {
		t.Fatalf("chat_id = %v, want %d", got, want)
	}
}

func newNotificator(t *testing.T, adminID int64) (*notificator.Service, *fakeTelegram) {
	t.Helper()
	fake := newFakeTelegram(t)
	teleBot, err := telegofx.New(telegofx.Config{Token: testToken},
		[]telego.BotOption{telego.WithAPIServer(fake.server.URL), telego.WithDiscardLogger()},
		zap.NewNop())
	if err != nil {
		t.Fatalf("telegofx.New: %v", err)
	}
	svc := notificator.NewService(teleBot, notificator.Config{AdminID: adminID}, zap.NewNop())
	return svc, fake
}

func TestNewUserTitleFallbacks(t *testing.T) {
	tests := []struct {
		name string
		user telego.User
		want string
	}{
		{
			name: "username",
			user: telego.User{ID: 333, Username: "vasya", FirstName: "Вася"},
			want: "👤 *Новая подписка:* [vasya](tg://user?id=333)",
		},
		{
			name: "first name with markdown chars is quoted",
			user: telego.User{ID: 333, FirstName: "Лена* (гл.)!"},
			want: "👤 *Новая подписка:* [Лена\\* \\(гл\\.\\)\\!](tg://user?id=333)",
		},
		{
			name: "id fallback",
			user: telego.User{ID: 333},
			want: "👤 *Новая подписка:* [333](tg://user?id=333)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, fake := newNotificator(t, 4242)

			user := tt.user
			if err := svc.NewUser(context.Background(), &user); err != nil {
				t.Fatalf("NewUser: %v", err)
			}

			sends := fake.calls("sendMessage")
			if len(sends) != 1 {
				t.Fatalf("sendMessage calls = %d, want 1", len(sends))
			}
			got := decodeSend(t, sends[0])
			wantChatID(t, got.ChatID, 4242)
			if got.ParseMode != telego.ModeMarkdown {
				t.Errorf("parse_mode = %q, want %q", got.ParseMode, telego.ModeMarkdown)
			}
			if got.Text != tt.want {
				t.Errorf("notify text = %q, want %q", got.Text, tt.want)
			}
		})
	}
}

func TestAdminDisabled(t *testing.T) {
	svc, fake := newNotificator(t, 0)
	ctx := context.Background()

	if err := svc.NewUser(ctx, &telego.User{ID: 333, Username: "vasya"}); err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if err := svc.Feedback(ctx, &telego.Message{
		MessageID: 7,
		Chat:      telego.Chat{ID: 555, Type: telego.ChatTypePrivate},
	}); err != nil {
		t.Fatalf("Feedback: %v", err)
	}
	if err := svc.Notify(ctx, "тест"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if n := len(fake.calls("sendMessage")) + len(fake.calls("forwardMessage")); n != 0 {
		t.Errorf("API calls = %d, want 0 (no-op with AdminID 0)", n)
	}
}

func TestFeedback(t *testing.T) {
	svc, fake := newNotificator(t, 4242)

	if err := svc.Feedback(context.Background(), &telego.Message{
		MessageID: 77,
		Chat:      telego.Chat{ID: 555, Type: telego.ChatTypePrivate},
	}); err != nil {
		t.Fatalf("Feedback: %v", err)
	}

	forwards := fake.calls("forwardMessage")
	if len(forwards) != 1 {
		t.Fatalf("forwardMessage calls = %d, want 1", len(forwards))
	}
	got := decodeForward(t, forwards[0])
	wantChatID(t, got.ChatID, 4242)
	wantChatID(t, got.FromChatID, 555)
	if got.MessageID != 77 {
		t.Errorf("message_id = %d, want 77", got.MessageID)
	}
}

func TestNotify(t *testing.T) {
	svc, fake := newNotificator(t, 4242)

	const text = "👤 *Новая подписка:* [vasya](tg://user?id=333)"
	if err := svc.Notify(context.Background(), text); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	sends := fake.calls("sendMessage")
	if len(sends) != 1 {
		t.Fatalf("sendMessage calls = %d, want 1", len(sends))
	}
	got := decodeSend(t, sends[0])
	wantChatID(t, got.ChatID, 4242)
	if got.Text != text {
		t.Errorf("notify text = %q, want %q", got.Text, text)
	}
	if got.ParseMode != telego.ModeMarkdown {
		t.Errorf("parse_mode = %q, want %q", got.ParseMode, telego.ModeMarkdown)
	}
}
