package cmdutil

import (
	"fmt"
	"io"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
)

// PollOutage is how long a polling loop keeps retrying transient errors before it gives up: long
// enough to ride out a dropped Wi-Fi, a VPN reconnecting or a laptop waking up.
const PollOutage = 5 * time.Minute

// pollMaxWait caps the backoff between failed polls, unless the polling interval is longer.
const pollMaxWait = 30 * time.Second

// PollRetry keeps a polling loop (`pipeline watch`, `pr checks --watch`) going through transient
// errors (429, 5xx, network). It doubles the wait after each failure in a row, announces every
// retry on Notices so a wait is never silent, and gives up once polls have been failing for
// PollOutage.
type PollRetry struct {
	Interval time.Duration
	Notices  io.Writer
	// Now reads the clock (default time.Now).
	Now func() time.Time

	failures int
	since    time.Time
}

// Failed records a failed poll and returns how long to wait before the next one. It returns err
// instead when err is not transient or polls have been failing for PollOutage.
func (r *PollRetry) Failed(err error) (time.Duration, error) {
	if !bitbucket.IsTransient(err) {
		return 0, err
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	if r.failures == 0 {
		r.since = now()
	}
	r.failures++
	if now().Sub(r.since) >= PollOutage {
		return 0, err
	}
	limit := max(r.Interval, pollMaxWait)
	wait := r.Interval
	for i := 1; i < r.failures && wait < limit; i++ {
		wait *= 2
	}
	wait = min(wait, limit)
	if r.Notices != nil {
		fmt.Fprintf(r.Notices, "%v; retrying in %s\n", err, wait)
	}
	return wait, nil
}

// Succeeded ends a run of failed polls.
func (r *PollRetry) Succeeded() {
	r.failures = 0
}
