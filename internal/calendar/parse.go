package calendar

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"
)

// event is a single, fully expanded calendar occurrence.
type event struct {
	Summary  string
	Location string
	Start    time.Time
	// End is inclusive. For all-day events it is one second before midnight
	// on the last day.
	End    time.Time
	AllDay bool
}

// parse reads an iCalendar feed and returns every occurrence overlapping
// [windowStart, windowEnd], with recurring events expanded.
func parse(r io.Reader, windowStart, windowEnd time.Time, tz *time.Location) ([]*event, error) {
	cal, err := ics.ParseCalendar(r)
	if err != nil {
		return nil, err
	}

	vevents := cal.Events()

	// Overrides are individual instances of a recurring series that were
	// edited (RECURRENCE-ID). They replace the generated occurrence with the
	// same UID and original start time.
	overrides := make(map[string]map[int64]struct{})
	for _, ve := range vevents {
		rid := ve.GetProperty(ics.ComponentPropertyRecurrenceId)
		if rid == nil {
			continue
		}
		t, _, err := parseTimeProp(rid, tz)
		if err != nil {
			continue
		}
		uid := ve.Id()
		if overrides[uid] == nil {
			overrides[uid] = make(map[int64]struct{})
		}
		overrides[uid][t.Unix()] = struct{}{}
	}

	events := make([]*event, 0, len(vevents))
	for _, ve := range vevents {
		if status := ve.GetProperty(ics.ComponentPropertyStatus); status != nil &&
			strings.EqualFold(status.Value, string(ics.ObjectStatusCancelled)) {
			continue
		}

		start, end, allDay, err := eventTimes(ve, tz)
		if err != nil {
			// Skip a malformed event rather than fail the whole feed.
			continue
		}

		base := event{
			Summary:  propValue(ve, ics.ComponentPropertySummary),
			Location: propValue(ve, ics.ComponentPropertyLocation),
			AllDay:   allDay,
		}
		duration := end.Sub(start)

		rrule := ve.GetProperty(ics.ComponentPropertyRrule)
		if rrule == nil {
			if !start.After(windowEnd) && !end.Before(windowStart) {
				e := base
				e.Start, e.End = start, end
				events = append(events, &e)
			}
			continue
		}

		set, err := recurrenceSet(ve, rrule.Value, start, tz)
		if err != nil {
			continue
		}

		skip := overrides[ve.Id()]
		// Widen the lower bound so occurrences that started before the window
		// but are still in progress are included.
		for _, occStart := range set.Between(windowStart.Add(-duration), windowEnd, true) {
			if _, ok := skip[occStart.Unix()]; ok {
				continue
			}
			e := base
			e.Start, e.End = occStart, occStart.Add(duration)
			events = append(events, &e)
		}
	}

	return events, nil
}

func recurrenceSet(ve *ics.VEvent, rule string, start time.Time, tz *time.Location) (*rrule.Set, error) {
	opt, err := rrule.StrToROptionInLocation(rule, start.Location())
	if err != nil {
		return nil, err
	}
	opt.Dtstart = start
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return nil, err
	}

	set := &rrule.Set{}
	set.DTStart(start)
	set.RRule(r)
	for _, t := range dateListProps(ve, ics.ComponentPropertyExdate, tz) {
		set.ExDate(t)
	}
	for _, t := range dateListProps(ve, ics.ComponentPropertyRdate, tz) {
		set.RDate(t)
	}
	return set, nil
}

// dateListProps parses properties like EXDATE and RDATE, which may appear
// multiple times and may hold comma-separated lists.
func dateListProps(ve *ics.VEvent, name ics.ComponentProperty, tz *time.Location) []time.Time {
	var out []time.Time
	for _, prop := range ve.GetProperties(name) {
		for v := range strings.SplitSeq(prop.Value, ",") {
			p := *prop
			p.Value = v
			if t, _, err := parseTimeProp(&p, tz); err == nil {
				out = append(out, t)
			}
		}
	}
	return out
}

var errMissingStart = errors.New("event has no DTSTART")

