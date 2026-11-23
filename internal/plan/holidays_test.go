package plan

import "testing"

// The full set of Japanese national holidays for 2026, checked
// against the published calendar: 18 days including the substitute
// holiday on 2026-05-06 (憲法記念日 falls on a Sunday, and both
// following days are already holidays) and the citizens' holiday on
// 2026-09-22 (sandwiched between 敬老の日 and 秋分の日).
func TestHolidays2026(t *testing.T) {
	want := map[string]string{
		"2026-01-01": "元日",
		"2026-01-12": "成人の日",
		"2026-02-11": "建国記念の日",
		"2026-02-23": "天皇誕生日",
		"2026-03-20": "春分の日",
		"2026-04-29": "昭和の日",
		"2026-05-03": "憲法記念日",
		"2026-05-04": "みどりの日",
		"2026-05-05": "こどもの日",
		"2026-05-06": "振替休日",
		"2026-07-20": "海の日",
		"2026-08-11": "山の日",
		"2026-09-21": "敬老の日",
		"2026-09-22": "国民の休日",
		"2026-09-23": "秋分の日",
		"2026-10-12": "スポーツの日",
		"2026-11-03": "文化の日",
		"2026-11-23": "勤労感謝の日",
	}
	got := HolidaysIn(2026)
	if len(got) != len(want) {
		t.Errorf("2026 holiday count = %d, want %d (%v)", len(got), len(want), got)
	}
	for date, name := range want {
		if got[date] != name {
			t.Errorf("holiday %s = %q, want %q", date, got[date], name)
		}
	}
	// Neighbouring ordinary days must not be holidays.
	for _, date := range []string{"2026-05-07", "2026-09-24", "2026-01-13", "2026-12-31"} {
		if _, ok := got[date]; ok {
			t.Errorf("%s must not be a holiday, got %q", date, got[date])
		}
	}
}

// A year where Golden Week has no Sunday holiday must not gain a
// substitute: 2025-05-03 was a Saturday, so 2025-05-06 stays a
// regular day off only via the weekend rules, not as 振替休日.
// (2025: 5/3 Sat, 5/4 Sun みどりの日 → 5/5 Mon is こどもの日, and
// the substitute for Sunday 5/4 lands on 5/6 Tue.)
func TestHolidays2025Substitute(t *testing.T) {
	got := HolidaysIn(2025)
	if got["2025-05-06"] != "振替休日" {
		t.Errorf("2025-05-06 = %q, want 振替休日 (みどりの日 5/4 was a Sunday)", got["2025-05-06"])
	}
	if got["2025-02-24"] != "振替休日" {
		t.Errorf("2025-02-24 = %q, want 振替休日 (天皇誕生日 2/23 was a Sunday)", got["2025-02-24"])
	}
}
