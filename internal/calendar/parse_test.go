package calendar

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const icsHeader = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:test\r\n"

func wrap(events ...string) string {
	var b strings.Builder
	b.WriteString(icsHeader)
	for _, e := range events {
		b.WriteString("BEGIN:VEVENT\r\n")
		b.WriteString(strings.ReplaceAll(strings.TrimSpace(e), "\n", "\r\n"))
		b.WriteString("\r\nEND:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func chicago(t *testing.T) *time.Location {
	t.Helper()
	tz, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)
	return tz
}

func starts(events []*event, tz *time.Location) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Start.In(tz).Format("2006-01-02 15:04"))
	}
	slices.Sort(out)
	return out
}

func TestParse_Recurrence(t *testing.T) {
	tz := chicago(t)
	windowStart := time.Date(2026, 9, 1, 0, 0, 0, 0, tz)
	windowEnd := time.Date(2026, 9, 30, 23, 59, 59, 0, tz)

	tests := []struct {
		name  string
		ics   string
		want  []string
		check func(t *testing.T, events []*event)
	}{
		{
			// Regression: gocal expanded a first-Wednesday rule onto the second Wednesday.
			name: "monthly first wednesday",
			ics: wrap(`
UID:first-wed
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Chicago:20260506T154500
DTEND;TZID=America/Chicago:20260506T163000
RRULE:FREQ=MONTHLY;UNTIL=20261007T045959Z;BYDAY=1WE
SUMMARY:First and Third Wednesday`),
			want: []string{"2026-09-02 15:45"},
		},
		{
			name: "monthly third wednesday",
			ics: wrap(`
UID:third-wed
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Chicago:20251217T154500
DTEND;TZID=America/Chicago:20251217T163000
RRULE:FREQ=MONTHLY;UNTIL=20260916T045959Z;BYDAY=3WE
SUMMARY:First and Third Wednesday`),
			// UNTIL is before the Sep 16 occurrence, so it is excluded.
			want: []string{},
		},
		{
			name: "monthly second and fourth wednesday",
			ics: wrap(`
UID:second-wed
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Chicago:20260812T150000
DTEND;TZID=America/Chicago:20260812T153000
RRULE:FREQ=MONTHLY;BYDAY=2WE
SUMMARY:Second and Fourth Wednesday`, `
UID:fourth-wed
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Chicago:20260826T143000
DTEND;TZID=America/Chicago:20260826T150000
RRULE:FREQ=MONTHLY;UNTIL=20270923T045959Z;BYDAY=4WE
SUMMARY:Second and Fourth Wednesday`),
			want: []string{"2026-09-09 15:00", "2026-09-23 14:30"},
		},
		{
			name: "monthly first thursday",
			ics: wrap(`
UID:first-thu
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Chicago:20260903T103000
DTEND;TZID=America/Chicago:20260903T110000
RRULE:FREQ=MONTHLY;BYDAY=1TH
SUMMARY:First Thursday`),
			want: []string{"2026-09-03 10:30"},
		},
		{
			name: "biweekly",
			ics: wrap(`
UID:biweekly
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Chicago:20260812T130000
DTEND;TZID=America/Chicago:20260812T133000
RRULE:FREQ=WEEKLY;WKST=SU;INTERVAL=2;BYDAY=WE
SUMMARY:Biweekly`),
			want: []string{"2026-09-09 13:00", "2026-09-23 13:00"},
		},
		{
			name: "weekly with exdate and recurrence-id override",
			ics: wrap(`
UID:weekly
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Chicago:20260819T150000
DTEND;TZID=America/Chicago:20260819T153000
RRULE:FREQ=WEEKLY;BYDAY=WE
EXDATE;TZID=America/Chicago:20260909T150000
SUMMARY:Weekly`, `
UID:weekly
DTSTAMP:20260101T000000Z
RECURRENCE-ID;TZID=America/Chicago:20260916T150000
DTSTART;TZID=America/Chicago:20260917T100000
DTEND;TZID=America/Chicago:20260917T103000
SUMMARY:Weekly (moved)`),
			want: []string{"2026-09-02 15:00", "2026-09-17 10:00", "2026-09-23 15:00", "2026-09-30 15:00"},
		},
		{
			name: "weekly multiple days keeps wall clock in event tz",
			ics: wrap(`
UID:mon-fri
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Denver:20240415T074500
DTEND;TZID=America/Denver:20240415T080000
RRULE:FREQ=WEEKLY;WKST=SU;BYDAY=MO,FR
SUMMARY:Monday and Friday`),
			check: func(t *testing.T, events []*event) {
				require.Len(t, events, 8) // 4 Mondays and 4 Fridays
				for _, e := range events {
					assert.Equal(t, "08:45", e.Start.In(tz).Format("15:04"))
					assert.Contains(t, []time.Weekday{time.Monday, time.Friday}, e.Start.In(tz).Weekday())
				}
			},
		},
		{
			name: "count",
			ics: wrap(`
UID:count
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Chicago:20260901T090000
DTEND;TZID=America/Chicago:20260901T093000
RRULE:FREQ=DAILY;COUNT=3
SUMMARY:Daily`),
			want: []string{"2026-09-01 09:00", "2026-09-02 09:00", "2026-09-03 09:00"},
		},
		{
			name: "cancelled events are skipped",
			ics: wrap(`
UID:cancelled
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Chicago:20260910T090000
DTEND;TZID=America/Chicago:20260910T093000
STATUS:CANCELLED
SUMMARY:Nope`),
			want: []string{},
		},
		{
			name: "all-day recurring",
			ics: wrap(`
UID:allday
DTSTAMP:20260101T000000Z
DTSTART;VALUE=DATE:20260907
DTEND;VALUE=DATE:20260908
RRULE:FREQ=WEEKLY;COUNT=2
SUMMARY:Holiday`),
			want: []string{"2026-09-07 00:00", "2026-09-14 00:00"},
			check: func(t *testing.T, events []*event) {
				for _, e := range events {
					assert.True(t, e.AllDay)
					assert.Equal(t, "23:59:59", e.End.In(tz).Format("15:04:05"))
					assert.Equal(t, e.Start.In(tz).Day(), e.End.In(tz).Day())
				}
			},
		},
		{
			name: "duration and utc start",
			ics: wrap(`
UID:dur
DTSTAMP:20260101T000000Z
DTSTART:20260910T140000Z
DURATION:PT1H30M
SUMMARY:UTC`),
			want: []string{"2026-09-10 09:00"},
			check: func(t *testing.T, events []*event) {
				assert.Equal(t, 90*time.Minute, events[0].End.Sub(events[0].Start))
			},
		},
		{
			name: "in-progress multi-day event before window start is kept",
			ics: wrap(`
UID:long
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Chicago:20260830T090000
DTEND;TZID=America/Chicago:20260902T170000
SUMMARY:Multi-day`),
			want: []string{"2026-08-30 09:00"},
		},
		{
			name: "recurring occurrence before window start still in progress is kept",
			ics: wrap(`
UID:longrec
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Chicago:20260731T090000
DTEND;TZID=America/Chicago:20260802T170000
RRULE:FREQ=MONTHLY;BYMONTHDAY=31
SUMMARY:Month end`),
			want: []string{"2026-08-31 09:00"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := parse(strings.NewReader(tt.ics), windowStart, windowEnd, tz)
			require.NoError(t, err)
			if tt.want != nil {
				assert.Equal(t, tt.want, starts(events, tz))
			}
			if tt.check != nil {
				tt.check(t, events)
			}
		})
	}
}

