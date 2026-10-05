package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// KeyringService is the system keyring service under which tokens are stored (account = email).
const KeyringService = "khbb:bitbucket.org"

// TokenSource says where a token came from.
type TokenSource string

const (
	SourceEnv     TokenSource = "env"
	SourceKeyring TokenSource = "keyring"
	SourceFile    TokenSource = "file"
)

// Credentials authenticate API requests.
type Credentials struct {
	Email  string
	Token  string
	Source TokenSource
}

// ErrNoCredentials means the user is not logged in.
var ErrNoCredentials = errors.New("no credentials found")

// ErrKeyringTimeout means a keyring call did not return within keyringTimeout. The
// system keyring (go-keyring's Linux backend in particular) can block indefinitely
// waiting for an unlock prompt that will never come, such as over SSH or in WSL.
var ErrKeyringTimeout = errors.New("timed out waiting for the system keyring")

// Indirections over the keyring package so tests can replace them without touching the
// real OS keyring.
var (
	keyringGet     = keyring.Get
	keyringSet     = keyring.Set
	keyringDelete  = keyring.Delete
	keyringTimeout = 3 * time.Second
)

// withKeyringTimeout runs call in a goroutine and returns ErrKeyringTimeout if it has
// not finished within keyringTimeout. The goroutine is left to finish (or hang) on its
// own; the result channel is buffered so it never blocks.
func withKeyringTimeout(call func() error) error {
	done := make(chan error, 1)
	go func() { done <- call() }()
	select {
	case err := <-done:
		return err
	case <-time.After(keyringTimeout):
		return ErrKeyringTimeout
	}
}

// ResolveCredentials finds credentials in KHBB_TOKEN/KHBB_EMAIL, then the file token
// written by the latest `auth login --insecure-storage`, then the system keyring.
func ResolveCredentials(cfg *Config) (Credentials, error) {
	if token := strings.TrimSpace(os.Getenv("KHBB_TOKEN")); token != "" {
		email := strings.TrimSpace(os.Getenv("KHBB_EMAIL"))
		if email == "" {
			email = cfg.Email
		}
		if email == "" {
			return Credentials{}, errors.New("KHBB_EMAIL must be set when KHBB_TOKEN is set")
		}
		return Credentials{Email: email, Token: token, Source: SourceEnv}, nil
	}
	if cfg.Email == "" {
		return Credentials{}, ErrNoCredentials
	}
	// A file token is only written by the latest --insecure-storage login (a keyring
	// login clears it), so this avoids touching the keyring at all for those users.
	if cfg.InsecureToken != "" {
		return Credentials{Email: cfg.Email, Token: cfg.InsecureToken, Source: SourceFile}, nil
	}
	var token string
	err := withKeyringTimeout(func() error {
		t, err := keyringGet(KeyringService, cfg.Email)
		token = t
		return err
	})
	if err == nil && token != "" {
		return Credentials{Email: cfg.Email, Token: token, Source: SourceKeyring}, nil
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return Credentials{}, fmt.Errorf("could not read the token from the system keyring: %w; set KHBB_EMAIL and KHBB_TOKEN, or run `khbb auth login --insecure-storage`", err)
	}
	return Credentials{}, ErrNoCredentials
}

// StoreToken saves token in the system keyring.
func StoreToken(email, token string) error {
	return withKeyringTimeout(func() error { return keyringSet(KeyringService, email, token) })
}

// DeleteToken removes the keyring entry for email. A missing entry is not an error.
func DeleteToken(email string) error {
	err := withKeyringTimeout(func() error { return keyringDelete(KeyringService, email) })
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
