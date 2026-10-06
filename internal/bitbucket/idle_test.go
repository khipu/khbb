package bitbucket_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
)

// stallingServer answers with headers and a first chunk of body, then sends nothing more until
// the client gives up or the test ends.
func stallingServer(t *testing.T) *httptest.Server {
	t.Helper()
	stop := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("first line\n"))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(stop) })
	return srv
}

func TestGetText_FailsWhenTheBodyStalls(t *testing.T) {
	srv := stallingServer(t)
	c := bitbucket.New(bitbucket.Options{BaseURL: srv.URL, IdleTimeout: 100 * time.Millisecond})

	start := time.Now()
	_, err := c.GetText(context.Background(), "repositories/acme/widgets/pipelines/7/steps/x/log")
	var netErr *bitbucket.NetworkError
	if !errors.As(err, &netErr) || !strings.Contains(err.Error(), "no data from Bitbucket for 100ms") {
		t.Fatalf("err = %v, want a network error about the stalled body", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("gave up after %s", elapsed)
	}
}

func TestGetText_KeepsReadingASlowSteadyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// 10 chunks 60 ms apart: the whole body takes longer than the idle timeout, no gap comes close.
		for i := range 10 {
			_, _ = w.Write([]byte{byte('a' + i)})
			w.(http.Flusher).Flush()
			time.Sleep(60 * time.Millisecond)
		}
	}))
	t.Cleanup(srv.Close)
	c := bitbucket.New(bitbucket.Options{BaseURL: srv.URL, IdleTimeout: 500 * time.Millisecond})

	got, err := c.GetText(context.Background(), "log")
	if err != nil || got != "abcdefghij" {
		t.Fatalf("GetText = %q, %v; a body that keeps arriving must never be cut", got, err)
	}
}
