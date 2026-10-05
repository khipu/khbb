package login

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/internal/iostreams"
)

const userJSON = `{"display_name":"Ada Example","nickname":"ada","uuid":"{00000000-0000-0000-0000-000000000001}","account_id":"000000:aaaa"}`

type fixture struct {
	opts    *LoginOptions
	ios     *iostreams.IOStreams
	cfg     *config.Config
	reg     *httpmock.Registry
	stdin   *bytes.Buffer
	stderr  *bytes.Buffer
	stored  map[string]string
	deleted []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("KHBB_TOKEN", "")
	ios, stdin, _, stderr := iostreams.Test()
	cfg, err := config.LoadFile(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	fx := &fixture{ios: ios, cfg: cfg, reg: httpmock.New(t), stdin: stdin, stderr: stderr, stored: map[string]string{}}
	fx.opts = &LoginOptions{
		IO:     ios,
		Config: func() (*config.Config, error) { return cfg, nil },
		NewClient: func(email, token string) *bitbucket.Client {
			return bitbucket.New(bitbucket.Options{Email: email, Token: token, HTTPClient: fx.reg.Client()})
		},
		StoreToken:  func(email, token string) error { fx.stored[email] = token; return nil },
		DeleteToken: func(email string) error { fx.deleted = append(fx.deleted, email); return nil },
	}
	return fx
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestNewCmdLogin_ParsesFlags(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	var got *LoginOptions
	cmd := NewCmdLogin(&cmdutil.Factory{IOStreams: ios}, func(o *LoginOptions) error { got = o; return nil })
	cmd.SetArgs([]string{"--email", "dev@example.com", "--with-token", "--insecure-storage"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Email != "dev@example.com" || !got.WithToken || !got.InsecureStorage {
		t.Errorf("parsed %+v", got)
	}
}

func TestLogin_WithTokenFromStdin(t *testing.T) {
	fx := newFixture(t)
	fx.stdin.WriteString("s3cret\r\n")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if err := loginRun(context.Background(), fx.opts); err != nil {
		t.Fatal(err)
	}
	if fx.stored["dev@example.com"] != "s3cret" {
		t.Errorf("stored = %v", fx.stored)
	}
	user, pass, _ := (&http.Request{Header: fx.reg.Calls[0].Header}).BasicAuth()
	if user != "dev@example.com" || pass != "s3cret" {
		t.Errorf("validated with %q/%q", user, pass)
	}
	saved, _ := config.LoadFile(fx.cfg.Path())
	if saved.Email != "dev@example.com" || saved.Username != "ada" || saved.InsecureToken != "" {
		t.Errorf("saved config = %+v", saved)
	}
	if !strings.Contains(fx.stderr.String(), "Logged in to bitbucket.org as ada") {
		t.Errorf("stderr = %q", fx.stderr.String())
	}
}

func TestLogin_NonInteractiveRequiresEmail(t *testing.T) {
	fx := newFixture(t)
	fx.opts.WithToken = true
	err := loginRun(context.Background(), fx.opts)
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "--email") {
		t.Errorf("err = %v", err)
	}
}

func TestLogin_NonInteractiveRequiresWithToken(t *testing.T) {
	fx := newFixture(t)
	fx.opts.Email = "dev@example.com"
	err := loginRun(context.Background(), fx.opts)
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "--with-token") {
		t.Errorf("err = %v", err)
	}
}

func TestLogin_InteractivePromptsAndListsScopes(t *testing.T) {
	fx := newFixture(t)
	fx.ios.SetStdinTTY(true)
	fx.ios.SetStdoutTTY(true)
	pm := prompter.NewMock(t)
	pm.RegisterInput("Atlassian account email:", func(_, _ string) (string, error) { return " dev@example.com ", nil })
	pm.RegisterPassword("API token:", func(string) (string, error) { return "s3cret", nil })
	fx.opts.Prompter = pm
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if err := loginRun(context.Background(), fx.opts); err != nil {
		t.Fatal(err)
	}
	if fx.stored["dev@example.com"] != "s3cret" {
		t.Errorf("stored = %v", fx.stored)
	}
	for _, want := range []string{tokenURL, "write:pipeline:bitbucket", "read:workspace:bitbucket"} {
		if !strings.Contains(fx.stderr.String(), want) {
			t.Errorf("stderr missing %q:\n%s", want, fx.stderr.String())
		}
	}
}

func TestLogin_RejectsInvalidToken(t *testing.T) {
	fx := newFixture(t)
	fx.stdin.WriteString("wrong")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(401, `{"type":"error","error":{"message":"Unauthorized"}}`))

	err := loginRun(context.Background(), fx.opts)
	var authErr *cmdutil.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthError, got %v", err)
	}
	if len(fx.stored) != 0 || fileExists(fx.cfg.Path()) {
		t.Error("an invalid token must not be stored")
	}
}

