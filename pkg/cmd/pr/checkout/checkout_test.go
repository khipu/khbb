package checkout

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const remotes = "origin\tgit@bitbucket.org:acme/widgets.git (fetch)\norigin\tgit@bitbucket.org:acme/widgets.git (push)"

// forkPR42 is PR 42 opened from the fork dev/widgets-fork.
var forkPR42 = strings.Replace(prtest.PR42, `"repository":{"full_name":"acme/widgets"}`, `"repository":{"full_name":"dev/widgets-fork"}`, 1)

func setup(t *testing.T, pr string, git *prtest.FakeGit) (*cmdutil.Factory, func() string) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, pr))
	f, _, _, errOut := prtest.NewFactory(reg)
	prtest.SetGit(f, git)
	return f, errOut.String
}

func TestCheckout_NewBranchFromOrigin(t *testing.T) {
	git := &prtest.FakeGit{
		Outputs: map[string]string{
			"remote -v": remotes,
			"fetch origin +refs/heads/feature/widgets:refs/remotes/origin/feature/widgets": "",
			"checkout -b feature/widgets --track origin/feature/widgets":                   "",
		},
		Errors: map[string]string{"rev-parse --verify --quiet refs/heads/feature/widgets": "exit status 1"},
	}
	f, stderr := setup(t, prtest.PR42, git)

	if err := prtest.Run(NewCmdCheckout(f, nil), "42"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"remote -v",
		"rev-parse --verify --quiet refs/heads/feature/widgets",
		"fetch origin +refs/heads/feature/widgets:refs/remotes/origin/feature/widgets",
		"checkout -b feature/widgets --track origin/feature/widgets",
	}
	if !slices.Equal(git.Calls, want) {
		t.Errorf("git calls:\n%s", strings.Join(git.Calls, "\n"))
	}
	if stderr() != "Checked out pull request #42 (feature/widgets → main) on branch feature/widgets\n" {
		t.Errorf("stderr = %q", stderr())
	}
}

func TestCheckout_ExistingBranchFastForwards(t *testing.T) {
	git := &prtest.FakeGit{Outputs: map[string]string{
		"remote -v": remotes,
		"rev-parse --verify --quiet refs/heads/feature/widgets":                        "abc1234",
		"fetch origin +refs/heads/feature/widgets:refs/remotes/origin/feature/widgets": "",
		"checkout feature/widgets":                            "",
		"merge --ff-only refs/remotes/origin/feature/widgets": "",
	}}
	f, _ := setup(t, prtest.PR42, git)

	if err := prtest.Run(NewCmdCheckout(f, nil), "42"); err != nil {
		t.Fatal(err)
	}
	if last := git.Calls[len(git.Calls)-1]; last != "merge --ff-only refs/remotes/origin/feature/widgets" {
		t.Errorf("last git call = %q", last)
	}
}

func TestCheckout_ForceResetsANamedBranch(t *testing.T) {
	git := &prtest.FakeGit{Outputs: map[string]string{
		"remote -v": remotes,
		"rev-parse --verify --quiet refs/heads/review-42":                              "abc1234",
		"fetch origin +refs/heads/feature/widgets:refs/remotes/origin/feature/widgets": "",
		"checkout review-42": "",
		"reset --hard refs/remotes/origin/feature/widgets": "",
	}}
	f, _ := setup(t, prtest.PR42, git)

	if err := prtest.Run(NewCmdCheckout(f, nil), "42", "-b", "review-42", "--force"); err != nil {
		t.Fatal(err)
	}
	if last := git.Calls[len(git.Calls)-1]; last != "reset --hard refs/remotes/origin/feature/widgets" {
		t.Errorf("last git call = %q", last)
	}
}

