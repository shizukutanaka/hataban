package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pickupPlan is the sample plan switched to the pickup preset, with
// a dismissal time on the first school day only (2026-04-01, a
// Wednesday). Real dismissal times differ per day, so most days
// deliberately have none.
func pickupPlan(t *testing.T) *Plan {
	t.Helper()
	p := samplePlan()
	p.Settings.Purpose = PurposePickup
	p.Settings.DismissalTimes = map[string]string{"2026-04-01": "14:40"}
	assignments, unassigned, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Assignments, p.Unassigned = assignments, unassigned
	return p
}

func TestDutyLabelAndDismissalTime(t *testing.T) {
	flag := Settings{DismissalTimes: map[string]string{"2026-04-01": "14:40"}}
	if flag.DutyLabel() != "旗当番" {
		t.Errorf("default label = %q, want 旗当番", flag.DutyLabel())
	}
	if got := flag.DismissalTime("2026-04-01"); got != "" {
		t.Errorf("flag preset must not use times, got %q", got)
	}
	flag.Purpose = PurposeFlag
	if flag.DutyLabel() != "旗当番" || flag.DismissalTime("2026-04-01") != "" {
		t.Error("explicit flag preset must behave like the default")
	}
	pickup := Settings{Purpose: PurposePickup, DismissalTimes: map[string]string{"2026-04-01": "14:40"}}
	if pickup.DutyLabel() != "お迎え当番" {
		t.Errorf("pickup label = %q, want お迎え当番", pickup.DutyLabel())
	}
	if got := pickup.DismissalTime("2026-04-01"); got != "14:40" {
		t.Errorf("pickup dismissal time = %q, want 14:40", got)
	}
	if got := pickup.DismissalTime("2026-04-02"); got != "" {
		t.Errorf("unset date must have no time, got %q", got)
	}
}

func TestPickupExportCSV(t *testing.T) {
	p := pickupPlan(t)
	csv := string(ExportCSV(p))
	if !strings.HasPrefix(csv, "\ufeff日付,曜日,地点,担当世帯,下校時刻") {
		t.Errorf("CSV header wrong: %.60q", csv)
	}
	lines := strings.Split(strings.TrimRight(csv, "\r\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r") // CSV rows end in CRLF
	}
	if len(lines) != 43 {
		t.Fatalf("CSV lines = %d, want 43", len(lines))
	}
	if !strings.HasSuffix(lines[1], ",14:40") {
		t.Errorf("first row must carry the dismissal time: %q", lines[1])
	}
	// The next school day (2026-04-02) has no time: empty last cell.
	found := false
	for _, ln := range lines[2:] {
		if strings.Contains(ln, "2026-04-02,") {
			found = true
			if !strings.HasSuffix(ln, ",") {
				t.Errorf("untimed row must end with an empty time cell: %q", ln)
			}
		}
	}
	if !found {
		t.Error("row for 2026-04-02 missing")
	}
}

func TestPickupExportICS(t *testing.T) {
	p := pickupPlan(t)
	ics := string(ExportICS(p, mustDate(t, "2026-04-01")))
	if !strings.Contains(ics, "SUMMARY:お迎え当番: 正門前 - ") {
		t.Error("ICS summary must use the pickup label")
	}
	if strings.Contains(ics, "旗当番") {
		t.Error("pickup ICS must not mention 旗当番")
	}
	// The timed day becomes a timed event 14:40–15:10 local.
	if !strings.Contains(ics, "DTSTART:20260401T144000") ||
		!strings.Contains(ics, "DTEND:20260401T151000") {
		t.Error("timed day must be a timed event at the dismissal time")
	}
	// Days without a time stay all-day events.
	if !strings.Contains(ics, "DTSTART;VALUE=DATE:20260402") {
		t.Error("untimed day must stay an all-day event")
	}
	if n := strings.Count(ics, "BEGIN:VEVENT"); n != 42 {
		t.Errorf("VEVENT count = %d, want 42", n)
	}
}

