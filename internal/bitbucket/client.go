// Package bitbucket is a minimal client for the Bitbucket Cloud REST API 2.0.
package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the Bitbucket Cloud REST API root.
const DefaultBaseURL = "https://api.bitbucket.org/2.0/"

// Options configures a Client. Zero values select defaults.
type Options struct {
	BaseURL    string
	Email      string
	Token      string
	UserAgent  string
	HTTPClient *http.Client
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
		opts.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "khbb"
	}
	return &Client{base: base, opts: opts}
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
// The caller must close the response body.
func (c *Client) Request(ctx context.Context, method, path string, header http.Header, body []byte) (*http.Response, error) {
	u, err := c.URL(path)
	if err != nil {
		return nil, err
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
	req, err := c.newRequest(ctx, method, u, header, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.opts.HTTPClient.Do(req)
	if err != nil {
		return nil, &NetworkError{Err: err}
	}
	if resp.Request == nil {
		resp.Request = req
	}
	return resp, nil
}
