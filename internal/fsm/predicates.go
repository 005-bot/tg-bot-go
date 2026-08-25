package fsm

import (
	"context"
	"strings"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegohandler"
)

// IsCommand matches updates whose message text starts with '/'. Custom
// predicate: the built-in telegohandler.AnyCommand applies a stricter
// command regexp (it requires a command word after the slash).
func IsCommand() telegohandler.Predicate {
	return func(_ context.Context, update telego.Update) bool {
		return update.Message != nil && strings.HasPrefix(update.Message.Text, "/")
	}
}

// TextNotCommand matches updates with non-empty message text that does not
// start with '/'. Custom predicate: telegohandler.AnyMessageWithText checks
// only text non-emptiness and would also match commands.
func TextNotCommand() telegohandler.Predicate {
	return func(_ context.Context, update telego.Update) bool {
		return update.Message != nil && update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/")
	}
}

// StateFilter matches updates whose chat/user pair has the given FSM state.
// The predicate reads the store per update; a missing state or a storage
// error does not match. telegohandler.AnyMessageWithFrom can be combined
// with this predicate when only sender-initiated messages are relevant.
func StateFilter(store *Store, state string) telegohandler.Predicate {
	return func(ctx context.Context, update telego.Update) bool {
		if update.Message == nil || update.Message.From == nil {
			return false
		}
		got, err := store.GetState(ctx, update.Message.Chat.ID, update.Message.From.ID)
		return err == nil && got == state
	}
}
