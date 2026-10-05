package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/config"
)

func TestDir_Precedence(t *testing.T) {
	t.Setenv("KHBB_CONFIG_DIR", filepath.Join("x", "explicit"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join("x", "xdg"))
	if got, _ := config.Dir(); got != filepath.Join("x", "explicit") {
		t.Errorf("with KHBB_CONFIG_DIR: %q", got)
	}
	t.Setenv("KHBB_CONFIG_DIR", "")
	if got, _ := config.Dir(); got != filepath.Join("x", "xdg", "khbb") {
		t.Errorf("with XDG_CONFIG_HOME: %q", got)
	}
}

func TestLoadFile_MissingReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Email != "" || cfg.Path() != path {
		t.Errorf("unexpected config: %+v (path %q)", cfg, cfg.Path())
	}
}

func TestSave_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yml")
	cfg, _ := config.LoadFile(path)
	cfg.Email = "dev@example.com"
	cfg.Username = "ada"
	cfg.GitProtocol = "ssh"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "dev@example.com" || got.Username != "ada" || got.GitProtocol != "ssh" {
		t.Errorf("round trip lost data: %+v", got)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "token") {
		t.Errorf("token key written without insecure storage:\n%s", data)
	}
}

func TestSave_OverwritesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	cfg, _ := config.LoadFile(path)
	cfg.Email = "first@example.com"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg.Email = "second@example.com"
	if err := cfg.Save(); err != nil {
		t.Fatalf("second save failed: %v", err)
	}
	got, _ := config.LoadFile(path)
	if got.Email != "second@example.com" {
		t.Errorf("Email = %q", got.Email)
	}
}

func TestSave_FileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions only")
	}
	path := filepath.Join(t.TempDir(), "config.yml")
	cfg, _ := config.LoadFile(path)
	cfg.Email = "dev@example.com"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

func TestLoadFile_InvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("email: [unclosed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadFile(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("expected parse error naming the file, got %v", err)
	}
}
