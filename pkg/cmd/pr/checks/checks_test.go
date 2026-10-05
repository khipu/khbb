package checks

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const statusesPath = prtest.PRs + "/42/statuses"

func newReg(t *testing.T, responses ...httpmock.Responder) *httpmock.Registry {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	for _, r := range responses {
		reg.Register("GET", statusesPath, r)
	}
	return reg
}

func page(items ...string) httpmock.Responder {
	return httpmock.JSONResponse(200, prtest.Page(items...))
}

func run(t *testing.T, reg *httpmock.Registry, tty bool, args ...string) (string, string, []time.Duration, error) {
	t.Helper()
	f, ios, out, errOut := prtest.NewFactory(reg)
	ios.SetStdoutTTY(tty)
	var slept []time.Duration
	cmd := NewCmdChecks(f, func(o *ChecksOptions) error {
		o.Sleep = func(d time.Duration) { slept = append(slept, d) }
		return checksRun(context.Background(), o)
	})
	err := prtest.Run(cmd, append([]string{"42"}, args...)...)
	return out.String(), errOut.String(), slept, err
}

func exitCode(err error) int {
	var exitErr *cmdutil.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	if err != nil {
		return -1
	}
	return 0
}

func TestChecks_AllSuccessful(t *testing.T) {
	out, _, _, err := run(t, newReg(t, page(prtest.Status("lint", "SUCCESSFUL"), prtest.Status("build", "SUCCESSFUL"))), false)
	want := "build\tsuccessful\thttps://ci.example.com/build\nlint\tsuccessful\thttps://ci.example.com/lint\n"
	if err != nil || out != want {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestChecks_FailureExitsOne(t *testing.T) {
	out, _, _, err := run(t, newReg(t, page(prtest.Status("lint", "SUCCESSFUL"), prtest.Status("build", "FAILED"), prtest.Status("deploy", "STOPPED"))), false)
	want := "build\tfailed\thttps://ci.example.com/build\ndeploy\tstopped\thttps://ci.example.com/deploy\nlint\tsuccessful\thttps://ci.example.com/lint\n"
	if exitCode(err) != 1 || out != want {
		t.Errorf("exit %d out %q", exitCode(err), out)
	}
}

func TestChecks_PendingExitsEight(t *testing.T) {
	_, _, _, err := run(t, newReg(t, page(prtest.Status("build", "INPROGRESS"))), false)
	if exitCode(err) != 8 {
		t.Errorf("exit %d (%v)", exitCode(err), err)
	}
}

func TestChecks_NoChecks(t *testing.T) {
	out, errOut, _, err := run(t, newReg(t, page()), false)
	if err != nil || out != "" || errOut != "no checks reported on pull request #42\n" {
		t.Errorf("out %q stderr %q err %v", out, errOut, err)
	}
}

func TestChecks_TTYSummary(t *testing.T) {
	out, _, _, err := run(t, newReg(t, page(prtest.Status("build", "FAILED"), prtest.Status("lint", "SUCCESSFUL"))), true)
	if exitCode(err) != 1 {
		t.Errorf("exit %d", exitCode(err))
	}
	for _, want := range []string{"Some checks were not successful", "1 failed, 1 successful, 0 pending", "X", "build", "✓", "lint"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestChecks_WatchUntilFinished(t *testing.T) {
	reg := newReg(t, page(prtest.Status("build", "INPROGRESS")), page(prtest.Status("build", "SUCCESSFUL")))
	out, _, slept, err := run(t, reg, false, "--watch", "--interval", "2")
	if err != nil || out != "build\tsuccessful\thttps://ci.example.com/build\n" || !slices.Equal(slept, []time.Duration{2 * time.Second}) {
		t.Errorf("out %q slept %v err %v", out, slept, err)
	}
}

func TestChecks_FailFast(t *testing.T) {
	reg := newReg(t, page(prtest.Status("build", "FAILED"), prtest.Status("lint", "INPROGRESS")))
	_, _, slept, err := run(t, reg, false, "--watch", "--fail-fast")
	if exitCode(err) != 1 || len(slept) != 0 {
		t.Errorf("exit %d slept %v", exitCode(err), slept)
	}
}

func TestChecks_WatchRetriesTransientErrors(t *testing.T) {
	reg := newReg(t, httpmock.StringResponse(503, "busy"), page(prtest.Status("build", "SUCCESSFUL")))
	_, _, slept, err := run(t, reg, false, "--watch")
	if err != nil || !slices.Equal(slept, []time.Duration{5 * time.Second}) {
		t.Errorf("slept %v err %v", slept, err)
	}
}

func TestChecks_WatchStopsOnNotFound(t *testing.T) {
	reg := newReg(t, httpmock.JSONResponse(404, `{"type":"error","error":{"message":"Not found"}}`))
	_, _, slept, err := run(t, reg, false, "--watch")
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 || len(slept) != 0 {
		t.Errorf("err %v slept %v", err, slept)
	}
}

func TestChecks_JSON(t *testing.T) {
	out, _, _, err := run(t, newReg(t, page(prtest.Status("build", "FAILED"))), false, "--json", "name,state")
	if exitCode(err) != 1 || out != `[{"name":"build","state":"failed"}]`+"\n" {
		t.Errorf("exit %d out %q", exitCode(err), out)
	}
}

func TestNewCmdChecks_BadFlags(t *testing.T) {
	for _, args := range [][]string{{"--fail-fast"}, {"--watch", "--interval", "0"}} {
		reg := httpmock.New(t)
		f, _, _, _ := prtest.NewFactory(reg)
		err := prtest.Run(NewCmdChecks(f, nil), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

func TestChecks_WatchSurvivesFourTransientErrors(t *testing.T) {
	reg := newReg(t,
		httpmock.StringResponse(503, "busy"),
		httpmock.StringResponse(503, "busy"),
		httpmock.StringResponse(503, "busy"),
		httpmock.StringResponse(503, "busy"),
		page(prtest.Status("build", "SUCCESSFUL")),
	)
	out, _, slept, err := run(t, reg, false, "--watch")
	if err != nil || out != "build\tsuccessful\thttps://ci.example.com/build\n" || !slices.Equal(slept, []time.Duration{5 * time.Second, 5 * time.Second, 5 * time.Second, 5 * time.Second}) {
		t.Errorf("out %q slept %v err %v", out, slept, err)
	}
}

func TestChecks_WatchGivesUpOnFifthTransientError(t *testing.T) {
	reg := newReg(t,
		httpmock.StringResponse(503, "busy"),
		httpmock.StringResponse(503, "busy"),
		httpmock.StringResponse(503, "busy"),
		httpmock.StringResponse(503, "busy"),
		httpmock.StringResponse(503, "busy"),
	)
	_, _, slept, err := run(t, reg, false, "--watch")
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 503 || !slices.Equal(slept, []time.Duration{5 * time.Second, 5 * time.Second, 5 * time.Second, 5 * time.Second}) {
		t.Errorf("err %v slept %v", err, slept)
	}
}
