package iostreams_test

import (
	"testing"

	"github.com/khipu/khbb/internal/iostreams"
)

func TestColors(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if got := ios.Green("ok"); got != "ok" {
		t.Errorf("disabled: %q", got)
	}
	ios.SetColorEnabled(true)
	if got := ios.Green("ok"); got != "\x1b[32mok\x1b[m" {
		t.Errorf("enabled: %q", got)
	}
	if got := ios.Bold(""); got != "" {
		t.Errorf("empty text must stay empty: %q", got)
	}
}
