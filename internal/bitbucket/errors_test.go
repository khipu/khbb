package bitbucket_test

import (
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
