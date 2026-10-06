// Package skill groups the `khbb skill` commands.
package skill

import (
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/skill/install"
)

// NewCmdSkill returns `khbb skill`.
func NewCmdSkill(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill <command>",
		Short: "Install the khbb skill for AI coding agents",
		Args:  cobra.ArbitraryArgs,
		RunE:  cmdutil.GroupRunE,
	}
	cmd.AddCommand(install.NewCmdInstall(f, nil))
	return cmd
}
