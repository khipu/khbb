package cmdutil_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
)

func TestGroupRunE_UnknownSubcommandIsFlagError(t *testing.T) {
	cmd := &cobra.Command{Use: "auth"}
	err := cmdutil.GroupRunE(cmd, []string{"stauts"})
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) {
		t.Fatalf("expected *FlagError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "stauts") {
		t.Errorf("error should mention the unknown subcommand, got %q", err.Error())
	}
}

func TestGroupRunE_NoArgsShowsHelp(t *testing.T) {
	var helped bool
	cmd := &cobra.Command{
		Use: "auth",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	cmd.SetHelpFunc(func(*cobra.Command, []string) { helped = true })
	if err := cmdutil.GroupRunE(cmd, nil); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !helped {
		t.Error("expected help to be shown")
	}
}
