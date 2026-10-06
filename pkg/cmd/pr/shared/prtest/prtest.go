// Package prtest holds Bitbucket fixtures and helpers for `khbb pr` tests. Every identity is fake.
package prtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/internal/iostreams"
)

// Fake accounts. Ada is the authenticated user wherever a test serves GET /user.
const (
	AdaUUID = "{00000000-0000-0000-0000-000000000001}"
	BobUUID = "{00000000-0000-0000-0000-000000000002}"
	CyUUID  = "{00000000-0000-0000-0000-000000000003}"

	Ada = `{"display_name":"Ada Example","nickname":"ada","uuid":"` + AdaUUID + `","account_id":"000000:aaaa"}`
	Bob = `{"display_name":"Bob Example","nickname":"bob","uuid":"` + BobUUID + `","account_id":"000000:bbbb"}`
	Cy  = `{"display_name":"Cy Example","nickname":"cy","uuid":"` + CyUUID + `","account_id":"000000:cccc"}`
)

// PRs is the request path of acme/widgets pull requests.
const PRs = "/2.0/repositories/acme/widgets/pullrequests"

// Repo and Members are the request paths of the acme/widgets repository and the acme workspace's members.
const (
	Repo    = "/2.0/repositories/acme/widgets"
	Members = "/2.0/workspaces/acme/members"
)

// PR42 is an open pull request by ada from feature/widgets into main; bob approved, cy has not reviewed.
const PR42 = `{"id":42,"title":"Add widgets","description":"Adds the widget factory.","state":"OPEN","draft":false,"author":` + Ada +
	`,"source":{"branch":{"name":"feature/widgets"},"commit":{"hash":"abc1234"},"repository":{"full_name":"acme/widgets"}}` +
	`,"destination":{"branch":{"name":"main"},"commit":{"hash":"def5678"},"repository":{"full_name":"acme/widgets"}}` +
	`,"merge_commit":null,"reviewers":[` + Bob + `,` + Cy + `]` +
	`,"participants":[{"user":` + Bob + `,"role":"REVIEWER","approved":true,"state":"approved"},` +
	`{"user":` + Cy + `,"role":"REVIEWER","approved":false,"state":null},` +
	`{"user":` + Ada + `,"role":"PARTICIPANT","approved":false,"state":null}]` +
	`,"comment_count":2,"task_count":1,"close_source_branch":true,"closed_by":null` +
	`,"created_on":"2026-10-01T12:00:00.000000+00:00","updated_on":"2026-10-02T08:30:00.000000+00:00"` +
	`,"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/42"}}}`

// PR9 is an open pull request by bob that asks ada (pending) and cy (approved) for review.
const PR9 = `{"id":9,"title":"Update docs","description":"","state":"OPEN","draft":false,"author":` + Bob +
	`,"source":{"branch":{"name":"feature/docs"},"commit":{"hash":"999aaaa"},"repository":{"full_name":"acme/widgets"}}` +
	`,"destination":{"branch":{"name":"main"},"commit":{"hash":"def5678"},"repository":{"full_name":"acme/widgets"}}` +
	`,"merge_commit":null,"reviewers":[` + Ada + `,` + Cy + `]` +
	`,"participants":[{"user":` + Cy + `,"role":"REVIEWER","approved":true,"state":"approved"}]` +
	`,"comment_count":0,"task_count":0,"close_source_branch":false,"closed_by":null` +
	`,"created_on":"2026-09-30T09:00:00.000000+00:00","updated_on":"2026-10-01T09:00:00.000000+00:00"` +
	`,"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/9"}}}`

// PR7 is a merged pull request by bob, shaped like a list item (no reviewers or participants).
const PR7 = `{"id":7,"title":"Fix gears","description":"","state":"MERGED","draft":false,"author":` + Bob +
	`,"source":{"branch":{"name":"fix/gears"},"commit":{"hash":"aaa1111"},"repository":{"full_name":"acme/widgets"}}` +
	`,"destination":{"branch":{"name":"main"},"commit":{"hash":"bbb2222"},"repository":{"full_name":"acme/widgets"}}` +
	`,"merge_commit":{"hash":"ccc3333"},"comment_count":0,"task_count":0,"close_source_branch":false,"closed_by":` + Bob +
	`,"created_on":"2026-09-20T10:00:00.000000+00:00","updated_on":"2026-09-21T10:00:00.000000+00:00"` +
	`,"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/7"}}}`

