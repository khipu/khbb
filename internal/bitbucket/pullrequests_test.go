package bitbucket_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

const prPath = "/2.0/repositories/acme/widgets/pullrequests"

const prJSON = `{"id":42,"title":"Add widgets","description":"Adds the widget factory.","state":"OPEN","draft":true,` +
	`"author":{"display_name":"Ada Example","nickname":"ada","uuid":"{00000000-0000-0000-0000-000000000001}","account_id":"000000:aaaa"},` +
	`"source":{"branch":{"name":"feature/widgets"},"commit":{"hash":"abc1234"},"repository":{"full_name":"acme/widgets"}},` +
	`"destination":{"branch":{"name":"main"},"commit":{"hash":"def5678"},"repository":{"full_name":"acme/widgets"}},` +
	`"merge_commit":null,` +
	`"reviewers":[{"display_name":"Bob Example","nickname":"bob","uuid":"{00000000-0000-0000-0000-000000000002}","account_id":"000000:bbbb"}],` +
	`"participants":[{"user":{"display_name":"Bob Example","nickname":"bob","uuid":"{00000000-0000-0000-0000-000000000002}","account_id":"000000:bbbb"},"role":"REVIEWER","approved":true,"state":"approved"}],` +
	`"comment_count":2,"task_count":1,"close_source_branch":true,"closed_by":null,` +
	`"created_on":"2026-10-01T12:00:00.000000+00:00","updated_on":"2026-10-02T08:30:00.000000+00:00",` +
	`"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/42"}}}`

func TestListPullRequests_BuildsQuery(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath, httpmock.JSONResponse(200, `{"values":[`+prJSON+`]}`))

	prs, err := c.ListPullRequests(context.Background(), "acme", "widgets", bitbucket.PRListOptions{
		States: []string{"OPEN", "MERGED"}, Query: `author.uuid = "{x}"`, WithParticipants: true,
	}, 30)
	if err != nil {
		t.Fatal(err)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("q") != `(state = "OPEN" OR state = "MERGED") AND (author.uuid = "{x}")` || len(q["state"]) != 0 ||
		q.Get("fields") != "+values.participants,+values.reviewers" || q.Get("pagelen") != "30" {
		t.Errorf("query = %v", q)
	}
	if !strings.Contains(reg.Calls[0].URL.RawQuery, "fields=%2Bvalues.participants") {
		t.Errorf("'+' must be percent-encoded: %s", reg.Calls[0].URL.RawQuery)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d pull requests", len(prs))
	}
	pr := prs[0]
	if pr.ID != 42 || !pr.Draft || pr.Source.Branch.Name != "feature/widgets" || pr.Source.Commit.Hash != "abc1234" ||
		pr.Destination.Repository.FullName != "acme/widgets" || pr.Author.Nickname != "ada" || pr.MergeCommit != nil {
		t.Errorf("decoded %+v", pr)
	}
	if len(pr.Participants) != 1 || pr.Participants[0].State == nil || *pr.Participants[0].State != "approved" {
		t.Errorf("participants = %+v", pr.Participants)
	}
	if !pr.CreatedOn.Equal(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)) || pr.Links.HTML.Href != "https://bitbucket.org/acme/widgets/pull-requests/42" {
		t.Errorf("created %v url %q", pr.CreatedOn, pr.Links.HTML.Href)
	}
}

