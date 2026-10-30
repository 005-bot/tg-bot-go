package handlers_test

import (
	"context"
	"testing"

	"github.com/005-bot/tg-bot-go/internal/state"
)

func TestFeedbackCommand(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, 0)
	env.send(t, updateWithText(1212, 1212, "vasya", "Вася", "/feedback"))

	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	wantChatID(t, user.ChatID, 1212)
	if user.Text != "Что бы Вы хотели нам сказать?" {
		t.Errorf("prompt = %q", user.Text)
	}
	wantKeyboard(t, user.ReplyMarkup, [][]string{{"Ничего"}})

	ctx := context.Background()
	if curState, err := env.fsm.Get(ctx, 1212, 1212); err != nil || curState != state.FeedbackState {
		t.Errorf("fsm state = %q, err %v; want %q", curState, err, state.FeedbackState)
	}
}

func TestFeedbackCancel(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, 0)
	ctx := context.Background()
	if err := env.fsm.Set(ctx, 1313, 1313, state.FeedbackState); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	env.send(t, updateWithText(1313, 1313, "", "Петя", "Ничего"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	wantChatID(t, user.ChatID, 1313)
	if user.Text != "Отзыв не отправлен" {
		t.Errorf("reply = %q", user.Text)
	}
	wantRemoveKeyboard(t, user.ReplyMarkup)

	if state, err := env.fsm.Get(ctx, 1313, 1313); err != nil || state != "" {
		t.Errorf("fsm state = %q, err %v; want empty", state, err)
	}
	if n := len(env.fake.calls("forwardMessage")); n != 0 {
		t.Errorf("forwardMessage calls = %d, want 0", n)
	}
}

func TestFeedbackCancelWithoutStateIsIgnored(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, 0)
	env.send(t, updateWithText(1414, 1414, "", "Петя", "Ничего"))
	settle(t)
	if n := len(env.fake.calls("sendMessage")); n != 0 {
		t.Errorf("sendMessage calls = %d, want 0 (no active feedback state)", n)
	}
	if n := len(env.fake.calls("forwardMessage")); n != 0 {
		t.Errorf("forwardMessage calls = %d, want 0", n)
	}
}

func TestFeedbackValueForwardsToAdmin(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, 4242)
	ctx := context.Background()
	if err := env.fsm.Set(ctx, 1515, 1515, state.FeedbackState); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	env.send(t, updateWithText(1515, 1515, "", "Петя", "Бот сломался"))
	waitFor(t, func() bool { return len(env.fake.calls("forwardMessage")) == 1 })
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	forward := decodeForward(t, env.fake.calls("forwardMessage")[0])
	wantChatID(t, forward.ChatID, 4242)
	wantChatID(t, forward.FromChatID, 1515)
	if forward.MessageID != 10 {
		t.Errorf("message_id = %d, want 10", forward.MessageID)
	}

	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	wantChatID(t, user.ChatID, 1515)
	if user.Text != "Спасибо за отзыв!" {
		t.Errorf("reply = %q", user.Text)
	}
	wantRemoveKeyboard(t, user.ReplyMarkup)

	if state, err := env.fsm.Get(ctx, 1515, 1515); err != nil || state != "" {
		t.Errorf("fsm state = %q, err %v; want empty", state, err)
	}
}

func TestFeedbackValueWithoutAdminDoesNotForward(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, 0)
	ctx := context.Background()
	if err := env.fsm.Set(ctx, 1616, 1616, state.FeedbackState); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	env.send(t, updateWithText(1616, 1616, "", "Петя", "Спасибо, всё работает"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	if user.Text != "Спасибо за отзыв!" {
		t.Errorf("reply = %q", user.Text)
	}
	wantRemoveKeyboard(t, user.ReplyMarkup)
	if n := len(env.fake.calls("forwardMessage")); n != 0 {
		t.Errorf("forwardMessage calls = %d, want 0 (AdminID 0)", n)
	}
	if state, err := env.fsm.Get(ctx, 1616, 1616); err != nil || state != "" {
		t.Errorf("fsm state = %q, err %v; want empty", state, err)
	}
}

func TestFeedbackCancelTextIsValueInFilterState(t *testing.T) {
	t.Parallel()

	// "Ничего" is only a cancel in the feedback state; in the filter state it
	// flows through the filter value handler as a street input (low-confidence
	// match -> confirmation keyboard, state retained).
	env := newTestEnv(t, 0)
	ctx := context.Background()
	if err := env.fsm.Set(ctx, 1717, 1717, state.FilterState); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	env.send(t, updateWithText(1717, 1717, "", "Петя", "Ничего"))
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	match, err := env.parser.Normalize(ctx, "Ничего")
	if err != nil {
		t.Fatalf("parser.Normalize: %v", err)
	}
	user := decodeSend(t, env.fake.calls("sendMessage")[0])
	if user.Text != confirmText(match.Name) {
		t.Errorf("reply = %q, want %q", user.Text, confirmText(match.Name))
	}
	if curState, stateErr := env.fsm.Get(ctx, 1717, 1717); stateErr != nil || curState != state.FilterState {
		t.Errorf("fsm state = %q, err %v; want %q (retained)", curState, stateErr, state.FilterState)
	}
}

func TestFeedbackValueForwardsCancelText(t *testing.T) {
	t.Parallel()

	// In the feedback state "Отмена" is ordinary text and is forwarded.
	env := newTestEnv(t, 4242)
	ctx := context.Background()
	if err := env.fsm.Set(ctx, 1818, 1818, state.FeedbackState); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	env.send(t, updateWithText(1818, 1818, "", "Петя", "Отмена"))
	waitFor(t, func() bool { return len(env.fake.calls("forwardMessage")) == 1 })
	waitFor(t, func() bool { return len(env.fake.calls("sendMessage")) == 1 })

	forward := decodeForward(t, env.fake.calls("forwardMessage")[0])
	wantChatID(t, forward.ChatID, 4242)
	if curState, err := env.fsm.Get(ctx, 1818, 1818); err != nil || curState != "" {
		t.Errorf("fsm state = %q, err %v; want empty", curState, err)
	}
}
