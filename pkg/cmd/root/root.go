// Package root assembles the khbb command tree.
package root

import (
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	apiCmd "github.com/khipu/khbb/pkg/cmd/api"
	authCmd "github.com/khipu/khbb/pkg/cmd/auth"
	versionCmd "github.com/khipu/khbb/pkg/cmd/version"
)

// NewCmdRoot returns the top-level `khbb` command.
func NewCmdRoot(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "khbb <command> <subcommand> [flags]",
		Short:         "Bitbucket Cloud CLI",
		Long:          "Work with Bitbucket Cloud pull requests and pipelines from the command line.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       f.AppVersion,
		Args:          cobra.ArbitraryArgs,
		RunE:          cmdutil.GroupRunE,
	}
	cmd.SetVersionTemplate(versionCmd.Format(f.AppVersion, f.BuildCommit, f.BuildDate))
	cmd.SetOut(f.IOStreams.Out)
	cmd.SetErr(f.IOStreams.ErrOut)
	cmd.CompletionOptions.HiddenDefaultCmd = true
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &cmdutil.FlagError{Err: err}
	})

	cmd.AddCommand(
		apiCmd.NewCmdAPI(f, nil),
		authCmd.NewCmdAuth(f),
		versionCmd.NewCmdVersion(f),
	)
	return cmd
}
