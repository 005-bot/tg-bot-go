package bot_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"sync"
	"testing"
	"time"

	address "github.com/005-bot/address-parser-go"
	"github.com/005-bot/tg-bot-go/internal/bot"
	boterr "github.com/005-bot/tg-bot-go/internal/bot/errors"
	"github.com/005-bot/tg-bot-go/internal/bot/handler"
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

	welcomeNoStreet = "✅ Вы подписаны на уведомления об отключениях\n\n" +
		"🔍 Чтобы получать уведомления только по конкретной улице, используйте /filter\n\n" +
		"ℹ️ Источник информации об отключениях: https://005красноярск.рф"

	noMatchPrompt = "⚠️ Не удалось определить улицу!\n\n" +
		"Пожалуйста, укажите *только название улицы*, например:\n" +
		"- Ленина\n- Мира"

	systemErrorText = "🚨 Системная ошибка - наша команда уведомлена"

	helpText = "👋 *Добро пожаловать!*\n\n" +
		"📋 **Доступные команды:**\n" +
		"- /start - Подписаться на уведомления об отключениях 🔔\n" +
		"- /stop - Отписаться от уведомлений 🔕\n" +
		"- /filter - Подписаться на уведомления по конкретной улице 🏘️\n" +
		"- /feedback - Оставить отзыв 💬\n" +
		"- /help - Показать эту справку ❓\n\n" +
		"📬 *По всем вопросам:*\n" +
		"Пишите нам: help@xn--005-ddd9dya.xn--p1ai\n\n" +
		"ℹ️ *Источник информации об отключениях:*\n" +
		"https://005красноярск.рф"
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
	}
	for _, h := range register {
		h.Register(router)
	}

	// Error-middleware mapping probes.
	router.Handle(func(*th.Context, telego.Update) error {
		return boterr.NewUserInputError(errors.New("invalid street name"))
	}, th.CommandEqual("boom_input"), th.AnyMessageWithFrom())
	router.Handle(func(*th.Context, telego.Update) error {
		return boterr.NewAPIError(errors.New("telegram api down"))
	}, th.CommandEqual("boom_api"), th.AnyMessageWithFrom())
	router.Handle(func(*th.Context, telego.Update) error {
		return errors.New("boom generic")
	}, th.CommandEqual("boom_other"), th.AnyMessageWithFrom())

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

func deepLinkPayload(t *testing.T, street string) string {
	t.Helper()
	encoded := base64.RawURLEncoding.EncodeToString([]byte(street))
	return "/start " + encoded + "==" // trailing padding must be tolerated
}

func TestStartNoPayload(t *testing.T) {
	env := newTestEnv(t, 4242)
	env.send(t, updateWithText(111, 111, "vasya", "Вася", "/start"))

	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 2 })

	sends := env.fake.calls("sendMessage")
	user := decodeSend(t, sends[0])
	wantChatID(t, user.ChatID, 111)
	if user.ParseMode != telego.ModeMarkdown {
		t.Errorf("parse_mode = %q, want %q", user.ParseMode, telego.ModeMarkdown)
	}
	if user.Text != welcomeNoStreet {
		t.Errorf("welcome text mismatch:\n got %q\nwant %q", user.Text, welcomeNoStreet)
	}
	if user.ReplyMarkup != nil {
		t.Errorf("unexpected reply markup: %+v", user.ReplyMarkup)
	}

	admin := decodeSend(t, sends[1])
	wantChatID(t, admin.ChatID, 4242)
	if admin.Text != "👤 *Новая подписка:* [vasya](tg://user?id=111)" {
		t.Errorf("admin notify = %q", admin.Text)
	}

	ctx := context.Background()
	filter, err := env.storage.GetFilter(ctx, "111")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Street != nil {
		t.Errorf("street = %v, want nil", *filter.Street)
	}
	state, err := env.fsm.GetState(ctx, 111, 111)
	if err != nil || state != "" {
		t.Errorf("fsm state = %q, err %v; want empty", state, err)
	}
}

func TestStartValidStreetExact(t *testing.T) {
	env := newTestEnv(t, 4242)
	env.send(t, updateWithText(222, 222, "petr", "Петя", deepLinkPayload(t, "улица Ленина")))

	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 2 })

	match, err := env.parser.Normalize(context.Background(), "улица Ленина")
	if err != nil {
		t.Fatalf("parser.Normalize: %v", err)
	}
	if match.Confidence < 0.85 {
		t.Fatalf("fixture: expected confidence >= 0.85, got %f", match.Confidence)
	}

	sends := env.fake.calls("sendMessage")
	user := decodeSend(t, sends[0])
	wantChatID(t, user.ChatID, 222)
	wantText := fmt.Sprintf(
		"✅ Вы подписаны на уведомления об отключениях по адресу:\n*%s*\n\n"+
			"ℹ️ Источник информации об отключениях: https://005красноярск.рф", match.Name)
	if user.Text != wantText {
		t.Errorf("welcome text mismatch:\n got %q\nwant %q", user.Text, wantText)
	}

	admin := decodeSend(t, sends[1])
	wantChatID(t, admin.ChatID, 4242)
	if admin.Text != "👤 *Новая подписка:* [petr](tg://user?id=222)" {
		t.Errorf("admin notify = %q", admin.Text)
	}

	ctx := context.Background()
	filter, err := env.storage.GetFilter(ctx, "222")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Street == nil || *filter.Street != match.Name {
		t.Errorf("street = %v, want %q (original name)", filter.Street, match.Name)
	}
}

