package plan

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := ParseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// SchoolDays for April 2026: 22 weekdays minus 昭和の日 (4/29) = 21.
func TestSchoolDaysApril2026(t *testing.T) {
	days, err := SchoolDays(Settings{Start: "2026-04-01", End: "2026-04-30"})
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 21 {
		t.Fatalf("school days = %d, want 21", len(days))
	}
	for _, d := range days {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			t.Errorf("%s is a weekend", dateKey(d))
		}
		if dateKey(d) == "2026-04-29" {
			t.Error("2026-04-29 (昭和の日) must not be a school day")
		}
	}
	// An extra day off removes exactly that day.
	days2, err := SchoolDays(Settings{
		Start: "2026-04-01", End: "2026-04-30",
		DaysOff: []string{"2026-04-10"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(days2) != 20 {
		t.Fatalf("school days with a day off = %d, want 20", len(days2))
	}
}

func samplePlan() *Plan {
	names := []string{"山田", "佐藤", "鈴木", "高橋", "田中", "伊藤", "渡辺", "山本", "中村", "小林"}
	p := &Plan{Spots: []string{"正門前", "郵便局前"}}
	for _, n := range names {
		p.Households = append(p.Households, Household{Name: n, Group: "1班"})
	}
	p.Settings = Settings{Start: "2026-04-01", End: "2026-04-30"}
	return p
}

// With 10 households, 2 spots and 21 school days there are 42 slots:
// the counts must be 4 or 5 per household (difference at most 1),
// and no household may appear twice on the same day.
func TestGenerateFairAndNoSameDayDuplicate(t *testing.T) {
	p := samplePlan()
	assignments, unassigned, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(unassigned) != 0 {
		t.Fatalf("unassigned = %v, want none", unassigned)
	}
	if len(assignments) != 42 {
		t.Fatalf("assignments = %d, want 42", len(assignments))
	}
	p.Assignments = assignments
	min, max := 1<<30, 0
	for _, c := range p.Counts() {
		if c.Count < min {
			min = c.Count
		}
		if c.Count > max {
			max = c.Count
		}
	}
	if max-min > 1 {
		t.Errorf("count spread = %d..%d, want difference at most 1", min, max)
	}
	byDay := map[string]map[string]bool{}
	for _, a := range assignments {
		if byDay[a.Date] == nil {
			byDay[a.Date] = map[string]bool{}
		}
		if byDay[a.Date][a.Household] {
			t.Errorf("%s assigned twice on %s", a.Household, a.Date)
		}
		byDay[a.Date][a.Household] = true
	}
}

// Constraints: a household that cannot serve a spot never gets it, a
// household never serves on its unavailable weekday, and a slot with
// too few eligible households is reported with a reason.
func TestGenerateConstraints(t *testing.T) {
	p := &Plan{
		Spots: []string{"正門前", "郵便局前"},
		Households: []Household{
			{Name: "A", Spots: []string{"正門前"}},
			{Name: "B", Spots: []string{"郵便局前"}, Unavailable: []int{int(time.Monday)}},
			{Name: "C"}, // any spot, any weekday
		},
		// 2026-04-06 is a Monday; one week Mon–Fri.
		Settings: Settings{Start: "2026-04-06", End: "2026-04-10"},
	}
	assignments, _, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range assignments {
		day := mustDate(t, a.Date)
		switch a.Household {
		case "A":
			if a.Spot != "正門前" {
				t.Errorf("A assigned to %s, can only serve 正門前", a.Spot)
			}
		case "B":
			if a.Spot != "郵便局前" {
				t.Errorf("B assigned to %s, can only serve 郵便局前", a.Spot)
			}
			if day.Weekday() == time.Monday {
				t.Errorf("B assigned on Monday %s despite being unavailable", a.Date)
			}
		}
	}
}

// A spot nobody can serve must surface as Unassigned with a reason,
// not disappear.
func TestGenerateUnassignedReason(t *testing.T) {
	p := &Plan{
		Spots: []string{"正門前", "誰も行けない地点"},
		Households: []Household{
			{Name: "A", Spots: []string{"正門前"}},
			{Name: "B", Spots: []string{"正門前"}},
		},
		Settings: Settings{Start: "2026-04-06", End: "2026-04-06"},
	}
	_, unassigned, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(unassigned) != 1 {
		t.Fatalf("unassigned = %v, want exactly 1", unassigned)
	}
	if unassigned[0].Spot != "誰も行けない地点" || unassigned[0].Reason == "" {
		t.Errorf("unassigned = %+v, want the impossible spot with a reason", unassigned[0])
	}
}

// When every capable household is unavailable that weekday, the
// reason must say so.
func TestGenerateUnassignedWeekdayReason(t *testing.T) {
	p := &Plan{
		Spots: []string{"正門前"},
		Households: []Household{
			{Name: "A", Unavailable: []int{int(time.Monday)}},
		},
		Settings: Settings{Start: "2026-04-06", End: "2026-04-06"}, // a Monday
	}
	_, unassigned, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(unassigned) != 1 || !strings.Contains(unassigned[0].Reason, "月曜不可") {
		t.Errorf("unassigned = %v, want one entry citing Monday unavailability", unassigned)
	}
}

func TestExportCSV(t *testing.T) {
	p := samplePlan()
	assignments, unassigned, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Assignments, p.Unassigned = assignments, unassigned
	csv := string(ExportCSV(p))
	if !strings.HasPrefix(csv, "\ufeff日付,曜日,地点,担当世帯") {
		t.Errorf("CSV header wrong: %.40q", csv)
	}
	lines := strings.Split(strings.TrimRight(csv, "\r\n"), "\n")
	if len(lines) != 43 { // header + 42 slots
		t.Fatalf("CSV lines = %d, want 43", len(lines))
	}
	// First school day is Wednesday 2026-04-01.
	if !strings.Contains(lines[1], "2026-04-01,水,正門前,") {
		t.Errorf("first CSV row = %q", lines[1])
	}
}

func TestExportICS(t *testing.T) {
	p := samplePlan()
	assignments, _, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Assignments = assignments
	ics := string(ExportICS(p, mustDate(t, "2026-04-01")))
	if !strings.Contains(ics, "BEGIN:VCALENDAR") || !strings.Contains(ics, "END:VCALENDAR") {
		t.Error("ICS envelope missing")
	}
	if n := strings.Count(ics, "BEGIN:VEVENT"); n != 42 {
		t.Errorf("VEVENT count = %d, want 42", n)
	}
	if !strings.Contains(ics, "DTSTART;VALUE=DATE:20260401") {
		t.Error("ICS lacks the first school day event")
	}
	if !strings.Contains(ics, "SUMMARY:旗当番: 正門前 - ") {
		t.Error("ICS summary format wrong")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	st := &Store{Path: filepath.Join(t.TempDir(), "hataban.json")}
	p := samplePlan()
	assignments, unassigned, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Assignments, p.Unassigned = assignments, unassigned
	if err := st.Save(p); err != nil {
		t.Fatal(err)
	}
	got, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Households) != 10 || len(got.Spots) != 2 || len(got.Assignments) != 42 {
		t.Errorf("round trip mismatch: households=%d spots=%d assignments=%d",
			len(got.Households), len(got.Spots), len(got.Assignments))
	}
	if got.Settings.Start != "2026-04-01" || got.Households[0].Name != "山田" {
		t.Errorf("round trip content mismatch: %+v", got.Settings)
	}
	// A missing file loads as an empty plan.
	empty, err := (&Store{Path: filepath.Join(t.TempDir(), "none.json")}).Load()
	if err != nil || len(empty.Households) != 0 {
		t.Errorf("missing file: got %+v, err %v", empty, err)
	}
}

func TestParseImport(t *testing.T) {
	text := "世帯名,班,担当地点,不可曜日\n" +
		"山田,1班,正門前・郵便局前,月・水\n" +
		"佐藤\t2班\t正門前\t\n" +
		"鈴木,1班,,\n"
	households, errs := ParseImport(text)
	if len(errs) != 0 {
		t.Fatalf("errs = %v", errs)
	}
	if len(households) != 3 {
		t.Fatalf("households = %d, want 3", len(households))
	}
	y := households[0]
	if y.Group != "1班" || len(y.Spots) != 2 || len(y.Unavailable) != 2 ||
		y.Unavailable[0] != int(time.Monday) || y.Unavailable[1] != int(time.Wednesday) {
		t.Errorf("parsed 山田 = %+v", y)
	}
	if len(households[1].Spots) != 1 || households[1].Spots[0] != "正門前" {
		t.Errorf("parsed 佐藤 = %+v", households[1])
	}
	// A line without a name is an error, not silently skipped.
	_, errs = ParseImport(",1班,,\n")
	if len(errs) != 1 {
		t.Errorf("errs = %v, want 1 for the nameless line", errs)
	}
}

// Reassign must honour the same constraints as the generator.
func TestReassignValidation(t *testing.T) {
	p := &Plan{
		Spots: []string{"正門前", "郵便局前"},
		Households: []Household{
			{Name: "A", Spots: []string{"正門前"}},
			{Name: "B", Unavailable: []int{int(time.Monday)}},
			{Name: "C"},
		},
		Settings: Settings{Start: "2026-04-06", End: "2026-04-10"},
	}
	assignments, _, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Assignments = assignments
	// 2026-04-06 is a Monday: B is unavailable that day.
	if err := p.Reassign("2026-04-06", "正門前", "B"); err == nil {
		t.Error("reassigning Monday duty to B (Monday-unavailable) must fail")
	}
	// A can only serve 正門前.
	if err := p.Reassign("2026-04-07", "郵便局前", "A"); err == nil {
		t.Error("reassigning 郵便局前 to A (cannot serve it) must fail")
	}
	// A valid change goes through.
	if err := p.Reassign("2026-04-07", "郵便局前", "C"); err != nil {
		t.Errorf("valid reassign failed: %v", err)
	}
}
