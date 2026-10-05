package shared_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func newFinder(reg *httpmock.Registry, branch string) *shared.Finder {
	f, _, _, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, branch)
	client, _ := f.HTTPClient()
	return &shared.Finder{Client: client, BaseRepo: f.BaseRepo, Branch: f.Branch}
}

func TestParseSelector(t *testing.T) {
	cases := []struct {
		in   string
		id   int
		repo string
		bad  bool
	}{
		{in: "", id: 0},
		{in: "42", id: 42},
		{in: "#42", id: 42},
		{in: "https://bitbucket.org/other/tools/pull-requests/7", id: 7, repo: "other/tools"},
		{in: "https://bitbucket.org/other/tools/pull-requests/7/diff", id: 7, repo: "other/tools"},
		{in: "abc", bad: true},
		{in: "0", bad: true},
		{in: "-3", bad: true},
		{in: "https://github.com/a/b/pull/1", bad: true},
		{in: "https://bitbucket.org/other/tools/pull-requests/0", bad: true},
	}
	for _, tc := range cases {
		id, repo, err := shared.ParseSelector(tc.in)
		if tc.bad {
			var flagErr *cmdutil.FlagError
			if !errors.As(err, &flagErr) {
				t.Errorf("ParseSelector(%q): err = %v, want FlagError", tc.in, err)
			}
			continue
		}
		gotRepo := ""
		if repo != nil {
			gotRepo = repo.FullName()
		}
		if err != nil || id != tc.id || gotRepo != tc.repo {
			t.Errorf("ParseSelector(%q) = %d %q %v", tc.in, id, gotRepo, err)
		}
	}
}

func TestFind_ByNumber(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	pr, repo, err := newFinder(reg, "").Find(context.Background(), "#42")
	if err != nil || pr.ID != 42 || repo.FullName() != "acme/widgets" {
		t.Fatalf("pr %v repo %v err %v", pr, repo, err)
	}
}

func TestFind_ByURLUsesThatRepository(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/repositories/other/tools/pullrequests/7", httpmock.JSONResponse(200, prtest.PR7))
	pr, repo, err := newFinder(reg, "").Find(context.Background(), "https://bitbucket.org/other/tools/pull-requests/7")
	if err != nil || pr.ID != 7 || repo.FullName() != "other/tools" {
		t.Fatalf("pr %v repo %v err %v", pr, repo, err)
	}
}

func TestFind_CurrentBranchQuotesTheName(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page(prtest.PR42)))
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	pr, _, err := newFinder(reg, `feature/"quoted"`).Find(context.Background(), "")
	if err != nil || pr.ID != 42 {
		t.Fatalf("pr %v err %v", pr, err)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("q") != `source.branch.name = "feature/\"quoted\""` || q.Get("state") != "OPEN" {
		t.Errorf("query = %v", q)
	}
}

func TestFind_NoOpenPullRequest(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page()))
	_, _, err := newFinder(reg, "feature/widgets").Find(context.Background(), "")
	if err == nil || err.Error() != `no open pull request found for branch "feature/widgets" in acme/widgets` {
		t.Errorf("err = %v", err)
	}
}

func TestFind_SeveralOpenPullRequests(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page(prtest.PR42, prtest.PR9)))
	_, _, err := newFinder(reg, "feature/widgets").Find(context.Background(), "")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "#42, #9") {
		t.Errorf("err = %v", err)
	}
}

func TestFind_DetachedHEAD(t *testing.T) {
	reg := httpmock.New(t)
	_, _, err := newFinder(reg, "").Find(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "not on a branch") {
		t.Errorf("err = %v", err)
	}
}