func TestStartBoundaryConfidenceSubscribes(t *testing.T) {
	// "Ленина" fuzzy-matches "улица Ленина" at exactly 0.85: the
	// subscription branch requires confidence >= 0.85.
	env := newTestEnv(t, 0)
	env.send(t, updateWithText(333, 333, "", "Петя", "/start "+base64.RawURLEncoding.EncodeToString([]byte("Ленина"))))

	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	match, err := env.parser.Normalize(context.Background(), "Ленина")
	if err != nil {
		t.Fatalf("parser.Normalize: %v", err)
	}
	if match.Confidence < 0.85 {
		t.Fatalf("fixture: expected confidence >= 0.85, got %f", match.Confidence)
	}

	ctx := context.Background()
	filter, err := env.storage.GetFilter(ctx, "333")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Street == nil || *filter.Street != match.Name {
		t.Errorf("street = %v, want %q", filter.Street, match.Name)
	}
}

func TestStartLowConfidence(t *testing.T) {
	env := newTestEnv(t, 4242)
	env.send(t, updateWithText(444, 444, "", "Петя", "/start "+base64.RawURLEncoding.EncodeToString([]byte("Мира"))))

	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	match, err := env.parser.Normalize(context.Background(), "Мира")
	if err != nil {
		t.Fatalf("parser.Normalize: %v", err)
	}
	if match.Confidence < 0.6 || match.Confidence >= 0.85 {
		t.Fatalf("fixture: expected 0.6 <= confidence < 0.85, got %f", match.Confidence)
	}

	sends := env.fake.calls("sendMessage")
	user := decodeSend(t, sends[0])
	wantChatID(t, user.ChatID, 444)
	wantText := fmt.Sprintf(
		"🔍 Вы имели в виду *%s*?\n\n✅ Используйте кнопку для подтверждения\n🔄 Или введите другой вариант.",
		match.Name)
	if user.Text != wantText {
		t.Errorf("confirmation text mismatch:\n got %q\nwant %q", user.Text, wantText)
	}
	if user.ReplyMarkup == nil {
		t.Fatal("expected reply keyboard, got nil")
	}
	wantKeyboard := [][]string{{match.Name}, {"Отмена"}}
	if len(user.ReplyMarkup.Keyboard) != len(wantKeyboard) {
		t.Fatalf("keyboard rows = %d, want %d", len(user.ReplyMarkup.Keyboard), len(wantKeyboard))
	}
	for i := range wantKeyboard {
		gotRow := user.ReplyMarkup.Keyboard[i]
		if len(gotRow) != 1 || gotRow[0].Text != wantKeyboard[i][0] {
			t.Errorf("keyboard row %d = %+v, want %q", i, gotRow, wantKeyboard[i])
		}
	}
	if !user.ReplyMarkup.ResizeKeyboard || !user.ReplyMarkup.OneTimeKeyboard {
		t.Errorf("keyboard flags = resize %v, one_time %v; want both true",
			user.ReplyMarkup.ResizeKeyboard, user.ReplyMarkup.OneTimeKeyboard)
	}

	ctx := context.Background()
	state, err := env.fsm.GetState(ctx, 444, 444)
	if err != nil || state != fsm.FilterState {
		t.Errorf("fsm state = %q, err %v; want %q", state, err, fsm.FilterState)
	}
	filter, err := env.storage.GetFilter(ctx, "444")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Street != nil {
		t.Errorf("street = %v, want nil (subscribed before parse)", *filter.Street)
	}
	settle(t)
	if n := len(env.fake.calls("sendMessage")); n != 1 {
		t.Errorf("sendMessage calls = %d, want 1 (no admin notify, no welcome)", n)
	}
}

func TestStartNoMatch(t *testing.T) {
	env := newTestEnv(t, 4242)
	env.send(t, updateWithText(555, 555, "", "Петя",
		"/start "+base64.RawURLEncoding.EncodeToString([]byte("абвгдежзиклмнопрст"))))

	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	sends := env.fake.calls("sendMessage")
	user := decodeSend(t, sends[0])
	wantChatID(t, user.ChatID, 555)
	if user.Text != noMatchPrompt {
		t.Errorf("no-match text mismatch:\n got %q\nwant %q", user.Text, noMatchPrompt)
	}

	ctx := context.Background()
	if state, err := env.fsm.GetState(ctx, 555, 555); err != nil || state != fsm.FilterState {
		t.Errorf("fsm state = %q, err %v; want %q", state, err, fsm.FilterState)
	}
	filter, err := env.storage.GetFilter(ctx, "555")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Street != nil {
		t.Errorf("street = %v, want nil (subscribed before parse)", *filter.Street)
	}
	settle(t)
	if n := len(env.fake.calls("sendMessage")); n != 1 {
		t.Errorf("sendMessage calls = %d, want 1 (no welcome, no admin notify)", n)
	}
}

