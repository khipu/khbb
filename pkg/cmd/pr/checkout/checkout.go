// Package checkout implements `khbb pr checkout`.
package checkout

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// CheckoutOptions holds the inputs and dependencies of `khbb pr checkout`.
type CheckoutOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Config     func() (*config.Config, error)
	Git        *gitctx.Resolver

	Selector   string
	BranchName string
	Force      bool
}

// NewCmdCheckout returns `khbb pr checkout`.
func NewCmdCheckout(f *cmdutil.Factory, runF func(*CheckoutOptions) error) *cobra.Command {
	opts := &CheckoutOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Git: f.Git}
	cmd := &cobra.Command{
		Use:   "checkout {<number> | <url>}",
		Short: "Check out a pull request in git",
		Long: `Fetch the source branch of a pull request and switch to it, creating a local branch that
tracks it. An existing local branch is fast-forwarded; --force resets it instead. Pull requests
from forks are fetched from the fork's URL (git_protocol: ssh or https). Your git credentials
are used for fetching.`,
		Example: `  $ khbb pr checkout 42
  $ khbb pr checkout 42 --branch review-42 --force`,
		Args: cmdutil.ExactArgs(1, "<number> | <url>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Selector = args[0]
			opts.Config = f.Config
			if runF != nil {
				return runF(opts)
			}
			return checkoutRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.BranchName, "branch", "b", "", "Local branch `name` to use (default: the source branch name)")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Reset an existing local branch to the pull request")
	return cmd
}

func checkoutRun(ctx context.Context, opts *CheckoutOptions) error {
	if opts.Git == nil {
		return errors.New("pr checkout needs git")
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	source := repo
	if pr.Source.Repository != nil && pr.Source.Repository.FullName != "" {
		if source, err = gitctx.ParseRepo(pr.Source.Repository.FullName); err != nil {
			return err
		}
	}
	branch := pr.Source.Branch.Name
	local := opts.BranchName
	if local == "" {
		local = branch
	}
	remote, err := opts.Git.RemoteFor(source)
	if err != nil {
		return err
	}
	git := opts.Git.Git
	_, revErr := git("rev-parse", "--verify", "--quiet", "refs/heads/"+local)
	exists := revErr == nil

	if remote != "" {
		tracking := "refs/remotes/" + remote + "/" + branch
		if _, err := git("fetch", remote, "+refs/heads/"+branch+":"+tracking); err != nil {
			return err
		}
		if exists {
			err = update(git, local, tracking, opts.Force)
		} else {
			_, err = git("checkout", "-b", local, "--track", remote+"/"+branch)
		}
	} else {
		url, uerr := cloneURL(opts.Config, source)
		if uerr != nil {
			return uerr
		}
		if _, err := git("fetch", url, "refs/heads/"+branch); err != nil {
			return err
		}
		if exists {
			err = update(git, local, "FETCH_HEAD", opts.Force)
		} else {
			err = newForkBranch(git, local, url, branch)
		}
	}
	if err != nil {
		return err
	}
	shared.PrintSuccess(opts.IO, "Checked out pull request %s on branch %s", shared.Describe(pr), local)
	return nil
}

// update switches to an existing local branch and brings it to ref: fast-forward only, or a hard
// reset with --force.
func update(git func(...string) (string, error), local, ref string, force bool) error {
	if _, err := git("checkout", local); err != nil {
		return err
	}
	if force {
		_, err := git("reset", "--hard", ref)
		return err
	}
	if _, err := git("merge", "--ff-only", ref); err != nil {
		return fmt.Errorf("branch %s cannot be fast-forwarded to the pull request; use --force to reset it (%w)", local, err)
	}
	return nil
}

// newForkBranch creates local at FETCH_HEAD and makes it track branch in the fork at url, so that
// `git pull` keeps working.
func newForkBranch(git func(...string) (string, error), local, url, branch string) error {
	if _, err := git("checkout", "-b", local, "FETCH_HEAD"); err != nil {
		return err
	}
	if _, err := git("config", "branch."+local+".remote", url); err != nil {
		return err
	}
	_, err := git("config", "branch."+local+".merge", "refs/heads/"+branch)
	return err
}

// cloneURL returns the URL to fetch a repository that has no local remote, following the
// git_protocol setting: ssh, or https by default.
func cloneURL(cfg func() (*config.Config, error), repo gitctx.Repo) (string, error) {
	protocol := ""
	if cfg != nil {
		c, err := cfg()
		if err != nil {
			return "", err
		}
		protocol = c.GitProtocol
	}
	if protocol == "ssh" {
		return "git@bitbucket.org:" + repo.FullName() + ".git", nil
	}
	return "https://bitbucket.org/" + repo.FullName() + ".git", nil
}