// CommentInline is bob's comment on src/widget.go line 12 of PR 42; CommentReply is ada's reply to
// it; CommentPending is bob's unpublished draft comment, which never appears in output.
const (
	CommentInline = `{"id":101,"content":{"raw":"Please rename this."},"user":` + Bob +
		`,"inline":{"path":"src/widget.go","from":null,"to":12},"deleted":false` +
		`,"created_on":"2026-10-01T13:00:00.000000+00:00","updated_on":"2026-10-01T13:00:00.000000+00:00"` +
		`,"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/42/_/diff#comment-101"}}}`
	CommentReply = `{"id":102,"content":{"raw":"Done."},"user":` + Ada + `,"parent":{"id":101},"deleted":false` +
		`,"created_on":"2026-10-01T14:00:00.000000+00:00","updated_on":"2026-10-01T14:00:00.000000+00:00"` +
		`,"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/42/_/diff#comment-102"}}}`
	CommentPending = `{"id":103,"content":{"raw":"Draft thought, not published yet."},"user":` + Bob +
		`,"pending":true,"deleted":false` +
		`,"created_on":"2026-10-01T15:00:00.000000+00:00","updated_on":"2026-10-01T15:00:00.000000+00:00"` +
		`,"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/42/_/diff#comment-103"}}}`
)

// Status returns a commit status whose key and name are key and whose Bitbucket state is state.
func Status(key, state string) string {
	return `{"key":"` + key + `","name":"` + key + `","state":"` + state + `","description":"` + key + ` ` + strings.ToLower(state) +
		`","url":"https://ci.example.com/` + key + `","updated_on":"2026-10-02T09:00:00.000000+00:00"}`
}

// Page wraps items in a single-page Bitbucket collection.
func Page(items ...string) string {
	return `{"values":[` + strings.Join(items, ",") + `],"pagelen":50}`
}

// NewFactory returns a non-TTY Factory bound to acme/widgets whose client talks to reg without
// retries. The client honors f.DryRun (--dry-run), printing to the returned stdout buffer.
func NewFactory(reg *httpmock.Registry) (*cmdutil.Factory, *iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	ios, _, out, errOut := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, RepoOverride: "acme/widgets"}
	f.HTTPClient = func() (*bitbucket.Client, error) {
		return bitbucket.New(bitbucket.Options{
			Email: "dev@example.com", Token: "t", HTTPClient: reg.Client(), MaxAttempts: 1,
			DryRun: f.DryRun, DryRunOut: ios.Out,
		}), nil
	}
	return f, ios, out, errOut
}

// SetBranch makes f report branch as the checked-out branch; "" simulates a detached HEAD.
func SetBranch(f *cmdutil.Factory, branch string) {
	f.Git = &gitctx.Resolver{Git: func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "symbolic-ref" && branch != "" {
			return branch, nil
		}
		return "", errors.New("not on a branch")
	}}
}

// Run executes cmd with args (never falling back to os.Args) and returns its error.
func Run(cmd *cobra.Command, args ...string) error {
	cmd.SetArgs(append([]string{}, args...))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

// SetTTY makes every stream of ios behave like a terminal.
func SetTTY(ios *iostreams.IOStreams) {
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)
}

// WithState returns a pull request fixture with the pull request's state replaced, such as
// WithState(PR42, "MERGED"). Participant states are left alone.
func WithState(pr, state string) string {
	return strings.Replace(pr, `"state":"OPEN"`, `"state":"`+state+`"`, 1)
}

// Member wraps an account fixture in a workspace membership, as GET /workspaces/{ws}/members returns it.
func Member(user string) string {
	return `{"type":"workspace_membership","user":` + user + `}`
}

// Writes returns the requests reg received that change something: every method but GET.
func Writes(reg *httpmock.Registry) []httpmock.Call {
	var writes []httpmock.Call
	for _, c := range reg.Calls {
		if c.Method != "GET" {
			writes = append(writes, c)
		}
	}
	return writes
}

// AssertJSONBody fails t unless the request body is the JSON value want (key order and spacing ignored).
func AssertJSONBody(t testing.TB, c httpmock.Call, want string) {
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
