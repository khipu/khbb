package bitbucket_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

func TestRequest_RetriesGETOn429HonoringRetryAfter(t *testing.T) {
	var slept []time.Duration
	c, reg := newTestClient(t, bitbucket.Options{Sleep: func(d time.Duration) { slept = append(slept, d) }})
	reg.Register("GET", "/2.0/user", httpmock.WithHeader(httpmock.JSONResponse(429, `{}`), "Retry-After", "2"))
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	u, err := c.CurrentUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Nickname != "ada" {
		t.Errorf("Nickname = %q", u.Nickname)
	}
	if !slices.Equal(slept, []time.Duration{2 * time.Second}) {
		t.Errorf("slept %v, want [2s]", slept)
	}
}

func TestRequest_GivesUpAfterMaxAttempts(t *testing.T) {
	var slept []time.Duration
	c, reg := newTestClient(t, bitbucket.Options{Sleep: func(d time.Duration) { slept = append(slept, d) }})
	for range 3 {
		reg.Register("GET", "/2.0/user", httpmock.StringResponse(503, "unavailable"))
	}

	err := c.Do(context.Background(), "GET", "user", nil, nil)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 503 {
		t.Fatalf("expected HTTP 503 error, got %v", err)
	}
	if !slices.Equal(slept, []time.Duration{time.Second, 2 * time.Second}) {
		t.Errorf("slept %v, want [1s 2s]", slept)
	}
}

func TestRequest_DoesNotRetryMutating(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{Sleep: func(time.Duration) { t.Error("mutating requests must not be retried") }})
	reg.Register("POST", "/2.0/repositories/acme/widgets/pullrequests", httpmock.StringResponse(503, "unavailable"))

	resp, err := c.Request(context.Background(), "POST", "repositories/acme/widgets/pullrequests", nil, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 503 || len(reg.Calls) != 1 {
		t.Errorf("status %d after %d calls", resp.StatusCode, len(reg.Calls))
	}
}

func TestRequest_DryRunSkipsMutatingRequests(t *testing.T) {
	var out bytes.Buffer
	c, _ := newTestClient(t, bitbucket.Options{Email: "dev@example.com", Token: "s3cret", DryRun: true, DryRunOut: &out})

	_, err := c.Request(context.Background(), "POST", "repositories/acme/widgets/pullrequests", nil, []byte(`{"title":"Add widgets"}`))
	if !errors.Is(err, bitbucket.ErrDryRun) {
		t.Fatalf("expected ErrDryRun, got %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("dry-run output is not JSON: %v\n%s", err, out.String())
	}
	if got["dryRun"] != true || got["method"] != "POST" || got["url"] != "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests" {
		t.Errorf("unexpected dry-run output: %v", got)
	}
	if body, _ := got["body"].(map[string]any); body["title"] != "Add widgets" {
		t.Errorf("body = %v", got["body"])
	}
	if strings.Contains(out.String(), "s3cret") || strings.Contains(out.String(), "Authorization") {
		t.Error("dry-run output leaks credentials")
	}
}

func TestRequest_DryRunStillSendsGET(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{DryRun: true})
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))
	if _, err := c.CurrentUser(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDryRun_MasksSecuredValues(t *testing.T) {
	var out bytes.Buffer
	c, _ := newTestClient(t, bitbucket.Options{DryRun: true, DryRunOut: &out})
	body := `{"variables":[{"key":"PLAIN","value":"visible"},{"key":"SECRET","value":"hunter2","secured":true}]}`

	_, err := c.Request(context.Background(), "POST", "repositories/acme/widgets/pipelines", nil, []byte(body))
	if !errors.Is(err, bitbucket.ErrDryRun) {
		t.Fatalf("expected ErrDryRun, got %v", err)
	}
	s := out.String()
	if strings.Contains(s, "hunter2") || !strings.Contains(s, `"****"`) || !strings.Contains(s, "visible") {
		t.Errorf("secured value not masked correctly:\n%s", s)
	}
}

func TestDebug_RedactsAuthorization(t *testing.T) {
	var debug bytes.Buffer
	c, reg := newTestClient(t, bitbucket.Options{Email: "dev@example.com", Token: "s3cret", Debug: &debug})
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if _, err := c.CurrentUser(context.Background()); err != nil {
		t.Fatal(err)
	}
	log := debug.String()
	for _, want := range []string{"> GET https://api.bitbucket.org/2.0/user", "> Authorization: [REDACTED]", "< 200 OK"} {
		if !strings.Contains(log, want) {
			t.Errorf("debug log missing %q:\n%s", want, log)
		}
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("dev@example.com:s3cret"))
	if strings.Contains(log, "s3cret") || strings.Contains(log, encoded) {
		t.Errorf("debug log leaks credentials:\n%s", log)
	}
}

func TestList_FollowsNextUntilLimit(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/items", httpmock.JSONResponse(200, `{"values":[1,2],"next":"https://api.bitbucket.org/2.0/items?page=2&pagelen=3"}`))
	reg.Register("GET", "/2.0/items", httpmock.JSONResponse(200, `{"values":[3,4]}`))

	got, err := bitbucket.List[int](context.Background(), c, "items", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []int{1, 2, 3}) {
		t.Errorf("got %v, want [1 2 3]", got)
	}
	if q := reg.Calls[0].URL.Query().Get("pagelen"); q != "3" {
		t.Errorf("pagelen = %q, want 3", q)
	}
}

func TestList_AllPagesWhenNoLimit(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/items", httpmock.JSONResponse(200, `{"values":[1,2],"next":"https://api.bitbucket.org/2.0/items?page=2"}`))
	reg.Register("GET", "/2.0/items", httpmock.JSONResponse(200, `{"values":[3]}`))

	got, err := bitbucket.List[int](context.Background(), c, "items?sort=-created_on", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []int{1, 2, 3}) {
		t.Errorf("got %v", got)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("pagelen") != "50" || q.Get("sort") != "-created_on" {
		t.Errorf("first query = %v", q)
	}
}

func TestList_RefusesForeignNextHost(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/items", httpmock.JSONResponse(200, `{"values":[1],"next":"https://evil.example.com/2.0/items?page=2"}`))

	if _, err := bitbucket.List[int](context.Background(), c, "items", 0); err == nil {
		t.Fatal("expected an error for a next link on another host")
	}
	if len(reg.Calls) != 1 {
		t.Errorf("made %d calls, want 1", len(reg.Calls))
	}
}
