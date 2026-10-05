package main

import (
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
