package iostreams_test

import (
	"testing"

	"github.com/khipu/khbb/internal/iostreams"
)

func TestCanPrompt(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if ios.CanPrompt() {
		t.Fatal("test streams must not prompt by default")
	}
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	if !ios.CanPrompt() {
		t.Fatal("expected prompting with stdin and stdout on a terminal")
	}
	ios.SetNeverPrompt(true)
	if ios.CanPrompt() {
		t.Fatal("SetNeverPrompt must disable prompting")
	}
}

func TestCanPrompt_RequiresStdinTTY(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	if ios.CanPrompt() {
		t.Fatal("piped stdin must not prompt")
	}
}

func TestTerminalWidthDefault(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if got := ios.TerminalWidth(); got != 80 {
		t.Fatalf("TerminalWidth() = %d, want 80", got)
	}
}
