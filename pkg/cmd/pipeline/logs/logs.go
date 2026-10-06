// Package logs implements `khbb pipeline logs`.
package logs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// LogsOptions holds the inputs and dependencies of `khbb pipeline logs`.
type LogsOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)

	Selector string
	Step     string
	Failed   bool
	Tail     int
}

// NewCmdLogs returns `khbb pipeline logs`.
func NewCmdLogs(f *cmdutil.Factory, runF func(*LogsOptions) error) *cobra.Command {
	opts := &LogsOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch}
	cmd := &cobra.Command{
		Use:   "logs [<number> | <url>]",
		Short: "Show the logs of a pipeline's steps",
		Long: `Print the logs of a pipeline's steps: every step that has a log, each after a "==> <step> <=="
header when there are several, only --step (a name or UUID), or only the --failed ones. Steps
that have not run have no log; they are skipped with a notice on stderr. Without an argument,
use the newest pipeline of the current branch.`,
		Example: `  $ khbb pipeline logs 42 --failed
  $ khbb pipeline logs --step Build --tail 50`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prshared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			switch {
			case opts.Step != "" && opts.Failed:
				return cmdutil.FlagErrorf("--step cannot be combined with --failed")
			case cmd.Flags().Changed("tail") && opts.Tail < 1:
				return cmdutil.FlagErrorf("invalid --tail %d: must be at least 1", opts.Tail)
			}
			if runF != nil {
				return runF(opts)
			}
			return logsRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Step, "step", "s", "", "Only the step with this `name` or UUID")
	cmd.Flags().BoolVar(&opts.Failed, "failed", false, "Only the steps that failed")
	cmd.Flags().IntVar(&opts.Tail, "tail", 0, "Only the last `lines` of each log")
	return cmd
}

func logsRun(ctx context.Context, opts *LogsOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	p, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	steps, err := client.ListPipelineSteps(ctx, repo.Workspace, repo.Slug, p.BuildNumber)
	if err != nil {
		return err
	}
	selected, err := selectSteps(p.BuildNumber, steps, opts)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		if opts.Failed {
			fmt.Fprintf(opts.IO.ErrOut, "no failed steps in pipeline #%d\n", p.BuildNumber)
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "pipeline #%d has no steps\n", p.BuildNumber)
		}
		return nil
	}
	headers := len(selected) > 1
	for _, s := range selected {
		status := shared.Status(s.State)
		if status == shared.StatusPending || status == shared.StatusPaused || status == shared.StatusSkipped {
			fmt.Fprintf(opts.IO.ErrOut, "no log for step %q (%s)\n", s.Name, status)
			continue
		}
		text, err := client.StepLog(ctx, repo.Workspace, repo.Slug, p.BuildNumber, s.UUID)
		var httpErr *bitbucket.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			fmt.Fprintf(opts.IO.ErrOut, "no log for step %q\n", s.Name)
			continue
		}
		if err != nil {
			return err
		}
		if headers {
			fmt.Fprintf(opts.IO.Out, "==> %s <==\n", s.Name)
		}
		if err := writeLog(opts.IO.Out, lastLines(text, opts.Tail)); err != nil {
			return err
		}
	}
	return nil
}

// selectSteps applies --step and --failed.
func selectSteps(number int, steps []bitbucket.PipelineStep, opts *LogsOptions) ([]bitbucket.PipelineStep, error) {
	switch {
	case opts.Step != "":
		for _, s := range steps {
			if strings.EqualFold(s.Name, opts.Step) || s.UUID == opts.Step {
				return []bitbucket.PipelineStep{s}, nil
			}
		}
		names := make([]string, len(steps))
		for i, s := range steps {
			names[i] = s.Name
		}
		return nil, &cmdutil.NotFoundError{Msg: fmt.Sprintf("pipeline #%d has no step %q; its steps are: %s", number, opts.Step, strings.Join(names, ", "))}
	case opts.Failed:
		var failed []bitbucket.PipelineStep
		for _, s := range steps {
			if status := shared.Status(s.State); status == shared.StatusFailed || status == shared.StatusError {
				failed = append(failed, s)
			}
		}
		return failed, nil
	}
	return steps, nil
}

// lastLines keeps the last n lines of text, or all of it when n <= 0.
func lastLines(text string, n int) string {
	if n <= 0 {
		return text
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= n {
		return text
	}
	return strings.Join(lines[len(lines)-n:], "")
}

// writeLog writes text and ends it with a newline if it lacks one.
func writeLog(w io.Writer, text string) error {
	if _, err := io.WriteString(w, text); err != nil {
		return err
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		_, err := io.WriteString(w, "\n")
		return err
	}
	return nil
}
