package gitctx_test

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/gitctx"
)

func fakeResolver(remotes string, branch string, branchErr error) *gitctx.Resolver {
	return &gitctx.Resolver{
		Git: func(args ...string) (string, error) {
			switch args[0] {
			case "remote":
				return remotes, nil
			case "symbolic-ref":
				return branch, branchErr
			}
			return "", errors.New("unexpected git call: " + strings.Join(args, " "))
		},
	}
}

func TestBaseRepo_PrefersOrigin(t *testing.T) {
	r := fakeResolver(strings.Join([]string{
		"upstream\tgit@bitbucket.org:acme/upstream.git (fetch)",
		"upstream\tgit@bitbucket.org:acme/upstream.git (push)",
		"origin\thttps://dev@bitbucket.org/acme/widgets.git (fetch)",
		"origin\thttps://dev@bitbucket.org/acme/widgets.git (push)",
	}, "\n"), "", nil)
	repo, err := r.BaseRepo()
	if err != nil || repo.FullName() != "acme/widgets" {
		t.Errorf("BaseRepo = %v, %v", repo, err)
	}
}

func TestBaseRepo_SkipsNonBitbucketRemotes(t *testing.T) {
	r := fakeResolver("origin\tgit@github.com:acme/mirror.git (fetch)\nbb\tgit@bitbucket.org:acme/widgets.git (fetch)", "", nil)
	repo, err := r.BaseRepo()
	if err != nil || repo.FullName() != "acme/widgets" {
		t.Errorf("BaseRepo = %v, %v", repo, err)
	}
}

func TestBaseRepo_ResolvesSSHAlias(t *testing.T) {
	r := fakeResolver("origin\tgit@bb-work:acme/widgets.git (fetch)", "", nil)
	var asked []string
	r.SSHHostname = func(alias string) (string, error) {
		asked = append(asked, alias)
		return "bitbucket.org", nil
	}
	repo, err := r.BaseRepo()
	if err != nil || repo.FullName() != "acme/widgets" {
		t.Errorf("BaseRepo = %v, %v", repo, err)
	}
	if len(asked) != 1 || asked[0] != "bb-work" {
		t.Errorf("SSHHostname called with %v", asked)
	}
}

func TestBaseRepo_NoBitbucketRemote(t *testing.T) {
	r := fakeResolver("origin\tgit@github.com:acme/widgets.git (fetch)", "", nil)
	if _, err := r.BaseRepo(); !errors.Is(err, gitctx.ErrNoRepo) {
		t.Errorf("expected ErrNoRepo, got %v", err)
	}
}

func TestBaseRepo_NotAGitRepository(t *testing.T) {
	r := &gitctx.Resolver{Git: func(...string) (string, error) { return "", errors.New("not a git repository") }}
	_, err := r.BaseRepo()
	if !errors.Is(err, gitctx.ErrNoRepo) {
		t.Errorf("expected ErrNoRepo, got %v", err)
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("expected error message to contain 'not a git repository', got %v", err)
	}
}

func TestCurrentBranch(t *testing.T) {
	r := fakeResolver("", "feature/widgets", nil)
	if b, err := r.CurrentBranch(); err != nil || b != "feature/widgets" {
		t.Errorf("CurrentBranch = %q, %v", b, err)
	}
	r = fakeResolver("", "", errors.New("fatal: ref HEAD is not a symbolic ref"))
	if _, err := r.CurrentBranch(); err == nil || !strings.Contains(err.Error(), "not on a branch") {
		t.Errorf("expected detached-HEAD error, got %v", err)
	}
}

func TestResolver_RealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"remote", "add", "origin", "git@bitbucket.org:acme/widgets.git"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	r := gitctx.NewResolver(dir)
	if repo, err := r.BaseRepo(); err != nil || repo.FullName() != "acme/widgets" {
		t.Errorf("BaseRepo = %v, %v", repo, err)
	}
	if b, err := r.CurrentBranch(); err != nil || b != "main" {
		t.Errorf("CurrentBranch = %q, %v", b, err)
	}
}
