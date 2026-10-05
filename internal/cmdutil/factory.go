// Package cmdutil holds what every khbb command shares: the Factory, flag helpers and error types.
package cmdutil

import "github.com/khipu/khbb/internal/iostreams"

// Factory provides commands with their dependencies.
type Factory struct {
	AppVersion  string
	BuildCommit string
	BuildDate   string

	IOStreams *iostreams.IOStreams
}
