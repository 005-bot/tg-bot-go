package bot_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/005-bot/tg-bot-go/internal/bot"
)

func TestMapError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "bare user input error",
			err:  bot.NewUserInputError(errors.New("invalid street name")),
			want: "⚠️ Ошибка ввода: invalid street name",
		},
		{
			// The reply must come from the matched UserInputError, not the
			// outer chain, so internal wrapping context never reaches the user.
			name: "wrapped user input error hides wrapper context",
			err:  fmt.Errorf("filter: %w", bot.NewUserInputError(errors.New("invalid street name"))),
			want: "⚠️ Ошибка ввода: invalid street name",
		},
		{
			name: "bare api error",
			err:  bot.NewAPIError(errors.New("telegram api down")),
			want: "🔧 Временная проблема с сервисом, пожалуйста, попробуйте позже",
		},
		{
			name: "wrapped api error",
			err:  fmt.Errorf("webhook: %w", bot.NewAPIError(errors.New("telegram api down"))),
			want: "🔧 Временная проблема с сервисом, пожалуйста, попробуйте позже",
		},
		{
			name: "deeply nested user input error",
			err: fmt.Errorf("outer: %w", fmt.Errorf("inner: %w",
				bot.NewUserInputError(errors.New("invalid street name")))),
			want: "⚠️ Ошибка ввода: invalid street name",
		},
		{
			name: "unknown error",
			err:  errors.New("boom"),
			want: "🚨 Системная ошибка - наша команда уведомлена",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := bot.MapError(tt.err); got != tt.want {
				t.Errorf("MapError() = %q, want %q", got, tt.want)
			}
		})
	}
}
