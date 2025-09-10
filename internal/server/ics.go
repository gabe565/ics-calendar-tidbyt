package server

import (
	"log/slog"
	"net/http"

	"gabe565.com/ics-calendar-tidbyt/internal/calendar"
	"github.com/go-chi/render"
)

func ICS() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cal, err := calendar.NewCalendarRequest(r)
		if err != nil {
			slog.Error("Failed to fetch calendar", "error", err)
			_ = render.Render(w, r,
				calendar.NewErrorResponse(http.StatusInternalServerError, "Failed to fetch calendar"),
			)
			return
		}
		defer func() {
			_ = cal.Close()
		}()

		if err := cal.Parse(); err != nil {
			slog.Error("Failed to parse calendar", "error", err)
			_ = render.Render(w, r,
				calendar.NewErrorResponse(http.StatusInternalServerError, "Failed to parse calendar"),
			)
			return
		}

		if cal.Len() == 0 {
			_ = render.Render(w, r, calendar.BaseResponse{})
			return
		}

		next := cal.NextEvent()
		if next == nil {
			_ = render.Render(w, r, calendar.BaseResponse{})
			return
		}

		_ = render.Render(w, r, calendar.BaseResponse{Event: *next})
	}
}
