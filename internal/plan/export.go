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
func ExportCSV(p *Plan) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	w := csv.NewWriter(&buf)
	w.UseCRLF = true
	_ = w.Write([]string{"日付", "曜日", "地点", "担当世帯"})
	for _, s := range allSlots(p) {
		weekday := ""
		if t, err := ParseDate(s.date); err == nil {
			weekday = WeekdayJP(t.Weekday())
		}
		_ = w.Write([]string{s.date, weekday, s.spot, s.household})
	}
	w.Flush()
	return buf.Bytes()
}

// ExportICS renders the whole table as one iCalendar file: one
// all-day event per slot, titled with the spot and the household on
// duty (or 未定 for unfilled slots).
func ExportICS(p *Plan, now time.Time) []byte {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//HataBan//hataban//JA\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	stamp := now.UTC().Format("20060102T150405Z")
	for i, s := range allSlots(p) {
		t, err := ParseDate(s.date)
		if err != nil {
			continue
		}
		name := s.household
		if name == "" {
			name = "未定"
		}
		summary := fmt.Sprintf("旗当番: %s - %s", s.spot, name)
		b.WriteString("BEGIN:VEVENT\r\n")
		fmt.Fprintf(&b, "UID:hataban-%s-%d@hataban.local\r\n", t.Format("20060102"), i)
		fmt.Fprintf(&b, "DTSTAMP:%s\r\n", stamp)
		fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\n", t.Format("20060102"))
		fmt.Fprintf(&b, "DTEND;VALUE=DATE:%s\r\n", t.AddDate(0, 0, 1).Format("20060102"))
		fmt.Fprintf(&b, "SUMMARY:%s\r\n", escapeICS(summary))
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return []byte(b.String())
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
