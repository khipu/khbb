package bitbucket

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// HTTPError is a non-2xx response from the Bitbucket API.
type HTTPError struct {
	StatusCode     int
	Method         string
	URL            string
	Message        string
	Detail         string
	RequiredScopes []string
	// Fields holds per-field validation messages from Bitbucket's error.fields.
	Fields map[string][]string
}

func (e *HTTPError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	if msg == "" {
		msg = "request failed"
	}
	return fmt.Sprintf("%s (HTTP %d)", msg, e.StatusCode)
}

// NetworkError wraps a transport failure (DNS, TLS, timeout, connection reset).
type NetworkError struct{ Err error }

func (e *NetworkError) Error() string { return "network error: " + e.Err.Error() }
func (e *NetworkError) Unwrap() error { return e.Err }

// ParseHTTPError builds an HTTPError from a response and its already-read body.
func ParseHTTPError(resp *http.Response, body []byte) *HTTPError {
	e := &HTTPError{StatusCode: resp.StatusCode}
	if resp.Request != nil {
		e.Method = resp.Request.Method
		e.URL = resp.Request.URL.String()
	}
	var payload struct {
		Error struct {
			Message string                     `json:"message"`
			Detail  json.RawMessage            `json:"detail"`
			Fields  map[string]json.RawMessage `json:"fields"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil {
		e.Message = payload.Error.Message
		e.Detail, e.RequiredScopes = parseDetail(payload.Error.Detail)
		e.Fields = parseFields(payload.Error.Fields)
	}
	return e
}

func readHTTPError(resp *http.Response) *HTTPError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return ParseHTTPError(resp, body)
}

// parseDetail accepts Bitbucket's `detail`, which is either a string or an object
// such as {"required": [...], "granted": [...]}.
func parseDetail(raw json.RawMessage) (string, []string) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var obj struct {
		Required []string `json:"required"`
	}
	_ = json.Unmarshal(raw, &obj)
	return string(raw), obj.Required
}

// parseFields accepts each field's messages as a list of strings or a single string.
func parseFields(raw map[string]json.RawMessage) map[string][]string {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string][]string, len(raw))
	for k, v := range raw {
		var list []string
		if json.Unmarshal(v, &list) == nil {
			out[k] = list
			continue
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			out[k] = []string{s}
			continue
		}
		out[k] = []string{string(v)}
	}
	return out
}

// IsTransient reports whether err is worth retrying later: rate limiting, a server error or a
// network failure.
func IsTransient(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusTooManyRequests || httpErr.StatusCode >= 500
	}
	var netErr *NetworkError
	return errors.As(err, &netErr)
}
