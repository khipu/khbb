package cmdutil

import "github.com/spf13/cobra"

// NoArgs rejects positional arguments as a usage error.
func NoArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return FlagErrorf("unexpected argument %q for %q", args[0], cmd.CommandPath())
	}
	return nil
}

// ExactArgs requires exactly n positional arguments; what names them in messages (for example "<path>").
func ExactArgs(n int, what string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		switch {
		case len(args) < n:
			return FlagErrorf("%s requires %s", cmd.CommandPath(), what)
		case len(args) > n:
			return FlagErrorf("%s accepts %d argument(s) (%s), received %d", cmd.CommandPath(), n, what, len(args))
		}
		return nil
	}
}

// MaximumNArgs allows at most n positional arguments; what names them in messages.
func MaximumNArgs(n int, what string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return FlagErrorf("%s accepts at most %d argument(s) (%s), received %d", cmd.CommandPath(), n, what, len(args))
		}
		return nil
	}
}