func TestCheckout_DivergedBranchSuggestsForce(t *testing.T) {
	git := &prtest.FakeGit{
		Outputs: map[string]string{
			"remote -v": remotes,
			"rev-parse --verify --quiet refs/heads/feature/widgets":                        "abc1234",
			"fetch origin +refs/heads/feature/widgets:refs/remotes/origin/feature/widgets": "",
			"checkout feature/widgets": "",
		},
		Errors: map[string]string{"merge --ff-only refs/remotes/origin/feature/widgets": "git merge: fatal: Not possible to fast-forward, aborting."},
	}
	f, _ := setup(t, prtest.PR42, git)

	err := prtest.Run(NewCmdCheckout(f, nil), "42")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("err = %v", err)
	}
}

func TestCheckout_ForkFetchesByURL(t *testing.T) {
	url := "git@bitbucket.org:dev/widgets-fork.git"
	git := &prtest.FakeGit{
		Outputs: map[string]string{
			"remote -v": remotes,
			"fetch " + url + " refs/heads/feature/widgets":                   "",
			"checkout -b feature/widgets FETCH_HEAD":                         "",
			"config branch.feature/widgets.remote " + url:                    "",
			"config branch.feature/widgets.merge refs/heads/feature/widgets": "",
		},
		Errors: map[string]string{"rev-parse --verify --quiet refs/heads/feature/widgets": "exit status 1"},
	}
	f, _ := setup(t, forkPR42, git)
	f.Config = func() (*config.Config, error) { return &config.Config{GitProtocol: "ssh"}, nil }

	if err := prtest.Run(NewCmdCheckout(f, nil), "42"); err != nil {
		t.Fatal(err)
	}
	if n := len(git.Calls); n != 6 || git.Calls[n-1] != "config branch.feature/widgets.merge refs/heads/feature/widgets" {
		t.Errorf("git calls:\n%s", strings.Join(git.Calls, "\n"))
	}
}

func TestCheckout_ForkUsesHTTPSByDefault(t *testing.T) {
	url := "https://bitbucket.org/dev/widgets-fork.git"
	git := &prtest.FakeGit{Outputs: map[string]string{
		"remote -v": remotes,
		"rev-parse --verify --quiet refs/heads/feature/widgets": "abc1234",
		"fetch " + url + " refs/heads/feature/widgets":          "",
		"checkout feature/widgets":                              "",
		"merge --ff-only FETCH_HEAD":                            "",
	}}
	f, _ := setup(t, forkPR42, git)

	if err := prtest.Run(NewCmdCheckout(f, nil), "42"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckout_NeedsOneArgument(t *testing.T) {
	f, _, _, _ := prtest.NewFactory(httpmock.New(t))
	for _, args := range [][]string{{}, {"1", "2"}} {
		err := prtest.Run(NewCmdCheckout(f, nil), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

func TestCheckout_RefusesBranchNamesStartingWithADash(t *testing.T) {
	// Case 1: PR whose source branch is "-x"
	dashPR42 := strings.Replace(prtest.PR42, `"branch":{"name":"feature/widgets"}`, `"branch":{"name":"-x"}`, 1)
	git := &prtest.FakeGit{}
	f, _ := setup(t, dashPR42, git)

	err := prtest.Run(NewCmdCheckout(f, nil), "42")
	if err == nil || !strings.Contains(err.Error(), "refusing to check out branch") {
		t.Errorf("Case 1 (PR source branch is -x): err = %v", err)
	}
	if len(git.Calls) != 0 {
		t.Errorf("Case 1: expected no git calls, got %d: %v", len(git.Calls), git.Calls)
	}

	// Case 2: PR42 with -b -x
	git2 := &prtest.FakeGit{}
	f2, _ := setup(t, prtest.PR42, git2)

	err = prtest.Run(NewCmdCheckout(f2, nil), "42", "-b", "-x")
	if err == nil || !strings.Contains(err.Error(), "refusing to check out branch") {
		t.Errorf("Case 2 (user-supplied branch -x): err = %v", err)
	}
	if len(git2.Calls) != 0 {
		t.Errorf("Case 2: expected no git calls, got %d: %v", len(git2.Calls), git2.Calls)
	}
}
