package logout

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/iostreams"
)

func newOpts(t *testing.T) (*LogoutOptions, *config.Config, *[]string, func() string) {
	t.Helper()
	t.Setenv("KHBB_TOKEN", "")
	ios, _, _, errOut := iostreams.Test()
	cfg, _ := config.LoadFile(filepath.Join(t.TempDir(), "config.yml"))
	var deleted []string
	return &LogoutOptions{
		IO:          ios,
		Config:      func() (*config.Config, error) { return cfg, nil },
		DeleteToken: func(email string) error { deleted = append(deleted, email); return nil },
	}, cfg, &deleted, errOut.String
}

func TestLogout_RemovesTokenAndClearsConfig(t *testing.T) {
	opts, cfg, deleted, errOut := newOpts(t)
	cfg.Email, cfg.Username, cfg.GitProtocol = "dev@example.com", "ada", "ssh"

	if err := logoutRun(opts); err != nil {
		t.Fatal(err)
	}
	if len(*deleted) != 1 || (*deleted)[0] != "dev@example.com" {
		t.Errorf("deleted = %v", *deleted)
	}
	saved, _ := config.LoadFile(cfg.Path())
	if saved.Email != "" || saved.Username != "" || saved.GitProtocol != "ssh" {
		t.Errorf("saved = %+v (preferences must be kept)", saved)
	}
	if !strings.Contains(errOut(), "Logged out of bitbucket.org (ada)") {
		t.Errorf("stderr = %q", errOut())
	}
}

func TestLogout_NotLoggedIn(t *testing.T) {
	opts, _, _, _ := newOpts(t)
	if err := logoutRun(opts); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("err = %v", err)
	}
}

func TestLogout_InsecureStorageIgnoresKeyringErrors(t *testing.T) {
	opts, cfg, _, _ := newOpts(t)
	cfg.Email, cfg.InsecureToken = "dev@example.com", "plain"
	opts.DeleteToken = func(string) error { return errors.New("no secret service") }

	if err := logoutRun(opts); err != nil {
		t.Fatal(err)
	}
	saved, _ := config.LoadFile(cfg.Path())
	if saved.InsecureToken != "" {
		t.Error("plain-text token must be removed")
	}
}

func TestLogout_KeyringErrorFails(t *testing.T) {
	opts, cfg, _, _ := newOpts(t)
	cfg.Email = "dev@example.com"
	opts.DeleteToken = func(string) error { return errors.New("no secret service") }
	if err := logoutRun(opts); err == nil || !strings.Contains(err.Error(), "keyring") {
		t.Errorf("err = %v", err)
	}
}

func TestLogout_WarnsWhenEnvTokenSet(t *testing.T) {
	opts, cfg, _, errOut := newOpts(t)
	cfg.Email = "dev@example.com"
	t.Setenv("KHBB_TOKEN", "from-env")
	if err := logoutRun(opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut(), "KHBB_TOKEN is still set") {
		t.Errorf("stderr = %q", errOut())
	}
}
