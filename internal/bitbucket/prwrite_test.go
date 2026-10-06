package bitbucket_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

// assertBody fails t unless the request body is the JSON value want (key order and spacing ignored).
func assertBody(t *testing.T, c httpmock.Call, want string) {
	t.Helper()
	var got, exp any
	if err := json.Unmarshal(c.Body, &got); err != nil {
		t.Fatalf("request body %q: %v", c.Body, err)
	}
	if err := json.Unmarshal([]byte(want), &exp); err != nil {
		t.Fatalf("bad expectation %q: %v", want, err)
	}
	if !reflect.DeepEqual(got, exp) {
		t.Errorf("request body = %s, want %s", c.Body, want)
	}
}

func TestCreatePullRequest(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", prPath, httpmock.JSONResponse(201, prJSON))

	pr, err := c.CreatePullRequest(context.Background(), "acme", "widgets", bitbucket.PRCreate{
		Title: "Add widgets", Description: "Adds the widget factory.", Source: "feature/widgets", Destination: "main",
		Draft: true, CloseSourceBranch: true, Reviewers: []string{"{b}", "{c}"},
	})
	if err != nil || pr.ID != 42 {
		t.Fatalf("pr %+v err %v", pr, err)
	}
	assertBody(t, reg.Calls[0], `{"title":"Add widgets","description":"Adds the widget factory.",
		"source":{"branch":{"name":"feature/widgets"}},"destination":{"branch":{"name":"main"}},
		"draft":true,"close_source_branch":true,"reviewers":[{"uuid":"{b}"},{"uuid":"{c}"}]}`)
}

func TestCreatePullRequest_NoReviewersIsAnEmptyList(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", prPath, httpmock.JSONResponse(201, prJSON))

	if _, err := c.CreatePullRequest(context.Background(), "acme", "widgets", bitbucket.PRCreate{
		Title: "Fix", Source: "fix/gears", Destination: "main",
	}); err != nil {
		t.Fatal(err)
	}
	assertBody(t, reg.Calls[0], `{"title":"Fix","description":"","source":{"branch":{"name":"fix/gears"}},
		"destination":{"branch":{"name":"main"}},"draft":false,"close_source_branch":false,"reviewers":[]}`)
}

func TestUpdatePullRequest_SendsOnlyChangedFields(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("PUT", prPath+"/42", httpmock.JSONResponse(200, prJSON))

	title, draft := "New title", false
	if _, err := c.UpdatePullRequest(context.Background(), "acme", "widgets", 42, bitbucket.PRUpdate{Title: &title, Draft: &draft}); err != nil {
		t.Fatal(err)
	}
	assertBody(t, reg.Calls[0], `{"title":"New title","draft":false}`)
}

func TestUpdatePullRequest_DescriptionDestinationAndNoReviewers(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("PUT", prPath+"/42", httpmock.JSONResponse(200, prJSON))

	body, base := "", "develop"
	if _, err := c.UpdatePullRequest(context.Background(), "acme", "widgets", 42, bitbucket.PRUpdate{
		Description: &body, Destination: &base, Reviewers: []string{},
	}); err != nil {
		t.Fatal(err)
	}
	assertBody(t, reg.Calls[0], `{"description":"","destination":{"branch":{"name":"develop"}},"reviewers":[]}`)
}

func TestCreatePRComment(t *testing.T) {
	cases := []struct {
		name string
		in   bitbucket.PRCommentInput
		want string
	}{
		{"general", bitbucket.PRCommentInput{Body: "LGTM"}, `{"content":{"raw":"LGTM"}}`},
		{"file", bitbucket.PRCommentInput{Body: "x", Path: "src/widget.go"}, `{"content":{"raw":"x"},"inline":{"path":"src/widget.go"}}`},
		{"line", bitbucket.PRCommentInput{Body: "x", Path: "src/widget.go", Line: 12}, `{"content":{"raw":"x"},"inline":{"path":"src/widget.go","to":12}}`},
		{"reply", bitbucket.PRCommentInput{Body: "Done.", ParentID: 101}, `{"content":{"raw":"Done."},"parent":{"id":101}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, reg := newTestClient(t, bitbucket.Options{})
			reg.Register("POST", prPath+"/42/comments", httpmock.JSONResponse(201, `{"id":7,"content":{"raw":"x"}}`))
			cm, err := c.CreatePRComment(context.Background(), "acme", "widgets", 42, tc.in)
			if err != nil || cm.ID != 7 {
				t.Fatalf("comment %+v err %v", cm, err)
			}
			assertBody(t, reg.Calls[0], tc.want)
		})
	}
}

func TestReviewActions(t *testing.T) {
	cases := []struct {
		name   string
		call   func(*bitbucket.Client) error
		method string
		path   string
		status int
	}{
		{"approve", func(c *bitbucket.Client) error { return c.ApprovePR(context.Background(), "acme", "widgets", 42) }, "POST", prPath + "/42/approve", 200},
		{"unapprove", func(c *bitbucket.Client) error { return c.UnapprovePR(context.Background(), "acme", "widgets", 42) }, "DELETE", prPath + "/42/approve", 204},
		{"request changes", func(c *bitbucket.Client) error { return c.RequestChangesPR(context.Background(), "acme", "widgets", 42) }, "POST", prPath + "/42/request-changes", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, reg := newTestClient(t, bitbucket.Options{})
			reg.Register(tc.method, tc.path, httpmock.JSONResponse(tc.status, `{"approved":true}`))
			if err := tc.call(c); err != nil {
				t.Fatal(err)
			}
			if len(reg.Calls[0].Body) != 0 {
				t.Errorf("body = %q, want none", reg.Calls[0].Body)
			}
		})
	}
}

func TestDeclinePR(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", prPath+"/42/decline", httpmock.JSONResponse(200, prJSON))
	reg.Register("POST", prPath+"/42/decline", httpmock.JSONResponse(200, prJSON))

	if pr, err := c.DeclinePR(context.Background(), "acme", "widgets", 42, "Superseded by #43"); err != nil || pr.ID != 42 {
		t.Fatalf("pr %+v err %v", pr, err)
	}
	assertBody(t, reg.Calls[0], `{"message":"Superseded by #43"}`)
	if _, err := c.DeclinePR(context.Background(), "acme", "widgets", 42, ""); err != nil {
		t.Fatal(err)
	}
	if len(reg.Calls[1].Body) != 0 {
		t.Errorf("body without a message = %q, want none", reg.Calls[1].Body)
	}
}
