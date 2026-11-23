package plan

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"sort"
	"strings"
	"time"
)

// slot is one row of the full table: an assignment or an unfilled
// slot, unified for export.
type slot struct {
	date      string
	spot      string
	household string // "" when unassigned
}

func allSlots(p *Plan) []slot {
	var out []slot
	for _, a := range p.Assignments {
		out = append(out, slot{date: a.Date, spot: a.Spot, household: a.Household})
	}
	for _, u := range p.Unassigned {
		out = append(out, slot{date: u.Date, spot: u.Spot})
	}
	// Keep the spot order of the master list within each day.
	order := map[string]int{}
	for i, s := range p.Spots {
		order[s] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].date != out[j].date {
			return out[i].date < out[j].date
		}
		return order[out[i].spot] < order[out[j].spot]
	})
	return out
}

// ExportCSV renders the whole table as CSV (UTF-8 with BOM, so it
// opens cleanly in Excel): 日付, 曜日, 地点, 担当世帯. Unfilled slots
// are included with an empty 担当世帯 cell — nothing is hidden.
// When dismissal times are in effect (pickup preset) and at least
// one day carries one, a 下校時刻 column is appended; otherwise the
// output is byte-for-byte what it has always been.
func ExportCSV(p *Plan) []byte {
	slots := allSlots(p)
	showTime := false
	for _, s := range slots {
		if p.Settings.DismissalTime(s.date) != "" {
			showTime = true
			break
		}
	}
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	w := csv.NewWriter(&buf)
	w.UseCRLF = true
	header := []string{"日付", "曜日", "地点", "担当世帯"}
	if showTime {
		header = append(header, "下校時刻")
	}
	_ = w.Write(header)
	for _, s := range slots {
		weekday := ""
		if t, err := ParseDate(s.date); err == nil {
			weekday = WeekdayJP(t.Weekday())
		}
		row := []string{s.date, weekday, s.spot, s.household}
		if showTime {
			row = append(row, p.Settings.DismissalTime(s.date))
		}
		_ = w.Write(row)
	}
	w.Flush()
	return buf.Bytes()
}

// ExportICS renders the whole table as one iCalendar file: one
// event per slot, titled with the duty kind, the spot and the
// household on duty (or 未定 for unfilled slots). Slots are all-day
// events, except on days that carry a dismissal time (pickup
// preset): those become timed events starting at the dismissal
// time, lasting 30 minutes, in floating local time.
func ExportICS(p *Plan, now time.Time) []byte {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//HataBan//hataban//JA\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	stamp := now.UTC().Format("20060102T150405Z")
	label := p.Settings.DutyLabel()
	for i, s := range allSlots(p) {
		t, err := ParseDate(s.date)
		if err != nil {
			continue
		}
		name := s.household
		if name == "" {
			name = "未定"
		}
		summary := fmt.Sprintf("%s: %s - %s", label, s.spot, name)
		b.WriteString("BEGIN:VEVENT\r\n")
		fmt.Fprintf(&b, "UID:hataban-%s-%d@hataban.local\r\n", t.Format("20060102"), i)
		fmt.Fprintf(&b, "DTSTAMP:%s\r\n", stamp)
		if start, ok := dismissalStart(s.date, p.Settings.DismissalTime(s.date)); ok {
			fmt.Fprintf(&b, "DTSTART:%s\r\n", start.Format("20060102T150405"))
			fmt.Fprintf(&b, "DTEND:%s\r\n", start.Add(30*time.Minute).Format("20060102T150405"))
		} else {
			fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\n", t.Format("20060102"))
			fmt.Fprintf(&b, "DTEND;VALUE=DATE:%s\r\n", t.AddDate(0, 0, 1).Format("20060102"))
		}
		fmt.Fprintf(&b, "SUMMARY:%s\r\n", escapeICS(summary))
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return []byte(b.String())
}

// dismissalStart combines a date and a "15:04" dismissal time into
// one wall-clock moment. ok is false when either part is missing or
// malformed (Validate rejects malformed data on save, so this is
// only a belt-and-braces fallback to the all-day form).
func dismissalStart(date, tm string) (time.Time, bool) {
	if tm == "" {
		return time.Time{}, false
	}
	start, err := time.Parse("2006-01-02 15:04", date+" "+tm)
	if err != nil {
		return time.Time{}, false
	}
	return start, true
}

func escapeICS(s string) string {
	r := strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\n", "\\n",
	)
	return r.Replace(s)
}
