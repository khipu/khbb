package gitctx

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cli/safeexec"
)

// ErrNoRepo means no Bitbucket repository could be inferred.
var ErrNoRepo = errors.New("could not determine repository; use -R WORKSPACE/REPO")

// Resolver inspects a git working copy. Its functions are fields so tests can fake them.
type Resolver struct {
	// Git runs git with args and returns trimmed stdout.
	Git func(args ...string) (string, error)
	// SSHHostname resolves an SSH host alias to its real hostname. Nil disables alias resolution.
	SSHHostname func(alias string) (string, error)
}

// NewResolver returns a Resolver for the git working copy at dir ("" = current directory).
func NewResolver(dir string) *Resolver {
	return &Resolver{
		Git:         func(args ...string) (string, error) { return runGit(dir, args...) },
		SSHHostname: sshHostname,
	}
}

// Remotes lists remotes with their fetch URLs, in `git remote -v` order.
func (r *Resolver) Remotes() ([]Remote, error) {
	out, err := r.Git("remote", "-v")
	if err != nil {
		return nil, err
	}
	var remotes []Remote
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || f[2] != "(fetch)" || seen[f[0]] {
			continue
		}
		seen[f[0]] = true
		remotes = append(remotes, Remote{Name: f[0], URL: f[1]})
	}
	return remotes, nil
}

// BaseRepo returns the Bitbucket repository of the working copy, preferring the "origin" remote.
func (r *Resolver) BaseRepo() (Repo, error) {
	remotes, err := r.Remotes()
	if err != nil {
		return Repo{}, fmt.Errorf("%w (%v)", ErrNoRepo, err)
	}
	var found []Repo
	for _, rem := range remotes {
		host, repo, isSSH, err := parseRemoteURL(rem.URL)
		if err != nil {
			continue
		}
		if host != bitbucketHost && isSSH && r.SSHHostname != nil {
			if real, err := r.SSHHostname(host); err == nil {
				host = real
			}
		}
		if host != bitbucketHost {
			continue
		}
		if rem.Name == "origin" {
			return repo, nil
		}
		found = append(found, repo)
	}
	if len(found) > 0 {
		return found[0], nil
	}
	return Repo{}, ErrNoRepo
}

// CurrentBranch returns the checked-out branch name.
func (r *Resolver) CurrentBranch() (string, error) {
	b, err := r.Git("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || b == "" {
		return "", errors.New("could not determine current branch: not on a branch (detached HEAD?)")
	}
	return b, nil
}

func runGit(dir string, args ...string) (string, error) {
	exe, err := safeexec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git not found in PATH: %w", err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(string(out)), nil
}

func sshHostname(alias string) (string, error) {
	exe, err := safeexec.LookPath("ssh")
	if err != nil {
		return "", err
	}
	out, err := exec.Command(exe, "-G", alias).Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) == 2 && strings.EqualFold(f[0], "hostname") {
			return strings.ToLower(f[1]), nil
		}
	}
	return "", fmt.Errorf("ssh -G %s: no hostname", alias)
}
