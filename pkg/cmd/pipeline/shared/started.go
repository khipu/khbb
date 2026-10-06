package shared

import (
	"fmt"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// ReportStarted reports a pipeline that was just started: a confirmation on stderr, then its URL
// on stdout — or its JSON with --json.
func ReportStarted(ios *iostreams.IOStreams, exporter cmdutil.Exporter, p Pipeline, what string) error {
	prshared.PrintSuccess(ios, "Started pipeline #%d (%s)", p.Number, what)
	if exporter != nil {
		return exporter.Write(ios, p)
	}
	_, err := fmt.Fprintln(ios.Out, p.URL)
	return err
}
