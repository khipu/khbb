package cmdutil_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
)

func fakeGit(remotes string) *gitctx.Resolver {
	return &gitctx.Resolver{Git: func(args ...string) (string, error) {
		if args[0] == "remote" {
			return remotes, nil
		}
		return "main", nil
	}}
}

func TestBaseRepo_Precedence(t *testing.T) {
	git := fakeGit("origin\tgit@bitbucket.org:acme/from-git.git (fetch)")

	t.Setenv("KHBB_REPO", "acme/from-env")
	f := &cmdutil.Factory{RepoOverride: "acme/from-flag", Git: git}
	if r, err := f.BaseRepo(); err != nil || r.FullName() != "acme/from-flag" {
		t.Errorf("flag: %v, %v", r, err)
	}
	f.RepoOverride = ""
	if r, err := f.BaseRepo(); err != nil || r.FullName() != "acme/from-env" {
		t.Errorf("env: %v, %v", r, err)
	}
	t.Setenv("KHBB_REPO", "")
	if r, err := f.BaseRepo(); err != nil || r.FullName() != "acme/from-git" {
		t.Errorf("git: %v, %v", r, err)
	}
}

func TestBaseRepo_InvalidFlagIsUsageError(t *testing.T) {
	t.Setenv("KHBB_REPO", "")
	f := &cmdutil.Factory{RepoOverride: "widgets"}
	_, err := f.BaseRepo()
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "WORKSPACE/REPO") {
		t.Errorf("err = %v", err)
	}
}

func TestBaseRepo_NoGit(t *testing.T) {
	t.Setenv("KHBB_REPO", "")
	if _, err := (&cmdutil.Factory{}).BaseRepo(); !errors.Is(err, gitctx.ErrNoRepo) {
		t.Errorf("err = %v", err)
	}
}

func TestSharedFlagsBindToFactory(t *testing.T) {
	f := &cmdutil.Factory{}
	var yes bool
	cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	cmdutil.EnableRepoOverride(cmd, f)
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddYesFlag(cmd, &yes)
	cmd.SetArgs([]string{"-R", "acme/widgets", "--dry-run", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if f.RepoOverride != "acme/widgets" || !f.DryRun || !yes {
		t.Errorf("RepoOverride=%q DryRun=%v yes=%v", f.RepoOverride, f.DryRun, yes)
	}
}

func TestBaseRepo_InvalidEnvNamesVariable(t *testing.T) {
	t.Setenv("KHBB_REPO", "widgets")
	_, err := (&cmdutil.Factory{}).BaseRepo()
	if err == nil || !strings.Contains(err.Error(), "KHBB_REPO") || !strings.Contains(err.Error(), "WORKSPACE/REPO") {
		t.Fatalf("err = %v", err)
	}
	var flagErr *cmdutil.FlagError
	if errors.As(err, &flagErr) {
		t.Error("a malformed KHBB_REPO is an environment error, not a usage error")
	}
}
