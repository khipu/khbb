package rerun

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const note = "note: Bitbucket does not return the variables of pipeline #45; if it needs any, pass them with --var or --secret-var\n"

func run(t *testing.T, reg *httpmock.Registry, args ...string) (string, string, error) {
	t.Helper()
	f, _, out, errOut := prtest.NewFactory(reg)
	cmd := NewCmdRerun(f, func(o *RerunOptions) error {
		o.Sleep = func(time.Duration) {}
		return rerunRun(context.Background(), o)
	})
	err := prtest.Run(cmd, args...)
	return out.String(), errOut.String(), err
}

func original(reg *httpmock.Registry, n string, pipeline string) {
	reg.Register("GET", ptest.Pipelines+"/"+n, httpmock.JSONResponse(200, pipeline))
	reg.Register("POST", ptest.Pipelines, httpmock.JSONResponse(201, ptest.Pipeline(43, ptest.StatePending)))
}

func TestRerun_BranchPipeline(t *testing.T) {
	reg := httpmock.New(t)
	original(reg, "42", ptest.Pipeline(42, ptest.StateFailed))
	out, errOut, err := run(t, reg, "42")
	if err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[1], `{"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main",`+
		`"commit":{"type":"commit","hash":"`+ptest.Commit+`"},"selector":{"type":"branches","pattern":"main"}}}`)
	if out != "https://bitbucket.org/acme/widgets/pipelines/results/43\n" || errOut != "Started pipeline #43 (a rerun of #42, branch main)\n" {
		t.Errorf("out %q stderr %q", out, errOut)
	}
}

func TestRerun_PullRequestPipeline(t *testing.T) {
	reg := httpmock.New(t)
	original(reg, "44", ptest.PullRequestPipeline(44, ptest.StateFailed))
	if _, _, err := run(t, reg, "44"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[1], `{"target":{"type":"pipeline_pullrequest_target","source":"feature/widgets","destination":"main",`+
		`"destination_commit":{"hash":"def5678"},"commit":{"type":"commit","hash":"`+ptest.Commit+`"},"pullrequest":{"id":7},`+
		`"selector":{"type":"pull-requests","pattern":"**"}}}`)
}

func TestRerun_CustomPipelineVariables(t *testing.T) {
	reg := httpmock.New(t)
	original(reg, "45", ptest.CustomPipeline(45, ptest.StateSuccessful, "deploy"))
	if _, errOut, err := run(t, reg, "45"); err != nil || !strings.HasPrefix(errOut, note) {
		t.Errorf("stderr %q err %v", errOut, err)
	}

	reg = httpmock.New(t)
	original(reg, "45", ptest.CustomPipeline(45, ptest.StateSuccessful, "deploy"))
	_, errOut, err := run(t, reg, "45", "--var", "ENV=prod", "--secret-var", "TOKEN=s3cret")
	if err != nil || strings.Contains(errOut, "note:") {
		t.Errorf("stderr %q err %v", errOut, err)
	}
	body := prtest.JSONBodyOf(t, reg.Calls[1])
	if vars, ok := body["variables"].([]any); !ok || len(vars) != 2 {
		t.Errorf("variables = %v", body["variables"])
	}
}

func TestRerun_Watch(t *testing.T) {
	reg := httpmock.New(t)
	original(reg, "42", ptest.Pipeline(42, ptest.StateFailed))
	reg.Register("GET", ptest.Pipelines+"/43", httpmock.JSONResponse(200, ptest.Pipeline(43, ptest.StateSuccessful)))
	reg.Register("GET", ptest.Pipelines+"/43/steps", httpmock.JSONResponse(200, ptest.Steps(ptest.Step("{s1}", "Build", ptest.StateSuccessful))))

	out, _, err := run(t, reg, "42", "--watch")
	if err != nil || !strings.HasSuffix(out, "#43 successful (1m02s)\n") {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestRerun_DryRun(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, ptest.StateFailed)))
	out, _, err := run(t, reg, "42", "--dry-run")
	if !errors.Is(err, bitbucket.ErrDryRun) || !strings.Contains(out, `"method": "POST"`) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err %v out %q", err, out)
	}
}

func TestRerun_FlagErrors(t *testing.T) {
	for _, args := range [][]string{{"42", "--watch", "--json", "number"}, {"42", "--var", "bad"}, {"abc"}, {"1", "2"}} {
		_, _, err := run(t, httpmock.New(t), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
