package bitbucket_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newTestClient(t *testing.T, opts bitbucket.Options) (*bitbucket.Client, *httpmock.Registry) {
	t.Helper()
	reg := httpmock.New(t)
	opts.HTTPClient = reg.Client()
	if opts.Sleep == nil {
		opts.Sleep = func(time.Duration) {}
	}
	return bitbucket.New(opts), reg
}

const userJSON = `{"display_name":"Ada Example","nickname":"ada","uuid":"{00000000-0000-0000-0000-000000000001}","account_id":"000000:aaaa"}`

func TestURL(t *testing.T) {
	c := bitbucket.New(bitbucket.Options{})
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "user", want: "https://api.bitbucket.org/2.0/user"},
		{in: "/user", want: "https://api.bitbucket.org/2.0/user"},
		{in: "/2.0/user", want: "https://api.bitbucket.org/2.0/user"},
		{in: "repositories/acme/widgets/pullrequests?state=OPEN", want: "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests?state=OPEN"},
		{in: "https://api.bitbucket.org/2.0/user?page=2", want: "https://api.bitbucket.org/2.0/user?page=2"},
		{in: "https://evil.example.com/2.0/user", wantErr: true},
		{in: "http://api.bitbucket.org/2.0/user", wantErr: true},
	}
	for _, tc := range cases {
		got, err := c.URL(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("URL(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("URL(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("URL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCurrentUser_SendsBasicAuthAndDecodes(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{Email: "dev@example.com", Token: "s3cret", UserAgent: "khbb/test"})
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	u, err := c.CurrentUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Nickname != "ada" || u.DisplayName != "Ada Example" || u.AccountID != "000000:aaaa" {
		t.Errorf("unexpected user: %+v", u)
	}
	call := reg.Calls[0]
	user, pass, ok := (&http.Request{Header: call.Header}).BasicAuth()
	if !ok || user != "dev@example.com" || pass != "s3cret" {
		t.Errorf("basic auth = %q / %q / %v", user, pass, ok)
	}
	if got := call.Header.Get("User-Agent"); got != "khbb/test" {
		t.Errorf("User-Agent = %q", got)
	}
	if got := call.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q", got)
	}
}

func TestDo_ReturnsHTTPErrorWithBitbucketMessage(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/repositories/acme/nope", httpmock.JSONResponse(404,
		`{"type":"error","error":{"message":"Repository acme/nope not found"}}`))

	err := c.Do(context.Background(), "GET", "repositories/acme/nope", nil, nil)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != 404 || httpErr.Message != "Repository acme/nope not found" {
		t.Errorf("unexpected error fields: %+v", httpErr)
	}
	if httpErr.Method != "GET" || httpErr.URL != "https://api.bitbucket.org/2.0/repositories/acme/nope" {
		t.Errorf("unexpected request fields: %+v", httpErr)
	}
	if got, want := err.Error(), "Repository acme/nope not found (HTTP 404)"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestDo_HTTPErrorWithRequiredScopes(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/repositories/acme/widgets/pipelines", httpmock.JSONResponse(403,
		`{"type":"error","error":{"message":"Your credentials lack one or more required privilege scopes.","detail":{"required":["read:pipeline:bitbucket"],"granted":["read:user:bitbucket"]}}}`))

	err := c.Do(context.Background(), "GET", "repositories/acme/widgets/pipelines", nil, nil)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if !slices.Equal(httpErr.RequiredScopes, []string{"read:pipeline:bitbucket"}) {
		t.Errorf("RequiredScopes = %v", httpErr.RequiredScopes)
	}
}

func TestDo_NonJSONErrorBody(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", "/2.0/repositories/acme/widgets/pullrequests", httpmock.StringResponse(502, "<html><body>Bad Gateway</body></html>"))

	err := c.Do(context.Background(), "POST", "repositories/acme/widgets/pullrequests", map[string]string{"title": "x"}, nil)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if got, want := err.Error(), "Bad Gateway (HTTP 502)"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestDo_EncodesBodyAndHandlesNoContent(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", "/2.0/repositories/acme/widgets/pullrequests/1/approve", httpmock.StringResponse(204, ""))

	out := map[string]any{"untouched": true}
	err := c.Do(context.Background(), "POST", "repositories/acme/widgets/pullrequests/1/approve", map[string]string{"a": "b"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if out["untouched"] != true {
		t.Errorf("out modified on 204: %v", out)
	}
	call := reg.Calls[0]
	var sent map[string]string
	if err := json.Unmarshal(call.Body, &sent); err != nil || sent["a"] != "b" {
		t.Errorf("body = %s (%v)", call.Body, err)
	}
	if got := call.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestRequest_WrapsNetworkErrors(t *testing.T) {
	boom := errors.New("connection reset")
	c := bitbucket.New(bitbucket.Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, boom
	})}})

	_, err := c.Request(context.Background(), "GET", "user", nil, nil)
	var netErr *bitbucket.NetworkError
	if !errors.As(err, &netErr) || !errors.Is(err, boom) {
		t.Fatalf("expected NetworkError wrapping %v, got %T: %v", boom, err, err)
	}
}
