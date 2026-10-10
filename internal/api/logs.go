package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Limits on the logs request, as in the API.
const (
	MaxLogsSince = 7 * 24 * time.Hour
	MaxLogsLimit = 1000
)

// LogLine is one line of an app's log.
type LogLine struct {
	// TS is nanoseconds since the Unix epoch, as the API gives it. It is the
	// cursor for following the log.
	TS        string
	Time      time.Time
	Line      string
	Container string
}

// LogsQuery selects log lines. Zero values use the API's defaults.
type LogsQuery struct {
	Since time.Duration
	Limit int
	Text  string
	// After gives only the lines after this TS, oldest first.
	After string
}

// Logs returns an app's log lines, oldest first, and the After for the next
// request.
func (c *Client) Logs(ctx context.Context, appID string, q LogsQuery) ([]LogLine, string, error) {
	query := url.Values{}
	if q.Since > 0 {
		query.Set("since", strconv.Itoa(int(q.Since.Seconds())))
	}
	if q.Limit > 0 {
		query.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.Text != "" {
		query.Set("text", q.Text)
	}
	if q.After != "" {
		query.Set("after", q.After)
	}

	var doc struct {
		Meta struct {
			Lines []struct {
				TS        string  `json:"ts"`
				Time      apiTime `json:"time"`
				Line      string  `json:"line"`
				Container *string `json:"container"`
			} `json:"lines"`
			NextAfter *string `json:"next_after"`
		} `json:"meta"`
	}
	if err := c.document(ctx, request{method: http.MethodGet, path: []string{"apps", appID, "logs"}, query: query}, &doc); err != nil {
		return nil, "", err
	}

	lines := make([]LogLine, 0, len(doc.Meta.Lines))
	for _, l := range doc.Meta.Lines {
		if l.TS == "" {
			return nil, "", fmt.Errorf("unexpected response from the API: a log line without ts")
		}
		lines = append(lines, LogLine{TS: l.TS, Time: l.Time.Time, Line: l.Line, Container: deref(l.Container)})
	}
	return lines, deref(doc.Meta.NextAfter), nil
}
