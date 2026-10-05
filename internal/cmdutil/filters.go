package cmdutil

import (
	"io"
	"os"

	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/itchyny/gojq"
)

// ValidateJQ reports a usage error when expr is not a valid jq expression ("" is valid). Parsing
// alone accepts expressions that only fail at compile time (an unknown function, an undefined
// variable), so this also compiles the query with the same option go-gh uses.
func ValidateJQ(expr string) error {
	if expr == "" {
		return nil
	}
	query, err := gojq.Parse(expr)
	if err != nil {
		return FlagErrorf("invalid --jq expression: %v", err)
	}
	if _, err := gojq.Compile(query, gojq.WithEnvironLoader(os.Environ)); err != nil {
		return FlagErrorf("invalid --jq expression: %v", err)
	}
	return nil
}

// ValidateTemplate reports a usage error when tmpl is not a valid Go template ("" is valid).
func ValidateTemplate(tmpl string) error {
	if tmpl == "" {
		return nil
	}
	if err := template.New(io.Discard, 80, false).Parse(tmpl); err != nil {
		return FlagErrorf("invalid --template: %v", err)
	}
	return nil
}
