package version_test

import (
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/version"
)

func TestFormat(t *testing.T) {
	cases := []struct{ version, commit, date, want string }{
		{"1.2.3", "abc123", "2026-10-05T00:00:00Z", "khbb version 1.2.3 (abc123, 2026-10-05T00:00:00Z)\n"},
		{"1.2.3", "abc123", "", "khbb version 1.2.3 (abc123)\n"},
		{"dev", "", "", "khbb version dev\n"},
	}
	for _, tc := range cases {
		if got := version.Format(tc.version, tc.commit, tc.date); got != tc.want {
			t.Errorf("Format(%q, %q, %q) = %q, want %q", tc.version, tc.commit, tc.date, got, tc.want)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	cmd := version.NewCmdVersion(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "khbb version 1.2.3\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
