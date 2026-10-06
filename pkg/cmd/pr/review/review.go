// Package review implements `khbb pr approve`, `pr unapprove` and `pr request-changes`.
package review

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// ReviewOptions holds the inputs and dependencies of the review commands.
type ReviewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)

	Selector string
	Action   Action
}

// Action is a review decision: the API call and how to report it.
type Action struct {
	Name   string // command name
	Done   string // message prefix, such as "Approved"
	submit func(c *bitbucket.Client, ctx context.Context, workspace, slug string, id int) error
}

// The review decisions.
var (
	Approve        = Action{Name: "approve", Done: "Approved", submit: (*bitbucket.Client).ApprovePR}
	Unapprove      = Action{Name: "unapprove", Done: "Removed your approval from", submit: (*bitbucket.Client).UnapprovePR}
	RequestChanges = Action{Name: "request-changes", Done: "Requested changes on", submit: (*bitbucket.Client).RequestChangesPR}
)

// NewCmdApprove returns `khbb pr approve`.
func NewCmdApprove(f *cmdutil.Factory, runF func(*ReviewOptions) error) *cobra.Command {
	return newCmd(f, runF, Approve, "Approve a pull request", `  $ khbb pr approve 42
  $ khbb pr approve`)
}

// NewCmdUnapprove returns `khbb pr unapprove`.
func NewCmdUnapprove(f *cmdutil.Factory, runF func(*ReviewOptions) error) *cobra.Command {
	return newCmd(f, runF, Unapprove, "Remove your approval from a pull request", `  $ khbb pr unapprove 42`)
}

// NewCmdRequestChanges returns `khbb pr request-changes`.
func NewCmdRequestChanges(f *cmdutil.Factory, runF func(*ReviewOptions) error) *cobra.Command {
	return newCmd(f, runF, RequestChanges, "Request changes on a pull request", `  $ khbb pr request-changes 42
  $ khbb pr comment 42 --body "Please add tests" && khbb pr request-changes 42`)
}

func newCmd(f *cmdutil.Factory, runF func(*ReviewOptions) error, action Action, short, example string) *cobra.Command {
	opts := &ReviewOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Action: action}
	cmd := &cobra.Command{
		Use:     action.Name + " [<number> | <url>]",
		Short:   short,
		Long:    short + ". Without an argument, act on the open pull request of the current branch. Only open pull requests can be reviewed.",
		Example: example,
		Args:    cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			if runF != nil {
				return runF(opts)
			}
			return reviewRun(cmd.Context(), opts)
		},
	}
	cmdutil.AddDryRunFlag(cmd, f)
	return cmd
}

func reviewRun(ctx context.Context, opts *ReviewOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if err := shared.RequireOpen(pr, "reviewed"); err != nil {
		return err
	}
	if err := opts.Action.submit(client, ctx, repo.Workspace, repo.Slug, pr.ID); err != nil {
		return err
	}
	shared.PrintSuccess(opts.IO, "%s pull request %s", opts.Action.Done, shared.Describe(pr))
	return nil
}
