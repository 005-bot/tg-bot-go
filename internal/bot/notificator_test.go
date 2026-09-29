package bot_test

import (
	"context"
	"testing"

	"github.com/005-bot/tg-bot-go/internal/bot"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

func newNotificator(t *testing.T, adminID int64) (*bot.Notificator, *fakeTelegram) {
	t.Helper()
	fake := newFakeTelegram(t)
	teleBot, err := telegofx.New(telegofx.Config{Token: testToken},
		[]telego.BotOption{telego.WithAPIServer(fake.server.URL), telego.WithDiscardLogger()},
		zap.NewNop())
	if err != nil {
		t.Fatalf("telegofx.New: %v", err)
	}
	notificator := bot.NewNotificator(teleBot, bot.Config{AdminID: adminID}, zap.NewNop())
	return notificator, fake
}

func TestNotificatorNewUserTitleFallbacks(t *testing.T) {
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
			notificator, fake := newNotificator(t, 4242)

			user := tt.user
			if err := notificator.NewUser(context.Background(), &user); err != nil {
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

func TestNotificatorAdminDisabled(t *testing.T) {
	notificator, fake := newNotificator(t, 0)
	ctx := context.Background()

	if err := notificator.NewUser(ctx, &telego.User{ID: 333, Username: "vasya"}); err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if err := notificator.Feedback(ctx, &telego.Message{
		MessageID: 7,
		Chat:      telego.Chat{ID: 555, Type: telego.ChatTypePrivate},
	}); err != nil {
		t.Fatalf("Feedback: %v", err)
	}
	if err := notificator.Notify(ctx, "тест"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if n := len(fake.calls("sendMessage")) + len(fake.calls("forwardMessage")); n != 0 {
		t.Errorf("API calls = %d, want 0 (no-op with AdminID 0)", n)
	}
}

func TestNotificatorFeedback(t *testing.T) {
	notificator, fake := newNotificator(t, 4242)

	if err := notificator.Feedback(context.Background(), &telego.Message{
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

func TestNotificatorNotify(t *testing.T) {
	notificator, fake := newNotificator(t, 4242)

	const text = "👤 *Новая подписка:* [vasya](tg://user?id=333)"
	if err := notificator.Notify(context.Background(), text); err != nil {
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
