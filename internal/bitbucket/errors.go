package bitbucket

import (
	"encoding/json"
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
			Message string          `json:"message"`
			Detail  json.RawMessage `json:"detail"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil {
		e.Message = payload.Error.Message
		e.Detail, e.RequiredScopes = parseDetail(payload.Error.Detail)
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
