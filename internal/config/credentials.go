package config

import (
	"errors"
	"os"

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

// ResolveCredentials finds credentials in KHBB_TOKEN/KHBB_EMAIL, then the keyring, then the config file.
func ResolveCredentials(cfg *Config) (Credentials, error) {
	if token := os.Getenv("KHBB_TOKEN"); token != "" {
		email := os.Getenv("KHBB_EMAIL")
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
	// Keyring errors (for example no Secret Service on a headless Linux box) fall through to the file.
	if token, err := keyring.Get(KeyringService, cfg.Email); err == nil && token != "" {
		return Credentials{Email: cfg.Email, Token: token, Source: SourceKeyring}, nil
	}
	if cfg.InsecureToken != "" {
		return Credentials{Email: cfg.Email, Token: cfg.InsecureToken, Source: SourceFile}, nil
	}
	return Credentials{}, ErrNoCredentials
}

// StoreToken saves token in the system keyring.
func StoreToken(email, token string) error {
	return keyring.Set(KeyringService, email, token)
}

// DeleteToken removes the keyring entry for email. A missing entry is not an error.
func DeleteToken(email string) error {
	err := keyring.Delete(KeyringService, email)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
