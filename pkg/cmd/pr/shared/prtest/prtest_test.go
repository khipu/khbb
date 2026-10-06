package prtest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func TestNewFactory_HonorsDryRun(t *testing.T) {
	reg := httpmock.New(t)
	f, _, out, _ := prtest.NewFactory(reg)
	f.DryRun = true
	client, err := f.HTTPClient()
	if err != nil {
		t.Fatal(err)
	}
	err = client.ApprovePR(context.Background(), "acme", "widgets", 42)
	if !errors.Is(err, bitbucket.ErrDryRun) || !strings.Contains(out.String(), `"method": "POST"`) || len(reg.Calls) != 0 {
		t.Errorf("err %v out %q calls %d", err, out.String(), len(reg.Calls))
	}
}

func TestWithState(t *testing.T) {
	got := prtest.WithState(prtest.PR42, "MERGED")
	if !strings.Contains(got, `"id":42,"title":"Add widgets","description":"Adds the widget factory.","state":"MERGED"`) ||
		!strings.Contains(got, `"state":"approved"`) {
		t.Errorf("WithState must change only the pull request's state: %s", got)
	}
}

func TestWrites(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))
	reg.Register("POST", prtest.PRs+"/42/approve", httpmock.JSONResponse(200, `{}`))
	f, _, _, _ := prtest.NewFactory(reg)
	client, _ := f.HTTPClient()
	_, _ = client.CurrentUser(context.Background())
	_ = client.ApprovePR(context.Background(), "acme", "widgets", 42)
	if w := prtest.Writes(reg); len(w) != 1 || w[0].Method != "POST" {
		t.Errorf("Writes = %+v", w)
	}
}
