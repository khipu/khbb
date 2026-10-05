package config_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/khipu/khbb/internal/config"
)

func newConfig(t *testing.T) *config.Config {
	t.Helper()
	keyring.MockInit()
	t.Setenv("KHBB_TOKEN", "")
	t.Setenv("KHBB_EMAIL", "")
	cfg, err := config.LoadFile(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestResolve_EnvWins(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "stored@example.com"
	_ = config.StoreToken("stored@example.com", "from-keyring")
	t.Setenv("KHBB_TOKEN", "from-env")
	t.Setenv("KHBB_EMAIL", "env@example.com")

	got, err := config.ResolveCredentials(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := config.Credentials{Email: "env@example.com", Token: "from-env", Source: config.SourceEnv}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestResolve_EnvTokenFallsBackToConfigEmail(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "stored@example.com"
	t.Setenv("KHBB_TOKEN", "from-env")

	got, err := config.ResolveCredentials(cfg)
	if err != nil || got.Email != "stored@example.com" || got.Source != config.SourceEnv {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolve_EnvTokenWithoutEmailFails(t *testing.T) {
	cfg := newConfig(t)
	t.Setenv("KHBB_TOKEN", "from-env")
	if _, err := config.ResolveCredentials(cfg); err == nil {
		t.Fatal("expected an error when no email is available")
	}
}

func TestResolve_Keyring(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "dev@example.com"
	if err := config.StoreToken("dev@example.com", "s3cret"); err != nil {
		t.Fatal(err)
	}
	got, err := config.ResolveCredentials(cfg)
	if err != nil || got.Token != "s3cret" || got.Source != config.SourceKeyring {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolve_InsecureFile(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "dev@example.com"
	cfg.InsecureToken = "plain"
	got, err := config.ResolveCredentials(cfg)
	if err != nil || got.Token != "plain" || got.Source != config.SourceFile {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolve_None(t *testing.T) {
	cfg := newConfig(t)
	if _, err := config.ResolveCredentials(cfg); !errors.Is(err, config.ErrNoCredentials) {
		t.Errorf("expected ErrNoCredentials, got %v", err)
	}
	cfg.Email = "dev@example.com"
	if _, err := config.ResolveCredentials(cfg); !errors.Is(err, config.ErrNoCredentials) {
		t.Errorf("email without token: expected ErrNoCredentials, got %v", err)
	}
}

func TestDeleteToken(t *testing.T) {
	newConfig(t)
	if err := config.DeleteToken("nobody@example.com"); err != nil {
		t.Errorf("deleting a missing token must succeed: %v", err)
	}
	_ = config.StoreToken("dev@example.com", "s3cret")
	if err := config.DeleteToken("dev@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := keyring.Get(config.KeyringService, "dev@example.com"); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("token still present: %v", err)
	}
}
