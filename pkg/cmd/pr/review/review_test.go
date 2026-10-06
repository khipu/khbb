package review

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

type newCmdFunc func(*cmdutil.Factory, func(*ReviewOptions) error) *cobra.Command

func run(reg *httpmock.Registry, newCmd newCmdFunc, args ...string) (string, string, error) {
	f, _, out, errOut := prtest.NewFactory(reg)
	err := prtest.Run(newCmd(f, nil), args...)
	return out.String(), errOut.String(), err
}

func TestReviewCommands(t *testing.T) {
	cases := []struct {
		name   string
		newCmd newCmdFunc
		method string
		path   string
		want   string
	}{
		{"approve", NewCmdApprove, "POST", "/42/approve", "Approved pull request #42 (feature/widgets → main)\n"},
		{"unapprove", NewCmdUnapprove, "DELETE", "/42/approve", "Removed your approval from pull request #42 (feature/widgets → main)\n"},
		{"request-changes", NewCmdRequestChanges, "POST", "/42/request-changes", "Requested changes on pull request #42 (feature/widgets → main)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := httpmock.New(t)
			reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
			reg.Register(tc.method, prtest.PRs+tc.path, httpmock.JSONResponse(200, `{"approved":true}`))
			out, errOut, err := run(reg, tc.newCmd, "42")
			if err != nil || out != "" || errOut != tc.want {
				t.Errorf("out %q stderr %q err %v", out, errOut, err)
			}
		})
	}
}

func TestReview_RefusesClosedPullRequest(t *testing.T) {
	for _, state := range []string{"MERGED", "DECLINED"} {
		reg := httpmock.New(t)
		reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.WithState(prtest.PR42, state)))
		_, _, err := run(reg, NewCmdApprove, "42")
		var conflict *cmdutil.ConflictError
		if !errors.As(err, &conflict) || !strings.Contains(err.Error(), "is "+strings.ToLower(state)) {
			t.Errorf("%s: err = %v", state, err)
		}
		if w := prtest.Writes(reg); len(w) != 0 {
			t.Errorf("%s: sent %d writes", state, len(w))
		}
	}
}

func TestReview_DryRun(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	out, errOut, err := run(reg, NewCmdRequestChanges, "42", "--dry-run")
	if !errors.Is(err, bitbucket.ErrDryRun) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, `"method": "POST"`) || !strings.Contains(out, "/pullrequests/42/request-changes") || errOut != "" {
		t.Errorf("out %q stderr %q", out, errOut)
	}
}

func TestReview_TooManyArguments(t *testing.T) {
	_, _, err := run(httpmock.New(t), NewCmdApprove, "1", "2")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) {
		t.Errorf("err = %v", err)
	}
}
