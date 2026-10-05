package config

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func newConfig(t *testing.T) *Config {
	t.Helper()
	keyring.MockInit()
	t.Setenv("KHBB_TOKEN", "")
	t.Setenv("KHBB_EMAIL", "")
	cfg, err := LoadFile(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// blockingKeyring replaces keyringGet/keyringSet/keyringDelete with functions that
// block until the test ends, and shortens keyringTimeout so tests run fast. Production
// code abandons the goroutine calling these on a timeout, so Cleanup must wait for it to
// actually return before restoring the package vars underneath it - otherwise the
// restore races with whichever stub is still running.
//
// expectCall says whether the code under test will actually invoke one of the three
// stubs. The wait group's Add must happen here, synchronously, before the caller goes on
// to invoke ResolveCredentials/StoreToken/DeleteToken (and so before the goroutine that
// might call a stub even exists): adding from inside the stub instead would race with
// Cleanup's Wait, since nothing orders "the abandoned goroutine reaches the stub" before
// "Cleanup runs" other than wall-clock timing, which the race detector rightly does not
// trust. Callers must not use t.Parallel(): only one such swap can be live at a time.
func blockingKeyring(t *testing.T, expectCall bool) {
	t.Helper()
	origGet, origSet, origDelete, origTimeout := keyringGet, keyringSet, keyringDelete, keyringTimeout
	block := make(chan struct{})
	var wg sync.WaitGroup
	if expectCall {
		wg.Add(1)
	}
	done := func() {
		if expectCall {
			wg.Done()
		}
	}
	keyringTimeout = 50 * time.Millisecond
	keyringGet = func(string, string) (string, error) {
		defer done()
		<-block
		return "", nil
	}
	keyringSet = func(string, string, string) error {
		defer done()
		<-block
		return nil
	}
	keyringDelete = func(string, string) error {
		defer done()
		<-block
		return nil
	}
	t.Cleanup(func() {
		close(block)
		wg.Wait()
		keyringGet, keyringSet, keyringDelete, keyringTimeout = origGet, origSet, origDelete, origTimeout
	})
}

func TestResolve_EnvWins(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "stored@example.com"
	_ = StoreToken("stored@example.com", "from-keyring")
	t.Setenv("KHBB_TOKEN", "from-env")
	t.Setenv("KHBB_EMAIL", "env@example.com")

	got, err := ResolveCredentials(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := Credentials{Email: "env@example.com", Token: "from-env", Source: SourceEnv}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestResolve_EnvTokenFallsBackToConfigEmail(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "stored@example.com"
	t.Setenv("KHBB_TOKEN", "from-env")

	got, err := ResolveCredentials(cfg)
	if err != nil || got.Email != "stored@example.com" || got.Source != SourceEnv {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolve_EnvTokenWithoutEmailFails(t *testing.T) {
	cfg := newConfig(t)
	t.Setenv("KHBB_TOKEN", "from-env")
	if _, err := ResolveCredentials(cfg); err == nil {
		t.Fatal("expected an error when no email is available")
	}
}

func TestResolve_TrimsEnvValues(t *testing.T) {
	cfg := newConfig(t)
	t.Setenv("KHBB_TOKEN", "s3cret\r\n")
	t.Setenv("KHBB_EMAIL", " dev@example.com ")

	got, err := ResolveCredentials(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "s3cret" || got.Email != "dev@example.com" {
		t.Errorf("got %+v", got)
	}
}

func TestResolve_Keyring(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "dev@example.com"
	if err := StoreToken("dev@example.com", "s3cret"); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveCredentials(cfg)
	if err != nil || got.Token != "s3cret" || got.Source != SourceKeyring {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolve_InsecureFile(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "dev@example.com"
	cfg.InsecureToken = "plain"
	got, err := ResolveCredentials(cfg)
	if err != nil || got.Token != "plain" || got.Source != SourceFile {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolve_PrefersFileTokenOverKeyring(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "dev@example.com"
	cfg.InsecureToken = "plain"
	if err := StoreToken("dev@example.com", "from-keyring"); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveCredentials(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != SourceFile || got.Token != "plain" {
		t.Errorf("got %+v, want file token", got)
	}
}

func TestResolve_FileTokenSkipsKeyring(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "dev@example.com"
	cfg.InsecureToken = "plain"
	blockingKeyring(t, false) // the file token must short-circuit before any keyring call

	got, err := ResolveCredentials(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != SourceFile || got.Token != "plain" {
		t.Errorf("got %+v, want an immediate file token", got)
	}
}

func TestResolve_None(t *testing.T) {
	cfg := newConfig(t)
	if _, err := ResolveCredentials(cfg); !errors.Is(err, ErrNoCredentials) {
		t.Errorf("expected ErrNoCredentials, got %v", err)
	}
	cfg.Email = "dev@example.com"
	if _, err := ResolveCredentials(cfg); !errors.Is(err, ErrNoCredentials) {
		t.Errorf("email without token: expected ErrNoCredentials, got %v", err)
	}
}

func TestResolve_KeyringNotFoundIsNoCredentials(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "dev@example.com"
	orig := keyringGet
	t.Cleanup(func() { keyringGet = orig })
	keyringGet = func(string, string) (string, error) { return "", keyring.ErrNotFound }

	if _, err := ResolveCredentials(cfg); !errors.Is(err, ErrNoCredentials) {
		t.Errorf("got %v, want ErrNoCredentials", err)
	}
}

func TestResolve_KeyringErrorSurfacesInsteadOfNoCredentials(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "dev@example.com"
	orig := keyringGet
	t.Cleanup(func() { keyringGet = orig })
	keyringGet = func(string, string) (string, error) { return "", errors.New("dbus: no session") }

	_, err := ResolveCredentials(cfg)
	if err == nil || !strings.Contains(err.Error(), "system keyring") {
		t.Errorf("err = %v, want it to mention the system keyring", err)
	}
	if errors.Is(err, ErrNoCredentials) {
		t.Error("a keyring error must not be reported as ErrNoCredentials")
	}
}

func TestResolve_KeyringTimeoutSurfacesError(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "dev@example.com"
	blockingKeyring(t, true)

	_, err := ResolveCredentials(cfg)
	if !errors.Is(err, ErrKeyringTimeout) {
		t.Errorf("err = %v, want it to wrap ErrKeyringTimeout", err)
	}
	if errors.Is(err, ErrNoCredentials) {
		t.Error("a keyring timeout must not be reported as ErrNoCredentials")
	}
}

func TestStoreToken_KeyringTimeout(t *testing.T) {
	newConfig(t)
	blockingKeyring(t, true)
	if err := StoreToken("dev@example.com", "s3cret"); !errors.Is(err, ErrKeyringTimeout) {
		t.Errorf("err = %v, want ErrKeyringTimeout", err)
	}
}

func TestDeleteToken_KeyringTimeout(t *testing.T) {
	newConfig(t)
	blockingKeyring(t, true)
	if err := DeleteToken("dev@example.com"); !errors.Is(err, ErrKeyringTimeout) {
		t.Errorf("err = %v, want ErrKeyringTimeout", err)
	}
}

func TestDeleteToken(t *testing.T) {
	newConfig(t)
	if err := DeleteToken("nobody@example.com"); err != nil {
		t.Errorf("deleting a missing token must succeed: %v", err)
	}
	_ = StoreToken("dev@example.com", "s3cret")
	if err := DeleteToken("dev@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := keyring.Get(KeyringService, "dev@example.com"); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("token still present: %v", err)
	}
}
