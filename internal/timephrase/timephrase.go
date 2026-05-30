// Package timephrase resolves natural-language time phrases into absolute
// timestamps. It mirrors the Python implementation's regex order and fallback
// rules so that the CLI behaves identically against existing data.
package timephrase

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Extract pulls the snooze/due phrase out of a free-text message. If the
// message contains "until", "after", "on", or "in" as a word boundary, the
// phrase starts there; otherwise the entire message is the phrase.
func Extract(message string) string {
	m := extractRe.FindStringSubmatchIndex(message)
	if m == nil {
		return strings.TrimSpace(message)
	}
	return strings.TrimSpace(message[m[0]:])
}

var (
	extractRe   = regexp.MustCompile(`(?i)\b(?:until|after|on|in)\b(.+)$`)
	isoDateRe   = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`)
	quarterRe   = regexp.MustCompile(`(?i)after\s+q([1-4])(?:\s+ends?)?(?:\s+(\d{4}))?`)
	inDaysRe    = regexp.MustCompile(`(?i)in\s+(\d+)\s+day`)
	inWeeksRe   = regexp.MustCompile(`(?i)in\s+(\d+)\s+week`)
	tomorrowRe  = regexp.MustCompile(`(?i)\btomorrow\b`)
	nextWeekRe  = regexp.MustCompile(`(?i)\bnext week\b`)
	nextMonthRe = regexp.MustCompile(`(?i)\bnext month\b`)
)

// Resolve converts text to an absolute time using `now` as the reference
// point and `tz` as the result timezone. The fallback (when nothing matches)
// is now + 7 days at 09:00 in tz, matching the Python.
func Resolve(text string, now time.Time, tz *time.Location) time.Time {
	lower := strings.ToLower(strings.TrimSpace(text))

	if m := isoDateRe.FindStringSubmatch(lower); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		return time.Date(y, time.Month(mo), d, 9, 0, 0, 0, tz)
	}

	if m := quarterRe.FindStringSubmatch(lower); m != nil {
		q, _ := strconv.Atoi(m[1])
		year := now.In(tz).Year()
		if m[2] != "" {
			year, _ = strconv.Atoi(m[2])
		}
		dt := quarterRollover(year, q, tz)
		if !dt.After(now) {
			dt = quarterRollover(year+1, q, tz)
		}
		return dt
	}

	if tomorrowRe.MatchString(lower) {
		return atNineAM(now.In(tz).AddDate(0, 0, 1), tz)
	}

	if nextWeekRe.MatchString(lower) {
		// Python: (7 - now.weekday()) % 7, then bump to 7 if zero.
		// Python's weekday: Monday=0..Sunday=6. Go: time.Monday=1..Sunday=0.
		// Convert Go's weekday to Python's so the math matches.
		nowLocal := now.In(tz)
		pyWeekday := (int(nowLocal.Weekday()) + 6) % 7 // Mon=0..Sun=6
		days := (7 - pyWeekday) % 7
		if days == 0 {
			days = 7
		}
		return atNineAM(nowLocal.AddDate(0, 0, days), tz)
	}

	if nextMonthRe.MatchString(lower) {
		nowLocal := now.In(tz)
		y := nowLocal.Year()
		mo := int(nowLocal.Month()) + 1
		if mo > 12 {
			mo = 1
			y++
		}
		return time.Date(y, time.Month(mo), 1, 9, 0, 0, 0, tz)
	}

	if m := inDaysRe.FindStringSubmatch(lower); m != nil {
		n, _ := strconv.Atoi(m[1])
		return atNineAM(now.In(tz).AddDate(0, 0, n), tz)
	}

	if m := inWeeksRe.FindStringSubmatch(lower); m != nil {
		n, _ := strconv.Atoi(m[1])
		return atNineAM(now.In(tz).AddDate(0, 0, n*7), tz)
	}

	// Fallback: 7 days from now at 09:00.
	return atNineAM(now.In(tz).AddDate(0, 0, 7), tz)
}

func quarterRollover(year, quarter int, tz *time.Location) time.Time {
	switch quarter {
	case 1:
		return time.Date(year, time.April, 1, 0, 0, 0, 0, tz)
	case 2:
		return time.Date(year, time.July, 1, 0, 0, 0, 0, tz)
	case 3:
		return time.Date(year, time.October, 1, 0, 0, 0, 0, tz)
	default:
		return time.Date(year+1, time.January, 1, 0, 0, 0, 0, tz)
	}
}

func atNineAM(t time.Time, tz *time.Location) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 9, 0, 0, 0, tz)
}
