package list

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func TestNewCmdList_ParsesFlags(t *testing.T) {
	reg := httpmock.New(t)
	f, _, _, _ := prtest.NewFactory(reg)
	var got *ListOptions
	cmd := NewCmdList(f, func(o *ListOptions) error { got = o; return nil })
	err := prtest.Run(cmd, "-s", "MERGED", "-A", "@me", "--reviewer", "bob", "-B", "main", "-H", "feat",
		"--query", `title ~ "x"`, "-L", "5", "--json", "id,title")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "merged" || got.Author != "@me" || got.Reviewer != "bob" || got.Base != "main" || got.Head != "feat" ||
		got.Query != `title ~ "x"` || got.Limit != 5 || got.Exporter == nil {
		t.Errorf("parsed %+v", got)
	}
}

func TestNewCmdList_RejectsBadInput(t *testing.T) {
	for _, args := range [][]string{{"--state", "closed"}, {"--limit", "0"}, {"extra"}} {
		reg := httpmock.New(t)
		f, _, _, _ := prtest.NewFactory(reg)
		err := prtest.Run(NewCmdList(f, func(*ListOptions) error { return nil }), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v, want FlagError", args, err)
		}
	}
}

func TestList_FiltersAndTTYTable(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page(prtest.PR42)))
	f, ios, out, _ := prtest.NewFactory(reg)
	ios.SetStdoutTTY(true)

	err := prtest.Run(NewCmdList(f, nil), "--state", "merged", "--author", "@me", "--reviewer", "bob",
		"--base", "main", "--head", `feat/"x"`, "--query", `title ~ "x"`, "--limit", "5")
	if err != nil {
		t.Fatal(err)
	}
	q := reg.Calls[1].URL.Query()
	wantQ := `state = "MERGED" AND (author.uuid = "` + prtest.AdaUUID + `" AND reviewers.nickname = "bob" AND destination.branch.name = "main"` +
		` AND source.branch.name = "feat/\"x\"" AND (title ~ "x"))`
	if q.Get("q") != wantQ || q.Get("state") != "" || q.Get("pagelen") != "5" || q.Get("fields") != "" {
		t.Errorf("query = %v", q)
	}
	for _, want := range []string{"ID", "TITLE", "#42", "Add widgets", "feature/widgets → main", "ada", "2026-10-02"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestList_NonTTYIsTabSeparated(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page(prtest.PR42, prtest.PR7)))
	f, _, out, _ := prtest.NewFactory(reg)
	if err := prtest.Run(NewCmdList(f, nil), "--state", "all"); err != nil {
		t.Fatal(err)
	}
	q := reg.Calls[0].URL.Query()
	if !slices.Equal(q["state"], []string{"OPEN", "MERGED", "DECLINED", "SUPERSEDED"}) || q.Get("q") != "" {
		t.Errorf("query = %v", q)
	}
	want := "42\tAdd widgets\tfeature/widgets\tmain\tOPEN\tada\t2026-10-02T08:30:00Z\n" +
		"7\tFix gears\tfix/gears\tmain\tMERGED\tbob\t2026-09-21T10:00:00Z\n"
	if out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
}

func TestList_JSONAsksForParticipantsOnlyWhenNeeded(t *testing.T) {
	for _, tc := range []struct {
		fields string
		want   bool
	}{{"id,title", false}, {"id,reviewers", true}} {
		reg := httpmock.New(t)
		reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page(prtest.PR42)))
		f, _, out, _ := prtest.NewFactory(reg)
		if err := prtest.Run(NewCmdList(f, nil), "--json", tc.fields); err != nil {
			t.Fatal(err)
		}
		if got := reg.Calls[0].URL.Query().Get("fields") != ""; got != tc.want {
			t.Errorf("--json %s: participants requested = %v", tc.fields, got)
		}
		if !strings.HasPrefix(out.String(), `[{"id":42`) {
			t.Errorf("--json %s: out = %q", tc.fields, out.String())
		}
	}
}

func TestList_EmptyResults(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page()))
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page()))

	f, _, out, _ := prtest.NewFactory(reg)
	if err := prtest.Run(NewCmdList(f, nil), "--json", "id"); err != nil || out.String() != "[]\n" {
		t.Errorf("JSON: out %q err %v", out.String(), err)
	}

	f, ios, out, errOut := prtest.NewFactory(reg)
	ios.SetStdoutTTY(true)
	if err := prtest.Run(NewCmdList(f, nil)); err != nil || out.Len() != 0 ||
		errOut.String() != "No pull requests match your filters in acme/widgets\n" {
		t.Errorf("TTY: out %q stderr %q err %v", out.String(), errOut.String(), err)
	}
}
