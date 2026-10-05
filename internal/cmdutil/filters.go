package cmdutil

import (
	"io"

	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/itchyny/gojq"
)

// ValidateJQ reports a usage error when expr is not a valid jq expression ("" is valid).
func ValidateJQ(expr string) error {
	if expr == "" {
		return nil
	}
	if _, err := gojq.Parse(expr); err != nil {
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
