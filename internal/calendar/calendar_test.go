package calendar

import (
	"testing"
	"time"

	"github.com/apognu/gocal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func newAllDay(day time.Time) *gocal.Event {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, time.UTC)
	return &gocal.Event{Summary: "all-day", Start: &start, End: &end}
}

func newEvent(name string, start, end int64) *gocal.Event {
	s := time.Unix(start, 0).In(time.UTC)
	e := time.Unix(end, 0).In(time.UTC)
	return &gocal.Event{Summary: name, Start: &s, End: &e}
}

func TestNextEvent(t *testing.T) {
	tz := "Etc/UTC"
	loc, err := time.LoadLocation(tz)
	require.NoError(t, err)
	now := time.Now().In(time.UTC)
	nowUnix := now.Unix()

	tomorrow := now.AddDate(0, 0, 1)

	tests := []struct {
		name           string
		showInProgress bool
		includeAllDay  bool
		onlyAllDay     bool
		build          func() []*gocal.Event
		wantName       string
		check          func(t *testing.T, e *Event)
	}{
		{
			name:           "empty returns error",
			showInProgress: true,
			includeAllDay:  true,
			build:          func() []*gocal.Event { return nil },
		},
		{
			name:           "prefers in-progress when shown",
			showInProgress: true,
			includeAllDay:  true,
			build: func() []*gocal.Event {
				inProg := newEvent("in-progress", nowUnix-5*60, nowUnix+30*60)
				future := newEvent("future", nowUnix+10*60, nowUnix+70*60)
				return []*gocal.Event{future, inProg}
			},
			wantName: "in-progress",
			check: func(t *testing.T, e *Event) {
				require.NotNil(t, e.Detail)
				assert.True(t, e.Detail.InProgress)
			},
		},
		{
			name:           "picks soonest end among in-progress",
			showInProgress: true,
			includeAllDay:  true,
			build: func() []*gocal.Event {
				longer := newEvent("longer", nowUnix-20*60, nowUnix+40*60)
				shorter := newEvent("shorter", nowUnix-10*60, nowUnix+5*60)
				return []*gocal.Event{longer, shorter}
			},
			wantName: "shorter",
		},
		{
			name:           "ignore in-progress when disabled",
			showInProgress: false,
			includeAllDay:  true,
			build: func() []*gocal.Event {
				inProg := newEvent("in-progress", nowUnix-5*60, nowUnix+30*60)
				future1 := newEvent("future1", nowUnix+15*60, nowUnix+25*60)
				future2 := newEvent("future2", nowUnix+10*60, nowUnix+20*60)
				return []*gocal.Event{inProg, future1, future2}
			},
			wantName: "future2",
			check: func(t *testing.T, e *Event) {
				require.NotNil(t, e.Detail)
				assert.False(t, e.Detail.InProgress)
			},
		},
		{
			name:           "only all-day shows today's all-day",
			showInProgress: true,
			includeAllDay:  true,
			onlyAllDay:     true,
			build: func() []*gocal.Event {
				allDay := newAllDay(now)
				dsu := allDay.Start.Unix()
				timed := newEvent("timed", dsu+12*60*60, dsu+13*60*60)
				return []*gocal.Event{timed, allDay}
			},
			wantName: "all-day",
			check: func(t *testing.T, e *Event) {
				require.NotNil(t, e.Detail)
				assert.True(t, e.Detail.IsAllDay)
				assert.True(t, e.Detail.IsToday)
			},
		},
		{
			name:           "exclude all-day when disabled",
			showInProgress: false, // avoid hasInProgress filtering out future events
			includeAllDay:  false,
			build: func() []*gocal.Event {
				allDay := newAllDay(now)
				// Ensure the timed event is always in the future relative to now
				start := now.Add(1 * time.Hour).Unix()
				end := now.Add(2 * time.Hour).Unix()
				timed := newEvent("timed", start, end)
				return []*gocal.Event{allDay, timed}
			},
			wantName: "timed",
		},
		{
			name:           "no in-progress sorts by start (all-day first)",
			showInProgress: true,
			includeAllDay:  true,
			build: func() []*gocal.Event {
				// Place both events tomorrow so none are in-progress
				dayStart := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, time.UTC)
				dayEnd := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 23, 59, 59, 0, time.UTC)
				allDay := &gocal.Event{Summary: "all-day", Start: &dayStart, End: &dayEnd}
				timed := newEvent("timed", dayStart.Unix()+15*60*60, dayStart.Unix()+16*60*60)
				return []*gocal.Event{timed, allDay}
			},
			wantName: "all-day",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cal := &Calendar{
				params: &Request{
					TZ:                   tz,
					ShowInProgress:       ptr.To(tt.showInProgress),
					IncludeAllDayEvents:  ptr.To(tt.includeAllDay),
					OnlyShowAllDayEvents: tt.onlyAllDay,
				},
				events: tt.build(),
				tz:     loc,
			}
			got := cal.NextEvent()
			if got != nil {
				assert.Equal(t, tt.wantName, got.Name)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
