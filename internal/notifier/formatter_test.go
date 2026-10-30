package notifier_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	apidev "github.com/005-bot/apis-go"
	"github.com/005-bot/tg-bot-go/internal/notifier"
)

const ellipsis = "…"

// sampleOutage is the fixture outage: cold water, one street with a
// building, a repair reason and two UTC dates.
func sampleOutage() apidev.Outage {
	return apidev.Outage{
		Area: "Тест",
		OrganizationInfo: apidev.OrganizationInfo{
			ResourceType: new(apidev.ResourceTypeColdWater),
			Resource:     "ХВС",
			Organization: "Тестовая",
			Phones:       []string{"+7 111"},
		},
		Details: apidev.OutageDetails{
			Streets: []apidev.Street{
				{Name: "ул. Ленина", Buildings: []string{"1"}},
			},
			Reason: &apidev.Reason{Type: "Ремонт", Description: "Тест"},
		},
		Period: []time.Time{
			time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
			time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC),
		},
	}
}

// wantFullOutage is the exact byte output for sampleOutage: uppercase suffix
// (including the HTML tags), RU dates, reason prefix, emoji street line.
const wantFullOutage = "<B>РЕМОНТ: 20 АВГУСТА 10:00 20 АВГУСТА 14:00</B>\n\n❄️🚰 ул. Ленина 1"

func TestFormatOutage_FullOutageGolden(t *testing.T) {
	t.Parallel()

	if got := notifier.FormatOutage(sampleOutage()); got != wantFullOutage {
		t.Errorf("FormatOutage =\n %q\nwant\n %q", got, wantFullOutage)
	}
}

func TestFormatOutage_ResourceEmojiPerType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		res       *apidev.ResourceType
		raw       string
		wantEmoji string
	}{
		{name: "electricity", res: new(apidev.ResourceTypeElectricity), raw: "ХВС", wantEmoji: "⚡️"},
		{name: "gas", res: new(apidev.ResourceTypeGas), raw: "ХВС", wantEmoji: "🔥"},
		{name: "cold water", res: new(apidev.ResourceTypeColdWater), raw: "ХВС", wantEmoji: "❄️🚰"},
		{name: "hot water", res: new(apidev.ResourceTypeHotWater), raw: "ХВС", wantEmoji: "🌡️🚰"},
		{name: "heating", res: new(apidev.ResourceTypeHeating), raw: "ХВС", wantEmoji: "♨️"},
		{name: "nil resource type falls back to resource", res: nil, raw: "ХВС", wantEmoji: "(ХВС)"},
		{name: "unknown resource type falls back to resource",
			res: new(apidev.ResourceType("Тест")), raw: "Тест", wantEmoji: "(Тест)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o := apidev.Outage{
				OrganizationInfo: apidev.OrganizationInfo{
					ResourceType: tt.res,
					Resource:     tt.raw,
				},
				Details: apidev.OutageDetails{
					Streets: []apidev.Street{{Name: "ул. Ленина"}},
				},
			}

			want := "<B></B>\n\n" + tt.wantEmoji + " ул. Ленина"
			if got := notifier.FormatOutage(o); got != want {
				t.Errorf("FormatOutage = %q, want %q", got, want)
			}
		})
	}
}

func TestFormatOutage_NoReason(t *testing.T) {
	t.Parallel()

	o := sampleOutage()
	o.Details.Reason = nil

	const want = "<B>20 АВГУСТА 10:00 20 АВГУСТА 14:00</B>\n\n❄️🚰 ул. Ленина 1"
	if got := notifier.FormatOutage(o); got != want {
		t.Errorf("FormatOutage = %q, want %q", got, want)
	}
}

func TestFormatOutage_EmptyPeriod(t *testing.T) {
	t.Parallel()

	o := sampleOutage()
	o.Period = []time.Time{}

	const want = "<B>РЕМОНТ: </B>\n\n❄️🚰 ул. Ленина 1"
	if got := notifier.FormatOutage(o); got != want {
		t.Errorf("FormatOutage = %q, want %q", got, want)
	}
}

