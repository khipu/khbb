// Package pr groups the `khbb pr` commands.
package pr

import (
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/pr/checks"
	"github.com/khipu/khbb/pkg/cmd/pr/comment"
	"github.com/khipu/khbb/pkg/cmd/pr/create"
	"github.com/khipu/khbb/pkg/cmd/pr/diff"
	"github.com/khipu/khbb/pkg/cmd/pr/list"
	"github.com/khipu/khbb/pkg/cmd/pr/review"
	prStatus "github.com/khipu/khbb/pkg/cmd/pr/status"
	"github.com/khipu/khbb/pkg/cmd/pr/view"
)

// NewCmdPR returns `khbb pr`.
func NewCmdPR(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <command>",
		Short: "Work with Bitbucket pull requests",
		Long:  "Create, review, merge and inspect pull requests. Commands that take a pull request default to the open pull request of the current branch.",
		Args:  cobra.ArbitraryArgs,
		RunE:  cmdutil.GroupRunE,
	}
	cmdutil.EnableRepoOverride(cmd, f)
	cmd.AddCommand(
		create.NewCmdCreate(f, nil),
		list.NewCmdList(f, nil),
		view.NewCmdView(f, nil),
		diff.NewCmdDiff(f, nil),
		prStatus.NewCmdStatus(f, nil),
		checks.NewCmdChecks(f, nil),
		review.NewCmdApprove(f, nil),
		review.NewCmdUnapprove(f, nil),
		review.NewCmdRequestChanges(f, nil),
		comment.NewCmdComment(f, nil),
	)
	return cmd
}