func TestStartMalformedPayload(t *testing.T) {
	env := newTestEnv(t, 4242)
	env.send(t, updateWithText(666, 666, "", "Петя", "/start !!!!"))

	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	sends := env.fake.calls("sendMessage")
	user := decodeSend(t, sends[0])
	wantChatID(t, user.ChatID, 666)
	if user.Text != systemErrorText {
		t.Errorf("reply = %q, want %q", user.Text, systemErrorText)
	}

	ctx := context.Background()
	if exists := env.mr.Exists("bot-005:filters"); exists {
		t.Errorf("filters key exists = %v; want absent (no subscription)", exists)
	}
	if state, err := env.fsm.GetState(ctx, 666, 666); err != nil || state != "" {
		t.Errorf("fsm state = %q, err %v; want empty", state, err)
	}
	settle(t)
	if n := len(env.fake.calls("sendMessage")); n != 1 {
		t.Errorf("sendMessage calls = %d, want 1 (no admin notify, no welcome)", n)
	}
}

func TestStop(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()
	street := "улица Ленина"
	if err := env.storage.Subscribe(ctx, "777", &street); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := env.fsm.SetState(ctx, 777, 777, fsm.FilterState); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	env.send(t, updateWithText(777, 777, "", "Петя", "/stop"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	wantChatID(t, user.ChatID, 777)
	if user.Text != "🔕 Вы *отписались* от уведомлений об отключениях" {
		t.Errorf("reply = %q", user.Text)
	}

	filter, err := env.storage.GetFilter(ctx, "777")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Street != nil {
		t.Errorf("street = %v, want nil after unsubscribe", *filter.Street)
	}
	// FSM state must stay untouched.
	state, err := env.fsm.GetState(ctx, 777, 777)
	if err != nil || state != fsm.FilterState {
		t.Errorf("fsm state = %q, err %v; want %q (untouched)", state, err, fsm.FilterState)
	}
}

func TestHelp(t *testing.T) {
	env := newTestEnv(t, 0)
	env.send(t, updateWithText(888, 888, "", "Петя", "/help"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	wantChatID(t, user.ChatID, 888)
	if user.Text != helpText {
		t.Errorf("help text mismatch:\n got %q\nwant %q", user.Text, helpText)
	}
}

func TestErrorMiddlewareMapping(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()

	cases := []struct {
		command string
		want    string
	}{
		{command: "/boom_input", want: "⚠️ Ошибка ввода: invalid street name"},
		{command: "/boom_api", want: "🔧 Временная проблема с сервисом, пожалуйста, попробуйте позже"},
		{command: "/boom_other", want: systemErrorText},
	}

	for i, tc := range cases {
		env.send(t, updateWithText(999, 999, "", "Петя", tc.command))
		waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == i+1 })

		sends := env.fake.calls("sendMessage")
		reply := decodeSend(t, sends[i])
		wantChatID(t, reply.ChatID, 999)
		if reply.Text != tc.want {
			t.Errorf("case %s: reply = %q, want %q", tc.command, reply.Text, tc.want)
		}
		if reply.ParseMode != telego.ModeMarkdown {
			t.Errorf("case %s: parse_mode = %q, want %q", tc.command, reply.ParseMode, telego.ModeMarkdown)
		}
	}

	// The error middleware must consume errors: no further sends happen.
	settle(t)
	if n := len(env.fake.calls("sendMessage")); n != len(cases) {
		t.Errorf("sendMessage calls = %d, want %d", n, len(cases))
	}
	if _, err := env.fsm.GetState(ctx, 999, 999); err != nil {
		t.Fatalf("GetState: %v", err)
	}
}

func TestStartPayloadPaddingToleranceAndEmpty(t *testing.T) {
	env := newTestEnv(t, 0)
	// A payload that decodes to an empty string behaves like no payload.
	env.send(t, updateWithText(1010, 1010, "bob", "Боб", "/start =="))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	if user.Text != welcomeNoStreet {
		t.Errorf("empty payload reply = %q, want no-payload welcome", user.Text)
	}

	ctx := context.Background()
	filter, err := env.storage.GetFilter(ctx, "1010")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Street != nil {
		t.Errorf("street = %v, want nil", *filter.Street)
	}
	state, err := env.fsm.GetState(ctx, 1010, 1010)
	if err != nil || state != "" {
		t.Errorf("fsm state = %q, err %v; want empty", state, err)
	}
}
