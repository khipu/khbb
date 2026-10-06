package stop

import (
	"errors"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func pipeline42(t *testing.T, state string) *httpmock.Registry {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, state)))
	return reg
}

func TestStop_Running(t *testing.T) {
	reg := pipeline42(t, ptest.StateRunning)
	reg.Register("POST", ptest.Pipelines+"/42/stopPipeline", httpmock.StringResponse(204, ""))
	f, _, out, errOut := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdStop(f, nil), "42", "--yes"); err != nil {
		t.Fatal(err)
	}
	if out.String() != "" || errOut.String() != "Stopped pipeline #42 (branch main)\n" {
		t.Errorf("out %q stderr %q", out.String(), errOut.String())
	}
}

func TestStop_RefusesFinishedPipeline(t *testing.T) {
	reg := pipeline42(t, ptest.StateFailed)
	f, _, _, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdStop(f, nil), "42", "--yes")
	var conflict *cmdutil.ConflictError
	if !errors.As(err, &conflict) || err.Error() != "pipeline #42 already finished (failed)" || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestStop_RefusesPausedPipeline(t *testing.T) {
	reg := pipeline42(t, ptest.StatePaused)
	f, _, _, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdStop(f, nil), "42", "--yes")
	var conflict *cmdutil.ConflictError
	if !errors.As(err, &conflict) || !strings.Contains(err.Error(), "paused on a manual step") ||
		!strings.Contains(err.Error(), "https://bitbucket.org/acme/widgets/pipelines/results/42") || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestStop_NeedsConfirmationWithoutATerminal(t *testing.T) {
	reg := pipeline42(t, ptest.StateRunning)
	f, _, _, _ := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdStop(f, nil), "42"); !errors.Is(err, cmdutil.ErrConfirmationRequired) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestStop_PromptsOnATerminal(t *testing.T) {
	reg := pipeline42(t, ptest.StateRunning)
	f, ios, _, _ := prtest.NewFactory(reg)
	prtest.SetTTY(ios)
	pm := prompter.NewMock(t)
	pm.RegisterConfirm("Stop pipeline #42 (branch main)?", func(_ string, def bool) (bool, error) {
		if def {
			t.Error("the default answer must be No")
		}
		return false, nil
	})
	f.Prompter = pm

	if err := prtest.Run(NewCmdStop(f, nil), "42"); !errors.Is(err, cmdutil.ErrCancel) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestStop_DryRun(t *testing.T) {
	reg := pipeline42(t, ptest.StateRunning)
	f, _, out, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdStop(f, nil), "42", "--dry-run")
	if !errors.Is(err, bitbucket.ErrDryRun) || !strings.Contains(out.String(), "/pipelines/42/stopPipeline") || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err %v out %q", err, out.String())
	}
}