func TestListPullRequests_StatesWithoutQueryUseParams(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath, httpmock.JSONResponse(200, `{"values":[]}`))

	_, err := c.ListPullRequests(context.Background(), "acme", "widgets", bitbucket.PRListOptions{
		States: []string{"OPEN", "MERGED"},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	q := reg.Calls[0].URL.Query()
	if !slices.Equal(q["state"], []string{"OPEN", "MERGED"}) || q.Get("q") != "" {
		t.Errorf("query = %v", q)
	}
}

func TestListPullRequests_SingleStateWithQuery(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath, httpmock.JSONResponse(200, `{"values":[]}`))

	_, err := c.ListPullRequests(context.Background(), "acme", "widgets", bitbucket.PRListOptions{
		States: []string{"OPEN"}, Query: `source.branch.name = "x"`,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("q") != `state = "OPEN" AND (source.branch.name = "x")` || len(q["state"]) != 0 {
		t.Errorf("query = %v", q)
	}
}

func TestListPullRequests_DefaultsToPagelenOnly(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath, httpmock.JSONResponse(200, `{"values":[]}`))
	prs, err := c.ListPullRequests(context.Background(), "acme", "widgets", bitbucket.PRListOptions{}, 0)
	if err != nil || len(prs) != 0 {
		t.Fatalf("prs %v err %v", prs, err)
	}
	if got := reg.Calls[0].URL.RawQuery; got != "pagelen=50" {
		t.Errorf("query = %q", got)
	}
}

func TestGetPullRequest(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath+"/42", httpmock.JSONResponse(200, prJSON))
	pr, err := c.GetPullRequest(context.Background(), "acme", "widgets", 42)
	if err != nil || pr.Title != "Add widgets" || len(pr.Reviewers) != 1 {
		t.Fatalf("pr %+v err %v", pr, err)
	}
}

func TestPullRequestSubresources(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath+"/42/comments", httpmock.JSONResponse(200,
		`{"values":[{"id":101,"content":{"raw":"Please rename this."},"inline":{"path":"src/widget.go","from":null,"to":12},"deleted":false},`+
			`{"id":102,"content":{"raw":"Done."},"parent":{"id":101},"deleted":false}]}`))
	reg.Register("GET", prPath+"/42/statuses", httpmock.JSONResponse(200,
		`{"values":[{"key":"build","name":"Build","state":"SUCCESSFUL","description":"ok","url":"https://ci.example.com/build","updated_on":"2026-10-02T09:00:00.000000+00:00"}]}`))
	reg.Register("GET", prPath+"/42/diffstat", httpmock.JSONResponse(200,
		`{"values":[{"status":"removed","lines_added":0,"lines_removed":3,"old":{"path":"docs/old.md"},"new":null}]}`))
	reg.Register("GET", prPath+"/42/diff", httpmock.StringResponse(200, "diff --git a/x b/x\n"))
	reg.Register("GET", prPath+"/42/patch", httpmock.StringResponse(200, "From abc\n"))

	ctx := context.Background()
	comments, err := c.ListPRComments(ctx, "acme", "widgets", 42)
	if err != nil || len(comments) != 2 || *comments[0].Inline.To != 12 || comments[1].Parent.ID != 101 {
		t.Fatalf("comments %+v err %v", comments, err)
	}
	statuses, err := c.ListPRStatuses(ctx, "acme", "widgets", 42)
	if err != nil || len(statuses) != 1 || statuses[0].State != "SUCCESSFUL" || statuses[0].URL != "https://ci.example.com/build" {
		t.Fatalf("statuses %+v err %v", statuses, err)
	}
	stats, err := c.PRDiffStat(ctx, "acme", "widgets", 42)
	if err != nil || len(stats) != 1 || stats[0].New != nil || stats[0].Old.Path != "docs/old.md" {
		t.Fatalf("stats %+v err %v", stats, err)
	}
	diff, err := c.PRDiff(ctx, "acme", "widgets", 42)
	if err != nil || diff != "diff --git a/x b/x\n" {
		t.Fatalf("diff %q err %v", diff, err)
	}
	patch, err := c.PRPatch(ctx, "acme", "widgets", 42)
	if err != nil || patch != "From abc\n" {
		t.Fatalf("patch %q err %v", patch, err)
	}
	for _, call := range reg.Calls[3:] {
		if got := call.Header.Get("Accept"); got != "*/*" {
			t.Errorf("%s: Accept = %q, want */*", call.URL.Path, got)
		}
	}
}

func TestGetText_ErrorStatus(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath+"/99/diff", httpmock.JSONResponse(404, `{"type":"error","error":{"message":"Pull request not found"}}`))
	_, err := c.PRDiff(context.Background(), "acme", "widgets", 99)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 || httpErr.Message != "Pull request not found" {
		t.Fatalf("err = %v", err)
	}
}
