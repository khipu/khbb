package shared_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func watchOptions(reg *httpmock.Registry, tty bool) (shared.WatchOptions, *bytes.Buffer, *[]time.Duration) {
	f, ios, out, _ := prtest.NewFactory(reg)
	ios.SetStdoutTTY(tty)
	client, _ := f.HTTPClient()
	slept := &[]time.Duration{}
	return shared.WatchOptions{
		IO: ios, Client: client, Repo: gitctx.Repo{Workspace: "acme", Slug: "widgets"}, Number: 42,
		Interval: 5 * time.Second, Sleep: func(d time.Duration) { *slept = append(*slept, d) },
	}, out, slept
}

func poll(reg *httpmock.Registry, state string, steps ...string) {
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, state)))
	reg.Register("GET", ptest.Pipelines+"/42/steps", httpmock.JSONResponse(200, ptest.Steps(steps...)))
}

func TestWatch_PrintsStateChanges(t *testing.T) {
	reg := httpmock.New(t)
	poll(reg, ptest.StatePending, ptest.Step("{s1}", "Build", ptest.StatePending), ptest.Step("{s2}", "Test", ptest.StatePending),
		ptest.Step("{s3}", "Deploy", ptest.StepNotRun))
	poll(reg, ptest.StateRunning, ptest.Step("{s1}", "Build", ptest.StepInProgress), ptest.Step("{s2}", "Test", ptest.StatePending),
		ptest.Step("{s3}", "Deploy", ptest.StepNotRun))
	poll(reg, ptest.StateFailed, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed),
		ptest.Step("{s3}", "Deploy", ptest.StepNotRun))
	opts, out, slept := watchOptions(reg, false)

	p, steps, err := shared.Watch(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	want := `#42 step "Build": pending
#42 step "Test": pending
#42 step "Deploy": skipped
#42 pending
#42 step "Build": running
#42 running
#42 step "Build": successful (12s)
#42 step "Test": failed (12s)
#42 failed (1m02s)
`
	if out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
	if p.Status != "failed" || len(steps) != 3 || !slices.Equal(*slept, []time.Duration{5 * time.Second, 5 * time.Second}) {
		t.Errorf("p %+v steps %d slept %v", p, len(steps), *slept)
	}
	var exitErr *cmdutil.ExitError
	if err := shared.ExitStatus(p); !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Errorf("ExitStatus = %v", err)
	}
}

func TestWatch_StopsWhenPaused(t *testing.T) {
	reg := httpmock.New(t)
	poll(reg, ptest.StatePaused, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Deploy", ptest.StepWaiting))
	opts, out, slept := watchOptions(reg, false)

	p, steps, err := shared.Watch(context.Background(), opts)
	if err != nil || p.Status != "paused" || len(*slept) != 0 {
		t.Fatalf("p %+v err %v slept %v", p, err, *slept)
	}
	if !strings.HasSuffix(out.String(), "#42 paused (waiting for manual step \"Deploy\")\n") || shared.PausedStep(steps) != "Deploy" {
		t.Errorf("out = %q", out.String())
	}
	if err := shared.ExitStatus(p); err != nil {
		t.Errorf("a paused pipeline is not a failure: %v", err)
	}
}

func TestWatch_TTYRedraws(t *testing.T) {
	reg := httpmock.New(t)
	poll(reg, ptest.StateRunning, ptest.Step("{s1}", "Build", ptest.StepInProgress))
	poll(reg, ptest.StateSuccessful, ptest.Step("{s1}", "Build", ptest.StateSuccessful))
	opts, out, _ := watchOptions(reg, true)

	p, _, err := shared.Watch(context.Background(), opts)
	if err != nil || p.Status != "successful" {
		t.Fatalf("p %+v err %v", p, err)
	}
	s := out.String()
	if strings.Count(s, "\x1b[H\x1b[2J") != 2 || strings.Count(s, "Refreshing every 5s; press Ctrl-C to stop.") != 1 ||
		!strings.Contains(s, "Pipeline #42 ✓ successful") {
		t.Errorf("out = %q", s)
	}
	if err := shared.ExitStatus(p); err != nil {
		t.Errorf("ExitStatus = %v", err)
	}
}

func TestWatch_RetriesTransientErrors(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(503, `{"type":"error","error":{"message":"Service unavailable"}}`))
	poll(reg, ptest.StateSuccessful, ptest.Step("{s1}", "Build", ptest.StateSuccessful))
	opts, _, slept := watchOptions(reg, false)

	if p, _, err := shared.Watch(context.Background(), opts); err != nil || p.Status != "successful" || len(*slept) != 1 {
		t.Errorf("p %+v err %v slept %v", p, err, *slept)
	}
}

func TestWatch_GivesUpAfterFiveTransientErrors(t *testing.T) {
	reg := httpmock.New(t)
	for range 5 {
		reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(502, `{"type":"error","error":{"message":"Bad gateway"}}`))
	}
	opts, _, slept := watchOptions(reg, false)

	_, _, err := shared.Watch(context.Background(), opts)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 502 || len(*slept) != 4 {
		t.Errorf("err %v slept %v", err, *slept)
	}
}

func TestWatch_StopsOnNotFound(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(404, `{"type":"error","error":{"message":"Not found","detail":"Pipeline with build number '42' not found"}}`))
	opts, _, slept := watchOptions(reg, false)

	_, _, err := shared.Watch(context.Background(), opts)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 || len(*slept) != 0 {
		t.Errorf("err %v slept %v", err, *slept)
	}
}
