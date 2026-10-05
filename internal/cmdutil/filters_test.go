package cmdutil_test

import (
	"errors"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
)

func TestValidateJQ(t *testing.T) {
	for _, ok := range []string{"", ".", ".values[].id", `.[] | select(.state == "OPEN") | .title`} {
		if err := cmdutil.ValidateJQ(ok); err != nil {
			t.Errorf("ValidateJQ(%q) = %v", ok, err)
		}
	}
	var flagErr *cmdutil.FlagError
	if err := cmdutil.ValidateJQ(".["); !errors.As(err, &flagErr) {
		t.Errorf("ValidateJQ(.[) = %v, want FlagError", err)
	}
}

func TestValidateTemplate(t *testing.T) {
	for _, ok := range []string{"", `{{range .}}{{.id}}{{"\n"}}{{end}}`, `{{tablerow "a" "b"}}{{tablerender}}`} {
		if err := cmdutil.ValidateTemplate(ok); err != nil {
			t.Errorf("ValidateTemplate(%q) = %v", ok, err)
		}
	}
	var flagErr *cmdutil.FlagError
	if err := cmdutil.ValidateTemplate("{{"); !errors.As(err, &flagErr) {
		t.Errorf("ValidateTemplate({{) = %v, want FlagError", err)
	}
}
