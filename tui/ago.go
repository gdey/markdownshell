package tui

import (
	"fmt"
	"math"
	"time"
)

// runAgo says how long before now t was, as mdsh-gui's output title bar
// does: "42 secs ago" under a minute, then in whole minutes, hours,
// days, weeks, months and years, the largest unit that fits. A time not
// yet past, as a clock set a little ahead can give, is "just now". next
// is when the words change, so the view need only be drawn again then.
func runAgo(t, now time.Time) (words string, next time.Time) {
	s := int64(now.Sub(t) / time.Second)
	if s < 1 {
		return "just now", t.Add(time.Second)
	}
	const day = 86400
	days := s / day
	at := func(secs int64) time.Time { return t.Add(time.Duration(secs) * time.Second) }
	var n int64
	var unit string
	switch {
	case s < 60:
		n, unit, next = s, "sec", at(s+1)
	case s < 3600:
		n, unit = s/60, "min"
		next = at((n + 1) * 60)
	case s < day:
		n, unit = s/3600, "hour"
		next = at((n + 1) * 3600)
	case days < 7:
		n, unit, next = days, "day", at((days+1)*day)
	case days < 30:
		n, unit = days/7, "week"
		next = at(min((n+1)*7, 30) * day)
	case days < 365:
		n, unit = max(1, int64(float64(days)/30.4375)), "month"
		next = at(min(int64(math.Ceil(float64(n+1)*30.4375)), 365) * day)
	default:
		n, unit = max(1, int64(float64(days)/365.25)), "year"
		next = at(int64(math.Ceil(float64(n+1)*365.25)) * day)
	}
	if n != 1 {
		unit += "s"
	}
	return fmt.Sprintf("%d %s ago", n, unit), next
}
