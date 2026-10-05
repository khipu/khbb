package factory_test

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/factory"
)

func TestHTTPClient_NotLoggedIn(t *testing.T) {
	keyring.MockInit()
	t.Setenv("KHBB_CONFIG_DIR", t.TempDir())
	t.Setenv("KHBB_TOKEN", "")
	f := factory.New("dev", "", "")
	_, err := f.HTTPClient()
	var authErr *cmdutil.AuthError
	if !errors.As(err, &authErr) || authErr.Msg != "not logged in to bitbucket.org" {
		t.Fatalf("expected AuthError, got %v", err)
	}
}

func TestHTTPClient_FromEnvironment(t *testing.T) {
	keyring.MockInit()
	t.Setenv("KHBB_CONFIG_DIR", t.TempDir())
	t.Setenv("KHBB_TOKEN", "s3cret")
	t.Setenv("KHBB_EMAIL", "dev@example.com")
	f := factory.New("dev", "", "")
	c, err := f.HTTPClient()
	if err != nil || c == nil {
		t.Fatalf("HTTPClient() = %v, %v", c, err)
	}
}

func TestConfigIsCached(t *testing.T) {
	t.Setenv("KHBB_CONFIG_DIR", t.TempDir())
	f := factory.New("dev", "", "")
	a, err := f.Config()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := f.Config()
	if a != b {
		t.Error("Config() must return the same instance")
	}
}
