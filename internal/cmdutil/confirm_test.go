package cmdutil_test

import (
	"errors"
	"testing"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
)

func TestConfirmDestructive_YesSkipsPrompt(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if err := cmdutil.ConfirmDestructive(ios, nil, true, "Merge #42?"); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmDestructive_NoTerminalRequiresYes(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if err := cmdutil.ConfirmDestructive(ios, nil, false, "Merge #42?"); !errors.Is(err, cmdutil.ErrConfirmationRequired) {
		t.Fatalf("expected ErrConfirmationRequired, got %v", err)
	}
}

func TestConfirmDestructive_NeverPrompt(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	ios.SetNeverPrompt(true)
	if err := cmdutil.ConfirmDestructive(ios, prompter.NewMock(t), false, "Merge #42?"); !errors.Is(err, cmdutil.ErrConfirmationRequired) {
		t.Fatalf("expected ErrConfirmationRequired, got %v", err)
	}
}

func TestConfirmDestructive_Prompt(t *testing.T) {
	for _, answer := range []bool{true, false} {
		ios, _, _, _ := iostreams.Test()
		ios.SetStdinTTY(true)
		ios.SetStdoutTTY(true)
		pm := prompter.NewMock(t)
		pm.RegisterConfirm("Merge #42?", func(_ string, defaultValue bool) (bool, error) {
			if defaultValue {
				t.Error("the default answer must be No")
			}
			return answer, nil
		})
		err := cmdutil.ConfirmDestructive(ios, pm, false, "Merge #42?")
		if answer && err != nil {
			t.Errorf("accepted: %v", err)
		}
		if !answer && !errors.Is(err, cmdutil.ErrCancel) {
			t.Errorf("declined: expected ErrCancel, got %v", err)
		}
	}
}
