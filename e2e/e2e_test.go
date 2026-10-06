// Package e2e runs khbb against a real Bitbucket repository. The tests are skipped unless
// KHBB_E2E=1. They push a khbb-probe/e2e-* branch to the sandbox repository, open a pull request,
// comment on and approve it, follow its pipelines, run a custom pipeline, then decline the pull
// request and delete the branch.
//
// Requirements: khbb logged in (keyring or KHBB_EMAIL/KHBB_TOKEN), git push access to the sandbox
// over SSH (or KHBB_E2E_REMOTE), and the sandbox's test bitbucket-pipelines.yml.
package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const defaultRepo = "khipu/khipubb-sandbox"

// khbbBin is the khbb binary built by TestMain.
var khbbBin string

func TestMain(m *testing.M) {
	if os.Getenv("KHBB_E2E") != "1" {
		os.Exit(m.Run()) // the live test skips itself
	}
	dir, err := os.MkdirTemp("", "khbb-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	khbbBin = filepath.Join(dir, "khbb")
	if runtime.GOOS == "windows" {
		khbbBin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", khbbBin, "github.com/khipu/khbb/cmd/khbb").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building khbb: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// checkSandbox refuses a repository or remote that is not a sandbox: the tests write to it. name is
// the environment variable that supplied value, for the error message.
func checkSandbox(name, value string) error {
	if !strings.Contains(strings.ToLower(value), "sandbox") {
		return fmt.Errorf("%s=%q: the end-to-end tests open pull requests and run pipelines, so they only run against a repository whose name contains \"sandbox\"", name, value)
	}
	return nil
}

func TestCheckSandbox(t *testing.T) {
	if err := checkSandbox("KHBB_E2E_REPO", "khipu/khipubb-sandbox"); err != nil {
		t.Error(err)
	}
	if err := checkSandbox("KHBB_E2E_REMOTE", "git@bitbucket.org:khipu/khipubb-sandbox.git"); err != nil {
		t.Error(err)
	}
	err := checkSandbox("KHBB_E2E_REMOTE", "git@bitbucket.org:khipu/payments.git")
	if err == nil || !strings.Contains(err.Error(), "KHBB_E2E_REMOTE=") {
		t.Errorf("a remote that is not a sandbox must be refused, got %v", err)
	}
}

// sandbox skips the test unless KHBB_E2E=1 and returns the sandbox repository.
func sandbox(t *testing.T) string {
	t.Helper()
	if os.Getenv("KHBB_E2E") != "1" {
		t.Skip("set KHBB_E2E=1 to run the end-to-end tests against the sandbox repository")
	}
	repo := os.Getenv("KHBB_E2E_REPO")
	if repo == "" {
		repo = defaultRepo
	}
	if err := checkSandbox("KHBB_E2E_REPO", repo); err != nil {
		t.Fatal(err)
	}
	return repo
}

// runner runs khbb and git in a clone of the sandbox.
type runner struct {
	t   *testing.T
	dir string
}

// env is the environment of every command: no prompts, no color, no git credential prompts, and
// KHBB_REPO cleared so that khbb resolves the repository from the clone's remote.
func env() []string {
	return append(os.Environ(), "KHBB_PROMPT_DISABLED=1", "NO_COLOR=1", "GIT_TERMINAL_PROMPT=0", "KHBB_REPO=")
}

// run executes khbb and returns its trimmed stdout, its stderr and its exit status.
func (r *runner) run(args ...string) (string, string, int) {
	r.t.Helper()
	cmd := exec.Command(khbbBin, args...)
	cmd.Dir, cmd.Env = r.dir, env()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return strings.TrimSpace(stdout.String()), stderr.String(), 0
	case errors.As(err, &exitErr):
		return strings.TrimSpace(stdout.String()), stderr.String(), exitErr.ExitCode()
	}
	r.t.Fatalf("khbb %s: %v", strings.Join(args, " "), err)
	return "", "", 0
}

// out runs khbb, requires exit status 0 and returns stdout.
func (r *runner) out(args ...string) string {
	r.t.Helper()
	stdout, stderr, code := r.run(args...)
	if code != 0 {
		r.t.Fatalf("khbb %s: exit status %d\n%s", strings.Join(args, " "), code, stderr)
	}
	return stdout
}

// equal runs khbb and requires exit status 0 and the given stdout.
func (r *runner) equal(want string, args ...string) {
	r.t.Helper()
	if got := r.out(args...); got != want {
		r.t.Errorf("khbb %s = %q, want %q", strings.Join(args, " "), got, want)
	}
}

// exits runs khbb, requires the given exit status and returns stderr.
func (r *runner) exits(want int, args ...string) string {
	r.t.Helper()
	_, stderr, code := r.run(args...)
	if code != want {
		r.t.Errorf("khbb %s: exit status %d, want %d\n%s", strings.Join(args, " "), code, want, stderr)
	}
	return stderr
}

// git runs git in dir and fails the test on error.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, env()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestPullRequestAndPipelineLifecycle(t *testing.T) {
	repo := sandbox(t)
	remote := os.Getenv("KHBB_E2E_REMOTE")
	if remote == "" {
		remote = "git@bitbucket.org:" + repo + ".git"
	}
	if err := checkSandbox("KHBB_E2E_REMOTE", remote); err != nil {
		t.Fatal(err)
	}
	clone := filepath.Join(t.TempDir(), "sandbox")
	git(t, "", "clone", "-q", "--depth", "1", remote, clone)
	k := &runner{t: t, dir: clone}
	k.out("auth", "status")

	stamp := time.Now().UTC().Format("20060102-150405")
	branch := "khbb-probe/e2e-" + stamp
	git(t, clone, "checkout", "-q", "-b", branch)
	if err := os.MkdirAll(filepath.Join(clone, "probe"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, "probe", "e2e-"+stamp+".txt"), []byte("khbb e2e\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, clone, "add", "probe")
	git(t, clone, "-c", "user.name=khbb-e2e", "-c", "user.email=e2e@example.invalid", "commit", "-q", "-m", "probe: khbb end-to-end test")
	git(t, clone, "push", "-q", "origin", branch)
	t.Cleanup(func() {
		cmd := exec.Command("git", "push", "-q", "origin", "--delete", branch)
		cmd.Dir, cmd.Env = clone, env()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("deleting %s: %v\n%s", branch, err, out)
		}
	})

	// The push starts the branch pipeline: Build, then a 60-second Slow step.
	k.out("pipeline", "watch", "--exit-status")
	k.equal("successful", "pipeline", "view", "--json", "status", "--jq", ".status")
	if logs := k.out("pipeline", "logs", "--step", "Build"); !strings.Contains(logs, "build") {
		t.Errorf("Build step log = %q", logs)
	}

	id := k.out("pr", "create", "--title", "khbb e2e "+stamp, "--body", "Opened by khbb's end-to-end tests; safe to decline.",
		"--no-default-reviewers", "--json", "id", "--jq", ".id")
	declined := false
	t.Cleanup(func() {
		if !declined {
			k.run("pr", "decline", id, "--yes", "--message", "End-to-end test cleanup")
		}
	})
	k.equal(branch, "pr", "view", "--json", "sourceBranch", "--jq", ".sourceBranch")

	commentID := k.out("pr", "comment", id, "--body", "e2e comment", "--json", "id", "--jq", ".id")
	k.out("pr", "comment", id, "--reply-to", commentID, "--body", "e2e reply")
	k.equal("2", "pr", "view", id, "--json", "comments", "--jq", "[.comments[] | select(.deleted | not)] | length")

	k.out("pr", "approve", id)
	k.equal("1", "pr", "view", id, "--json", "participants", "--jq", "[.participants[] | select(.approved)] | length")
	k.out("pr", "unapprove", id)

	// The pull-request pipeline may still be running.
	k.out("pr", "checks", id, "--watch")

	k.out("pr", "merge", id, "--dry-run")
	if stderr := k.exits(1, "pr", "merge", id, "--json", "state"); !strings.Contains(stderr, `"code":"confirmation_required"`) {
		t.Errorf("merge without --yes: stderr = %q", stderr)
	}

	number := k.out("pipeline", "run", "--branch", branch, "--custom", "khbb-fail", "--json", "number", "--jq", ".number")
	k.exits(1, "pipeline", "watch", number, "--exit-status")
	if logs := k.out("pipeline", "logs", number, "--failed"); !strings.Contains(logs, "about to fail") {
		t.Errorf("failed step log = %q", logs)
	}

	k.equal("DECLINED", "pr", "decline", id, "--yes", "--message", "End-to-end test done", "--json", "state", "--jq", ".state")
	declined = true
}
