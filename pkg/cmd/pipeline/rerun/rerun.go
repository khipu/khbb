// Package rerun implements `khbb pipeline rerun`.
package rerun

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// RerunOptions holds the inputs and dependencies of `khbb pipeline rerun`.
type RerunOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Exporter   cmdutil.Exporter
	Sleep      func(time.Duration)

	Selector   string
	Vars       []string
	SecretVars []string
	Watch      bool
}

// NewCmdRerun returns `khbb pipeline rerun`.
func NewCmdRerun(f *cmdutil.Factory, runF func(*RerunOptions) error) *cobra.Command {
	opts := &RerunOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Sleep: time.Sleep}
	cmd := &cobra.Command{
		Use:   "rerun [<number> | <url>]",
		Short: "Run a pipeline again",
		Long: `Start a new pipeline on the same commit and with the same definition as an earlier one — for a
pull-request pipeline, on the same pull request. Bitbucket never returns a pipeline's variables,
so they cannot be copied: pass them again with --var and --secret-var. Without an argument, rerun
the newest pipeline of the current branch.

With --watch, khbb then follows the pipeline and exits with status 1 if it ends failed, error,
stopped or expired (a pipeline paused on a manual step exits 0).`,
		Example: `  $ khbb pipeline rerun 42 --watch
  $ khbb pipeline rerun 45 --var ENV=staging --secret-var TOKEN="$TOKEN"`,
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
			if opts.Watch && opts.Exporter != nil {
				return cmdutil.FlagErrorf("--watch cannot be combined with --json")
			}
			if _, err := shared.ParseVariables(opts.Vars, opts.SecretVars); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return rerunRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringArrayVar(&opts.Vars, "var", nil, "Pass a variable as `KEY=VALUE` (repeatable)")
	fl.StringArrayVar(&opts.SecretVars, "secret-var", nil, "Pass a secured variable as `KEY=VALUE` (repeatable)")
	fl.BoolVar(&opts.Watch, "watch", false, "Follow the new pipeline until it finishes")
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PipelineFields)
	return cmd
}

func rerunRun(ctx context.Context, opts *RerunOptions) error {
	vars, err := shared.ParseVariables(opts.Vars, opts.SecretVars)
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	original, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if sel := original.Target.Selector; len(vars) == 0 && sel != nil && sel.Type == "custom" {
		fmt.Fprintf(opts.IO.ErrOut, "note: Bitbucket does not return the variables of pipeline #%d; if it needs any, pass them with --var or --secret-var\n",
			original.BuildNumber)
	}
	raw, err := client.RunPipeline(ctx, repo.Workspace, repo.Slug, original.Target, vars)
	if err != nil {
		return err
	}
	p := shared.Started(raw, original.Target.Selector, repo)
	what := fmt.Sprintf("a rerun of #%d, %s", original.BuildNumber, shared.RefLabel(p))
	if err := shared.ReportStarted(opts.IO, opts.Exporter, p, what); err != nil {
		return err
	}
	if !opts.Watch {
		return nil
	}
	return shared.WatchStarted(ctx, opts.IO, client, repo, p.Number, opts.Sleep)
}
