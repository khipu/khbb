package bitbucket_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

const (
	taskURL  = "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests/42/merge/task-status/t1"
	taskPath = "/2.0/repositories/acme/widgets/pullrequests/42/merge/task-status/t1"
)

func TestStartMerge_Accepted(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", prPath+"/42/merge", httpmock.WithHeader(httpmock.JSONResponse(202, `""`), "Location", taskURL))

	pr, loc, err := c.StartMerge(context.Background(), "acme", "widgets", 42, bitbucket.PRMerge{Strategy: "squash", Message: "Add widgets"})
	if err != nil || pr != nil || loc != taskURL {
		t.Fatalf("pr %v loc %q err %v", pr, loc, err)
	}
	if q := reg.Calls[0].URL.Query().Get("async"); q != "true" {
		t.Errorf("async = %q", q)
	}
	assertBody(t, reg.Calls[0], `{"type":"pullrequest","merge_strategy":"squash","message":"Add widgets","close_source_branch":false}`)
}

func TestStartMerge_FinishedAtOnce(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", prPath+"/42/merge", httpmock.JSONResponse(200, prJSON))

	pr, loc, err := c.StartMerge(context.Background(), "acme", "widgets", 42, bitbucket.PRMerge{CloseSourceBranch: true})
	if err != nil || pr == nil || pr.ID != 42 || loc != "" {
		t.Fatalf("pr %v loc %q err %v", pr, loc, err)
	}
	assertBody(t, reg.Calls[0], `{"type":"pullrequest","close_source_branch":true}`)
}

func TestStartMerge_Errors(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", prPath+"/42/merge", httpmock.JSONResponse(400,
		`{"type":"error","error":{"message":"You can't merge until you resolve all merge conflicts."}}`))
	reg.Register("POST", prPath+"/42/merge", httpmock.JSONResponse(202, `""`))

	_, _, err := c.StartMerge(context.Background(), "acme", "widgets", 42, bitbucket.PRMerge{})
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || !strings.Contains(httpErr.Message, "merge conflicts") {
		t.Errorf("err = %v", err)
	}
	if _, _, err := c.StartMerge(context.Background(), "acme", "widgets", 42, bitbucket.PRMerge{}); err == nil || !strings.Contains(err.Error(), "task location") {
		t.Errorf("202 without Location: err = %v", err)
	}
}

func TestMergeTaskStatus(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", taskPath, httpmock.JSONResponse(200, `{"task_status":"PENDING","links":{}}`))
	reg.Register("GET", taskPath, httpmock.JSONResponse(200, `{"task_status":"SUCCESS","merge_result":`+prJSON+`}`))
	reg.Register("GET", taskPath, httpmock.JSONResponse(400,
		`{"type":"error","error":{"message":"You can't merge until you resolve all merge conflicts."}}`))

	if pr, done, err := c.MergeTaskStatus(context.Background(), taskURL); pr != nil || done || err != nil {
		t.Errorf("pending: pr %v done %v err %v", pr, done, err)
	}
	if pr, done, err := c.MergeTaskStatus(context.Background(), taskURL); pr == nil || pr.ID != 42 || !done || err != nil {
		t.Errorf("success: pr %v done %v err %v", pr, done, err)
	}
	var httpErr *bitbucket.HTTPError
	if _, _, err := c.MergeTaskStatus(context.Background(), taskURL); !errors.As(err, &httpErr) || httpErr.StatusCode != 400 {
		t.Errorf("failed task: err = %v", err)
	}
}

func TestPRMergeStrategies(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath+"/42", httpmock.JSONResponse(200,
		`{"destination":{"branch":{"name":"main","merge_strategies":["merge_commit","squash"],"default_merge_strategy":"squash"}}}`))

	got, err := c.PRMergeStrategies(context.Background(), "acme", "widgets", 42)
	if err != nil || got.Default != "squash" || !slices.Equal(got.Allowed, []string{"merge_commit", "squash"}) {
		t.Errorf("got %+v err %v", got, err)
	}
	if f := reg.Calls[0].URL.Query().Get("fields"); f != "destination.branch.*" {
		t.Errorf("fields = %q", f)
	}
}

func TestPRMergeabilityChecks(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath+"/42/mergeability/checks", httpmock.JSONResponse(200, `{"size":2,"values":[`+
		`{"type":"pullrequest_state_check","status":"PASSED","required":true,"blocking":false,"reason":null,"state":"OPEN"},`+
		`{"type":"git_mergeability_check","status":"FAILED","required":true,"blocking":true,"reason":"conflicts"}]}`))

	checks, err := c.PRMergeabilityChecks(context.Background(), "acme", "widgets", 42)
	if err != nil || len(checks) != 2 {
		t.Fatalf("checks %+v err %v", checks, err)
	}
	want := bitbucket.MergeCheck{Type: "git_mergeability_check", Status: "FAILED", Reason: "conflicts", Required: true, Blocking: true}
	if checks[1] != want || checks[0].Reason != "" {
		t.Errorf("checks = %+v", checks)
	}
}
