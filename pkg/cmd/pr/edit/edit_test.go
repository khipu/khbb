package edit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func newReg(t *testing.T, pr string) *httpmock.Registry {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, pr))
	return reg
}

func put(reg *httpmock.Registry) {
	reg.Register("PUT", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
}

func lastCall(reg *httpmock.Registry) httpmock.Call { return reg.Calls[len(reg.Calls)-1] }

func TestEdit_TitleAndReady(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	put(reg)
	f, _, out, _ := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdEdit(f, nil), "42", "-t", "New title", "--ready"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, lastCall(reg), `{"title":"New title","draft":false}`)
	if out.String() != "https://bitbucket.org/acme/widgets/pull-requests/42\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestEdit_BodyFileBaseAndDraft(t *testing.T) {
	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte("New description"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := newReg(t, prtest.PR42)
	put(reg)
	f, _, _, _ := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdEdit(f, nil), "42", "-F", path, "-B", "develop", "--draft"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, lastCall(reg), `{"description":"New description","destination":{"branch":{"name":"develop"}},"draft":true}`)
}

func TestEdit_Reviewers(t *testing.T) {
	// PR 42 asks bob and cy; ada is its author.
	reg := newReg(t, prtest.PR42)
	reg.Register("GET", prtest.Members, httpmock.JSONResponse(200, prtest.Page(prtest.Member(prtest.Ada)))) // --add-reviewer ada
	reg.Register("GET", prtest.Members, httpmock.JSONResponse(200, prtest.Page(prtest.Member(prtest.Cy))))  // --remove-reviewer cy
	put(reg)
	f, _, _, _ := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdEdit(f, nil), "42", "--add-reviewer", "ada", "--remove-reviewer", "cy"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, lastCall(reg), `{"reviewers":[{"uuid":"`+prtest.BobUUID+`"}]}`)
}

func TestEdit_JSON(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	put(reg)
	f, _, out, _ := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdEdit(f, nil), "42", "-b", "", "--json", "title"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, lastCall(reg), `{"description":""}`)
	if out.String() != `{"title":"Add widgets"}`+"\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestEdit_RefusesClosedPullRequest(t *testing.T) {
	reg := newReg(t, prtest.WithState(prtest.PR42, "DECLINED"))
	f, _, _, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdEdit(f, nil), "42", "-t", "x")
	var conflict *cmdutil.ConflictError
	if !errors.As(err, &conflict) || !strings.Contains(err.Error(), "only open pull requests can be edited") || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestEdit_DryRun(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	f, _, out, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdEdit(f, nil), "42", "-t", "x", "--dry-run")
	if !errors.Is(err, bitbucket.ErrDryRun) || !strings.Contains(out.String(), `"method": "PUT"`) {
		t.Errorf("err %v out %q", err, out.String())
	}
}

func TestEdit_FlagErrors(t *testing.T) {
	for _, args := range [][]string{
		{"42"},
		{"42", "--draft", "--ready"},
		{"42", "-t", " "},
		{"42", "-b", "x", "-F", "y.md"},
		{"42", "-B", ""},
	} {
		f, _, _, _ := prtest.NewFactory(httpmock.New(t))
		err := prtest.Run(NewCmdEdit(f, nil), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