func TestFormatOutage_QuotesStreetsAndReason(t *testing.T) {
	t.Parallel()

	o := apidev.Outage{
		OrganizationInfo: apidev.OrganizationInfo{
			ResourceType: new(apidev.ResourceTypeElectricity),
		},
		Details: apidev.OutageDetails{
			Streets: []apidev.Street{{Name: "ул. Ленина <1> & 2"}},
			Reason:  &apidev.Reason{Type: "Ремонт <важный> & срочный"},
		},
		Period: []time.Time{time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)},
	}

	// The raw reason is uppercased before it is quoted, so the entities stay
	// in the lowercase spelling Telegram accepts (&lt; &gt; &amp;); the
	// streets part is quoted but NOT uppercased.
	const want = "<B>РЕМОНТ &lt;ВАЖНЫЙ&gt; &amp; СРОЧНЫЙ: 20 АВГУСТА 10:00</B>\n\n⚡️ ул. Ленина &lt;1&gt; &amp; 2"
	if got := notifier.FormatOutage(o); got != want {
		t.Errorf("FormatOutage =\n %q\nwant\n %q", got, want)
	}
}

// TestFormatOutage_EmitsOnlyLowercaseEntities pins the entity spelling of
// the whole message. Telegram's HTML parser only accepts "&lt;", "&gt;",
// "&amp;" and "&quot;"; an uppercase "&LT;" makes it reject the message with
// "can't parse entities", which fails the broadcast for every subscriber.
func TestFormatOutage_EmitsOnlyLowercaseEntities(t *testing.T) {
	t.Parallel()

	o := apidev.Outage{
		OrganizationInfo: apidev.OrganizationInfo{
			ResourceType: new(apidev.ResourceTypeElectricity),
		},
		Details: apidev.OutageDetails{
			Streets: []apidev.Street{{Name: "ул. Ленина <1> & 2"}},
			Reason:  &apidev.Reason{Type: "Ремонт <важный> & срочный"},
		},
		Period: []time.Time{time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)},
	}

	got := notifier.FormatOutage(o)

	for _, upper := range []string{"&LT;", "&GT;", "&AMP;", "&QUOT;"} {
		if strings.Contains(got, upper) {
			t.Errorf("message contains the unsupported entity %q: %q", upper, got)
		}
	}
	if hasPartialEntity(got) {
		t.Errorf("message holds a truncated HTML entity: %q", got)
	}
}

func TestFormatOutage_TruncatesStreetsAtRuneBoundary(t *testing.T) {
	t.Parallel()

	o := sampleOutage()

	streets := make([]apidev.Street, 0, 300)
	for range 300 {
		streets = append(streets, apidev.Street{Name: "ул. Ленина", Buildings: []string{"1"}})
	}
	o.Details.Streets = streets

	// Independent reconstruction of the un-truncated street block: emoji
	// line per street joined with newlines (no escapable characters here).
	streetLine := "❄️🚰 ул. Ленина 1"
	fullStreets := strings.Repeat(streetLine+"\n", len(streets)-1) + streetLine

	// The suffix that must survive truncation untouched.
	suffix := "<b>Ремонт: 20 августа 10:00 20 августа 14:00</b>"

	got := notifier.FormatOutage(o)

	if n := utf8.RuneCountInString(got); n != 4096 {
		t.Errorf("message rune count = %d, want 4096", n)
	}
	if !utf8.ValidString(got) {
		t.Error("message contains invalid UTF-8 (byte-level truncation)")
	}
	if !strings.HasSuffix(got, ellipsis) {
		t.Errorf("message %q does not end with ellipsis", got)
	}

	streetsGot, ok := strings.CutPrefix(got, strings.ToUpper(suffix)+"\n\n")
	if !ok {
		t.Fatalf("message %q does not start with the suffix block", got)
	}

	// The truncated block keeps the first maxStreets runes of the full
	// street text and appends the ellipsis (Python len() counts code
	// points; a byte-based slice would cut inside Cyrillic/emoji runes).
	maxStreets := 4096 - utf8.RuneCountInString(suffix) - utf8.RuneCountInString(ellipsis) - 2
	wantStreets := string([]rune(fullStreets)[:maxStreets]) + ellipsis
	if streetsGot != wantStreets {
		t.Errorf("truncated streets =\n %q\nwant\n %q", streetsGot, wantStreets)
	}
}

