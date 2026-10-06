// Package bitbucket is a minimal client for the Bitbucket Cloud REST API 2.0.
package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the Bitbucket Cloud REST API root.
const DefaultBaseURL = "https://api.bitbucket.org/2.0/"

// ErrDryRun is returned instead of sending a mutating request when Options.DryRun is set.
var ErrDryRun = errors.New("dry run: request not sent")

// Options configures a Client. Zero values select defaults.
type Options struct {
	BaseURL    string
	Email      string
	Token      string
	UserAgent  string
	HTTPClient *http.Client

	// DryRun prints mutating requests to DryRunOut instead of sending them.
	DryRun    bool
	DryRunOut io.Writer
	// Debug receives a log of every request and response, with credentials redacted.
	Debug io.Writer
	// MaxAttempts bounds retries of idempotent requests (default 3).
	MaxAttempts int
	// Sleep waits between retries (default time.Sleep).
	Sleep func(time.Duration)
	// Notices receives a line before each retry, so a wait is never silent (default: discarded).
	Notices io.Writer
	// IdleTimeout fails a response whose body sends nothing for this long (default 60s). Bodies
	// that keep arriving, such as long logs, are never cut.
	IdleTimeout time.Duration
}

// Client talks to the Bitbucket Cloud REST API 2.0.
type Client struct {
	base *url.URL
	opts Options
}

// New returns a Client. It panics only if BaseURL is not a valid URL.
func New(opts Options) *Client {
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if !strings.HasSuffix(opts.BaseURL, "/") {
		opts.BaseURL += "/"
	}
	base, err := url.Parse(opts.BaseURL)
	if err != nil {
		panic(fmt.Sprintf("bitbucket: invalid base URL %q: %v", opts.BaseURL, err))
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = NewHTTPClient()
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "khbb"
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 3
	}
	if opts.Sleep == nil {
		opts.Sleep = time.Sleep
	}
	if opts.DryRunOut == nil {
		opts.DryRunOut = io.Discard
	}
	if opts.Notices == nil {
		opts.Notices = io.Discard
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = time.Minute
	}
	return &Client{base: base, opts: opts}
}

// NewHTTPClient returns khbb's default HTTP client. It bounds connecting, the TLS handshake and the
// wait for response headers — not the whole request, so large step logs can stream — and it
// follows at most 10 redirects, never from https to http.
func NewHTTPClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Transport: tr, CheckRedirect: checkRedirect}
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refusing to follow a redirect from https to %s", req.URL.Scheme)
	}
	return nil
}

// URL resolves path against the API root. Absolute URLs must point at the API host,
// so credentials are never sent elsewhere (for example through a crafted `next` link).
func (c *Client) URL(path string) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		u, err := url.Parse(path)
		if err != nil {
			return "", fmt.Errorf("invalid URL %q: %w", path, err)
		}
		if u.Scheme != c.base.Scheme || !strings.EqualFold(u.Host, c.base.Host) {
			return "", fmt.Errorf("refusing to send credentials to %s://%s: only %s is allowed", u.Scheme, u.Host, c.base.Host)
		}
		return u.String(), nil
	}
	rel := strings.TrimPrefix(path, "/")
	rel = strings.TrimPrefix(rel, strings.TrimPrefix(c.base.Path, "/"))
	full := c.base.String() + rel
	if _, err := url.Parse(full); err != nil {
		return "", fmt.Errorf("invalid path %q: %w", path, err)
	}
	return full, nil
}

// Request sends a request and returns the raw response for any HTTP status.
// The caller must close the response body. With DryRun set, mutating requests
// are printed and ErrDryRun is returned instead.
func (c *Client) Request(ctx context.Context, method, path string, header http.Header, body []byte) (*http.Response, error) {
	u, err := c.URL(path)
	if err != nil {
		return nil, err
	}
	if c.opts.DryRun && isMutating(method) {
		if err := writeDryRun(c.opts.DryRunOut, method, u, body); err != nil {
			return nil, err
		}
		return nil, ErrDryRun
	}
	return c.send(ctx, method, u, header, body)
}

// Do sends in (JSON-encoded, if non-nil) and decodes a 2xx JSON response into out (if non-nil).
// Non-2xx responses become *HTTPError.
func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		body = b
	}
	resp, err := c.Request(ctx, method, path, nil, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return readHTTPError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response of %s %s: %w", method, resp.Request.URL, err)
	}
	return nil
}

func (c *Client) newRequest(ctx context.Context, method, u string, header http.Header, body []byte) (*http.Request, error) {
	var rdr io.Reader = http.NoBody
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	if c.opts.Token != "" {
		req.SetBasicAuth(c.opts.Email, c.opts.Token)
	}
	return req, nil
}

func (c *Client) send(ctx context.Context, method, u string, header http.Header, body []byte) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		// Each attempt gets its own context so that a stalled body can be abandoned (idleTimeoutBody).
		attemptCtx, cancel := context.WithCancel(ctx)
		req, err := c.newRequest(attemptCtx, method, u, header, body)
		if err != nil {
			cancel()
			return nil, err
		}
		c.debugRequest(req)
		start := time.Now()
		resp, err := c.opts.HTTPClient.Do(req)
		if err != nil {
			cancel()
			return nil, &NetworkError{Err: err}
		}
		if resp.Request == nil {
			resp.Request = req
		}
		resp.Body = newIdleTimeoutBody(resp.Body, c.opts.IdleTimeout, cancel)
		c.debugResponse(resp, time.Since(start))
		if attempt >= c.opts.MaxAttempts || !isRetryable(method, resp.StatusCode) {
			return resp, nil
		}
		wait := retryDelay(resp, attempt)
		fmt.Fprint(c.opts.Notices, retryNotice(resp.StatusCode, wait, attempt+1, c.opts.MaxAttempts))
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		c.opts.Sleep(wait)
	}
}

// retryNotice tells the user why khbb is waiting before attempt next of max.
func retryNotice(status int, wait time.Duration, next, max int) string {
	what := fmt.Sprintf("Bitbucket answered %d %s", status, http.StatusText(status))
	if status == http.StatusTooManyRequests {
		what = "Bitbucket is rate limiting requests (429 Too Many Requests)"
	}
	return fmt.Sprintf("%s; retrying in %s (attempt %d of %d)\n", what, wait, next, max)
}

func isMutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// isRetryable reports whether a response is a transient failure of an idempotent request.
func isRetryable(method string, status int) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryDelay honors Retry-After (in seconds, capped at one minute), else backs off 1s, 2s, 4s…
func retryDelay(resp *http.Response, attempt int) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return min(time.Duration(n)*time.Second, time.Minute)
		}
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}
