package searchq

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Span is a stretch of time, [Start, End). Relative is whether it was
// written as a length back from now ("3d"): after:3d and since:3d then
// both mean from its start.
type Span struct {
	Start, End time.Time
	Relative   bool
}

var relative = regexp.MustCompile(`^(\d+)(h|d|w|m|y)$`)

var weekdays = map[string]time.Weekday{}
var months = map[string]time.Month{}

func init() {
	for d := time.Sunday; d <= time.Saturday; d++ {
		n := strings.ToLower(d.String())
		weekdays[n], weekdays[n[:3]] = d, d
	}
	for m := time.January; m <= time.December; m++ {
		n := strings.ToLower(m.String())
		months[n], months[n[:3]] = m, m
	}
	weekdays["tues"], weekdays["thur"], weekdays["thurs"] = time.Tuesday, time.Thursday, time.Thursday
	months["sept"] = time.September
}

// TimeWords are the named times ParseTime takes, in the order to offer
// them.
var TimeWords = []string{"today", "yesterday", "this-week", "last-week", "this-month", "last-month", "this-year", "last-year"}

// ParseTime reads a time value, in now's location:
//
//	2026-10-01, 2026-10, 2026          a day, month or year
//	today, yesterday                   a day
//	monday … sunday (mon …)            the latest such day, today included
//	january … december (jan …)         the latest such month, this one included
//	this-week, last-week, this-month,  weeks start on Monday
//	last-month, this-year, last-year
//	4h, 3d, 2w, 6m, 1y                 that long back from now (Relative)
//
// Spaces may stand for the dashes ("last week").
func ParseTime(v string, now time.Time) (Span, error) {
	v = strings.Join(strings.Fields(strings.ToLower(v)), "-")
	loc := now.Location()
	day := func(t time.Time) time.Time {
		y, m, d := t.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	days := func(start time.Time, n int) Span { return Span{Start: start, End: start.AddDate(0, 0, n)} }
	today := day(now)
	if m := relative.FindStringSubmatch(v); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil || n <= 0 {
			return Span{}, fmt.Errorf("%q is not a length of time", v)
		}
		var start time.Time
		switch m[2] {
		case "h":
			start = now.Add(-time.Duration(n) * time.Hour)
		case "d":
			start = now.AddDate(0, 0, -n)
		case "w":
			start = now.AddDate(0, 0, -7*n)
		case "m":
			start = now.AddDate(0, -n, 0)
		case "y":
			start = now.AddDate(-n, 0, 0)
		}
		return Span{Start: start, End: now, Relative: true}, nil
	}
	monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	month := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc)
	year := time.Date(today.Year(), 1, 1, 0, 0, 0, 0, loc)
	switch v {
	case "today":
		return days(today, 1), nil
	case "yesterday":
		return days(today.AddDate(0, 0, -1), 1), nil
	case "this-week":
		return days(monday, 7), nil
	case "last-week":
		return days(monday.AddDate(0, 0, -7), 7), nil
	case "this-month":
		return Span{Start: month, End: month.AddDate(0, 1, 0)}, nil
	case "last-month":
		return Span{Start: month.AddDate(0, -1, 0), End: month}, nil
	case "this-year":
		return Span{Start: year, End: year.AddDate(1, 0, 0)}, nil
	case "last-year":
		return Span{Start: year.AddDate(-1, 0, 0), End: year}, nil
	}
	if d, ok := weekdays[v]; ok {
		back := (int(today.Weekday()) - int(d) + 7) % 7
		return days(today.AddDate(0, 0, -back), 1), nil
	}
	if m, ok := months[v]; ok {
		y := today.Year()
		if m > today.Month() {
			y--
		}
		start := time.Date(y, m, 1, 0, 0, 0, 0, loc)
		return Span{Start: start, End: start.AddDate(0, 1, 0)}, nil
	}
	for _, f := range []struct {
		layout string
		years  int
		months int
		days   int
	}{{"2006-01-02", 0, 0, 1}, {"2006-01", 0, 1, 0}, {"2006", 1, 0, 0}} {
		if t, err := time.ParseInLocation(f.layout, v, loc); err == nil && len(v) == len(f.layout) {
			return Span{Start: t, End: t.AddDate(f.years, f.months, f.days)}, nil
		}
	}
	return Span{}, fmt.Errorf("%q is not a time", v)
}
