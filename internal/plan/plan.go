package plan

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Household is one family on the roster. Spots lists the crossing
// points the household can cover; an empty list means "no
// restriction" (any spot). Unavailable lists weekdays the household
// can never take, as time.Weekday values (Sunday=0 … Saturday=6).
type Household struct {
	Name        string   `json:"name"`
	Group       string   `json:"group"`
	Spots       []string `json:"spots"`
	Unavailable []int    `json:"unavailable"`
}

// Purpose values for Settings.Purpose. The empty string and
// PurposeFlag both mean the original flag duty; data written before
// the preset existed has no purpose field and reads as flag duty.
const (
	PurposeFlag   = "flag"
	PurposePickup = "pickup"
)

// Settings is the duty period and the extra days off the user added
// on top of weekends and national holidays (school events, makeup
// holidays, and so on). Dates are "2006-01-02" strings.
//
// Purpose selects what the table is for: flag duty (旗当番, the
// default) or pickup duty (お迎え当番). DismissalTimes optionally
// records the school dismissal time ("15:04") per date; dismissal
// times vary from day to day, so they are keyed by date, not by
// weekday. They only take effect under the pickup preset.
type Settings struct {
	Start          string            `json:"start"`
	End            string            `json:"end"`
	DaysOff        []string          `json:"days_off"`
	Purpose        string            `json:"purpose"`
	DismissalTimes map[string]string `json:"dismissal_times"`
}

// DutyLabel is the Japanese name of the duty the settings select:
// お迎え当番 for the pickup preset, 旗当番 otherwise.
func (s Settings) DutyLabel() string {
	if s.Purpose == PurposePickup {
		return "お迎え当番"
	}
	return "旗当番"
}

// DismissalTime returns the dismissal time ("15:04") recorded for
// the date, or "" when none applies. Under the flag preset times are
// never used, so flag-duty output stays exactly as it has always
// been even if time data is present in the file.
func (s Settings) DismissalTime(date string) string {
	if s.Purpose != PurposePickup {
		return ""
	}
	return s.DismissalTimes[date]
}

// Assignment is one filled slot: one household at one spot on one day.
type Assignment struct {
	Date      string `json:"date"`
	Spot      string `json:"spot"`
	Household string `json:"household"`
}

// Unassigned is a slot the generator could not fill, with the reason
// why. Unfilled slots are always reported, never silently skipped.
type Unassigned struct {
	Date   string `json:"date"`
	Spot   string `json:"spot"`
	Reason string `json:"reason"`
}

// Plan is the whole document: roster, spots, period, and the
// generated (and possibly hand-edited) duty table.
type Plan struct {
	Households  []Household  `json:"households"`
	Spots       []string     `json:"spots"`
	Settings    Settings     `json:"settings"`
	Assignments []Assignment `json:"assignments"`
	Unassigned  []Unassigned `json:"unassigned"`
}

// Normalize replaces nil slices with empty ones so JSON output is
// always [] rather than null.
func (p *Plan) Normalize() {
	if p.Households == nil {
		p.Households = []Household{}
	}
	for i := range p.Households {
		if p.Households[i].Spots == nil {
			p.Households[i].Spots = []string{}
		}
		if p.Households[i].Unavailable == nil {
			p.Households[i].Unavailable = []int{}
		}
	}
	if p.Spots == nil {
		p.Spots = []string{}
	}
	if p.Settings.DaysOff == nil {
		p.Settings.DaysOff = []string{}
	}
	if p.Settings.DismissalTimes == nil {
		p.Settings.DismissalTimes = map[string]string{}
	}
	if p.Assignments == nil {
		p.Assignments = []Assignment{}
	}
	if p.Unassigned == nil {
		p.Unassigned = []Unassigned{}
	}
}

// Validate checks the editable master data (roster, spots, period).
func (p *Plan) Validate() error {
	seen := map[string]bool{}
	for _, h := range p.Households {
		name := strings.TrimSpace(h.Name)
		if name == "" {
			return errors.New("世帯名が空の行があります")
		}
		if seen[name] {
			return fmt.Errorf("世帯名 %q が重複しています", name)
		}
		seen[name] = true
		for _, wd := range h.Unavailable {
			if wd < 0 || wd > 6 {
				return fmt.Errorf("%s の不可曜日の値が不正です", name)
			}
		}
	}
	seenSpots := map[string]bool{}
	for _, s := range p.Spots {
		s = strings.TrimSpace(s)
		if s == "" {
			return errors.New("地点名が空の行があります")
		}
		if seenSpots[s] {
			return fmt.Errorf("地点名 %q が重複しています", s)
		}
		seenSpots[s] = true
	}
	if p.Settings.Start != "" && p.Settings.End != "" {
		start, err := ParseDate(p.Settings.Start)
		if err != nil {
			return errors.New("開始日の形式が不正です")
		}
		end, err := ParseDate(p.Settings.End)
		if err != nil {
			return errors.New("終了日の形式が不正です")
		}
		if end.Before(start) {
			return errors.New("終了日が開始日より前になっています")
		}
	}
	for _, d := range p.Settings.DaysOff {
		if _, err := ParseDate(d); err != nil {
			return fmt.Errorf("休業日 %q の形式が不正です", d)
		}
	}
	switch p.Settings.Purpose {
	case "", PurposeFlag, PurposePickup:
	default:
		return fmt.Errorf("用途 %q が不正です", p.Settings.Purpose)
	}
	for d, tm := range p.Settings.DismissalTimes {
		if _, err := ParseDate(d); err != nil {
			return fmt.Errorf("下校時刻の日付 %q の形式が不正です", d)
		}
		if _, err := time.Parse("15:04", tm); err != nil {
			return fmt.Errorf("%s の下校時刻 %q の形式が不正です", d, tm)
		}
	}
	return nil
}

