// Package decline implements `khbb pr decline`.
package decline

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// DeclineOptions holds the inputs and dependencies of `khbb pr decline`.
type DeclineOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Prompter   cmdutil.Prompter
	Exporter   cmdutil.Exporter

	Selector string
	Message  string
	Yes      bool
	DryRun   bool
}

// NewCmdDecline returns `khbb pr decline`.
func NewCmdDecline(f *cmdutil.Factory, runF func(*DeclineOptions) error) *cobra.Command {
	opts := &DeclineOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Prompter: f.Prompter}
	cmd := &cobra.Command{
		Use:   "decline [<number> | <url>]",
		Short: "Decline a pull request",
		Long: `Decline an open pull request. Bitbucket Cloud cannot reopen a declined pull request, so on a
terminal you are asked to confirm; otherwise --yes is required.`,
		Example: `  $ khbb pr decline 42 --message "Superseded by #43"
  $ khbb pr decline 42 --yes`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			opts.DryRun = f.DryRun
			if runF != nil {
				return runF(opts)
			}
			return declineRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Message, "message", "m", "", "The `reason` for declining")
	cmdutil.AddYesFlag(cmd, &opts.Yes)
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PullRequestFields)
	return cmd
}

func declineRun(ctx context.Context, opts *DeclineOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if err := shared.RequireOpen(pr, "declined"); err != nil {
		return err
	}
	question := fmt.Sprintf("Decline %s? Declined pull requests cannot be reopened.", shared.Describe(pr))
	if err := cmdutil.ConfirmDestructive(opts.IO, opts.Prompter, opts.Yes || opts.DryRun, question); err != nil {
		return err
	}
	declined, err := client.DeclinePR(ctx, repo.Workspace, repo.Slug, pr.ID, opts.Message)
	if err != nil {
		return err
	}
	shared.PrintSuccess(opts.IO, "Declined pull request %s", shared.Describe(pr))
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, shared.NewPullRequest(declined))
	}
	return nil
}
