package decline

import (
	"errors"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const question = "Decline #42 (feature/widgets → main)? Declined pull requests cannot be reopened."

func newReg(t *testing.T, pr string) *httpmock.Registry {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, pr))
	return reg
}

func declined() httpmock.Responder {
	return httpmock.JSONResponse(200, prtest.WithState(prtest.PR42, "DECLINED"))
}

func TestDecline_WithMessage(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	reg.Register("POST", prtest.PRs+"/42/decline", declined())
	f, _, out, errOut := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdDecline(f, nil), "42", "-m", "Superseded by #43", "--yes"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[1], `{"message":"Superseded by #43"}`)
	if out.String() != "" || errOut.String() != "Declined pull request #42 (feature/widgets → main)\n" {
		t.Errorf("out %q stderr %q", out.String(), errOut.String())
	}
}

func TestDecline_WithoutMessageAndJSON(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	reg.Register("POST", prtest.PRs+"/42/decline", declined())
	f, _, out, _ := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdDecline(f, nil), "42", "--yes", "--json", "state"); err != nil {
		t.Fatal(err)
	}
	if len(reg.Calls[1].Body) != 0 || out.String() != `{"state":"DECLINED"}`+"\n" {
		t.Errorf("body %q out %q", reg.Calls[1].Body, out.String())
	}
}

func TestDecline_NeedsConfirmationWithoutATerminal(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	f, _, _, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdDecline(f, nil), "42")
	if !errors.Is(err, cmdutil.ErrConfirmationRequired) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestDecline_PromptOnATerminal(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	f, ios, _, _ := prtest.NewFactory(reg)
	prtest.SetTTY(ios)
	pm := prompter.NewMock(t)
	pm.RegisterConfirm(question, func(_ string, def bool) (bool, error) { return false, nil })
	f.Prompter = pm

	if err := prtest.Run(NewCmdDecline(f, nil), "42"); !errors.Is(err, cmdutil.ErrCancel) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestDecline_RefusesClosedPullRequest(t *testing.T) {
	reg := newReg(t, prtest.WithState(prtest.PR42, "MERGED"))
	f, _, _, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdDecline(f, nil), "42", "--yes")
	var conflict *cmdutil.ConflictError
	if !errors.As(err, &conflict) || !strings.Contains(err.Error(), "only open pull requests can be declined") {
		t.Errorf("err = %v", err)
	}
}

func TestDecline_DryRun(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	f, _, out, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdDecline(f, nil), "42", "--dry-run", "-m", "Not needed")
	if !errors.Is(err, bitbucket.ErrDryRun) || !strings.Contains(out.String(), "/pullrequests/42/decline") ||
		!strings.Contains(out.String(), `"message": "Not needed"`) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err %v out %q", err, out.String())
	}
}
