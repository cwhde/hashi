package jobs

import (
	"context"
	"time"
)

// Noop proves the runner end to end in S0: it records a run every interval and does nothing else.
func Noop() Job {
	return Job{Kind: "noop", Interval: 5 * time.Minute, Run: func(context.Context) (string, error) {
		return "ok", nil
	}}
}
