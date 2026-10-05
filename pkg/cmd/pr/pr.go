// Package pr groups the `khbb pr` commands.
package pr

import (
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/pr/list"
)

// NewCmdPR returns `khbb pr`.
func NewCmdPR(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <command>",
		Short: "Work with Bitbucket pull requests",
		Long:  "List, inspect and check pull requests. Commands that take a pull request default to the open pull request of the current branch.",
		Args:  cobra.ArbitraryArgs,
		RunE:  cmdutil.GroupRunE,
	}
	cmdutil.EnableRepoOverride(cmd, f)
	cmd.AddCommand(
		list.NewCmdList(f, nil),
	)
	return cmd
}
