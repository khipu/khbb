package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/skills"
)

func newOpts(t *testing.T) (*InstallOptions, string, func() string, func() string) {
	t.Helper()
	ios, _, out, errOut := iostreams.Test()
	home := t.TempDir()
	return &InstallOptions{
		IO:      ios,
		HomeDir: func() (string, error) { return home, nil },
		Content: "---\nname: khbb\n---\nskill v2\n",
	}, home, out.String, errOut.String
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInstall_WritesToTheClaudeSkillsDirectory(t *testing.T) {
	opts, home, out, errOut := newOpts(t)
	if err := installRun(opts); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "skills", "khbb", "SKILL.md")
	if got := readFile(t, path); got != opts.Content {
		t.Errorf("SKILL.md = %q", got)
	}
	if out() != "" {
		t.Errorf("stdout = %q, want nothing", out())
	}
	if want := "Installed the khbb skill at " + path; !strings.Contains(errOut(), want) {
		t.Errorf("stderr = %q, want %q", errOut(), want)
	}
}

func TestInstall_Dir(t *testing.T) {
	opts, _, _, _ := newOpts(t)
	opts.Dir = filepath.Join(t.TempDir(), "repo", ".claude", "skills", "khbb")
	if err := installRun(opts); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(opts.Dir, "SKILL.md")); got != opts.Content {
		t.Errorf("SKILL.md = %q", got)
	}
}

func TestInstall_AlreadyUpToDate(t *testing.T) {
	opts, _, _, errOut := newOpts(t)
	opts.Dir = t.TempDir()
	path := filepath.Join(opts.Dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(opts.Content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installRun(opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut(), "already up to date") {
		t.Errorf("stderr = %q", errOut())
	}
}

func TestInstall_KeepsADifferentFileWithoutForce(t *testing.T) {
	opts, _, _, _ := newOpts(t)
	opts.Dir = t.TempDir()
	path := filepath.Join(opts.Dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("my own notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := installRun(opts)
	var conflict *cmdutil.ConflictError
	if !errors.As(err, &conflict) || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("err = %v, want a conflict that mentions --force", err)
	}
	if got := readFile(t, path); got != "my own notes\n" {
		t.Errorf("SKILL.md was changed to %q", got)
	}
}

func TestInstall_ForceReplaces(t *testing.T) {
	opts, _, _, errOut := newOpts(t)
	opts.Dir, opts.Force = t.TempDir(), true
	path := filepath.Join(opts.Dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("skill v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installRun(opts); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != opts.Content {
		t.Errorf("SKILL.md = %q", got)
	}
	if !strings.Contains(errOut(), "Updated the khbb skill at "+path) {
		t.Errorf("stderr = %q", errOut())
	}
}

func TestInstall_NoHomeDirectory(t *testing.T) {
	opts, _, _, _ := newOpts(t)
	opts.HomeDir = func() (string, error) { return "", errors.New("$HOME is not defined") }
	if err := installRun(opts); err == nil || !strings.Contains(err.Error(), "use --dir") {
		t.Errorf("err = %v", err)
	}
}

func TestInstall_DirIsAFile(t *testing.T) {
	opts, _, _, _ := newOpts(t)
	opts.Dir = filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(opts.Dir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installRun(opts); err == nil {
		t.Error("expected an error when --dir is a file")
	}
}

func TestNewCmdInstall(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	var got *InstallOptions
	cmd := NewCmdInstall(&cmdutil.Factory{IOStreams: ios}, func(o *InstallOptions) error { got = o; return nil })
	cmd.SetArgs([]string{"--dir", "skills/khbb", "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Dir != "skills/khbb" || !got.Force {
		t.Errorf("opts = %+v", got)
	}
	if got.Content != skills.Khbb || !strings.HasPrefix(got.Content, "---\nname: khbb\n") {
		t.Errorf("the embedded skill must be installed, got %.40q", got.Content)
	}

	cmd = NewCmdInstall(&cmdutil.Factory{IOStreams: ios}, func(*InstallOptions) error { return nil })
	cmd.SetArgs([]string{"extra"})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	var flagErr *cmdutil.FlagError
	if err := cmd.Execute(); !errors.As(err, &flagErr) {
		t.Errorf("an argument must be a usage error, got %v", err)
	}
}
