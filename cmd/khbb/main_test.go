package main

import (
	"runtime/debug"
	"testing"

	"github.com/spf13/cobra"
)

func TestWantsJSON(t *testing.T) {
	if wantsJSON(nil) {
		t.Error("nil command")
	}
	cmd := &cobra.Command{Use: "x"}
	if wantsJSON(cmd) {
		t.Error("no --json flag registered")
	}
	cmd.Flags().StringSlice("json", nil, "")
	if wantsJSON(cmd) {
		t.Error("--json not set")
	}
	if err := cmd.Flags().Set("json", "id"); err != nil {
		t.Fatal(err)
	}
	if !wantsJSON(cmd) {
		t.Error("--json set")
	}
}

func TestBuildVersion(t *testing.T) {
	vcs := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "64b0e38c0ffee0123456789abcdef0123456789a"},
		{Key: "vcs.time", Value: "2026-10-06T12:00:00Z"},
	}
	cases := []struct {
		name                  string
		version, commit, date string
		info                  *debug.BuildInfo
		want                  [3]string
	}{
		{"ldflags win", "v1.2.3", "abc1234", "2026-10-01T00:00:00Z",
			&debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}, Settings: vcs},
			[3]string{"1.2.3", "abc1234", "2026-10-01T00:00:00Z"}},
		{"goreleaser", "1.2.3", "abc1234", "2026-10-01T00:00:00Z", nil,
			[3]string{"1.2.3", "abc1234", "2026-10-01T00:00:00Z"}},
		{"go install", "dev", "", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0"}},
			[3]string{"0.2.0", "", ""}},
		{"git checkout", "dev", "", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: vcs},
			[3]string{"dev", "64b0e38", "2026-10-06T12:00:00Z"}},
		{"no build info", "dev", "", "", nil, [3]string{"dev", "", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, c, d := buildVersion(tc.version, tc.commit, tc.date, tc.info)
			if got := [3]string{v, c, d}; got != tc.want {
				t.Errorf("buildVersion = %q, want %q", got, tc.want)
			}
		})
	}
}
