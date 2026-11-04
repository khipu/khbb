package cmdutil_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
)

var errDNS = &bitbucket.NetworkError{Err: errors.New("dial tcp: lookup api.bitbucket.org: no such host")}

// clock is a fake clock that the test advances by each wait PollRetry returns.
type clock struct{ t time.Time }

func (c *clock) Now() time.Time { return c.t }

func newRetry(interval time.Duration) (*cmdutil.PollRetry, *clock, *bytes.Buffer) {
	c := &clock{t: time.Date(2026, 10, 7, 19, 45, 0, 0, time.UTC)}
	notices := &bytes.Buffer{}
	return &cmdutil.PollRetry{Interval: interval, Notices: notices, Now: c.Now}, c, notices
}

// failFor feeds err to r until it gives up or n failures pass, advancing c by each wait.
func failFor(r *cmdutil.PollRetry, c *clock, err error, n int) ([]time.Duration, error) {
	var waits []time.Duration
	for range n {
		wait, gaveUp := r.Failed(err)
		if gaveUp != nil {
			return waits, gaveUp
		}
		waits = append(waits, wait)
		c.t = c.t.Add(wait)
	}
	return waits, nil
}

func TestPollRetry_StopsOnANonTransientError(t *testing.T) {
	r, _, notices := newRetry(5 * time.Second)
	notFound := &bitbucket.HTTPError{StatusCode: 404}
	if _, err := r.Failed(notFound); err != notFound || notices.Len() != 0 {
		t.Errorf("err %v notices %q", err, notices.String())
	}
}

func TestPollRetry_BacksOffAndAnnouncesEachRetry(t *testing.T) {
	r, c, notices := newRetry(5 * time.Second)
	waits, err := failFor(r, c, errDNS, 5)
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second, 30 * time.Second}
	if err != nil || !slices.Equal(waits, want) {
		t.Fatalf("waits %v err %v", waits, err)
	}
	lines := strings.Split(strings.TrimSuffix(notices.String(), "\n"), "\n")
	if len(lines) != 5 || lines[0] != "network error: dial tcp: lookup api.bitbucket.org: no such host; retrying in 5s" ||
		lines[4] != "network error: dial tcp: lookup api.bitbucket.org: no such host; retrying in 30s" {
		t.Errorf("notices = %q", notices.String())
	}
}

func TestPollRetry_KeepsALongIntervalAsIs(t *testing.T) {
	r, c, _ := newRetry(time.Minute)
	if waits, err := failFor(r, c, errDNS, 2); err != nil || !slices.Equal(waits, []time.Duration{time.Minute, time.Minute}) {
		t.Errorf("waits %v err %v", waits, err)
	}
}

func TestPollRetry_GivesUpAfterFiveMinutesOfFailures(t *testing.T) {
	r, c, _ := newRetry(5 * time.Second)
	start := c.t
	waits, err := failFor(r, c, errDNS, 100)
	if err != errDNS {
		t.Fatalf("err = %v, want the last error", err)
	}
	failing := c.t.Sub(start)
	if failing < cmdutil.PollOutage || failing-waits[len(waits)-1] >= cmdutil.PollOutage {
		t.Errorf("gave up after %s of failures (last wait %s), want the first poll past %s", failing, waits[len(waits)-1], cmdutil.PollOutage)
	}
}

func TestPollRetry_ASuccessfulPollStartsOver(t *testing.T) {
	r, c, _ := newRetry(5 * time.Second)
	if _, err := failFor(r, c, errDNS, 10); err != nil { // 4m05s of failures
		t.Fatal(err)
	}
	r.Succeeded()
	waits, err := failFor(r, c, &bitbucket.HTTPError{StatusCode: 503}, 10)
	if err != nil || waits[0] != 5*time.Second {
		t.Errorf("waits %v err %v", waits, err)
	}
}
