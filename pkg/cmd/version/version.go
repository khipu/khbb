// Package version implements `khbb version`.
package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
)

// NewCmdVersion returns `khbb version`.
func NewCmdVersion(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the khbb version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprint(f.IOStreams.Out, Format(f.AppVersion, f.BuildCommit, f.BuildDate))
			return err
		},
	}
}

// Format renders the version line shared by `khbb version` and `khbb --version`.
func Format(version, commit, date string) string {
	s := "khbb version " + version
	switch {
	case commit != "" && date != "":
		s += fmt.Sprintf(" (%s, %s)", commit, date)
	case commit != "":
		s += fmt.Sprintf(" (%s)", commit)
	}
	return s + "\n"
}
