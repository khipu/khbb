// Package cmdutil holds what every khbb command shares: the Factory, flag helpers and error types.
package cmdutil

import (
	"errors"
	"fmt"
	"os"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
)

// Prompter asks the user questions on a terminal.
type Prompter interface {
	Input(prompt, defaultValue string) (string, error)
	Password(prompt string) (string, error)
	Confirm(prompt string, defaultValue bool) (bool, error)
}

// Browser opens web pages for --web.
type Browser interface {
	Browse(url string) error
}

// Factory provides commands with their dependencies. Function fields are lazy so that
// commands which do not need the network or a repository never touch them.
type Factory struct {
	AppVersion  string
	BuildCommit string
	BuildDate   string

	IOStreams *iostreams.IOStreams
	Prompter  Prompter
	Browser   Browser
	Git       *gitctx.Resolver

	Config     func() (*config.Config, error)
	HTTPClient func() (*bitbucket.Client, error)

	// Bound to flags by EnableRepoOverride and AddDryRunFlag.
	RepoOverride string
	DryRun       bool
}

// BaseRepo resolves the target repository: -R flag, then KHBB_REPO, then git remotes.
func (f *Factory) BaseRepo() (gitctx.Repo, error) {
	if f.RepoOverride != "" {
		r, err := gitctx.ParseRepo(f.RepoOverride)
		if err != nil {
			return gitctx.Repo{}, &FlagError{Err: err}
		}
		return r, nil
	}
	if env := os.Getenv("KHBB_REPO"); env != "" {
		r, err := gitctx.ParseRepo(env)
		if err != nil {
			return gitctx.Repo{}, fmt.Errorf("KHBB_REPO: %w", err)
		}
		return r, nil
	}
	if f.Git == nil {
		return gitctx.Repo{}, gitctx.ErrNoRepo
	}
	return f.Git.BaseRepo()
}

// Branch returns the current git branch.
func (f *Factory) Branch() (string, error) {
	if f.Git == nil {
		return "", errors.New("could not determine current branch: git is not available")
	}
	return f.Git.CurrentBranch()
}
