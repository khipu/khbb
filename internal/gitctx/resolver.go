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
		repo, ok := r.bitbucketRepo(rem.URL)
		if !ok {
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

// RemoteFor returns the name of a remote that points at repo, preferring "origin", or "" when no
// remote does.
func (r *Resolver) RemoteFor(repo Repo) (string, error) {
	remotes, err := r.Remotes()
	if err != nil {
		return "", err
	}
	name := ""
	for _, rem := range remotes {
		got, ok := r.bitbucketRepo(rem.URL)
		if !ok || !strings.EqualFold(got.FullName(), repo.FullName()) {
			continue
		}
		if rem.Name == "origin" {
			return rem.Name, nil
		}
		if name == "" {
			name = rem.Name
		}
	}
	return name, nil
}

// bitbucketRepo returns the repository a remote URL points at; ok is false for hosts other than
// bitbucket.org and altssh.bitbucket.org, after resolving SSH host aliases.
func (r *Resolver) bitbucketRepo(rawURL string) (Repo, bool) {
	host, repo, isSSH, err := parseRemoteURL(rawURL)
	if err != nil {
		return Repo{}, false
	}
	if !isBitbucketHost(host) && isSSH && r.SSHHostname != nil {
		if real, err := r.SSHHostname(host); err == nil {
			host = real
		}
	}
	return repo, isBitbucketHost(host)
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

// sshHostname asks ssh which hostname an alias from ~/.ssh/config stands for.
func sshHostname(alias string) (string, error) {
	// ssh would read a host that starts with "-" as an option, such as -oProxyCommand=….
	if alias == "" || strings.HasPrefix(alias, "-") {
		return "", fmt.Errorf("invalid ssh host %q", alias)
	}
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
