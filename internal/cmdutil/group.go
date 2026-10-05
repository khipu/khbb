package cmdutil

import "github.com/spf13/cobra"

// GroupRunE makes a command group fail on an unknown subcommand instead of printing help and succeeding.
func GroupRunE(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return FlagErrorf("unknown command %q for %q", args[0], cmd.CommandPath())
	}
	return cmd.Help()
}
