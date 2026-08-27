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

func strPtr(s string) *string {
	return &s
}

func resourcePtr(rt apidev.ResourceType) *apidev.ResourceType {
	return &rt
}

// sampleOutage is the fixture outage: cold water, one street with a
// building, a repair reason and two UTC dates.
func sampleOutage() apidev.Outage {
	return apidev.Outage{
		Area: "Тест",
		OrganizationInfo: apidev.OrganizationInfo{
			ResourceType: resourcePtr(apidev.ResourceTypeColdWater),
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
	if got := notifier.FormatOutage(sampleOutage()); got != wantFullOutage {
		t.Errorf("FormatOutage =\n %q\nwant\n %q", got, wantFullOutage)
	}
}

func TestFormatOutage_ResourceEmojiPerType(t *testing.T) {
	tests := []struct {
		name      string
		res       *apidev.ResourceType
		raw       string
		wantEmoji string
	}{
		{name: "electricity", res: resourcePtr(apidev.ResourceTypeElectricity), raw: "ХВС", wantEmoji: "⚡️"},
		{name: "gas", res: resourcePtr(apidev.ResourceTypeGas), raw: "ХВС", wantEmoji: "🔥"},
		{name: "cold water", res: resourcePtr(apidev.ResourceTypeColdWater), raw: "ХВС", wantEmoji: "❄️🚰"},
		{name: "hot water", res: resourcePtr(apidev.ResourceTypeHotWater), raw: "ХВС", wantEmoji: "🌡️🚰"},
		{name: "heating", res: resourcePtr(apidev.ResourceTypeHeating), raw: "ХВС", wantEmoji: "♨️"},
		{name: "nil resource type falls back to resource", res: nil, raw: "ХВС", wantEmoji: "(ХВС)"},
		{name: "unknown resource type falls back to resource",
			res: resourcePtr(apidev.ResourceType("Тест")), raw: "Тест", wantEmoji: "(Тест)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	o := sampleOutage()
	o.Details.Reason = nil

	const want = "<B>20 АВГУСТА 10:00 20 АВГУСТА 14:00</B>\n\n❄️🚰 ул. Ленина 1"
	if got := notifier.FormatOutage(o); got != want {
		t.Errorf("FormatOutage = %q, want %q", got, want)
	}
}

func TestFormatOutage_EmptyPeriod(t *testing.T) {
	o := sampleOutage()
	o.Period = []time.Time{}

	const want = "<B>РЕМОНТ: </B>\n\n❄️🚰 ул. Ленина 1"
	if got := notifier.FormatOutage(o); got != want {
		t.Errorf("FormatOutage = %q, want %q", got, want)
	}
}

func TestFormatOutage_QuotesStreetsAndReason(t *testing.T) {
	o := apidev.Outage{
		OrganizationInfo: apidev.OrganizationInfo{
			ResourceType: resourcePtr(apidev.ResourceTypeElectricity),
		},
		Details: apidev.OutageDetails{
			Streets: []apidev.Street{{Name: "ул. Ленина <1> & 2"}},
			Reason:  &apidev.Reason{Type: "Ремонт <важный> & срочный"},
		},
		Period: []time.Time{time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)},
	}

	// The suffix is uppercased after quoting, so entities become &LT; &GT;
	// &AMP;; the streets part is quoted but NOT uppercased.
	const want = "<B>РЕМОНТ &LT;ВАЖНЫЙ&GT; &AMP; СРОЧНЫЙ: 20 АВГУСТА 10:00</B>\n\n⚡️ ул. Ленина &lt;1&gt; &amp; 2"
	if got := notifier.FormatOutage(o); got != want {
		t.Errorf("FormatOutage =\n %q\nwant\n %q", got, want)
	}
}

func TestFormatOutage_TruncatesStreetsAtRuneBoundary(t *testing.T) {
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

func TestFormatOutage_ClampsBudgetAtZero(t *testing.T) {
	o := apidev.Outage{
		OrganizationInfo: apidev.OrganizationInfo{
			ResourceType: resourcePtr(apidev.ResourceTypeElectricity),
		},
		Details: apidev.OutageDetails{
			Streets: []apidev.Street{{Name: "ул. Ленина"}},
			Reason:  &apidev.Reason{Type: strings.Repeat("Р", 4095)},
		},
	}

	got := notifier.FormatOutage(o)

	// The suffix alone exceeds 4096, so the street budget clamps to zero
	// and the street block degenerates to a bare ellipsis.
	streetsGot, ok := strings.CutPrefix(got, strings.ToUpper("<b>"+strings.Repeat("Р", 4095)+": </b>")+"\n\n")
	if !ok {
		t.Fatalf("message %q does not start with the oversized suffix block", got)
	}
	if streetsGot != ellipsis {
		t.Errorf("streets block = %q, want %q", streetsGot, ellipsis)
	}
}

func TestFormatOutage_EmptyStreets(t *testing.T) {
	want := "<B>РЕМОНТ: 20 АВГУСТА 10:00 20 АВГУСТА 14:00</B>\n\n"

	for name, streets := range map[string][]apidev.Street{
		"empty slice": {},
		"nil slice":   nil,
	} {
		t.Run(name, func(t *testing.T) {
			o := sampleOutage()
			o.Details.Streets = streets

			if got := notifier.FormatOutage(o); got != want {
				t.Errorf("FormatOutage = %q, want %q", got, want)
			}
		})
	}
}

func TestFormatOutage_StreetBuildings(t *testing.T) {
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