// CanServe reports whether household h may cover the given spot.
func (h Household) CanServe(spot string) bool {
	if len(h.Spots) == 0 {
		return true // no restriction recorded
	}
	for _, s := range h.Spots {
		if s == spot {
			return true
		}
	}
	return false
}

// AvailableOn reports whether h can take duty on weekday wd.
func (h Household) AvailableOn(wd time.Weekday) bool {
	for _, u := range h.Unavailable {
		if u == int(wd) {
			return false
		}
	}
	return true
}

// SchoolDays lists the duty days in the period: weekdays (Mon–Fri)
// that are neither national holidays nor user-added days off.
func SchoolDays(s Settings) ([]time.Time, error) {
	start, err := ParseDate(s.Start)
	if err != nil {
		return nil, errors.New("開始日が設定されていません")
	}
	end, err := ParseDate(s.End)
	if err != nil {
		return nil, errors.New("終了日が設定されていません")
	}
	if end.Before(start) {
		return nil, errors.New("終了日が開始日より前になっています")
	}
	off := map[string]bool{}
	for _, d := range s.DaysOff {
		off[d] = true
	}
	holidays := map[string]string{}
	loadedYears := map[int]bool{}
	var days []time.Time
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		wd := day.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			continue
		}
		if year := day.Year(); !loadedYears[year] {
			for k, v := range HolidaysIn(year) {
				holidays[k] = v
			}
			loadedYears[year] = true
		}
		if _, isHoliday := holidays[dateKey(day)]; isHoliday {
			continue
		}
		if off[dateKey(day)] {
			continue
		}
		days = append(days, day)
	}
	return days, nil
}

// Counts returns how many duties each roster household has in the
// current table, in roster order. Households named in assignments
// but no longer on the roster are appended at the end.
func (p *Plan) Counts() []Count {
	counts := map[string]int{}
	for _, a := range p.Assignments {
		counts[a.Household]++
	}
	var out []Count
	seen := map[string]bool{}
	for _, h := range p.Households {
		out = append(out, Count{Household: h.Name, Count: counts[h.Name]})
		seen[h.Name] = true
	}
	var extra []string
	for name := range counts {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		out = append(out, Count{Household: name, Count: counts[name]})
	}
	return out
}

// Count is one household's duty total.
type Count struct {
	Household string `json:"household"`
	Count     int    `json:"count"`
}

// WeekdayJP is the one-letter Japanese weekday name.
func WeekdayJP(wd time.Weekday) string {
	return []string{"日", "月", "火", "水", "木", "金", "土"}[int(wd)]
}

// Reassign changes one slot of the generated table to another
// household ("" clears the slot). The new household must satisfy the
// same constraints the generator honours; violations are rejected
// with a reason instead of being silently accepted.
func (p *Plan) Reassign(date, spot, household string) error {
	day, err := ParseDate(date)
	if err != nil {
		return errors.New("日付の形式が不正です")
	}
	idx := -1
	for i, a := range p.Assignments {
		if a.Date == date && a.Spot == spot {
			idx = i
			break
		}
	}
	// A slot listed as unassigned can also be filled by hand.
	uIdx := -1
	if idx < 0 {
		for i, u := range p.Unassigned {
			if u.Date == date && u.Spot == spot {
				uIdx = i
				break
			}
		}
		if uIdx < 0 {
			return errors.New("その日・地点の割り当てが見つかりません")
		}
	}
	if household == "" && idx < 0 {
		return errors.New("その日・地点はすでに未割り当てです")
	}
	if household != "" {
		var h *Household
		for i := range p.Households {
			if p.Households[i].Name == household {
				h = &p.Households[i]
				break
			}
		}
		if h == nil {
			return fmt.Errorf("世帯 %q が名簿にありません", household)
		}
		if !h.CanServe(spot) {
			return fmt.Errorf("%s は地点「%s」を担当できません", household, spot)
		}
		if !h.AvailableOn(day.Weekday()) {
			return fmt.Errorf("%s は%s曜が不可曜日です", household, WeekdayJP(day.Weekday()))
		}
		for _, a := range p.Assignments {
			if a.Date == date && a.Household == household && a.Spot != spot {
				return fmt.Errorf("%s は同日の地点「%s」にすでに割り当てられています", household, a.Spot)
			}
		}
	}
	if idx >= 0 {
		if household == "" {
			p.Assignments = append(p.Assignments[:idx], p.Assignments[idx+1:]...)
			p.Unassigned = append(p.Unassigned, Unassigned{
				Date: date, Spot: spot, Reason: "手動で未割り当てに変更されました",
			})
		} else {
			p.Assignments[idx].Household = household
		}
	} else {
		p.Unassigned = append(p.Unassigned[:uIdx], p.Unassigned[uIdx+1:]...)
		p.Assignments = append(p.Assignments, Assignment{Date: date, Spot: spot, Household: household})
		sort.Slice(p.Assignments, func(i, j int) bool {
			if p.Assignments[i].Date != p.Assignments[j].Date {
				return p.Assignments[i].Date < p.Assignments[j].Date
			}
			return p.Assignments[i].Spot < p.Assignments[j].Spot
		})
	}
	return nil
}
