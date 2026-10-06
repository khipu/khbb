// Package run implements `khbb pipeline run`.
package run

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
)

// RunOptions holds the inputs and dependencies of `khbb pipeline run`.
type RunOptions struct {
	IO            *iostreams.IOStreams
	HTTPClient    func() (*bitbucket.Client, error)
	BaseRepo      func() (gitctx.Repo, error)
	CurrentBranch func() (string, error)
	Exporter      cmdutil.Exporter
	Sleep         func(time.Duration)

	Branch     string
	Commit     string
	Tag        string
	Custom     string
	Vars       []string
	SecretVars []string
	Watch      bool
}

// NewCmdRun returns `khbb pipeline run`.
func NewCmdRun(f *cmdutil.Factory, runF func(*RunOptions) error) *cobra.Command {
	opts := &RunOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, CurrentBranch: f.Branch, Sleep: time.Sleep}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start a pipeline",
		Long: `Start a pipeline on a branch (the current branch by default), a tag or a commit. --custom runs a
custom pipeline from bitbucket-pipelines.yml; --var and --secret-var pass variables (Bitbucket
hides secured values in logs, and khbb never prints them).

The pipeline URL is printed on stdout. With --watch, khbb then follows the pipeline and exits
with status 1 if it ends failed, error, stopped or expired (a pipeline paused on a manual step
exits 0).`,
		Example: `  $ khbb pipeline run
  $ khbb pipeline run --custom deploy --var ENV=staging --secret-var TOKEN="$TOKEN" --watch
  $ khbb pipeline run --tag v1.2.0 --dry-run`,
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			targets := 0
			for _, v := range []string{opts.Branch, opts.Commit, opts.Tag} {
				if v != "" {
					targets++
				}
			}
			switch {
			case targets > 1:
				return cmdutil.FlagErrorf("specify only one of --branch, --commit or --tag")
			case targets == 0 && cmd.Flags().Changed("repo"):
				return cmdutil.FlagErrorf("--branch, --commit or --tag required when using the --repo flag")
			case opts.Watch && opts.Exporter != nil:
				return cmdutil.FlagErrorf("--watch cannot be combined with --json")
			}
			if _, err := shared.ParseVariables(opts.Vars, opts.SecretVars); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return runRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.Branch, "branch", "b", "", "Run on this `branch` (default: the current branch)")
	fl.StringVar(&opts.Commit, "commit", "", "Run on this commit `sha`")
	fl.StringVar(&opts.Tag, "tag", "", "Run on this `tag`")
	fl.StringVar(&opts.Custom, "custom", "", "Run the custom pipeline with this `name`")
	fl.StringArrayVar(&opts.Vars, "var", nil, "Pass a variable as `KEY=VALUE` (repeatable)")
	fl.StringArrayVar(&opts.SecretVars, "secret-var", nil, "Pass a secured variable as `KEY=VALUE` (repeatable)")
	fl.BoolVar(&opts.Watch, "watch", false, "Follow the pipeline until it finishes")
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PipelineFields)
	return cmd
}

func runRun(ctx context.Context, opts *RunOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	target, err := runTarget(opts)
	if err != nil {
		return err
	}
	vars, err := shared.ParseVariables(opts.Vars, opts.SecretVars)
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	raw, err := client.RunPipeline(ctx, repo.Workspace, repo.Slug, target, vars)
	if err != nil {
		return err
	}
	p := shared.Started(raw, target.Selector, repo)
	if err := shared.ReportStarted(opts.IO, opts.Exporter, p, shared.RefLabel(p)); err != nil {
		return err
	}
	if !opts.Watch {
		return nil
	}
	return shared.WatchStarted(ctx, opts.IO, client, repo, p.Number, opts.Sleep)
}

// runTarget builds the pipeline target from --branch, --commit, --tag and --custom. A commit target
// needs a selector, so it uses the default pipeline unless --custom names one.
func runTarget(opts *RunOptions) (bitbucket.PipelineTarget, error) {
	var selector *bitbucket.PipelineSelector
	if opts.Custom != "" {
		selector = &bitbucket.PipelineSelector{Type: "custom", Pattern: opts.Custom}
	}
	switch {
	case opts.Commit != "":
		if selector == nil {
			selector = &bitbucket.PipelineSelector{Type: "default"}
		}
		return bitbucket.PipelineTarget{Type: "pipeline_commit_target", Commit: &bitbucket.Commit{Hash: opts.Commit}, Selector: selector}, nil
	case opts.Tag != "":
		return bitbucket.PipelineTarget{Type: "pipeline_ref_target", RefType: "tag", RefName: opts.Tag, Selector: selector}, nil
	}
	branch := opts.Branch
	if branch == "" {
		b, err := opts.CurrentBranch()
		if err != nil {
			return bitbucket.PipelineTarget{}, fmt.Errorf("%w; name the branch with --branch", err)
		}
		branch = b
	}
	return bitbucket.PipelineTarget{Type: "pipeline_ref_target", RefType: "branch", RefName: branch, Selector: selector}, nil
}
