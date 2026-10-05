package cmdutil

import "github.com/spf13/cobra"

// EnableRepoOverride adds a persistent -R/--repo flag bound to f.RepoOverride.
func EnableRepoOverride(cmd *cobra.Command, f *Factory) {
	cmd.PersistentFlags().StringVarP(&f.RepoOverride, "repo", "R", "", "Select a repository using the `WORKSPACE/REPO` format")
}

// AddDryRunFlag adds --dry-run bound to f.DryRun.
func AddDryRunFlag(cmd *cobra.Command, f *Factory) {
	cmd.Flags().BoolVar(&f.DryRun, "dry-run", false, "Print the request that would be sent without sending it")
}

// AddYesFlag adds --yes for destructive commands.
func AddYesFlag(cmd *cobra.Command, yes *bool) {
	cmd.Flags().BoolVar(yes, "yes", false, "Skip the confirmation prompt")
}
