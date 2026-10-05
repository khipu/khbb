package factory

import "testing"

func TestResolveLauncher(t *testing.T) {
	t.Setenv("BROWSER", "firefox")
	if got := resolveLauncher(""); got != "firefox" {
		t.Errorf("from $BROWSER: %q", got)
	}
	if got := resolveLauncher("open -a Safari"); got != "open -a Safari" {
		t.Errorf("config wins: %q", got)
	}
	t.Setenv("BROWSER", "")
	if got := resolveLauncher(""); got != "" {
		t.Errorf("system default expected, got %q", got)
	}
}
