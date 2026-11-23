package plan

import (
	"fmt"
	"strings"
)

// ParseImport parses a pasted roster: one household per line, with
// columns separated by tabs or commas, in the order
//
//	世帯名 [, 班] [, 担当地点] [, 不可曜日]
//
// 担当地点 is a list separated by 、・/ or spaces; 不可曜日 is written
// with weekday characters, e.g. 月・水. Lines that cannot be read
// are reported in errs instead of being silently dropped.
func ParseImport(text string) (households []Household, errs []string) {
	weekdayByRune := map[rune]int{
		'日': 0, '月': 1, '火': 2, '水': 3, '木': 4, '金': 5, '土': 6,
	}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var fields []string
		if strings.Contains(line, "\t") {
			fields = strings.Split(line, "\t")
		} else {
			fields = strings.Split(line, ",")
		}
		for j := range fields {
			fields[j] = strings.TrimSpace(fields[j])
		}
		// Skip a header row if one was pasted along.
		if fields[0] == "世帯名" || fields[0] == "名前" || fields[0] == "氏名" {
			continue
		}
		if fields[0] == "" {
			errs = append(errs, fmt.Sprintf("%d行目: 世帯名が空です", i+1))
			continue
		}
		h := Household{Name: fields[0]}
		if len(fields) > 1 {
			h.Group = fields[1]
		}
		if len(fields) > 2 && fields[2] != "" {
			parts := strings.FieldsFunc(fields[2], func(r rune) bool {
				return r == '、' || r == '・' || r == '/' || r == '／' ||
					r == ' ' || r == '　' || r == ','
			})
			for _, part := range parts {
				if part = strings.TrimSpace(part); part != "" {
					h.Spots = append(h.Spots, part)
				}
			}
		}
		if len(fields) > 3 && fields[3] != "" {
			seen := map[int]bool{}
			for _, r := range fields[3] {
				if wd, ok := weekdayByRune[r]; ok && !seen[wd] {
					h.Unavailable = append(h.Unavailable, wd)
					seen[wd] = true
				}
			}
		}
		households = append(households, h)
	}
	if households == nil {
		households = []Household{}
	}
	return households, errs
}
