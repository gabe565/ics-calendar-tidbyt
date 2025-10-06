package calendar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/apognu/gocal"
)

var ErrUpstreamStatus = errors.New("upstream status")

func LoadCalendar(ctx context.Context, params Request) (*Calendar, error) {
	tz, err := time.LoadLocation(params.TZ)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, params.ICSUrl, nil)
	if err != nil {
		return nil, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s", ErrUpstreamStatus, res.Status)
	}

	return Parse(res.Body, params, tz)
}

type Calendar struct {
	params Request
	events []*gocal.Event
	tz     *time.Location
}

func (c *Calendar) Len() int {
	return len(c.events)
}

func Parse(r io.Reader, params Request, tz *time.Location) (*Calendar, error) {
	parser := gocal.NewParser(r)
	now := time.Now().In(tz)
	parser.Start = new(now.AddDate(0, 0, -1))
	parser.End = new(now.AddDate(0, 0, 7))
	parser.AllDayEventsTZ = tz

	if err := parser.Parse(); err != nil {
		return nil, err
	}

	cal := &Calendar{params: params, tz: tz}
	cal.events = make([]*gocal.Event, 0, len(parser.Events))
	for _, e := range parser.Events {
		cal.events = append(cal.events, &e)
	}

	return cal, nil
}

func (c *Calendar) NextEvent() *Event {
	now := time.Now().In(c.tz)

	c.events = slices.DeleteFunc(c.events, func(event *gocal.Event) bool {
		eventIsAllDay := isAllDay(event)
		return (c.params.OnlyShowAllDayEvents && !eventIsAllDay) ||
			(!*c.params.IncludeAllDayEvents && eventIsAllDay) ||
			(!*c.params.ShowInProgress && event.Start.Before(now)) ||
			event.End.Before(now)
	})

	hasInProgress := *c.params.ShowInProgress && slices.ContainsFunc(c.events, func(event *gocal.Event) bool {
		return event.Start.Before(now) && event.End.After(now)
	})

	if hasInProgress {
		c.events = slices.DeleteFunc(c.events, func(event *gocal.Event) bool {
			return event.Start.After(now)
		})
		slices.SortFunc(c.events, func(a, b *gocal.Event) int {
			return a.End.Compare(*b.End)
		})
	} else {
		slices.SortFunc(c.events, func(a, b *gocal.Event) int {
			return a.Start.Compare(*b.Start)
		})
	}

	if len(c.events) == 0 {
		return nil
	}

	event := c.events[0]
	return &Event{
		Name:     event.Summary,
		Start:    event.Start.Unix(),
		End:      event.End.Unix(),
		Location: event.Location,
		Detail: EventDetail{
			IsToday:           dateEqual(now, *event.Start),
			IsTomorrow:        dateEqual(now.AddDate(0, 0, 1), *event.Start),
			IsThisWeek:        now.AddDate(0, 0, 7).After(*event.Start),
			MinutesUntilStart: int(time.Until(*event.Start).Minutes()),
			MinutesUntilEnd:   int(time.Until(*event.End).Minutes()),
			HoursToEnd:        int(time.Until(*event.End).Hours()),
			InProgress:        event.Start.Before(now),
			IsAllDay:          isAllDay(event),
		},
	}
}

// isAllDay verifies that Start is midnight and End is one second before midnight.
// All day events can span multiple days, but they always start and end at midnight.
func isAllDay(e *gocal.Event) bool {
	h1, m1, s1 := e.Start.Clock()
	h2, m2, s2 := e.End.Clock()
	return h1 == 0 && m1 == 0 && s1 == 0 && h2 == 23 && m2 == 59 && s2 == 59
}

func dateEqual(date1, date2 time.Time) bool {
	y1, m1, d1 := date1.Date()
	y2, m2, d2 := date2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}
