// Package notifier broadcasts outage events to subscribed Telegram users,
// mirroring the Python bot's listen() loop (tg-bot/app/bot.py).
package notifier

import (
	"strings"
	"unicode/utf8"

	apidev "github.com/005-bot/apis-go"
	"github.com/005-bot/tg-bot-go/internal/i18n"
)

const (
	// maxMessageLength is Telegram's hard text limit (Python
	// MAX_MESSAGE_LENGTH).
	maxMessageLength = 4096

	// messageSeparatorLength is the length of the "\n\n" between the
	// suffix and the street block (Python "- 2").
	messageSeparatorLength = 2
)

// ellipsis is appended when the street block must be truncated (Python
// ELLIPSIS). It is one rune, matching Python len(ELLIPSIS) == 1.
const ellipsis = "…"

// resourceEmojis maps the known utility types to their Telegram emoji
// prefix, mirroring the Python emojies dict.
//
//nolint:gochecknoglobals // static lookup table, not mutable state
var resourceEmojis = map[apidev.ResourceType]string{
	apidev.ResourceTypeElectricity: "⚡️",
	apidev.ResourceTypeGas:         "🔥",
	apidev.ResourceTypeColdWater:   "❄️🚰",
	apidev.ResourceTypeHotWater:    "🌡️🚰",
	apidev.ResourceTypeHeating:     "♨️",
}

// FormatOutage renders the outage broadcast message in Telegram HTML,
// matching the Python listen() formatter byte for byte:
//
//   - one emoji-prefixed street line per affected street, html-quoted and
//     joined with newlines;
//   - a suffix "<b>{reason}: {RU dates}</b>" that is uppercased as a whole
//     (including the tags, so "<B>...</B>");
//   - the street block truncated to keep the total at 4096 runes, counting
//     runes not bytes so Cyrillic and emoji are never split.
//
// Truncation appends the ellipsis; when the suffix alone exceeds the budget
// the street block degenerates to a bare ellipsis.
func FormatOutage(o apidev.Outage) string {
	lines := make([]string, len(o.Details.Streets))
	for i, street := range o.Details.Streets {
		lines[i] = resourceEmoji(o) + " " + street.String()
	}
	streetsFormatted := i18n.HTMLQuote(strings.Join(lines, "\n"))

	dates := make([]string, len(o.Period))
	for i, d := range o.Period {
		dates[i] = i18n.FormatDateRU(d)
	}
	reasonPrefix := ""
	if o.Details.Reason != nil {
		reasonPrefix = o.Details.Reason.Type + ": "
	}

	suffix := "<b>" + i18n.HTMLQuote(reasonPrefix+strings.Join(dates, " ")) + "</b>"

	maxStreets := maxMessageLength -
		utf8.RuneCountInString(suffix) -
		utf8.RuneCountInString(ellipsis) -
		messageSeparatorLength
	maxStreets = max(maxStreets, 0)

	if utf8.RuneCountInString(streetsFormatted) > maxStreets {
		streetsFormatted = string([]rune(streetsFormatted)[:maxStreets]) + ellipsis
	}

	return strings.ToUpper(suffix) + "\n\n" + streetsFormatted
}

// resourceEmoji returns the emoji prefix for the outage's resource type,
// falling back to "(resource)" for nil or unknown types (Python
// emojies.get(resource_type, f'({resource})')).
func resourceEmoji(o apidev.Outage) string {
	if rt := o.OrganizationInfo.ResourceType; rt != nil {
		if emoji, ok := resourceEmojis[*rt]; ok {
			return emoji
		}
	}
	return "(" + o.OrganizationInfo.Resource + ")"
}
