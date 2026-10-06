package run

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

const url43 = "https://bitbucket.org/acme/widgets/pipelines/results/43"

// started is a freshly created pipeline: Bitbucket fills in the selector only after parsing.
var started = strings.Replace(ptest.Pipeline(43, ptest.StatePending), `"selector":{"type":"branches","pattern":"main"}`, `"selector":null`, 1)

func run(t *testing.T, reg *httpmock.Registry, args ...string) (string, string, error) {
	t.Helper()
	f, _, out, errOut := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")
	cmd := NewCmdRun(f, func(o *RunOptions) error {
		o.Sleep = func(time.Duration) {}
		return runRun(context.Background(), o)
	})
	err := prtest.Run(cmd, args...)
	return out.String(), errOut.String(), err
}

func created(reg *httpmock.Registry) {
	reg.Register("POST", ptest.Pipelines, httpmock.JSONResponse(201, started))
}

func TestRun_CurrentBranch(t *testing.T) {
	reg := httpmock.New(t)
	created(reg)
	out, errOut, err := run(t, reg)
	if err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[0], `{"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"feature/widgets"}}`)
	if out != url43+"\n" || errOut != "Started pipeline #43 (branch main)\n" {
		t.Errorf("out %q stderr %q", out, errOut)
	}
}

func TestRun_CustomWithVariables(t *testing.T) {
	reg := httpmock.New(t)
	created(reg)
	_, errOut, err := run(t, reg, "-b", "main", "--custom", "deploy", "--var", "ENV=staging", "--secret-var", "TOKEN=s3cret")
	if err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[0], `{"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main",`+
		`"selector":{"type":"custom","pattern":"deploy"}},"variables":[{"key":"ENV","value":"staging"},{"key":"TOKEN","value":"s3cret","secured":true}]}`)
	if errOut != "Started pipeline #43 (custom pipeline deploy on branch main)\n" {
		t.Errorf("stderr %q", errOut)
	}
}

func TestRun_CommitAndTag(t *testing.T) {
	reg := httpmock.New(t)
	created(reg)
	created(reg)
	if _, _, err := run(t, reg, "--commit", "abc1234"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[0], `{"target":{"type":"pipeline_commit_target","commit":{"type":"commit","hash":"abc1234"},"selector":{"type":"default"}}}`)
	if _, _, err := run(t, reg, "--tag", "v1.2.0"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[1], `{"target":{"type":"pipeline_ref_target","ref_type":"tag","ref_name":"v1.2.0"}}`)
}

func TestRun_DryRunMasksSecrets(t *testing.T) {
	reg := httpmock.New(t)
	out, _, err := run(t, reg, "--dry-run", "-b", "main", "--secret-var", "TOKEN=s3cret")
	if !errors.Is(err, bitbucket.ErrDryRun) || !strings.Contains(out, `"value": "****"`) || strings.Contains(out, "s3cret") || len(reg.Calls) != 0 {
		t.Errorf("err %v out %q", err, out)
	}
}

func TestRun_JSON(t *testing.T) {
	reg := httpmock.New(t)
	created(reg)
	out, _, err := run(t, reg, "--json", "number,status")
	if err != nil || out != `{"number":43,"status":"pending"}`+"\n" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestRun_Watch(t *testing.T) {
	reg := httpmock.New(t)
	created(reg)
	reg.Register("GET", ptest.Pipelines+"/43", httpmock.JSONResponse(200, ptest.Pipeline(43, ptest.StateFailed)))
	reg.Register("GET", ptest.Pipelines+"/43/steps", httpmock.JSONResponse(200, ptest.Steps(ptest.Step("{s1}", "Build", ptest.StateFailed))))

	out, _, err := run(t, reg, "-b", "main", "--watch")
	var exitErr *cmdutil.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Errorf("err = %v", err)
	}
	if out != url43+"\n#43 step \"Build\": failed (12s)\n#43 failed (1m02s)\n" {
		t.Errorf("out %q", out)
	}
}

func TestRun_UnknownCustomPipeline(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("POST", ptest.Pipelines, httpmock.JSONResponse(400, `{"type":"error","error":{"message":"Bad request",`+
		`"detail":"Requested selector is not found in bitbucket-pipelines.yml.","data":{"key":"result-service.pipeline.selector-not-found"}}}`))
	_, _, err := run(t, reg, "--custom", "nope")
	if err == nil || err.Error() != "Requested selector is not found in bitbucket-pipelines.yml. (HTTP 400)" {
		t.Errorf("err = %v", err)
	}
}

func TestRun_FlagErrors(t *testing.T) {
	for _, args := range [][]string{
		{"-b", "main", "--tag", "v1"},
		{"--commit", "abc", "--tag", "v1"},
		{"--watch", "--json", "number"},
		{"--var", "novalue"},
		{"--secret-var", "novalue"},
		{"extra"},
	} {
		_, _, err := run(t, httpmock.New(t), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

func TestRun_RepoFlagNeedsATarget(t *testing.T) {
	f, _, _, _ := prtest.NewFactory(httpmock.New(t))
	cmd := NewCmdRun(f, nil)
	cmdutil.EnableRepoOverride(cmd, f) // the pipeline group normally adds --repo
	err := prtest.Run(cmd, "-R", "acme/widgets")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "--branch, --commit or --tag") {
		t.Errorf("err = %v", err)
	}
}
