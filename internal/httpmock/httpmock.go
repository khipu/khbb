// Package httpmock is an http.RoundTripper that serves registered stubs in tests.
package httpmock

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// Responder builds the response for a matched request.
type Responder func(*http.Request) (*http.Response, error)

// Call records a request the registry received.
type Call struct {
	Method string
	URL    *url.URL
	Header http.Header
	Body   []byte
}

// Registry matches requests against stubs by method and exact URL path.
// Each stub answers once, in registration order. When the test ends, unused stubs fail it.
type Registry struct {
	t     testing.TB
	mu    sync.Mutex
	stubs []*stub
	Calls []Call
}

type stub struct {
	method  string
	path    string
	respond Responder
	used    bool
}

// New returns a Registry bound to t.
func New(t testing.TB) *Registry {
	r := &Registry{t: t}
	t.Cleanup(r.verify)
	return r
}

// Register adds a stub for method and path (e.g. "GET", "/2.0/user").
func (r *Registry) Register(method, path string, respond Responder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stubs = append(r.stubs, &stub{method: method, path: path, respond: respond})
}

// Client returns an *http.Client that sends every request to the registry.
func (r *Registry) Client() *http.Client {
	return &http.Client{Transport: r}
}

// RoundTrip implements http.RoundTripper.
func (r *Registry) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body.Close()
	}
	r.mu.Lock()
	r.Calls = append(r.Calls, Call{Method: req.Method, URL: req.URL, Header: req.Header.Clone(), Body: body})
	var match *stub
	for _, s := range r.stubs {
		if !s.used && s.method == req.Method && s.path == req.URL.Path {
			s.used = true
			match = s
			break
		}
	}
	r.mu.Unlock()

	if match == nil {
		r.t.Errorf("httpmock: no stub for %s %s", req.Method, req.URL)
		return nil, fmt.Errorf("httpmock: no stub for %s %s", req.Method, req.URL)
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	resp, err := match.respond(req)
	if resp != nil && resp.Request == nil {
		resp.Request = req
	}
	return resp, err
}

func (r *Registry) verify() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.stubs {
		if !s.used {
			r.t.Errorf("httpmock: stub never called: %s %s", s.method, s.path)
		}
	}
}

// StringResponse responds with status and a plain body.
func StringResponse(status int, body string) Responder {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	}
}

// JSONResponse responds with status and a JSON body.
func JSONResponse(status int, body string) Responder {
	return WithHeader(StringResponse(status, body), "Content-Type", "application/json")
}

// WithHeader adds a response header to r.
func WithHeader(r Responder, key, value string) Responder {
	return func(req *http.Request) (*http.Response, error) {
		resp, err := r(req)
		if resp != nil {
			resp.Header.Set(key, value)
		}
		return resp, err
	}
}
