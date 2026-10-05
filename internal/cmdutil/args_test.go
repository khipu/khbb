package cmdutil_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
)

func runArgs(validator cobra.PositionalArgs, args ...string) error {
	cmd := &cobra.Command{Use: "thing", Args: validator, RunE: func(*cobra.Command, []string) error { return nil }}
	// A non-nil slice: cobra falls back to os.Args when SetArgs receives nil.
	cmd.SetArgs(append([]string{}, args...))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestArgValidatorsReturnUsageErrors(t *testing.T) {
	cases := []struct {
		name      string
		validator cobra.PositionalArgs
		args      []string
		wantErr   string // substring; "" means success
	}{
		{"no args ok", cmdutil.NoArgs, nil, ""},
		{"no args extra", cmdutil.NoArgs, []string{"x"}, `unexpected argument "x" for "thing"`},
		{"exact ok", cmdutil.ExactArgs(1, "<path>"), []string{"user"}, ""},
		{"exact missing", cmdutil.ExactArgs(1, "<path>"), nil, "thing requires <path>"},
		{"exact extra", cmdutil.ExactArgs(1, "<path>"), []string{"a", "b"}, "thing accepts 1 argument(s) (<path>), received 2"},
		{"max ok", cmdutil.MaximumNArgs(1, "[<number>]"), nil, ""},
		{"max extra", cmdutil.MaximumNArgs(1, "[<number>]"), []string{"1", "2"}, "thing accepts at most 1 argument(s) ([<number>]), received 2"},
	}
	for _, tc := range cases {
		err := runArgs(tc.validator, tc.args...)
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", tc.name, err)
			}
			continue
		}
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: err = %v, want a FlagError containing %q", tc.name, err, tc.wantErr)
		}
	}
}
