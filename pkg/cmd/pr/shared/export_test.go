package shared_test

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func decode[T any](t *testing.T, raw string) *T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	return &v
}

// assertJSON compares got's JSON encoding with want by value (key order does not matter).
func assertJSON(t *testing.T, got any, want string) {
	t.Helper()
	gotBytes, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var g, w any
	if err := json.Unmarshal(gotBytes, &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("bad expectation: %v", err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("JSON mismatch\n got: %s\nwant: %s", gotBytes, want)
	}
}

const (
	adaJSON = `{"displayName":"Ada Example","nickname":"ada","uuid":"{00000000-0000-0000-0000-000000000001}","accountId":"000000:aaaa"}`
	bobJSON = `{"displayName":"Bob Example","nickname":"bob","uuid":"{00000000-0000-0000-0000-000000000002}","accountId":"000000:bbbb"}`
	cyJSON  = `{"displayName":"Cy Example","nickname":"cy","uuid":"{00000000-0000-0000-0000-000000000003}","accountId":"000000:cccc"}`
)

func TestFieldListsMatchJSONKeys(t *testing.T) {
	check := func(name string, v any, fields []string) {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		if keys := slices.Sorted(maps.Keys(m)); !slices.Equal(keys, slices.Sorted(slices.Values(fields))) {
			t.Errorf("%s: JSON keys %v != field list %v", name, keys, fields)
		}
	}
	check("PullRequest", shared.PullRequest{}, shared.PullRequestFields)
	check("Comment", shared.Comment{}, shared.CommentFields)
	check("Check", shared.Check{}, shared.CheckFields)
}

func TestNewPullRequest_Golden(t *testing.T) {
	got := shared.NewPullRequest(decode[bitbucket.PullRequest](t, prtest.PR42))
	assertJSON(t, got, `{"id":42,"title":"Add widgets","body":"Adds the widget factory.","state":"OPEN","draft":false,`+
		`"author":`+adaJSON+`,"sourceBranch":"feature/widgets","sourceCommit":"abc1234","sourceRepo":"acme/widgets",`+
		`"destinationBranch":"main","destinationCommit":"def5678","mergeCommit":"",`+
		`"reviewers":[{"user":`+bobJSON+`,"state":"approved"},{"user":`+cyJSON+`,"state":"pending"}],`+
		`"participants":[{"user":`+bobJSON+`,"role":"reviewer","approved":true,"state":"approved"},`+
		`{"user":`+cyJSON+`,"role":"reviewer","approved":false,"state":"pending"},`+
		`{"user":`+adaJSON+`,"role":"participant","approved":false,"state":"pending"}],`+
		`"commentCount":2,"taskCount":1,"closeSourceBranch":true,"url":"https://bitbucket.org/acme/widgets/pull-requests/42",`+
		`"createdOn":"2026-10-01T12:00:00Z","updatedOn":"2026-10-02T08:30:00Z","closedBy":null}`)
}

func TestNewPullRequest_ListItemHasEmptyArrays(t *testing.T) {
	b, err := json.Marshal(shared.NewPullRequest(decode[bitbucket.PullRequest](t, prtest.PR7)))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"reviewers":[]`, `"participants":[]`, `"mergeCommit":"ccc3333"`, `"closedBy":` + bobJSON} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
}

func TestNewPullRequest_WithoutAuthor(t *testing.T) {
	raw := decode[bitbucket.PullRequest](t, prtest.PR42)
	raw.Author = nil
	got := shared.NewPullRequest(raw)
	b, _ := json.Marshal(got)
	if got.Author != nil || !strings.Contains(string(b), `"author":null`) {
		t.Errorf("author = %+v, json %s", got.Author, b)
	}
}

func TestNewComment(t *testing.T) {
	inline := shared.NewComment(decode[bitbucket.Comment](t, prtest.CommentInline))
	if inline.Path != "src/widget.go" || inline.Line == nil || *inline.Line != 12 || inline.ParentID != nil || inline.Author.Nickname != "bob" {
		t.Errorf("inline = %+v", inline)
	}
	reply := shared.NewComment(decode[bitbucket.Comment](t, prtest.CommentReply))
	assertJSON(t, reply, `{"id":102,"author":`+adaJSON+`,"body":"Done.","path":"","line":null,"parentId":101,"deleted":false,`+
		`"url":"https://bitbucket.org/acme/widgets/pull-requests/42/_/diff#comment-102",`+
		`"createdOn":"2026-10-01T14:00:00Z","updatedOn":"2026-10-01T14:00:00Z"}`)
}

func TestNewCheck(t *testing.T) {
	c := shared.NewCheck(decode[bitbucket.CommitStatus](t, prtest.Status("build", "INPROGRESS")))
	assertJSON(t, c, `{"key":"build","name":"build","state":"inprogress","description":"build inprogress",`+
		`"url":"https://ci.example.com/build","updatedOn":"2026-10-02T09:00:00Z"}`)
}
