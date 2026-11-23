package plan

import (
	"errors"
	"fmt"
	"sort"
)

// Generate builds the duty table for the plan's period.
//
// Every school day × every spot gets exactly one household, subject
// to three hard constraints: the household can serve that spot, the
// weekday is not one of its unavailable days, and it is not already
// on duty at another spot the same day. Among the households that
// satisfy the constraints, the one with the fewest duties so far is
// chosen, which keeps the totals as even as the constraints allow.
// Slots nobody can fill are returned as Unassigned with a reason —
// never silently skipped.
func Generate(p *Plan) ([]Assignment, []Unassigned, error) {
	if err := p.Validate(); err != nil {
		return nil, nil, err
	}
	if len(p.Households) == 0 {
		return nil, nil, errors.New("名簿に世帯が登録されていません")
	}
	if len(p.Spots) == 0 {
		return nil, nil, errors.New("地点が登録されていません")
	}
	days, err := SchoolDays(p.Settings)
	if err != nil {
		return nil, nil, err
	}
	if len(days) == 0 {
		return nil, nil, errors.New("期間内に対象日（平日・祝日と休業日を除く）がありません")
	}

	counts := make([]int, len(p.Households))
	var assignments []Assignment
	var unassigned []Unassigned

	for _, day := range days {
		wd := day.Weekday()
		date := dateKey(day)

		// Order the day's spots by fewest eligible households first,
		// so the hardest slots are filled before easy ones take the
		// remaining households.
		type slot struct {
			spot     string
			capable  []int // households that can serve the spot at all
			eligible []int // capable and available on this weekday
		}
		slots := make([]slot, 0, len(p.Spots))
		for _, spot := range p.Spots {
			var s slot
			s.spot = spot
			for i, h := range p.Households {
				if !h.CanServe(spot) {
					continue
				}
				s.capable = append(s.capable, i)
				if h.AvailableOn(wd) {
					s.eligible = append(s.eligible, i)
				}
			}
			slots = append(slots, s)
		}
		sort.SliceStable(slots, func(i, j int) bool {
			return len(slots[i].eligible) < len(slots[j].eligible)
		})

		usedToday := map[int]bool{}
		for _, s := range slots {
			best := -1
			for _, i := range s.eligible {
				if usedToday[i] {
					continue
				}
				if best < 0 || counts[i] < counts[best] {
					best = i
				}
			}
			if best < 0 {
				var reason string
				switch {
				case len(s.capable) == 0:
					reason = fmt.Sprintf("地点「%s」を担当できる世帯が登録されていません", s.spot)
				case len(s.eligible) == 0:
					reason = fmt.Sprintf("担当できる世帯がすべて%s曜不可のため割り当てできません", WeekdayJP(wd))
				default:
					reason = "担当できる世帯がすべて同日の他の地点に割り当て済みです"
				}
				unassigned = append(unassigned, Unassigned{Date: date, Spot: s.spot, Reason: reason})
				continue
			}
			assignments = append(assignments, Assignment{
				Date:      date,
				Spot:      s.spot,
				Household: p.Households[best].Name,
			})
			counts[best]++
			usedToday[best] = true
		}
	}
	if assignments == nil {
		assignments = []Assignment{}
	}
	if unassigned == nil {
		unassigned = []Unassigned{}
	}
	return assignments, unassigned, nil
}
