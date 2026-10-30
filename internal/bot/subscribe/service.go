// Package subscribe implements the street subscription flow shared by the
// /start deep link and the /filter conversation: it normalizes a street
// name, subscribes the user when the match is confident enough, and
// otherwise prompts the user to retry (aiogram parse_and_subscribe parity).
package subscribe

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	address "github.com/005-bot/address-parser-go"
	"github.com/005-bot/tg-bot-go/internal/bot/reply"
	"github.com/005-bot/tg-bot-go/internal/state"
	"github.com/005-bot/tg-bot-go/internal/storage"
	"github.com/go-core-fx/telegofx/fsm"
	tu "github.com/mymmrac/telego/telegoutil"
)

const (
	// minSubscribeConfidence is the parser confidence required to subscribe
	// without asking for confirmation (aiogram parity: 0.85).
	minSubscribeConfidence = 0.85

	noMatchPrompt = "⚠️ Не удалось определить улицу!\n\n" +
		"Пожалуйста, укажите *только название улицы*, например:\n" +
		"- Ленина\n- Мира"

	confirmFormat = "🔍 Вы имели в виду *%s*?\n\n✅ Используйте кнопку для подтверждения\n🔄 Или введите другой вариант."
)

// Service resolves a street name into a subscription for the user. It owns
// the prompts of the flow, so every entry point (/start deep link, /filter
// conversation) answers the user identically.
type Service struct {
	fsmStore      *fsm.Store
	addressParser *address.Parser

	storageSvc *storage.Service
	replySvc   *reply.Service
}

// NewService creates the street subscription service.
func NewService(
	fsmStore *fsm.Store,
	addressParser *address.Parser,
	storageSvc *storage.Service,
	replySvc *reply.Service,
) *Service {
	return &Service{
		fsmStore:      fsmStore,
		addressParser: addressParser,
		storageSvc:    storageSvc,
		replySvc:      replySvc,
	}
}

// ParseAndSubscribe normalizes value and subscribes the user with the
// ORIGINAL database street name when confidence is at least 0.85. Lower
// confidence offers a confirmation keyboard; no match re-asks with the
// filter state retained (aiogram parse_and_subscribe parity).
//
// The boolean result reports that the service has already replied: the
// caller must stop the flow and must not confirm the subscription itself.
// A non-nil match means the user was subscribed.
func (s *Service) ParseAndSubscribe(
	ctx context.Context,
	chatID int64,
	userID int64,
	value string,
) (*address.Match, bool, error) {
	parsed, err := s.addressParser.Normalize(ctx, strings.TrimSpace(value))
	if errors.Is(err, address.ErrNoMatch) {
		if stateErr := s.fsmStore.Set(ctx, chatID, userID, state.FilterState); stateErr != nil {
			return nil, false, fmt.Errorf("set fsm state: %w", stateErr)
		}
		if sendErr := s.replySvc.Send(ctx, chatID, noMatchPrompt); sendErr != nil {
			return nil, false, fmt.Errorf("send no-match prompt: %w", sendErr)
		}
		return nil, true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("normalize street %q: %w", value, err)
	}

	if parsed.Confidence < minSubscribeConfidence {
		if stateErr := s.fsmStore.Set(ctx, chatID, userID, state.FilterState); stateErr != nil {
			return nil, false, fmt.Errorf("set fsm state: %w", stateErr)
		}
		markup := tu.Keyboard(
			tu.KeyboardRow(tu.KeyboardButton(parsed.Name)),
			tu.KeyboardRow(tu.KeyboardButton("Отмена")),
		)
		markup.ResizeKeyboard = true
		markup.OneTimeKeyboard = true
		if sendErr := s.replySvc.SendWithKeyboard(
			ctx,
			chatID,
			fmt.Sprintf(confirmFormat, parsed.Name),
			markup,
		); sendErr != nil {
			return nil, false, fmt.Errorf("send confirmation prompt: %w", sendErr)
		}
		return nil, true, nil
	}

	userKey := strconv.FormatInt(userID, 10)
	if err = s.storageSvc.Subscribe(ctx, userKey, &parsed.Name); err != nil {
		return nil, false, fmt.Errorf("subscribe street for user %s: %w", userKey, err)
	}
	return parsed, false, nil
}