func Test_parseDuration(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    time.Duration
		wantErr assert.ErrorAssertionFunc
	}{
		{"minutes", "PT15M", 15 * time.Minute, assert.NoError},
		{"hours and minutes", "PT1H30M", 90 * time.Minute, assert.NoError},
		{"seconds", "PT5S", 5 * time.Second, assert.NoError},
		{"days", "P1D", 24 * time.Hour, assert.NoError},
		{"days and hours", "P1DT2H", 26 * time.Hour, assert.NoError},
		{"weeks", "P2W", 14 * 24 * time.Hour, assert.NoError},
		{"negative", "-PT10M", -10 * time.Minute, assert.NoError},
		{"explicit positive", "+PT10M", 10 * time.Minute, assert.NoError},
		{"empty", "", 0, assert.Error},
		{"missing P", "1H", 0, assert.Error},
		{"bare P", "P", 0, assert.Error},
		{"bare T", "PT", 0, assert.Error},
		{"unknown unit", "P1X", 0, assert.Error},
		{"time unit without T", "P1H", 0, assert.Error},
		{"date unit after T", "PT1H1D", 0, assert.Error},
		{"units out of order", "PT1M1H", 0, assert.Error},
		{"repeated unit", "PT1H1H", 0, assert.Error},
		{"missing number", "PTM", 0, assert.Error},
		{"missing unit", "PT30", 0, assert.Error},
		{"fractional", "P1.5D", 0, assert.Error},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.s)
			if !tt.wantErr(t, err, fmt.Sprintf("parseDuration(%q)", tt.s)) {
				return
			}
			assert.Equalf(t, tt.want, got, "parseDuration(%q)", tt.s)
		})
	}
}
