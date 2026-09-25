package handlers_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/005-bot/tg-bot-go/internal/fsm"
	"github.com/mymmrac/telego"
)

const (
	filterPromptBase = "📍 Введите название улицы для фильтрации уведомлений"

	noMatchPrompt = "⚠️ Не удалось определить улицу!\n\n" +
		"Пожалуйста, укажите *только название улицы*, например:\n" +
		"- Ленина\n- Мира"

	subscribedFormat = "Создана подписка: %s"
)

func confirmText(name string) string {
	return fmt.Sprintf(
		"🔍 Вы имели в виду *%s*?\n\n✅ Используйте кнопку для подтверждения\n🔄 Или введите другой вариант.",
		name,
	)
}

func TestFilterPromptWithoutCurrentValue(t *testing.T) {
	env := newTestEnv(t, 0)
	env.send(t, updateWithText(111, 111, "vasya", "Вася", "/filter"))

	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	wantChatID(t, user.ChatID, 111)
	if user.ParseMode != telego.ModeMarkdown {
		t.Errorf("parse_mode = %q, want %q", user.ParseMode, telego.ModeMarkdown)
	}
	if user.Text != filterPromptBase {
		t.Errorf("prompt = %q, want %q", user.Text, filterPromptBase)
	}
	wantKeyboard(t, user.ReplyMarkup, [][]string{{"Отмена"}})

	ctx := context.Background()
	if state, err := env.fsm.GetState(ctx, 111, 111); err != nil || state != fsm.FilterState {
		t.Errorf("fsm state = %q, err %v; want %q", state, err, fsm.FilterState)
	}
}

