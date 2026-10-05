package main

import (
	"fmt"
	"os"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/root"
)

// Set with -ldflags at build time.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	ios := iostreams.System()
	f := &cmdutil.Factory{AppVersion: version, BuildCommit: commit, BuildDate: date, IOStreams: ios}
	if err := root.NewCmdRoot(f).Execute(); err != nil {
		fmt.Fprintln(ios.ErrOut, "error:", err)
		os.Exit(1)
	}
}