// eventTimes returns the start and inclusive end of an event, resolving
// DTEND, DURATION, and the RFC 5545 defaults when both are absent.
func eventTimes(ve *ics.VEvent, tz *time.Location) (time.Time, time.Time, bool, error) {
	startProp := ve.GetProperty(ics.ComponentPropertyDtStart)
	if startProp == nil {
		return time.Time{}, time.Time{}, false, errMissingStart
	}
	start, allDay, err := parseTimeProp(startProp, tz)
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}

	var end time.Time
	switch {
	case ve.GetProperty(ics.ComponentPropertyDtEnd) != nil:
		end, _, err = parseTimeProp(ve.GetProperty(ics.ComponentPropertyDtEnd), tz)
		if err != nil {
			return time.Time{}, time.Time{}, false, err
		}
	case ve.GetProperty(ics.ComponentPropertyDuration) != nil:
		d, err := parseDuration(ve.GetProperty(ics.ComponentPropertyDuration).Value)
		if err != nil {
			return time.Time{}, time.Time{}, false, err
		}
		end = start.Add(d)
	case allDay:
		end = start.AddDate(0, 0, 1)
	default:
		end = start
	}

	if allDay {
		// DTEND for date values is exclusive; make it inclusive.
		end = end.Add(-time.Second)
	}
	return start, end, allDay, nil
}

const (
	dateFormat     = "20060102"
	dateTimeFormat = "20060102T150405"
)

// parseTimeProp parses a DATE or DATE-TIME property value. Floating times and
// DATE values are interpreted in tz. Reports whether the value was a DATE.
func parseTimeProp(prop *ics.IANAProperty, tz *time.Location) (time.Time, bool, error) {
	value := strings.TrimSpace(prop.Value)

	loc := tz
	if tzid, ok := prop.ICalParameters[string(ics.ParameterTzid)]; ok && len(tzid) == 1 {
		l, err := time.LoadLocation(tzid[0])
		if err != nil {
			return time.Time{}, false, fmt.Errorf("unknown TZID %q: %w", tzid[0], err)
		}
		loc = l
	}

	v, ok := prop.ICalParameters[string(ics.ParameterValue)]
	isDate := ok && len(v) == 1 && v[0] == string(ics.ValueDataTypeDate)
	if isDate || len(value) == len(dateFormat) {
		t, err := time.ParseInLocation(dateFormat, value, tz)
		return t, true, err
	}

	if before, ok := strings.CutSuffix(value, "Z"); ok {
		t, err := time.ParseInLocation(dateTimeFormat, before, time.UTC)
		return t, false, err
	}

	t, err := time.ParseInLocation(dateTimeFormat, value, loc)
	return t, false, err
}

var errInvalidDuration = errors.New("invalid duration")

// parseDuration parses an RFC 5545 DURATION value such as P1DT2H30M or PT15M.
// Units must appear in order (W, D, then H, M, S after the T designator).
func parseDuration(s string) (time.Duration, error) {
	rest, neg := strings.CutPrefix(s, "-")
	if !neg {
		rest = strings.TrimPrefix(rest, "+")
	}
	rest, ok := strings.CutPrefix(rest, "P")
	if !ok || rest == "" {
		return 0, fmt.Errorf("%w: %q", errInvalidDuration, s)
	}

	var d time.Duration
	var inTime bool
	last := time.Duration(math.MaxInt64) // Units must appear largest to smallest.
	for rest != "" {
		if !inTime {
			if rest, ok = strings.CutPrefix(rest, "T"); ok {
				inTime = true
				if rest == "" {
					return 0, fmt.Errorf("%w: %q", errInvalidDuration, s)
				}
				continue
			}
		}

		digits := strings.IndexFunc(rest, notDigit)
		if digits <= 0 {
			return 0, fmt.Errorf("%w: %q", errInvalidDuration, s)
		}
		n, err := strconv.Atoi(rest[:digits])
		if err != nil {
			return 0, fmt.Errorf("%w: %q: %w", errInvalidDuration, s, err)
		}

		var unit time.Duration
		var unitInTime bool
		switch rest[digits] {
		case 'W':
			unit = 7 * 24 * time.Hour
		case 'D':
			unit = 24 * time.Hour
		case 'H':
			unit, unitInTime = time.Hour, true
		case 'M':
			unit, unitInTime = time.Minute, true
		case 'S':
			unit, unitInTime = time.Second, true
		default:
			return 0, fmt.Errorf("%w: %q", errInvalidDuration, s)
		}
		if unit >= last || unitInTime != inTime {
			return 0, fmt.Errorf("%w: %q", errInvalidDuration, s)
		}
		last = unit

		d += time.Duration(n) * unit
		rest = rest[digits+1:]
	}

	if neg {
		d = -d
	}
	return d, nil
}

func notDigit(r rune) bool { return r < '0' || r > '9' }

func propValue(ve *ics.VEvent, name ics.ComponentProperty) string {
	if p := ve.GetProperty(name); p != nil {
		return p.Value
	}
	return ""
}
