package shared_test

import (
	"context"
	"errors"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
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
		n    int
		repo string
	}{
		{"", 0, ""},
		{"42", 42, ""},
		{" #42 ", 42, ""},
		{"https://bitbucket.org/acme/gears/pipelines/results/7", 7, "acme/gears"},
		{"https://bitbucket.org/acme/gears/pipelines/results/7/steps/{x}", 7, "acme/gears"},
	}
	for _, tc := range cases {
		n, repo, err := shared.ParseSelector(tc.in)
		got := ""
		if repo != nil {
			got = repo.FullName()
		}
		if err != nil || n != tc.n || got != tc.repo {
			t.Errorf("ParseSelector(%q) = %d, %q, %v", tc.in, n, got, err)
		}
	}
	for _, bad := range []string{"abc", "0", "#-1", "https://bitbucket.org/acme/gears/pull-requests/7"} {
		var flagErr *cmdutil.FlagError
		if _, _, err := shared.ParseSelector(bad); !errors.As(err, &flagErr) {
			t.Errorf("ParseSelector(%q): err = %v", bad, err)
		}
	}
}

func TestFind_ByNumber(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, ptest.StateFailed)))

	p, repo, err := newFinder(reg, "feature/widgets").Find(context.Background(), "#42")
	if err != nil || p.BuildNumber != 42 || repo.FullName() != "acme/widgets" {
		t.Errorf("p %v repo %v err %v", p, repo, err)
	}
}

func TestFind_ByURLUsesThatRepository(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/repositories/acme/gears/pipelines/7", httpmock.JSONResponse(200, ptest.Pipeline(7, ptest.StateSuccessful)))

	_, repo, err := newFinder(reg, "").Find(context.Background(), "https://bitbucket.org/acme/gears/pipelines/results/7")
	if err != nil || repo.FullName() != "acme/gears" {
		t.Errorf("repo %v err %v", repo, err)
	}
}

func TestFind_NewestOfTheCurrentBranch(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page(ptest.Pipeline(43, ptest.StateRunning))))

	p, _, err := newFinder(reg, "feature/widgets").Find(context.Background(), "")
	if err != nil || p.BuildNumber != 43 {
		t.Fatalf("p %v err %v", p, err)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("target.branch") != "feature/widgets" || q.Get("sort") != "-created_on" || q.Get("pagelen") != "1" {
		t.Errorf("query = %v", q)
	}
}

func TestFind_NoPipelineForTheBranch(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))

	_, _, err := newFinder(reg, "feature/widgets").Find(context.Background(), "")
	var notFound *cmdutil.NotFoundError
	if !errors.As(err, &notFound) || err.Error() != `no pipelines found for branch "feature/widgets" in acme/widgets` {
		t.Errorf("err = %v", err)
	}
}
