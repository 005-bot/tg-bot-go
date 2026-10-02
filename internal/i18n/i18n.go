// Package i18n is the bot's formatting integration point: Russian date
// formatting (delegated to the shared apis-go/format library) and
// Telegram-safe HTML escaping.
package i18n

import (
	"strings"
	"time"

	"github.com/005-bot/apis-go/format"
)

// FormatDateRU renders t in its own offset as "18 августа 14:30" using
// lowercase Russian genitive month names. The rule lives in the shared
// apis-go/format package; this wrapper only exposes a stable internal API.
func FormatDateRU(t time.Time) string {
	return format.DateRU(t)
}

// htmlReplacer escapes exactly the three characters aiogram's html.quote
// escapes: & -> &amp;, < -> &lt;, > -> &gt;. Replacements happen in a single
// pass, so quotes and all other characters stay untouched.
//
//nolint:gochecknoglobals // static lookup table, not mutable state
var htmlReplacer = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// HTMLQuote escapes &, < and > for Telegram HTML parse mode, leaving single
// and double quotes untouched (aiogram html.quote parity).
func HTMLQuote(s string) string {
	return htmlReplacer.Replace(s)
}
