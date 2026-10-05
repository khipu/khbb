package root_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/root"
)

func TestRootVersionFlag(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "khbb version 1.2.3") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRootUnknownFlagIsUsageError(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{"--nope"})
	err := cmd.Execute()
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) {
		t.Fatalf("expected FlagError, got %T: %v", err, err)
	}
}

func TestRootMistypedSubcommandIsUsageError(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{"auth", "stauts"})
	err := cmd.Execute()
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) {
		t.Fatalf("expected FlagError, got %T: %v", err, err)
	}
}

func TestRootUnknownCommandIsUsageError(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{"nope"})
	err := cmd.Execute()
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), `unknown command "nope" for "khbb"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestRootWithoutArgumentsPrintsHelp(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("out = %q", out.String())
	}
}

func TestArgumentCountErrorsAreUsageErrors(t *testing.T) {
	for _, args := range [][]string{{"version", "extra"}, {"api"}, {"auth", "status", "extra"}} {
		ios, _, _, _ := iostreams.Test()
		cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
		cmd.SetArgs(args)
		var flagErr *cmdutil.FlagError
		if err := cmd.Execute(); !errors.As(err, &flagErr) {
			t.Errorf("%v: expected FlagError, got %T: %v", args, err, err)
		}
	}
}
