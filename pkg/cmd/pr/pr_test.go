package pr_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

// -R without a pull request argument must not fall back to the local branch's pull request in
// the other repository: gh refuses this, and so do view, diff and checks.
func TestRepoFlagWithoutArgument_IsUsageError(t *testing.T) {
	for _, sub := range []string{"view", "diff", "checks"} {
		t.Run(sub, func(t *testing.T) {
			reg := httpmock.New(t)
			f, _, _, _ := prtest.NewFactory(reg)
			cmd := pr.NewCmdPR(f)
			err := prtest.Run(cmd, sub, "-R", "acme/other")
			var flagErr *cmdutil.FlagError
			if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "argument required when using the --repo flag") {
				t.Errorf("%s: err = %v, want FlagError mentioning the --repo flag", sub, err)
			}
			if len(reg.Calls) != 0 {
				t.Errorf("%s: unexpected HTTP calls: %v", sub, reg.Calls)
			}
		})
	}
}

// With a pull request argument, -R is fine: it names the repository to look the number up in.
func TestRepoFlagWithArgument_IsAllowed(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/repositories/acme/other/pullrequests/7", httpmock.JSONResponse(200, prtest.PR7))
	f, _, _, _ := prtest.NewFactory(reg)
	cmd := pr.NewCmdPR(f)
	if err := prtest.Run(cmd, "view", "7", "-R", "acme/other"); err != nil {
		t.Errorf("err = %v", err)
	}
}

func TestStatus_RepoFlagSkipsCurrentBranchQuery(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page())) // createdByMe
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page())) // needsMyReview
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")
	cmd := pr.NewCmdPR(f)
	if err := prtest.Run(cmd, "status", "-R", "acme/widgets"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "  Not shown when --repo is set") {
		t.Errorf("out = %q", out.String())
	}
	if len(reg.Calls) != 3 {
		t.Errorf("calls = %d, want 3 (user, createdByMe, needsMyReview); got %v", len(reg.Calls), reg.Calls)
	}
}

func TestStatus_RepoFlagJSONCurrentBranchStaysNull(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page()))
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page()))
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")
	cmd := pr.NewCmdPR(f)
	if err := prtest.Run(cmd, "status", "-R", "acme/widgets", "--json", "currentBranch,createdByMe,needsMyReview"); err != nil {
		t.Fatal(err)
	}
	if want := `{"createdByMe":[],"currentBranch":null,"needsMyReview":[]}` + "\n"; out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
}