func TestFilterPromptWithCurrentValue(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()
	street := "улица Ленина"
	if err := env.storage.Subscribe(ctx, "222", &street); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	env.send(t, updateWithText(222, 222, "petr", "Петя", "/filter"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	want := filterPromptBase + "\n\n*Текущее значение:* улица Ленина"
	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	wantChatID(t, user.ChatID, 222)
	if user.Text != want {
		t.Errorf("prompt = %q, want %q", user.Text, want)
	}
	wantKeyboard(t, user.ReplyMarkup, [][]string{{"Отмена"}})
}

func TestFilterCancelWithStreet(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()
	street := "улица Ленина"
	if err := env.storage.Subscribe(ctx, "333", &street); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := env.fsm.SetState(ctx, 333, 333, fsm.FilterState); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	env.send(t, updateWithText(333, 333, "", "Петя", "Отмена"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	wantChatID(t, user.ChatID, 333)
	if user.Text != "Вы подписаны на уведомления для улица Ленина" {
		t.Errorf("reply = %q", user.Text)
	}
	wantRemoveKeyboard(t, user.ReplyMarkup)

	if state, err := env.fsm.GetState(ctx, 333, 333); err != nil || state != "" {
		t.Errorf("fsm state = %q, err %v; want empty", state, err)
	}
	filter, err := env.storage.GetFilter(ctx, "333")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Street == nil || *filter.Street != street {
		t.Errorf("street = %v, want %q (cancel must not clear the filter)", filter.Street, street)
	}
}

func TestFilterCancelWithoutStreet(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()
	if err := env.fsm.SetState(ctx, 444, 444, fsm.FilterState); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	env.send(t, updateWithText(444, 444, "", "Петя", "Отмена"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	wantChatID(t, user.ChatID, 444)
	if user.Text != "Вы подписаны на все уведомления" {
		t.Errorf("reply = %q", user.Text)
	}
	wantRemoveKeyboard(t, user.ReplyMarkup)

	if state, err := env.fsm.GetState(ctx, 444, 444); err != nil || state != "" {
		t.Errorf("fsm state = %q, err %v; want empty", state, err)
	}
}

func TestFilterValueSubscribesOriginalName(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()
	env.send(t, updateWithText(555, 555, "", "Петя", "/filter"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	// "Ленина" fuzzy-matches at exactly 0.85 (boundary): subscribes with the
	// ORIGINAL database name, not the typed text.
	env.send(t, updateWithText(555, 555, "", "Петя", "Ленина"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 2 })

	match, err := env.parser.Normalize(ctx, "Ленина")
	if err != nil {
		t.Fatalf("parser.Normalize: %v", err)
	}

	sends := env.fake.calls("sendMessage")
	user := decodeSend(t, sends[1])
	wantChatID(t, user.ChatID, 555)
	wantText := fmt.Sprintf(subscribedFormat, match.Name)
	if user.Text != wantText {
		t.Errorf("reply = %q, want %q", user.Text, wantText)
	}
	wantRemoveKeyboard(t, user.ReplyMarkup)

	filter, err := env.storage.GetFilter(ctx, "555")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Street == nil || *filter.Street != match.Name {
		t.Errorf("street = %v, want %q (original name)", filter.Street, match.Name)
	}
	if state, stateErr := env.fsm.GetState(ctx, 555, 555); stateErr != nil || state != "" {
		t.Errorf("fsm state = %q, err %v; want empty (cleared)", state, stateErr)
	}
}

func TestFilterValueExactMatch(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()
	env.send(t, updateWithText(666, 666, "", "Петя", "/filter"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	env.send(t, updateWithText(666, 666, "", "Петя", "улица Ленина"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 2 })

	filter, err := env.storage.GetFilter(ctx, "666")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Street == nil || *filter.Street != "улица Ленина" {
		t.Errorf("street = %v, want %q", filter.Street, "улица Ленина")
	}
}

func TestFilterValueLowConfidence(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()
	env.send(t, updateWithText(777, 777, "", "Петя", "/filter"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	env.send(t, updateWithText(777, 777, "", "Петя", "Мира"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 2 })

	match, err := env.parser.Normalize(ctx, "Мира")
	if err != nil {
		t.Fatalf("parser.Normalize: %v", err)
	}
	if match.Confidence >= 0.85 {
		t.Fatalf("fixture: expected confidence < 0.85, got %f", match.Confidence)
	}

	user := decodeSend(t, env.fake.calls("sendMessage")[1])
	wantChatID(t, user.ChatID, 777)
	if user.Text != confirmText(match.Name) {
		t.Errorf("confirmation = %q, want %q", user.Text, confirmText(match.Name))
	}
	wantKeyboard(t, user.ReplyMarkup, [][]string{{match.Name}, {"Отмена"}})

	if state, stateErr := env.fsm.GetState(ctx, 777, 777); stateErr != nil || state != fsm.FilterState {
		t.Errorf("fsm state = %q, err %v; want %q (retained)", state, stateErr, fsm.FilterState)
	}
	filter, err := env.storage.GetFilter(ctx, "777")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Street != nil {
		t.Errorf("street = %v, want nil (no subscription yet)", *filter.Street)
	}
}

func TestFilterValueNoMatch(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()
	env.send(t, updateWithText(888, 888, "", "Петя", "/filter"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	// '!!!' and whitespace-only input both surface as ErrNoMatch.
	for _, input := range []string{"!!!", " "} {
		env.send(t, updateWithText(888, 888, "", "Петя", input))
		waitFor(t, func() bool {
			return len(env.fake.calls("sendMessage")) == 2
		})
		if state, stateErr := env.fsm.GetState(ctx, 888, 888); stateErr != nil || state != fsm.FilterState {
			t.Fatalf("after %q: fsm state = %q, err %v; want %q (retained)", input, state, stateErr, fsm.FilterState)
		}
	}

	for _, call := range env.fake.calls("sendMessage")[1:] {
		user := decodeSend(t, call)
		wantChatID(t, user.ChatID, 888)
		if user.Text != noMatchPrompt {
			t.Errorf("no-match reply = %q, want %q", user.Text, noMatchPrompt)
		}
		// Keyboard stays visible: no reply markup on the error prompt.
		if user.ReplyMarkup != nil {
			t.Errorf("unexpected reply markup on error prompt: %+v", user.ReplyMarkup)
		}
	}
}

func TestFilterConfirmationTap(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()
	env.send(t, updateWithText(999, 999, "", "Петя", "/filter"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	env.send(t, updateWithText(999, 999, "", "Петя", "Мира"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 2 })

	match, err := env.parser.Normalize(ctx, "Мира")
	if err != nil {
		t.Fatalf("parser.Normalize: %v", err)
	}
	button := decodeSend(t, env.fake.calls("sendMessage")[1])
	buttonName := button.ReplyMarkup.Keyboard[0][0].Text
	if buttonName != match.Name {
		t.Fatalf("fixture: keyboard button %q != parsed name %q", buttonName, match.Name)
	}

	// Tapping the suggestion sends the exact DB name: Normalize must return
	// confidence 1.0 so the tap subscribes immediately.
	exact, err := env.parser.Normalize(ctx, buttonName)
	if err != nil {
		t.Fatalf("Normalize(%q): %v", buttonName, err)
	}
	if exact.Confidence != 1.0 {
		t.Fatalf("fixture: expected confidence 1.0 for %q, got %f", buttonName, exact.Confidence)
	}

	env.send(t, updateWithText(999, 999, "", "Петя", buttonName))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 3 })

	sends := env.fake.calls("sendMessage")
	user := decodeSend(t, sends[2])
	wantChatID(t, user.ChatID, 999)
	wantText := fmt.Sprintf(subscribedFormat, match.Name)
	if user.Text != wantText {
		t.Errorf("reply = %q, want %q", user.Text, wantText)
	}
	wantRemoveKeyboard(t, user.ReplyMarkup)

	filter, err := env.storage.GetFilter(ctx, "999")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Street == nil || *filter.Street != match.Name {
		t.Errorf("street = %v, want %q", filter.Street, match.Name)
	}
	if state, stateErr := env.fsm.GetState(ctx, 999, 999); stateErr != nil || state != "" {
		t.Errorf("fsm state = %q, err %v; want empty", state, stateErr)
	}
}

func TestFilterCommandDuringFeedbackState(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()
	if err := env.fsm.SetState(ctx, 1010, 1010, fsm.FeedbackState); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	env.send(t, updateWithText(1010, 1010, "", "Петя", "/filter"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	if user.Text != filterPromptBase {
		t.Errorf("prompt = %q, want %q", user.Text, filterPromptBase)
	}
	if state, err := env.fsm.GetState(ctx, 1010, 1010); err != nil || state != fsm.FilterState {
		t.Errorf("fsm state = %q, err %v; want %q (switched)", state, err, fsm.FilterState)
	}
}

func TestStopDuringFilterState(t *testing.T) {
	env := newTestEnv(t, 0)
	ctx := context.Background()
	if err := env.fsm.SetState(ctx, 1919, 1919, fsm.FilterState); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	env.send(t, updateWithText(1919, 1919, "", "Петя", "/stop"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	wantChatID(t, user.ChatID, 1919)
	if user.Text != "🔕 Вы *отписались* от уведомлений об отключениях" {
		t.Errorf("reply = %q", user.Text)
	}
	// Python /stop does not clear the FSM state.
	if state, stateErr := env.fsm.GetState(ctx, 1919, 1919); stateErr != nil || state != fsm.FilterState {
		t.Errorf("fsm state = %q, err %v; want %q (untouched)", state, stateErr, fsm.FilterState)
	}
}
