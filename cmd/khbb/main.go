package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/factory"
	"github.com/khipu/khbb/pkg/cmd/root"
)

// Set with -ldflags at build time.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	f := factory.New(version, commit, date)
	rootCmd := root.NewCmdRoot(f)
	rootCmd.SetArgs(args)
	cmd, err := rootCmd.ExecuteC()
	if err != nil {
		return cmdutil.PrintError(f.IOStreams, err, wantsJSON(cmd))
	}
	return 0
}

// wantsJSON reports whether the command that failed was asked for JSON output.
func wantsJSON(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	fl := cmd.Flags().Lookup("json")
	return fl != nil && fl.Changed
}
