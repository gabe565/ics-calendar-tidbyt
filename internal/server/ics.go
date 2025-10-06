package server

import (
	"log/slog"
	"net/http"

	"gabe565.com/ics-calendar-tidbyt/internal/calendar"
	"github.com/go-chi/render"
)

func ICS() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var params calendar.Request
		if err := render.Bind(r, &params); err != nil {
			slog.Error("Failed to read request body", "error", err)
			_ = render.Render(w, r,
				calendar.NewErrorResponse(http.StatusBadRequest, "Failed to read request body"),
			)
			return
		}

		cal, err := calendar.LoadCalendar(r.Context(), params)
		if err != nil {
			slog.Error("Failed to fetch calendar", "error", err)
			_ = render.Render(w, r,
				calendar.NewErrorResponse(http.StatusInternalServerError, "Failed to fetch calendar"),
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
