package watch

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

type result struct {
	out, errOut string
	slept       []time.Duration
	err         error
}

func run(t *testing.T, reg *httpmock.Registry, setup func(*cmdutil.Factory), args ...string) result {
	t.Helper()
	f, _, out, errOut := prtest.NewFactory(reg)
	if setup != nil {
		setup(f)
	}
	var slept []time.Duration
	cmd := NewCmdWatch(f, func(o *WatchOptions) error {
		o.Sleep = func(d time.Duration) { slept = append(slept, d) }
		return watchRun(context.Background(), o)
	})
	err := prtest.Run(cmd, args...)
	return result{out.String(), errOut.String(), slept, err}
}

func finished(reg *httpmock.Registry, n int, state string) {
	path := ptest.Pipelines + "/" + strconv.Itoa(n)
	reg.Register("GET", path, httpmock.JSONResponse(200, ptest.Pipeline(n, state)))
	reg.Register("GET", path+"/steps", httpmock.JSONResponse(200, ptest.Steps(ptest.Step("{s1}", "Build", state))))
}

func headGit(f *cmdutil.Factory) {
	prtest.SetGit(f, &prtest.FakeGit{Outputs: map[string]string{
		"symbolic-ref --quiet --short HEAD": "feature/widgets",
		"rev-parse HEAD":                    ptest.Commit,
	}})
}

func TestWatch_ByNumberWithExitStatus(t *testing.T) {
	reg := httpmock.New(t)
	finished(reg, 42, ptest.StateFailed)
	r := run(t, reg, nil, "42", "--exit-status")
	var exitErr *cmdutil.ExitError
	if !errors.As(r.err, &exitErr) || exitErr.Code != 1 || !strings.HasSuffix(r.out, "#42 failed (1m02s)\n") {
		t.Errorf("err %v out %q", r.err, r.out)
	}
	if r := run(t, finishedReg(t, 42, ptest.StateFailed), nil, "42"); r.err != nil {
		t.Errorf("without --exit-status a failed pipeline exits 0: %v", r.err)
	}
}

func finishedReg(t *testing.T, n int, state string) *httpmock.Registry {
	reg := httpmock.New(t)
	finished(reg, n, state)
	return reg
}

func TestWatch_FollowsTheHeadCommit(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page(ptest.Pipeline(43, ptest.StatePending))))
	finished(reg, 43, ptest.StateSuccessful)

	r := run(t, reg, headGit, "--exit-status")
	if r.err != nil {
		t.Fatal(r.err)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("target.branch") != "feature/widgets" || q.Get("target.commit.hash") != ptest.Commit || q.Get("pagelen") != "1" {
		t.Errorf("query = %v", q)
	}
	if r.errOut != "Waiting for a pipeline for commit abc1234 on branch feature/widgets…\n" ||
		!slices.Equal(r.slept, []time.Duration{5 * time.Second}) || !strings.HasSuffix(r.out, "#43 successful (1m02s)\n") {
		t.Errorf("stderr %q slept %v out %q", r.errOut, r.slept, r.out)
	}
}

func TestWatch_NoPipelineForTheHeadCommit(t *testing.T) {
	reg := httpmock.New(t)
	// Polls at 0 s, 5 s, …, 60 s: 13 lists and 12 sleeps before giving up.
	for range 13 {
		reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))
	}
	r := run(t, reg, headGit)
	var notFound *cmdutil.NotFoundError
	if !errors.As(r.err, &notFound) || len(r.slept) != 12 ||
		r.err.Error() != `no pipeline started for commit abc1234 on branch "feature/widgets" within 1m0s; push it, or name a pipeline number` {
		t.Errorf("err %v slept %d", r.err, len(r.slept))
	}
}

func TestWatch_WithoutAHeadUsesTheNewestOfTheBranch(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page(ptest.Pipeline(43, ptest.StateRunning))))
	finished(reg, 43, ptest.StateSuccessful)

	r := run(t, reg, func(f *cmdutil.Factory) { prtest.SetBranch(f, "feature/widgets") })
	if r.err != nil {
		t.Fatal(r.err)
	}
	if q := reg.Calls[0].URL.Query(); q.Has("target.commit.hash") || q.Get("target.branch") != "feature/widgets" {
		t.Errorf("query = %v", q)
	}
}

func TestWatch_FlagErrors(t *testing.T) {
	for _, args := range [][]string{{"42", "--interval", "0"}, {"abc"}, {"1", "2"}} {
		r := run(t, httpmock.New(t), nil, args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(r.err, &flagErr) {
			t.Errorf("%v: err = %v", args, r.err)
		}
	}
}
