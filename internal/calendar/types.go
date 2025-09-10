package calendar

import (
	"net/http"

	"k8s.io/utils/ptr"
)

type Event struct {
	Name      string
	StartTime int64
	EndTime   int64
	Location  *string
	Detail    *EventDetail
	IsAllDay  bool
}

type EventDetail struct {
	IsToday           bool `json:"isToday"`
	IsTomorrow        bool `json:"isTomorrow"`
	IsThisWeek        bool `json:"isThisWeek"`
	MinutesUntilStart int  `json:"minutesUntilStart"`
	MinutesUntilEnd   int  `json:"minutesUntilEnd"`
	HoursToEnd        int  `json:"hoursToEnd"`
	InProgress        bool `json:"inProgress"`
	IsAllDay          bool `json:"isAllDay"`
}

type BaseResponse struct {
	Data  *Response      `json:"data"`
	Error *ErrorResponse `json:"message"`
}

func (b BaseResponse) Render(http.ResponseWriter, *http.Request) error {
	return nil
}

type Request struct {
	ICSUrl               string `json:"icsUrl"`
	TZ                   string `json:"tz"`
	ShowInProgress       *bool  `json:"showInProgress" `
	IncludeAllDayEvents  *bool  `json:"includeAllDayEvents" `
	OnlyShowAllDayEvents bool   `json:"onlyShowAllDayEvents" `
}

func (i *Request) Bind(*http.Request) error {
	if i.ShowInProgress == nil {
		i.ShowInProgress = ptr.To(true)
	}
	if i.IncludeAllDayEvents == nil {
		i.IncludeAllDayEvents = ptr.To(true)
	}
	return nil
}

type ErrorResponse struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
}

type Response struct {
	Name      string       `json:"name"`
	StartTime int64        `json:"start"`
	EndTime   int64        `json:"end"`
	Location  *string      `json:"location"`
	Detail    *EventDetail `json:"detail"`
}
