package bitbucket

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// idleTimeoutBody fails a response body that sends nothing for timeout: it cancels the request's
// context, so the pending Read returns, and reports the stall as a NetworkError. Every byte that
// arrives restarts the clock, so a long but steady download (a big pipeline log) is never cut.
type idleTimeoutBody struct {
	io.ReadCloser
	timeout time.Duration
	timer   *time.Timer
	cancel  context.CancelFunc
	stalled atomic.Bool
}

func newIdleTimeoutBody(rc io.ReadCloser, timeout time.Duration, cancel context.CancelFunc) *idleTimeoutBody {
	b := &idleTimeoutBody{ReadCloser: rc, timeout: timeout, cancel: cancel}
	b.timer = time.AfterFunc(timeout, func() {
		b.stalled.Store(true)
		cancel()
	})
	return b
}

func (b *idleTimeoutBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.timer.Reset(b.timeout)
	}
	if err != nil && err != io.EOF && b.stalled.Load() {
		err = &NetworkError{Err: fmt.Errorf("no data from Bitbucket for %s", b.timeout)}
	}
	return n, err
}

func (b *idleTimeoutBody) Close() error {
	b.timer.Stop()
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}