// TestFormatOutage_TruncatesBeforeEscaping pins the truncation order: the
// cut must land in the raw street text, never inside an escaped entity. The
// fixture puts a single escapable '&' exactly two runes before the budget
// cut, which is where the Python quote-then-slice order would emit "&a…" and
// make Telegram reject the whole broadcast with "can't parse entities".
func TestFormatOutage_TruncatesBeforeEscaping(t *testing.T) {
	t.Parallel()

	o := sampleOutage()

	suffix := "<b>Ремонт: 20 августа 10:00 20 августа 14:00</b>"
	maxStreets := 4096 - utf8.RuneCountInString(suffix) - utf8.RuneCountInString(ellipsis) - 2

	// The street line is "<emoji> <name>"; pad the name so that the raw
	// street text holds '&' at index maxStreets-2.
	head := "❄️🚰 "
	name := strings.Repeat("л", maxStreets-utf8.RuneCountInString(head)-2) + "&" + strings.Repeat("л", 50)
	o.Details.Streets = []apidev.Street{{Name: name}}

	got := notifier.FormatOutage(o)

	if hasPartialEntity(got) {
		t.Errorf("message ends with a truncated HTML entity: %q", got)
	}
	if !strings.HasSuffix(got, ellipsis) {
		t.Errorf("message %q does not end with ellipsis", got)
	}
	// Telegram applies the 4096 limit to the text it sees after parsing the
	// entities, so the unescaped message must hit the budget exactly.
	if n := utf8.RuneCountInString(unescapeHTML(got)); n != 4096 {
		t.Errorf("unescaped rune count = %d, want 4096", n)
	}
}

// unescapeHTML reverses i18n.HTMLQuote, accepting both spellings because the
// suffix block used to be uppercased as a whole (so its entities read
// "&AMP;").
func unescapeHTML(s string) string {
	return strings.NewReplacer(
		"&amp;", "&", "&AMP;", "&",
		"&lt;", "<", "&LT;", "<",
		"&gt;", ">", "&GT;", ">",
	).Replace(s)
}

// hasPartialEntity reports whether s holds an '&' that does not start one of
// the complete entities the formatter can emit. Telegram rejects such input.
func hasPartialEntity(s string) bool {
	for i := range len(s) {
		if s[i] != '&' {
			continue
		}
		rest := s[i:]
		if strings.HasPrefix(rest, "&amp;") || strings.HasPrefix(rest, "&AMP;") ||
			strings.HasPrefix(rest, "&lt;") || strings.HasPrefix(rest, "&LT;") ||
			strings.HasPrefix(rest, "&gt;") || strings.HasPrefix(rest, "&GT;") {
			continue
		}
		return true
	}

	return false
}

// TestFormatOutage_BoundsOversizedReason pins the suffix bound. The reason
// comes from the outage API and is not length-checked, so a reason that is
// long enough to push the message past Telegram's 4096 character limit would
// be rejected with "message is too long" and the outage would reach nobody.
// The reason is truncated to fit instead.
func TestFormatOutage_BoundsOversizedReason(t *testing.T) {
	t.Parallel()

	o := apidev.Outage{
		OrganizationInfo: apidev.OrganizationInfo{
			ResourceType: new(apidev.ResourceTypeElectricity),
		},
		Details: apidev.OutageDetails{
			Streets: []apidev.Street{{Name: "ул. Ленина"}},
			Reason:  &apidev.Reason{Type: strings.Repeat("Р", 4095)},
		},
	}

	got := notifier.FormatOutage(o)

	// Telegram applies the limit to the text it sees after parsing the
	// entities.
	if n := utf8.RuneCountInString(unescapeHTML(got)); n > 4096 {
		t.Errorf("message rune count = %d, want at most 4096", n)
	}
	if hasPartialEntity(got) {
		t.Errorf("message holds a truncated HTML entity: %q", got)
	}

	// The reason is cut and marked with the ellipsis; the street block
	// keeps what is left of the budget. The reason ellipsis sits right
	// before the closing tag, the street ellipsis ends the message, so
	// each assertion looks at its own place.
	if !strings.Contains(got, ellipsis+"</B>") {
		t.Errorf("message %q does not mark the cut reason", got)
	}
	if !strings.HasSuffix(got, ellipsis) {
		t.Errorf("message %q does not end with the street ellipsis", got)
	}
}

