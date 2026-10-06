package logs

import (
	"errors"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func pipelineWithSteps(reg *httpmock.Registry, steps ...string) {
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, ptest.StateFailed)))
	reg.Register("GET", ptest.Pipelines+"/42/steps", httpmock.JSONResponse(200, ptest.Steps(steps...)))
}

func stepLog(reg *httpmock.Registry, uuid, text string) {
	reg.Register("GET", ptest.Pipelines+"/42/steps/"+uuid+"/log", httpmock.StringResponse(200, text))
}

func run(reg *httpmock.Registry, args ...string) (string, string, error) {
	f, _, out, errOut := prtest.NewFactory(reg)
	err := prtest.Run(NewCmdLogs(f, nil), append([]string{"42"}, args...)...)
	return out.String(), errOut.String(), err
}

func TestLogs_AllStepsWithHeaders(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed),
		ptest.Step("{s3}", "Deploy", ptest.StepNotRun))
	stepLog(reg, "{s1}", "build ok\n")
	stepLog(reg, "{s2}", "test failed")

	out, errOut, err := run(reg)
	if err != nil {
		t.Fatal(err)
	}
	if out != "==> Build <==\nbuild ok\n==> Test <==\ntest failed\n" || errOut != "no log for step \"Deploy\" (skipped)\n" {
		t.Errorf("out %q stderr %q", out, errOut)
	}
}

func TestLogs_SkipsStepsWithoutALog(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed),
		ptest.Step("{s3}", "Gate", ptest.StepWaiting))
	reg.Register("GET", ptest.Pipelines+"/42/steps/{s1}/log", httpmock.JSONResponse(404,
		`{"error":{"message":"Not Found","detail":"Log in step {s1} does not exist."}}`))
	stepLog(reg, "{s2}", "test failed\n")

	out, errOut, err := run(reg)
	if err != nil || out != "==> Test <==\ntest failed\n" || errOut != "no log for step \"Build\"\nno log for step \"Gate\" (paused)\n" {
		t.Errorf("out %q stderr %q err %v", out, errOut, err)
	}
}

func TestLogs_FailedOnly(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed))
	stepLog(reg, "{s2}", "test failed\n")

	out, _, err := run(reg, "--failed")
	if err != nil || out != "test failed\n" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestLogs_NoSteps(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg)

	out, errOut, err := run(reg)
	if err != nil || out != "" || errOut != "pipeline #42 has no steps\n" {
		t.Errorf("out %q stderr %q err %v", out, errOut, err)
	}
}

func TestLogs_NoFailedSteps(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful))

	out, errOut, err := run(reg, "--failed")
	if err != nil || out != "" || errOut != "no failed steps in pipeline #42\n" {
		t.Errorf("out %q stderr %q err %v", out, errOut, err)
	}
}

func TestLogs_StepByNameWithTail(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Unit tests", ptest.StateFailed))
	stepLog(reg, "{s2}", "a\nb\nc\n")

	out, _, err := run(reg, "--step", "unit TESTS", "--tail", "2")
	if err != nil || out != "b\nc\n" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestLogs_StepByUUID(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed))
	stepLog(reg, "{s1}", "build ok\n")

	if out, _, err := run(reg, "-s", "{s1}"); err != nil || out != "build ok\n" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestLogs_UnknownStep(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed))

	_, _, err := run(reg, "--step", "nope")
	var notFound *cmdutil.NotFoundError
	if !errors.As(err, &notFound) || err.Error() != `pipeline #42 has no step "nope"; its steps are: Build, Test` {
		t.Errorf("err = %v", err)
	}
}

func TestLogs_FlagErrors(t *testing.T) {
	for _, args := range [][]string{{"--step", "Build", "--failed"}, {"--tail", "0"}} {
		_, _, err := run(httpmock.New(t), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
