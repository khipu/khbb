// Package pipeline groups the `khbb pipeline` commands.
package pipeline

import (
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/pipeline/list"
	"github.com/khipu/khbb/pkg/cmd/pipeline/logs"
	"github.com/khipu/khbb/pkg/cmd/pipeline/run"
	"github.com/khipu/khbb/pkg/cmd/pipeline/stop"
	"github.com/khipu/khbb/pkg/cmd/pipeline/view"
	"github.com/khipu/khbb/pkg/cmd/pipeline/watch"
)

// NewCmdPipeline returns `khbb pipeline`.
func NewCmdPipeline(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pipeline <command>",
		Short: "Work with Bitbucket Pipelines",
		Long: `List, inspect, run, stop and watch pipelines. A pipeline is named by its build number (42 or
#42) or its URL; commands that take one default to the newest pipeline of the current branch.`,
		Args: cobra.ArbitraryArgs,
		RunE: cmdutil.GroupRunE,
	}
	cmdutil.EnableRepoOverride(cmd, f)
	cmd.AddCommand(
		list.NewCmdList(f, nil),
		view.NewCmdView(f, nil),
		logs.NewCmdLogs(f, nil),
		run.NewCmdRun(f, nil),
		stop.NewCmdStop(f, nil),
		watch.NewCmdWatch(f, nil),
	)
	return cmd
}
