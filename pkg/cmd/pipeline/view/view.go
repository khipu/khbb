// Package view implements `khbb pipeline view`.
package view

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// ViewOptions holds the inputs and dependencies of `khbb pipeline view`.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Browser    cmdutil.Browser
	Exporter   cmdutil.Exporter

	Selector string
	Verbose  bool
	Web      bool
}

var viewFields = append(slices.Clone(shared.PipelineFields), "steps")

type viewExport struct {
	shared.Pipeline
	Steps []shared.Step `json:"steps"`
}

// NewCmdView returns `khbb pipeline view`.
func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Browser: f.Browser}
	cmd := &cobra.Command{
		Use:   "view [<number> | <url>]",
		Short: "Show a pipeline and its steps",
		Long:  "Show a pipeline's status, what it ran on, who started it and its steps. Without an argument, show the newest pipeline of the current branch.",
		Example: `  $ khbb pipeline view 42
  $ khbb pipeline view --verbose
  $ khbb pipeline view 42 --json status,steps`,
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
			if opts.Web && opts.Exporter != nil {
				return cmdutil.FlagErrorf("--web cannot be combined with --json")
			}
			if runF != nil {
				return runF(opts)
			}
			return viewRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.Verbose, "verbose", "v", false, "Show step UUIDs and timestamps")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open the pipeline in the browser")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, viewFields)
	return cmd
}

func viewRun(ctx context.Context, opts *ViewOptions) error {
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
	if opts.Web {
		if opts.IO.IsStderrTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", p.URL)
		}
		return opts.Browser.Browse(p.URL)
	}
	rawSteps, err := client.ListPipelineSteps(ctx, repo.Workspace, repo.Slug, p.Number)
	if err != nil {
		return err
	}
	steps := make([]shared.Step, len(rawSteps))
	for i := range rawSteps {
		steps[i] = shared.NewStep(&rawSteps[i])
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, viewExport{Pipeline: p, Steps: steps})
	}
	if opts.IO.IsStdoutTTY() {
		return shared.RenderSummary(opts.IO, p, steps, opts.Verbose)
	}
	printRaw(opts.IO.Out, p, steps, opts.Verbose)
	return nil
}

func printRaw(w io.Writer, p shared.Pipeline, steps []shared.Step, verbose bool) {
	fmt.Fprintf(w, "number:\t%d\nstatus:\t%s\nrun on:\t%s\ntrigger:\t%s\ncreator:\t%s\ncommit:\t%s\nduration:\t%d\nurl:\t%s\n--\n",
		p.Number, p.Status, shared.RefLabel(p), p.Trigger, shared.CreatorName(p), p.Commit, p.DurationSeconds, p.URL)
	for _, s := range steps {
		fields := []string{s.Name, s.Status, strconv.Itoa(s.DurationSeconds)}
		if verbose {
			fields = append(fields, s.UUID, rfc3339(s.StartedOn), rfc3339(s.CompletedOn))
		}
		fmt.Fprintln(w, strings.Join(fields, "\t"))
	}
}

func rfc3339(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