func TestLogin_KeyringFailureSuggestsAlternatives(t *testing.T) {
	fx := newFixture(t)
	fx.stdin.WriteString("s3cret")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.opts.StoreToken = func(string, string) error { return errors.New("no secret service") }
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	err := loginRun(context.Background(), fx.opts)
	if err == nil || !strings.Contains(err.Error(), "--insecure-storage") || !strings.Contains(err.Error(), "KHBB_TOKEN") {
		t.Fatalf("err = %v", err)
	}
	if fileExists(fx.cfg.Path()) {
		t.Error("config must not be written when storing the token failed")
	}
}

func TestLogin_InsecureStorage(t *testing.T) {
	fx := newFixture(t)
	fx.stdin.WriteString("s3cret")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.opts.InsecureStorage = true
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if err := loginRun(context.Background(), fx.opts); err != nil {
		t.Fatal(err)
	}
	if len(fx.stored) != 0 {
		t.Error("keyring must not be used with --insecure-storage")
	}
	saved, _ := config.LoadFile(fx.cfg.Path())
	if saved.InsecureToken != "s3cret" {
		t.Errorf("InsecureToken = %q", saved.InsecureToken)
	}
	if !strings.Contains(fx.stderr.String(), "plain text") {
		t.Errorf("missing plain-text warning: %q", fx.stderr.String())
	}
}

func TestLogin_SwitchingAccountsDeletesOldToken(t *testing.T) {
	fx := newFixture(t)
	fx.cfg.Email = "old@example.com"
	fx.stdin.WriteString("s3cret")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if err := loginRun(context.Background(), fx.opts); err != nil {
		t.Fatal(err)
	}
	if len(fx.deleted) != 1 || fx.deleted[0] != "old@example.com" {
		t.Errorf("deleted = %v", fx.deleted)
	}
}

func TestLogin_WarnsWhenEnvTokenSet(t *testing.T) {
	fx := newFixture(t)
	t.Setenv("KHBB_TOKEN", "from-env")
	fx.stdin.WriteString("s3cret")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if err := loginRun(context.Background(), fx.opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fx.stderr.String(), "KHBB_TOKEN is set") {
		t.Errorf("stderr = %q", fx.stderr.String())
	}
}

func TestLogin_CleanupFailureWarns(t *testing.T) {
	fx := newFixture(t)
	fx.cfg.Email = "old@example.com"
	fx.stdin.WriteString("s3cret")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.opts.DeleteToken = func(email string) error {
		if email == "old@example.com" {
			return errors.New("keyring locked")
		}
		fx.deleted = append(fx.deleted, email)
		return nil
	}
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if err := loginRun(context.Background(), fx.opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fx.stderr.String(), "could not remove the previous token for old@example.com") {
		t.Errorf("stderr = %q", fx.stderr.String())
	}
}

func TestLogin_SaveFailureKeepsOldToken(t *testing.T) {
	fx := newFixture(t)
	fx.cfg.Email = "old@example.com"
	fx.stdin.WriteString("s3cret")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	// Make Save fail by pointing the config path to an unwritable location
	originalCfg := fx.cfg
	fx.opts.Config = func() (*config.Config, error) {
		// Return a config with a path under a read-only directory
		cfg := *originalCfg
		cfg.Email = "old@example.com"
		// Set the path to something that will fail to write
		cfg.InsecureToken = "won't save"
		// Return the same config but override its Save to fail
		return &cfg, nil
	}
	// Override the config's Save to fail
	savedCfg := fx.cfg
	fx.opts.Config = func() (*config.Config, error) {
		cfg := *savedCfg
		cfg.Email = "old@example.com"
		return &cfg, nil
	}
	// Create a mock config that fails to save by making it read-only
	tempDir := t.TempDir()
	readOnlyDir := filepath.Join(tempDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0500); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(filepath.Join(readOnlyDir, "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Email = "old@example.com"
	fx.opts.Config = func() (*config.Config, error) { return cfg, nil }

	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	err = loginRun(context.Background(), fx.opts)
	if err == nil {
		t.Fatal("expected error when saving config")
	}
	if len(fx.deleted) != 0 {
		t.Errorf("deleted = %v, expected empty (old token should not be deleted if save fails)", fx.deleted)
	}
}

func TestLogin_InsecureStorageRemovesKeyringCopy(t *testing.T) {
	fx := newFixture(t)
	fx.cfg.Email = "dev@example.com"
	fx.stdin.WriteString("s3cret")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.opts.InsecureStorage = true
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if err := loginRun(context.Background(), fx.opts); err != nil {
		t.Fatal(err)
	}
	if len(fx.deleted) != 1 || fx.deleted[0] != "dev@example.com" {
		t.Errorf("deleted = %v, expected [dev@example.com]", fx.deleted)
	}
}
