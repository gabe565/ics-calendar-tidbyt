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

// parser holds the state for a single pass over a feed.
type parser struct {
	tz                     *time.Location
	windowStart, windowEnd time.Time
	// locations caches time.LoadLocation, which reads zoneinfo from disk on
	// every call and otherwise dominates parse time on large feeds.
	locations map[string]*time.Location

	events []*occurrence
	// overrides records RECURRENCE-ID instances by UID and original start.
	overrides map[string]map[int64]struct{}
}

func newParser(tz *time.Location) *parser {
	return &parser{
		tz:        tz,
		locations: map[string]*time.Location{tz.String(): tz},
		overrides: make(map[string]map[int64]struct{}),
	}
}

// location resolves a TZID, caching the result for the rest of the parse.
func (p *parser) location(tzid string) (*time.Location, error) {
	if loc, ok := p.locations[tzid]; ok {
		return loc, nil
	}
	loc, err := time.LoadLocation(tzid)
	if err != nil {
		return nil, fmt.Errorf("unknown TZID %q: %w", tzid, err)
	}
	p.locations[tzid] = loc
	return loc, nil
}

// parse streams an iCalendar feed and returns every occurrence overlapping
// [windowStart, windowEnd], with recurring events expanded. Only one VEVENT
// is held in memory at a time.
func parse(r io.Reader, windowStart, windowEnd time.Time, tz *time.Location) ([]*event, error) {
	p := newParser(tz)
	p.windowStart, p.windowEnd = windowStart, windowEnd

	stream := ics.NewCalendarStream(r)
	for {
		line, _, err := stream.ReadLine()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if line == nil || len(*line) == 0 {
			continue
		}

		prop, err := ics.ParseProperty(*line)
		if err != nil {
			return nil, err
		}
		if prop == nil || prop.IANAToken != "BEGIN" {
			continue
		}
		switch prop.Value {
		case string(ics.ComponentVCalendar):
			continue
		case string(ics.ComponentVEvent):
		default:
			// VTIMEZONE and friends are not needed; consume without building.
			if err := skipComponent(stream); err != nil {
				return nil, err
			}
			continue
		}

		component, err := ics.GeneralParseComponent(stream, prop)
		if err != nil {
			return nil, err
		}
		if ve, ok := component.(*ics.VEvent); ok {
			p.addEvent(ve)
		}
	}

	// Overrides may appear after the series they modify, so apply them once
	// the whole feed has been read.
	events := make([]*event, 0, len(p.events))
	for _, o := range p.events {
		if _, overridden := p.overrides[o.uid][o.seriesKey]; o.generated && overridden {
			continue
		}
		events = append(events, &o.event)
	}
	return events, nil
}

// skipComponent consumes lines through the END of the component whose BEGIN
// was just read, including any nested components.
func skipComponent(stream *ics.CalendarStream) error {
	for depth := 1; depth > 0; {
		line, _, err := stream.ReadLine()
		if err != nil {
			return err
		}
		if line == nil {
			continue
		}
		switch {
		case strings.HasPrefix(string(*line), "BEGIN:"):
			depth++
		case strings.HasPrefix(string(*line), "END:"):
			depth--
		}
	}
	return nil
}

// occurrence is an event plus the identity of the series instance it was
// generated from, so RECURRENCE-ID overrides can be applied afterwards.
type occurrence struct {
	event
	uid       string
	seriesKey int64
	generated bool
}

