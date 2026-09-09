package calendar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"
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
	events []*event
	tz     *time.Location
}

func (c *Calendar) Len() int {
	return len(c.events)
}

func Parse(r io.Reader, params Request, tz *time.Location) (*Calendar, error) {
	now := time.Now().In(tz)
	events, err := parse(r, now.AddDate(0, 0, -1), now.AddDate(0, 0, 7), tz)
	if err != nil {
		return nil, err
	}
	return &Calendar{params: params, events: events, tz: tz}, nil
}

func (c *Calendar) NextEvent() *Event {
	now := time.Now().In(c.tz)

	c.events = slices.DeleteFunc(c.events, func(event *event) bool {
		return (c.params.OnlyShowAllDayEvents && !event.AllDay) ||
			(!*c.params.IncludeAllDayEvents && event.AllDay) ||
			(!*c.params.ShowInProgress && event.Start.Before(now)) ||
			event.End.Before(now)
	})

	hasInProgress := *c.params.ShowInProgress && slices.ContainsFunc(c.events, func(event *event) bool {
		return event.Start.Before(now) && event.End.After(now)
	})

	if hasInProgress {
		c.events = slices.DeleteFunc(c.events, func(event *event) bool {
			return event.Start.After(now)
		})
		slices.SortFunc(c.events, func(a, b *event) int {
			return a.End.Compare(b.End)
		})
	} else {
		slices.SortFunc(c.events, func(a, b *event) int {
			return a.Start.Compare(b.Start)
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
			IsToday:           dateEqual(now, event.Start.In(c.tz)),
			IsTomorrow:        dateEqual(now.AddDate(0, 0, 1), event.Start.In(c.tz)),
			IsThisWeek:        now.AddDate(0, 0, 7).After(event.Start),
			MinutesUntilStart: int(time.Until(event.Start).Minutes()),
			MinutesUntilEnd:   int(time.Until(event.End).Minutes()),
			HoursToEnd:        int(time.Until(event.End).Hours()),
			InProgress:        event.Start.Before(now),
			IsAllDay:          event.AllDay,
		},
	}
}

func dateEqual(date1, date2 time.Time) bool {
	y1, m1, d1 := date1.Date()
	y2, m2, d2 := date2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}
