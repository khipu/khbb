package status

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/internal/iostreams"
)

func newOpts(t *testing.T, creds config.Credentials, credsErr error) (*StatusOptions, *httpmock.Registry, func() string) {
	t.Helper()
	ios, _, out, _ := iostreams.Test()
	cfg, _ := config.LoadFile(filepath.Join(t.TempDir(), "config.yml"))
	reg := httpmock.New(t)
	return &StatusOptions{
		IO:     ios,
		Config: func() (*config.Config, error) { return cfg, nil },
		NewClient: func(email, token string) *bitbucket.Client {
			return bitbucket.New(bitbucket.Options{Email: email, Token: token, HTTPClient: reg.Client()})
		},
		Resolve: func(*config.Config) (config.Credentials, error) { return creds, credsErr },
	}, reg, out.String
}

var keyringCreds = config.Credentials{Email: "dev@example.com", Token: "s3cret", Source: config.SourceKeyring}

func TestStatus_LoggedIn(t *testing.T) {
	opts, reg, out := newOpts(t, keyringCreds, nil)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, `{"display_name":"Ada Example","nickname":"ada"}`))

	if err := statusRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	want := "bitbucket.org\n  Logged in as ada (Ada Example)\n  Email: dev@example.com\n  Token source: keyring\n"
	if out() != want {
		t.Errorf("out = %q, want %q", out(), want)
	}
}

func TestStatus_NotLoggedIn(t *testing.T) {
	opts, _, _ := newOpts(t, config.Credentials{}, config.ErrNoCredentials)
	err := statusRun(context.Background(), opts)
	var authErr *cmdutil.AuthError
	if !errors.As(err, &authErr) || cmdutil.Classify(err).Exit != 4 {
		t.Errorf("err = %v", err)
	}
}

func TestStatus_InvalidToken(t *testing.T) {
	opts, reg, _ := newOpts(t, keyringCreds, nil)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(401, `{}`))
	err := statusRun(context.Background(), opts)
	var authErr *cmdutil.AuthError
	if !errors.As(err, &authErr) || !strings.Contains(err.Error(), "keyring") {
		t.Errorf("err = %v", err)
	}
}

func TestStatus_CredentialErrorPassesThrough(t *testing.T) {
	boom := errors.New("KHBB_EMAIL must be set when KHBB_TOKEN is set")
	opts, _, _ := newOpts(t, config.Credentials{}, boom)
	if err := statusRun(context.Background(), opts); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
}