// Under the flag preset, dismissal-time data (however it got into
// the file) must not change any output: byte-for-byte identical.
func TestFlagOutputUnchangedByTimeData(t *testing.T) {
	now := mustDate(t, "2026-04-01")
	for _, purpose := range []string{"", PurposeFlag} {
		plain := samplePlan()
		assignments, unassigned, err := Generate(plain)
		if err != nil {
			t.Fatal(err)
		}
		plain.Assignments, plain.Unassigned = assignments, unassigned

		withTimes := samplePlan()
		withTimes.Settings.Purpose = purpose
		withTimes.Settings.DismissalTimes = map[string]string{"2026-04-01": "14:40"}
		withTimes.Assignments, withTimes.Unassigned = assignments, unassigned

		if string(ExportCSV(withTimes)) != string(ExportCSV(plain)) {
			t.Errorf("purpose %q: CSV changed by time data", purpose)
		}
		if string(ExportICS(withTimes, now)) != string(ExportICS(plain, now)) {
			t.Errorf("purpose %q: ICS changed by time data", purpose)
		}
	}
}

// Pickup preset without any times: only the label changes, the
// structure (all-day events, no time column) stays as before.
func TestPickupWithoutTimes(t *testing.T) {
	p := samplePlan()
	p.Settings.Purpose = PurposePickup
	assignments, unassigned, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Assignments, p.Unassigned = assignments, unassigned
	csv := string(ExportCSV(p))
	if strings.Contains(csv, "下校時刻") {
		t.Error("CSV must not gain a time column when no times are set")
	}
	ics := string(ExportICS(p, mustDate(t, "2026-04-01")))
	if !strings.Contains(ics, "SUMMARY:お迎え当番: 正門前 - ") {
		t.Error("ICS summary must use the pickup label")
	}
	if strings.Contains(ics, "DTSTART:2026") {
		t.Error("all events must stay all-day when no times are set")
	}
}

// Data written by v0.1.0 (no purpose / dismissal_times fields) must
// load unchanged and behave as flag duty; a pickup plan must round
// trip through the store with its new fields intact.
func TestStoreLegacyAndPickupRoundTrip(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"version":1,"plan":{` +
		`"households":[{"name":"山田","group":"1班","spots":[],"unavailable":[]}],` +
		`"spots":["正門前"],` +
		`"settings":{"start":"2026-04-01","end":"2026-04-30","days_off":[]},` +
		`"assignments":[],"unassigned":[]}}`
	path := filepath.Join(dir, "hataban.json")
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &Store{Path: path}
	got, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Settings.Purpose != "" || got.Settings.DutyLabel() != "旗当番" {
		t.Errorf("legacy data must read as flag duty: %+v", got.Settings)
	}
	if got.Settings.DismissalTime("2026-04-01") != "" {
		t.Error("legacy data must have no dismissal times")
	}
	if len(got.Households) != 1 || got.Households[0].Name != "山田" {
		t.Errorf("legacy roster mismatch: %+v", got.Households)
	}

	p := samplePlan()
	p.Settings.Purpose = PurposePickup
	p.Settings.DismissalTimes = map[string]string{"2026-04-01": "11:40", "2026-04-02": "13:20"}
	if err := st.Save(p); err != nil {
		t.Fatal(err)
	}
	back, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if back.Settings.Purpose != PurposePickup ||
		back.Settings.DismissalTimes["2026-04-01"] != "11:40" ||
		back.Settings.DismissalTimes["2026-04-02"] != "13:20" {
		t.Errorf("pickup round trip mismatch: %+v", back.Settings)
	}
}

func TestValidatePurposeAndDismissalTimes(t *testing.T) {
	base := func() *Plan {
		p := samplePlan()
		return p
	}
	p := base()
	p.Settings.Purpose = "festival"
	if err := p.Validate(); err == nil {
		t.Error("unknown purpose must be rejected")
	}
	p = base()
	p.Settings.Purpose = PurposePickup
	p.Settings.DismissalTimes = map[string]string{"2026-04-01": "25:00"}
	if err := p.Validate(); err == nil {
		t.Error("impossible dismissal time must be rejected")
	}
	p = base()
	p.Settings.Purpose = PurposePickup
	p.Settings.DismissalTimes = map[string]string{"April 1": "14:40"}
	if err := p.Validate(); err == nil {
		t.Error("malformed dismissal date must be rejected")
	}
	p = base()
	p.Settings.Purpose = PurposePickup
	p.Settings.DismissalTimes = map[string]string{"2026-04-01": "14:40"}
	if err := p.Validate(); err != nil {
		t.Errorf("valid pickup settings rejected: %v", err)
	}
}
