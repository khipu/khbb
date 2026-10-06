// Package stop implements `khbb pipeline stop`.
package stop

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// StopOptions holds the inputs and dependencies of `khbb pipeline stop`.
type StopOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Prompter   cmdutil.Prompter

	Selector string
	Yes      bool
	DryRun   bool
}

// NewCmdStop returns `khbb pipeline stop`.
func NewCmdStop(f *cmdutil.Factory, runF func(*StopOptions) error) *cobra.Command {
	opts := &StopOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Prompter: f.Prompter}
	cmd := &cobra.Command{
		Use:   "stop [<number> | <url>]",
		Short: "Stop a running pipeline",
		Long: `Stop a pipeline that is pending or running. Stopping cannot be undone: on a terminal you are asked
to confirm; otherwise --yes is required. Bitbucket cannot stop a pipeline paused on a manual step
through its API; stop those in the web interface. Without an argument, stop the newest pipeline of
the current branch.`,
		Example: `  $ khbb pipeline stop 42
  $ khbb pipeline stop --yes`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prshared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			if _, _, err := shared.ParseSelector(opts.Selector); err != nil {
				return err
			}
			opts.DryRun = f.DryRun
			if runF != nil {
				return runF(opts)
			}
			return stopRun(cmd.Context(), opts)
		},
	}
	cmdutil.AddYesFlag(cmd, &opts.Yes)
	cmdutil.AddDryRunFlag(cmd, f)
	return cmd
}

func stopRun(ctx context.Context, opts *StopOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	raw, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	p := shared.NewPipeline(raw, repo)
	switch {
	case raw.State.Name == "COMPLETED":
		return &cmdutil.ConflictError{Msg: fmt.Sprintf("pipeline #%d already finished (%s)", p.Number, p.Status)}
	case p.Status == shared.StatusPaused:
		return &cmdutil.ConflictError{Msg: fmt.Sprintf("pipeline #%d is paused on a manual step; Bitbucket does not stop paused pipelines through its API: stop it at %s",
			p.Number, p.URL)}
	}
	question := fmt.Sprintf("Stop pipeline #%d (%s)?", p.Number, shared.RefLabel(p))
	if err := cmdutil.ConfirmDestructive(opts.IO, opts.Prompter, opts.Yes || opts.DryRun, question); err != nil {
		return err
	}
	if err := client.StopPipeline(ctx, repo.Workspace, repo.Slug, p.Number); err != nil {
		return err
	}
	prshared.PrintSuccess(opts.IO, "Stopped pipeline #%d (%s)", p.Number, shared.RefLabel(p))
	return nil
}
