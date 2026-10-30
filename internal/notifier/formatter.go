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

	// suffixTagsLength is the length of the "<B>" and "</B>" tags that
	// wrap the reason and the dates.
	suffixTagsLength = len("<B></B>")

	// searchDivisor halves the search range of the binary search that
	// finds the longest prefix fitting the budget.
	searchDivisor = 2
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
// mirroring the Python listen() formatter (the deliberate differences are
// documented below):
//
//   - one emoji-prefixed street line per affected street, html-quoted and
//     joined with newlines;
//   - a suffix "<B>{reason}: {RU dates}</B>", uppercased and html-quoted;
//   - the street block truncated to keep the total at 4096 runes, counting
//     runes not bytes so Cyrillic and emoji are never split.
//
// Truncation appends the ellipsis; when the suffix alone exceeds the budget
// the street block degenerates to a bare ellipsis.
//
// Three deliberate differences from the Python formatter (tg-bot/app/bot.py):
//
// Truncation runs on the raw street text and the result is html-quoted
// afterwards, so the cut can never land inside an entity such as "&amp;".
// Python quotes first and then slices the quoted string, which can emit a
// broken entity that Telegram rejects ("can't parse entities"); that failure
// would abort the broadcast for every subscriber.
//
// The suffix is uppercased before it is html-quoted. Python quotes first and
// uppercases the quoted text, so an escaped "&lt;" becomes "&LT;"; Telegram's
// HTML parser only accepts the lowercase entities ("&lt;", "&gt;", "&amp;",
// "&quot;") and rejects the whole message with "can't parse entities", which
// again aborts the broadcast for every subscriber.
//
// The suffix is also bounded: Telegram accepts at most 4096 characters after
// it has parsed the entities, so a reason that is long enough to push the
// message past the limit would be rejected with "message is too long" and the
// outage would reach nobody. The reason is truncated to fit, the cut runs on
// the raw text, and the dates keep their budget because they carry the
// outage time. Python applies no bound at all.
func FormatOutage(o apidev.Outage) string {
	lines := make([]string, len(o.Details.Streets))
	for i, street := range o.Details.Streets {
		lines[i] = resourceEmoji(o) + " " + street.String()
	}
	streetsRaw := strings.Join(lines, "\n")

	dates := make([]string, len(o.Period))
	for i, d := range o.Period {
		dates[i] = i18n.FormatDateRU(d)
	}

	// The suffix has to leave room for the separator and the street
	// ellipsis, otherwise the message is longer than Telegram accepts and
	// the whole broadcast is rejected ("message is too long"). The reason
	// comes from the outage API and is not length-checked, so it is
	// truncated to fit; the dates keep their budget because they carry the
	// outage time.
	maxSuffix := maxMessageLength - messageSeparatorLength - utf8.RuneCountInString(ellipsis)

	datesPart := truncateToFit(strings.ToUpper(strings.Join(dates, " ")), maxSuffix-suffixTagsLength)

	label := ""
	if o.Details.Reason != nil {
		label = o.Details.Reason.Type + ": "
	}
	labelPart := truncateToFit(strings.ToUpper(label), maxSuffix-suffixTagsLength-quotedLen(datesPart))

	suffix := "<B>" + i18n.HTMLQuote(labelPart+datesPart) + "</B>"

	maxStreets := maxMessageLength -
		utf8.RuneCountInString(suffix) -
		utf8.RuneCountInString(ellipsis) -
		messageSeparatorLength
	maxStreets = max(maxStreets, 0)

	if utf8.RuneCountInString(streetsRaw) > maxStreets {
		streetsRaw = string([]rune(streetsRaw)[:maxStreets]) + ellipsis
	}
	streetsFormatted := i18n.HTMLQuote(streetsRaw)

	return suffix + "\n\n" + streetsFormatted
}

// truncateToFit returns the longest prefix of raw whose html-quoted form is
// at most budget runes long, with the ellipsis appended when raw was cut.
// The cut runs on the raw text, so the quoted result never holds a partial
// entity.
func truncateToFit(raw string, budget int) string {
	if budget <= 0 {
		return ""
	}

	runes := []rune(raw)
	// i18n.HTMLQuote only replaces single characters, so the quoted length
	// of a prefix is the sum of the quoted lengths of its runes.
	lengths := make([]int, len(runes)+1)
	for i, r := range runes {
		lengths[i+1] = lengths[i] + utf8.RuneCountInString(i18n.HTMLQuote(string(r)))
	}
	if lengths[len(runes)] <= budget {
		return raw
	}

	// The text is cut, so the ellipsis is part of the budget.
	limit := budget - utf8.RuneCountInString(ellipsis)

	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / searchDivisor
		if lengths[mid] <= limit {
			lo = mid
		} else {
			hi = mid - 1
		}
	}

	return string(runes[:lo]) + ellipsis
}

// quotedLen returns the rune count of the html-quoted form of s.
func quotedLen(s string) int {
	n := 0
	for _, r := range s {
		n += utf8.RuneCountInString(i18n.HTMLQuote(string(r)))
	}
	return n
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
