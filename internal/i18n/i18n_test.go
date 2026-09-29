package i18n_test

import (
	"testing"
	"time"

	"github.com/005-bot/tg-bot-go/internal/i18n"
)

const (
	hourSeconds = 3600
	utcPlus3    = 3 * hourSeconds
	utcMinus5   = -5 * hourSeconds
)

// TestFormatDateRU pins golden values produced by the shared apis-go/format
// formatter: "18 августа 14:30", lowercase Russian genitive months, and the
// timestamp's own wall clock (no UTC conversion).
func TestFormatDateRU(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"august afternoon", time.Date(2026, time.August, 18, 14, 30, 0, 0, time.UTC), "18 августа 14:30"},
		{"single digit day and hour", time.Date(2026, time.January, 5, 9, 7, 0, 0, time.UTC), "5 января 09:07"},
		{"december evening", time.Date(2025, time.December, 31, 23, 59, 59, 0, time.UTC), "31 декабря 23:59"},
		{"zero time", time.Time{}, "1 января 00:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := i18n.FormatDateRU(tc.in); got != tc.want {
				t.Fatalf("FormatDateRU(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestFormatDateRULowercaseMonths asserts all months render in lowercase
// genitive via the shared formatter, not a local month table.
func TestFormatDateRULowercaseMonths(t *testing.T) {
	want := map[time.Month]string{
		time.January:   "января",
		time.February:  "февраля",
		time.March:     "марта",
		time.April:     "апреля",
		time.May:       "мая",
		time.June:      "июня",
		time.July:      "июля",
		time.August:    "августа",
		time.September: "сентября",
		time.October:   "октября",
		time.November:  "ноября",
		time.December:  "декабря",
	}
	for m, name := range want {
		expected := "1 " + name + " 00:00"
		if got := i18n.FormatDateRU(time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC)); got != expected {
			t.Fatalf("month %v: FormatDateRU = %q, want %q", m, got, expected)
		}
	}
}

// TestFormatDateRUOwnOffset asserts the timestamp's own offset is preserved:
// a UTC+3 timestamp renders its wall clock, not the UTC instant.
func TestFormatDateRUOwnOffset(t *testing.T) {
	instant := time.Date(2026, time.August, 18, 11, 30, 0, 0, time.UTC)

	cases := []struct {
		name string
		loc  *time.Location
		want string
	}{
		{"utc", time.UTC, "18 августа 11:30"},
		{"utc+3", time.FixedZone("UTC+3", utcPlus3), "18 августа 14:30"},
		{"utc-5", time.FixedZone("UTC-5", utcMinus5), "18 августа 06:30"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := i18n.FormatDateRU(instant.In(tc.loc)); got != tc.want {
				t.Fatalf("FormatDateRU in %s = %q, want %q", tc.loc, got, tc.want)
			}
		})
	}
}

// TestHTMLQuote pins aiogram html.quote parity: only &, < and > are escaped
// (in that semantic order); single/double quotes and all other chars are
// left untouched.
func TestHTMLQuote(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"ampersand angle and quotes",
			`ул. "Ленина" & <Мира>`,
			`ул. "Ленина" &amp; &lt;Мира&gt;`,
		},
		{"double quote preserved", `"Ленина"`, `"Ленина"`},
		{"single quote preserved", `'Ленина'`, `'Ленина'`},
		{"empty string", "", ""},
		{"cyrillic and punctuation untouched", `ул. Ленина, д. 5 — ХВС!`, `ул. Ленина, д. 5 — ХВС!`},
		{"ampersand only", "a & b", "a &amp; b"},
		{"less than only", "a < b", "a &lt; b"},
		{"greater than only", "a > b", "a &gt; b"},
		{"already escaped entities not double-escaped", "&amp;", "&amp;amp;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := i18n.HTMLQuote(tc.in); got != tc.want {
				t.Fatalf("HTMLQuote(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