func (p *parser) addEvent(ve *ics.VEvent) {
	uid := ve.Id()

	if rid := ve.GetProperty(ics.ComponentPropertyRecurrenceId); rid != nil {
		if t, _, err := p.parseTimeProp(rid); err == nil {
			if p.overrides[uid] == nil {
				p.overrides[uid] = make(map[int64]struct{})
			}
			p.overrides[uid][t.Unix()] = struct{}{}
		}
	}

	if status := ve.GetProperty(ics.ComponentPropertyStatus); status != nil &&
		strings.EqualFold(status.Value, string(ics.ObjectStatusCancelled)) {
		return
	}

	start, end, allDay, err := p.eventTimes(ve)
	if err != nil {
		// Skip a malformed event rather than fail the whole feed.
		return
	}

	base := event{
		Summary:  propValue(ve, ics.ComponentPropertySummary),
		Location: propValue(ve, ics.ComponentPropertyLocation),
		AllDay:   allDay,
	}
	duration := end.Sub(start)

	rrule := ve.GetProperty(ics.ComponentPropertyRrule)
	if rrule == nil {
		if !start.After(p.windowEnd) && !end.Before(p.windowStart) {
			e := base
			e.Start, e.End = start, end
			p.events = append(p.events, &occurrence{event: e, uid: uid})
		}
		return
	}

	set, err := p.recurrenceSet(ve, rrule.Value, start)
	if err != nil {
		return
	}

	// Widen the lower bound so occurrences that started before the window
	// but are still in progress are included.
	for _, occStart := range set.Between(p.windowStart.Add(-duration), p.windowEnd, true) {
		e := base
		e.Start, e.End = occStart, occStart.Add(duration)
		p.events = append(p.events, &occurrence{event: e, uid: uid, seriesKey: occStart.Unix(), generated: true})
	}
}

func (p *parser) recurrenceSet(ve *ics.VEvent, rule string, start time.Time) (*rrule.Set, error) {
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
	for _, t := range p.dateListProps(ve, ics.ComponentPropertyExdate) {
		set.ExDate(t)
	}
	for _, t := range p.dateListProps(ve, ics.ComponentPropertyRdate) {
		set.RDate(t)
	}
	return set, nil
}

// dateListProps parses properties like EXDATE and RDATE, which may appear
// multiple times and may hold comma-separated lists.
func (p *parser) dateListProps(ve *ics.VEvent, name ics.ComponentProperty) []time.Time {
	var out []time.Time
	for _, prop := range ve.GetProperties(name) {
		for v := range strings.SplitSeq(prop.Value, ",") {
			single := *prop
			single.Value = v
			if t, _, err := p.parseTimeProp(&single); err == nil {
				out = append(out, t)
			}
		}
	}
	return out
}

var errMissingStart = errors.New("event has no DTSTART")

// eventTimes returns the start and inclusive end of an event, resolving
// DTEND, DURATION, and the RFC 5545 defaults when both are absent.
func (p *parser) eventTimes(ve *ics.VEvent) (time.Time, time.Time, bool, error) {
	startProp := ve.GetProperty(ics.ComponentPropertyDtStart)
	if startProp == nil {
		return time.Time{}, time.Time{}, false, errMissingStart
	}
	start, allDay, err := p.parseTimeProp(startProp)
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}

	var end time.Time
	switch {
	case ve.GetProperty(ics.ComponentPropertyDtEnd) != nil:
		end, _, err = p.parseTimeProp(ve.GetProperty(ics.ComponentPropertyDtEnd))
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
// DATE values are interpreted in the configured timezone. Reports whether the
// value was a DATE.
func (p *parser) parseTimeProp(prop *ics.IANAProperty) (time.Time, bool, error) {
	value := strings.TrimSpace(prop.Value)

	loc := p.tz
	if tzid, ok := prop.ICalParameters[string(ics.ParameterTzid)]; ok && len(tzid) == 1 {
		l, err := p.location(tzid[0])
		if err != nil {
			return time.Time{}, false, err
		}
		loc = l
	}

	v, ok := prop.ICalParameters[string(ics.ParameterValue)]
	isDate := ok && len(v) == 1 && v[0] == string(ics.ValueDataTypeDate)
	if isDate || len(value) == len(dateFormat) {
		t, err := time.ParseInLocation(dateFormat, value, p.tz)
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
