package handlers_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"sync"
	"testing"
	"time"

	address "github.com/005-bot/address-parser-go"
	"github.com/005-bot/tg-bot-go/internal/bot"
	"github.com/005-bot/tg-bot-go/internal/bot/handler"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/feedback"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/filter"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/help"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/start"
	"github.com/005-bot/tg-bot-go/internal/bot/handlers/stop"
	"github.com/005-bot/tg-bot-go/internal/fsm"
	"github.com/005-bot/tg-bot-go/internal/storage"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-core-fx/telegofx"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	testToken   = "123456:abcdefghijklmnopqrstuvwxyzABCDEFGHI"
	waitTimeout = 10 * time.Second
)

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
	ChatID      any          `json:"chat_id"`
	Text        string       `json:"text"`
	ParseMode   string       `json:"parse_mode"`
	ReplyMarkup *replyMarkup `json:"reply_markup"`
}

type replyMarkup struct {
	Keyboard        [][]keyboardButton `json:"keyboard"`
	ResizeKeyboard  bool               `json:"resize_keyboard"`
	OneTimeKeyboard bool               `json:"one_time_keyboard"`
	RemoveKeyboard  *bool              `json:"remove_keyboard"`
}

type keyboardButton struct {
	Text string `json:"text"`
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

// wantKeyboard asserts the reply keyboard rows and one-time/resize flags.
func wantKeyboard(t *testing.T, got *replyMarkup, rows [][]string) {
	t.Helper()
	if got == nil {
		t.Fatal("expected reply keyboard, got nil")
	}
	if len(got.Keyboard) != len(rows) {
		t.Fatalf("keyboard rows = %d, want %d", len(got.Keyboard), len(rows))
	}
	for i := range rows {
		gotRow := got.Keyboard[i]
		if len(gotRow) != 1 || gotRow[0].Text != rows[i][0] {
			t.Errorf("keyboard row %d = %+v, want %q", i, gotRow, rows[i][0])
		}
	}
	if !got.ResizeKeyboard || !got.OneTimeKeyboard {
		t.Errorf("keyboard flags = resize %v, one_time %v; want both true",
			got.ResizeKeyboard, got.OneTimeKeyboard)
	}
}

func wantRemoveKeyboard(t *testing.T, got *replyMarkup) {
	t.Helper()
	if got == nil {
		t.Fatal("expected reply markup, got nil")
	}
	if got.RemoveKeyboard == nil || !*got.RemoveKeyboard {
		t.Errorf("remove_keyboard = %v, want true", got.RemoveKeyboard)
	}
}

// testEnv wires the full handler stack against a fake Telegram API, a real
// address parser, and miniredis-backed storage/fsm.
type testEnv struct {
	fake    *fakeTelegram
	bot     *telegofx.Bot
	router  *telegofx.Router
	updates chan telego.Update
	storage *storage.Service
	fsm     *fsm.Store
	parser  *address.Parser
	mr      *miniredis.Miniredis
}

func newTestEnv(t *testing.T, adminID int64) *testEnv {
	t.Helper()

	fake := newFakeTelegram(t)
	teleBot, err := telegofx.New(telegofx.Config{Token: testToken},
		[]telego.BotOption{telego.WithAPIServer(fake.server.URL), telego.WithDiscardLogger()},
		zap.NewNop())
	if err != nil {
		t.Fatalf("telegofx.New: %v", err)
	}

	updates := make(chan telego.Update)
	bh, err := th.NewBotHandler(teleBot.Bot, updates)
	if err != nil {
		t.Fatalf("th.NewBotHandler: %v", err)
	}
	router := &telegofx.Router{BotHandler: bh}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	svc := storage.NewService(rdb, storage.Config{Prefix: "bot-005"}, zap.NewNop())
	store := fsm.NewStore(rdb, fsm.Config{Prefix: "bot-005"})

	parser, err := address.NewParser(address.Config{})
	if err != nil {
		t.Fatalf("address.NewParser: %v", err)
	}
	t.Cleanup(parser.Stop)

	env := &testEnv{
		fake:    fake,
		bot:     teleBot,
		router:  router,
		updates: updates,
		storage: svc,
		fsm:     store,
		parser:  parser,
		mr:      mr,
	}

	reply := bot.NewReply(teleBot, zap.NewNop())
	notificator := bot.NewNotificator(teleBot, bot.Config{AdminID: adminID}, zap.NewNop())

	router.Use(bot.ErrorMiddleware(reply, zap.NewNop()))
	register := []handler.Handler{
		start.New(zap.NewNop(), svc, store, parser, reply, notificator),
		stop.New(zap.NewNop(), svc, reply),
		help.New(zap.NewNop(), reply),
		filter.New(zap.NewNop(), svc, store, parser, reply),
		feedback.New(zap.NewNop(), store, reply, notificator),
	}
	for _, h := range register {
		h.Register(router)
	}

	go func() { _ = router.Start() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = router.Stop(ctx)
	})

	return env
}

// send pushes an update through the live telegohandler pipeline.
func (env *testEnv) send(t *testing.T, update telego.Update) {
	t.Helper()
	env.updates <- update
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within", waitTimeout)
}

func settle(t *testing.T) {
	t.Helper()
	time.Sleep(150 * time.Millisecond)
}

func updateWithText(chatID, userID int64, username, firstName, text string) telego.Update {
	return telego.Update{
		UpdateID: 1,
		Message: &telego.Message{
			MessageID: 10,
			Chat:      telego.Chat{ID: chatID, Type: telego.ChatTypePrivate},
			From:      &telego.User{ID: userID, Username: username, FirstName: firstName},
			Text:      text,
		},
	}
}