// TestFormatOutage_OversizedReasonKeepsDates pins that the bound protects
// the dates: they carry the outage time, so they win the budget over the
// reason label.
func TestFormatOutage_OversizedReasonKeepsDates(t *testing.T) {
	t.Parallel()

	o := apidev.Outage{
		OrganizationInfo: apidev.OrganizationInfo{
			ResourceType: new(apidev.ResourceTypeElectricity),
		},
		Details: apidev.OutageDetails{
			Streets: []apidev.Street{{Name: "ул. Ленина"}},
			Reason:  &apidev.Reason{Type: strings.Repeat("Р", 5000)},
		},
		Period: []time.Time{time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)},
	}

	got := notifier.FormatOutage(o)

	if !strings.Contains(got, "20 АВГУСТА 10:00") {
		t.Errorf("message %q lost the outage date", got)
	}
	if n := utf8.RuneCountInString(unescapeHTML(got)); n > 4096 {
		t.Errorf("message rune count = %d, want at most 4096", n)
	}
	if hasPartialEntity(got) {
		t.Errorf("message holds a truncated HTML entity: %q", got)
	}
}

// TestFormatOutage_OversizedReasonWithEntities pins that the bound counts
// the html-quoted length: an "&" expands to "&amp;", so the reason has to be
// cut earlier than its rune count suggests.
func TestFormatOutage_OversizedReasonWithEntities(t *testing.T) {
	t.Parallel()

	o := apidev.Outage{
		OrganizationInfo: apidev.OrganizationInfo{
			ResourceType: new(apidev.ResourceTypeElectricity),
		},
		Details: apidev.OutageDetails{
			Streets: []apidev.Street{{Name: "ул. Ленина"}},
			Reason:  &apidev.Reason{Type: strings.Repeat("&", 3000)},
		},
	}

	got := notifier.FormatOutage(o)

	if n := utf8.RuneCountInString(unescapeHTML(got)); n > 4096 {
		t.Errorf("message rune count = %d, want at most 4096", n)
	}
	if hasPartialEntity(got) {
		t.Errorf("message holds a truncated HTML entity: %q", got)
	}
}

func TestFormatOutage_EmptyStreets(t *testing.T) {
	t.Parallel()

	want := "<B>РЕМОНТ: 20 АВГУСТА 10:00 20 АВГУСТА 14:00</B>\n\n"

	for name, streets := range map[string][]apidev.Street{
		"empty slice": {},
		"nil slice":   nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			o := sampleOutage()
			o.Details.Streets = streets

			if got := notifier.FormatOutage(o); got != want {
				t.Errorf("FormatOutage = %q, want %q", got, want)
			}
		})
	}
}

func TestFormatOutage_StreetBuildings(t *testing.T) {
	t.Parallel()

	o := sampleOutage()
	o.Details.Streets = []apidev.Street{
		{Name: "ул. Мира", Buildings: []string{"1", "3"}},
		{Name: "ул. Ленина"},
	}

	const want = "<B>РЕМОНТ: 20 АВГУСТА 10:00 20 АВГУСТА 14:00</B>\n\n" +
		"❄️🚰 ул. Мира 1, 3\n" +
		"❄️🚰 ул. Ленина"
	if got := notifier.FormatOutage(o); got != want {
		t.Errorf("FormatOutage =\n %q\nwant\n %q", got, want)
	}
}
