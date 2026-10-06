// Package list implements `khbb pipeline list`.
package list

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
)

// ListOptions holds the inputs and dependencies of `khbb pipeline list`.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Exporter   cmdutil.Exporter

	Branch string
	Status string
	Mine   bool
	Limit  int
}

// NewCmdList returns `khbb pipeline list`.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo}
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List pipelines in a repository",
		Long:    "List a repository's newest pipelines, optionally only those of a branch, with a status, or that you started.",
		Example: `  $ khbb pipeline list
  $ khbb pipeline list --branch main --status failed --limit 5
  $ khbb pipeline list --mine --json number,status,refName,url`,
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Status = strings.ToLower(opts.Status)
			if _, ok := shared.ListStatuses[opts.Status]; opts.Status != "" && !ok {
				return cmdutil.FlagErrorf("invalid --status %q: use pending, running, paused, successful, failed, error or stopped", opts.Status)
			}
			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid --limit %d: must be at least 1", opts.Limit)
			}
			if runF != nil {
				return runF(opts)
			}
			return listRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.Branch, "branch", "b", "", "Only pipelines of this `branch`")
	fl.StringVarP(&opts.Status, "status", "s", "", "Only pipelines with this `status`: pending, running, paused, successful, failed, error or stopped")
	fl.BoolVar(&opts.Mine, "mine", false, "Only pipelines you started")
	fl.IntVarP(&opts.Limit, "limit", "L", 20, "Maximum number of pipelines to fetch")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PipelineFields)
	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	filter := bitbucket.PipelineListOptions{Branch: opts.Branch, Statuses: shared.ListStatuses[opts.Status]}
	if opts.Mine {
		me, err := client.CurrentUser(ctx)
		if err != nil {
			return err
		}
		filter.CreatorUUID = me.UUID
	}
	raws, err := client.ListPipelines(ctx, repo.Workspace, repo.Slug, filter, opts.Limit)
	if err != nil {
		return err
	}
	pipelines := make([]shared.Pipeline, len(raws))
	for i := range raws {
		pipelines[i] = shared.NewPipeline(&raws[i], repo)
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, pipelines)
	}
	if len(pipelines) == 0 {
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "No pipelines match your filters in %s\n", repo.FullName())
		}
		return nil
	}
	return printTable(opts.IO, pipelines)
}

func printTable(ios *iostreams.IOStreams, pipelines []shared.Pipeline) error {
	tty := ios.IsStdoutTTY()
	tp := tableprinter.New(ios.Out, tty, ios.TerminalWidth())
	tp.AddHeader([]string{"STATUS", "NUMBER", "RUN ON", "TRIGGER", "DURATION", "STARTED"})
	for _, p := range pipelines {
		if tty {
			tp.AddField(shared.StatusLabel(ios, p.Status))
			tp.AddField("#" + strconv.Itoa(p.Number))
			tp.AddField(shared.RefLabel(p))
			tp.AddField(p.Trigger)
			tp.AddField(duration(p))
			tp.AddField(p.CreatedOn.Format("2006-01-02 15:04 MST"))
		} else {
			tp.AddField(strconv.Itoa(p.Number))
			tp.AddField(p.Status)
			tp.AddField(p.RefType)
			tp.AddField(p.RefName)
			tp.AddField(p.Trigger)
			tp.AddField(strconv.Itoa(p.DurationSeconds))
			tp.AddField(p.CreatedOn.Format(time.RFC3339))
		}
		tp.EndRow()
	}
	return tp.Render()
}

func duration(p shared.Pipeline) string {
	if p.CompletedOn == nil {
		return "-"
	}
	return shared.FormatDuration(p.DurationSeconds)
}
