package plan

import (
	"time"
)

// Japanese national holidays (国民の祝日), computed purely from the
// rules in the Act on National Holidays as amended (the law in force
// since 2022). No table of dates is embedded: fixed dates, "Happy
// Monday" dates and the equinoxes are all derived, and both
// substitute holidays (振替休日) and citizens' holidays (国民の休日)
// are applied.
//
// The equinox formulas are the standard approximations valid for
// 1980–2099, which covers every school year this tool will meet.

const dateLayout = "2006-01-02"

func dateKey(t time.Time) string { return t.Format(dateLayout) }

// ParseDate parses a "2006-01-02" calendar date. Dates are handled in
// UTC so the calendar day and its weekday never depend on the host
// timezone.
func ParseDate(s string) (time.Time, error) {
	return time.Parse(dateLayout, s)
}

func nthWeekday(year int, month time.Month, wd time.Weekday, n int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	offset := (int(wd) - int(first.Weekday()) + 7) % 7
	return first.AddDate(0, 0, offset+7*(n-1))
}

func equinoxDay(year int, base float64) int {
	return int(base+0.242194*float64(year-1980)) - (year-1980)/4
}

// HolidaysIn returns every national holiday of the given year,
// keyed by "2006-01-02" with the holiday name as the value.
func HolidaysIn(year int) map[string]string {
	out := map[string]string{}
	add := func(t time.Time, name string) { out[dateKey(t)] = name }
	d := func(month time.Month, day int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}

	// Base holidays fixed by the Act.
	add(d(1, 1), "元日")
	add(nthWeekday(year, 1, time.Monday, 2), "成人の日")
	add(d(2, 11), "建国記念の日")
	add(d(2, 23), "天皇誕生日")
	add(d(3, equinoxDay(year, 20.8431)), "春分の日")
	add(d(4, 29), "昭和の日")
	add(d(5, 3), "憲法記念日")
	add(d(5, 4), "みどりの日")
	add(d(5, 5), "こどもの日")
	add(nthWeekday(year, 7, time.Monday, 3), "海の日")
	add(d(8, 11), "山の日")
	add(nthWeekday(year, 9, time.Monday, 3), "敬老の日")
	add(d(9, equinoxDay(year, 23.2488)), "秋分の日")
	add(nthWeekday(year, 10, time.Monday, 2), "スポーツの日")
	add(d(11, 3), "文化の日")
	add(d(11, 23), "勤労感謝の日")

	// Substitute holidays (振替休日): when a holiday falls on a
	// Sunday, the nearest following day that is not itself a holiday
	// becomes a holiday.
	base := make([]time.Time, 0, len(out))
	for k := range out {
		t, _ := ParseDate(k)
		base = append(base, t)
	}
	for _, t := range base {
		if t.Weekday() != time.Sunday {
			continue
		}
		next := t.AddDate(0, 0, 1)
		for {
			if _, ok := out[dateKey(next)]; !ok {
				break
			}
			next = next.AddDate(0, 0, 1)
		}
		out[dateKey(next)] = "振替休日"
	}

	// Citizens' holidays (国民の休日): a day sandwiched between two
	// holidays becomes a holiday itself.
	day := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	for day.Year() == year {
		if _, ok := out[dateKey(day)]; !ok {
			_, prev := out[dateKey(day.AddDate(0, 0, -1))]
			_, next := out[dateKey(day.AddDate(0, 0, 1))]
			if prev && next {
				out[dateKey(day)] = "国民の休日"
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return out
}

// HolidayName returns the name of the national holiday on t, or ""
// when t is not a holiday.
func HolidayName(t time.Time) string {
	return HolidaysIn(t.Year())[dateKey(t)]
}

// IsHoliday reports whether t is a national holiday.
func IsHoliday(t time.Time) bool { return HolidayName(t) != "" }
