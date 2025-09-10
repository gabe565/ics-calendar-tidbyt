package server

import (
	"errors"
	"net/http"

	"gabe565.com/ics-calendar-tidbyt/internal/calendar"
	"github.com/go-chi/render"
)

func ICS() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cal, err := calendar.NewCalendarRequest(r)
		if err != nil {
			panic(err)
		}
		defer func() {
			_ = cal.Close()
		}()

		if err := cal.Parse(); err != nil {
			panic(err)
		}

		if cal.Len() == 0 {
			_ = render.Render(w, r, calendar.BaseResponse{})
			return
		}

		nextEvent, err := cal.NextEvent()
		if err != nil {
			if errors.Is(err, calendar.ErrEventsEmpty) {
				_ = render.Render(w, r, calendar.BaseResponse{})
				return
			}
			panic(err)
		}

		_ = render.Render(w, r, calendar.BaseResponse{Event: *nextEvent})
	}
}
