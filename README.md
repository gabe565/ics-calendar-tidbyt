# ICS Calendar Tidbyt

[![Build](https://github.com/gabe565/ics-calendar-tidbyt/actions/workflows/build.yml/badge.svg)](https://github.com/gabe565/ics-calendar-tidbyt/actions/workflows/build.yml)

Runs a simple HTTP server that handles requests for the Tidbyt Universal ICal app.

Inspired by [quesurifn/ics-calendar-tidbyt-lambda](https://github.com/quesurifn/ics-calendar-tidbyt-lambda), but can run in any environment.

## Installation

### Docker Compose

A [`compose.yaml`](compose.yaml) is included in this repo:

```shell
curl -O https://raw.githubusercontent.com/gabe565/ics-calendar-tidbyt/main/compose.yaml
docker compose up -d
```

### Docker

```shell
docker run -d --name ics-calendar-tidbyt -p 8080:8080 ghcr.io/gabe565/ics-calendar-tidbyt:latest
```

Images are published to [ghcr.io/gabe565/ics-calendar-tidbyt](https://github.com/gabe565/ics-calendar-tidbyt/pkgs/container/ics-calendar-tidbyt) for `linux/amd64` and `linux/arm64`.
Available tags:

- `latest`: the most recent release.
- `vX`, `vX.Y`, `vX.Y.Z`: a specific release.
- `beta`: built from the `main` branch.

### Binary

Download a prebuilt binary from [GitHub Releases](https://github.com/gabe565/ics-calendar-tidbyt/releases), or build from source:

```shell
git clone https://github.com/gabe565/ics-calendar-tidbyt.git
cd ics-calendar-tidbyt
go build
```

## Configuration

The server is configured with environment variables. See [envs.md](envs.md) for the full list.

## API

### `POST /`

Fetches an ICS calendar and returns the next upcoming event, or the event that is currently in progress.
Only events from the past day to the next 7 days are considered.

Request body:

| Field                  | Type   | Default  | Description                                  |
|------------------------|--------|----------|----------------------------------------------|
| `icsUrl`               | string | Required | URL of the ICS calendar to fetch.            |
| `tz`                   | string | `UTC`    | IANA time zone, like `America/Chicago`.      |
| `showInProgress`       | bool   | `true`   | Return an event that has already started.    |
| `includeAllDayEvents`  | bool   | `true`   | Include all-day events.                      |
| `onlyShowAllDayEvents` | bool   | `false`  | Ignore every event that is not all-day.      |

Example:

```shell
curl -X POST http://localhost:8080 \
  -H 'Content-Type: application/json' \
  -d '{"icsUrl": "https://example.com/calendar.ics", "tz": "America/Chicago"}'
```

Response:

```json
{
  "data": {
    "name": "Team Meeting",
    "start": 1790000000,
    "end": 1790003600,
    "location": "Conference Room",
    "detail": {
      "isToday": true,
      "isTomorrow": false,
      "isThisWeek": true,
      "minutesUntilStart": 42,
      "minutesUntilEnd": 102,
      "hoursToEnd": 1,
      "inProgress": false,
      "isAllDay": false
    }
  }
}
```

`start` and `end` are Unix timestamps. If there are no matching events, `data` is omitted and the response is `{}`.
On failure, the response has an `error` object with a `message` and a `4xx` or `5xx` status code.

Requests are rate limited to 10 per minute per client IP. This can be changed with `LIMIT_REQUESTS` and `LIMIT_WINDOW`.
If the server is behind a reverse proxy, set `TRUSTED_PROXIES` so the real client IP is read from the `X-Forwarded-For` header.

### `GET /ping`

Health check endpoint. Returns `200 OK` with the body `.`.
