package main

import (
	"os"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/factory"
	"github.com/khipu/khbb/pkg/cmd/root"
)

// Set with -ldflags at build time; buildVersion fills in what is missing.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	info, _ := debug.ReadBuildInfo()
	f := factory.New(buildVersion(version, commit, date, info))
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

// buildVersion completes the version details that -ldflags did not set with the build information
// the Go toolchain embeds: the module version for `go install …@v1.2.3`, and the commit and its
// time for a build from a git checkout. The leading "v" of a tag is dropped, so every kind of build
// prints "khbb version 1.2.3".
func buildVersion(version, commit, date string, info *debug.BuildInfo) (string, string, string) {
	if info != nil {
		if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
		for _, s := range info.Settings {
			switch {
			case s.Key == "vcs.revision" && commit == "":
				commit = s.Value
				if len(commit) > 7 {
					commit = commit[:7]
				}
			case s.Key == "vcs.time" && date == "":
				date = s.Value
			}
		}
	}
	return strings.TrimPrefix(version, "v"), commit, date
}
