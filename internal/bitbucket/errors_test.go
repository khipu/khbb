package bitbucket_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
)

func TestParseHTTPError_Fields(t *testing.T) {
	resp := &http.Response{StatusCode: 400, Request: httptest.NewRequest("POST", "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests", nil)}
	body := `{"type":"error","error":{"message":"Bad request","fields":{"source":["source branch not found"],"title":"too long"}}}`
	e := bitbucket.ParseHTTPError(resp, []byte(body))
	if e.Message != "Bad request" || e.Method != "POST" {
		t.Errorf("unexpected error: %+v", e)
	}
	if !slices.Equal(e.Fields["source"], []string{"source branch not found"}) || !slices.Equal(e.Fields["title"], []string{"too long"}) {
		t.Errorf("Fields = %v", e.Fields)
	}
}

func TestIsTransient(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{&bitbucket.HTTPError{StatusCode: 429}, true},
		{&bitbucket.HTTPError{StatusCode: 503}, true},
		{&bitbucket.HTTPError{StatusCode: 555}, true},
		{fmt.Errorf("polling: %w", &bitbucket.HTTPError{StatusCode: 502}), true},
		{&bitbucket.NetworkError{Err: errors.New("connection reset")}, true},
		{&bitbucket.HTTPError{StatusCode: 404}, false},
		{&bitbucket.HTTPError{StatusCode: 400}, false},
		{errors.New("boom"), false},
	}
	for _, tc := range cases {
		if got := bitbucket.IsTransient(tc.err); got != tc.want {
			t.Errorf("IsTransient(%v) = %v", tc.err, got)
		}
	}
}
