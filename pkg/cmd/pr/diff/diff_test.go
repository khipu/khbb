package diff

import (
	"errors"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const diffText = "diff --git a/src/widget.go b/src/widget.go\nindex 1111111..2222222 100644\n" +
	"--- a/src/widget.go\n+++ b/src/widget.go\n@@ -1,2 +1,2 @@\n package widget\n" +
	"-var Name = \"old\"\n+var Name = \"new\"\n"

func setup(t *testing.T, sub string, body string) (*httpmock.Registry, func(args ...string) (string, error)) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	reg.Register("GET", prtest.PRs+"/42/"+sub, httpmock.StringResponse(200, body))
	return reg, func(args ...string) (string, error) {
		f, _, out, _ := prtest.NewFactory(reg)
		err := prtest.Run(NewCmdDiff(f, nil), append([]string{"42"}, args...)...)
		return out.String(), err
	}
}

func TestDiff_Plain(t *testing.T) {
	_, run := setup(t, "diff", diffText)
	out, err := run()
	if err != nil || out != diffText {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestDiff_ColorAlways(t *testing.T) {
	_, run := setup(t, "diff", diffText)
	out, err := run("--color", "always")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"\x1b[1mdiff --git a/src/widget.go b/src/widget.go\x1b[m\n",
		"\x1b[36m@@ -1,2 +1,2 @@\x1b[m\n",
		"\n package widget\n",
		"\x1b[31m-var Name = \"old\"\x1b[m\n",
		"\x1b[32m+var Name = \"new\"\x1b[m\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

func TestDiff_NameOnly(t *testing.T) {
	_, run := setup(t, "diffstat", prtest.Page(
		`{"status":"modified","lines_added":1,"lines_removed":1,"old":{"path":"src/widget.go"},"new":{"path":"src/widget.go"}}`,
		`{"status":"removed","lines_added":0,"lines_removed":10,"old":{"path":"docs/old.md"},"new":null}`,
		`{"status":"added","lines_added":5,"lines_removed":0,"old":null,"new":{"path":"docs/new.md"}}`,
	))
	out, err := run("--name-only")
	if err != nil || out != "src/widget.go\ndocs/old.md\ndocs/new.md\n" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestDiff_Patch(t *testing.T) {
	reg, run := setup(t, "patch", "From abc1234\nSubject: Add widgets\n")
	out, err := run("--patch")
	if err != nil || out != "From abc1234\nSubject: Add widgets\n" {
		t.Errorf("out %q err %v", out, err)
	}
	if got := reg.Calls[1].Header.Get("Accept"); got != "*/*" {
		t.Errorf("Accept = %q", got)
	}
}

func TestDiff_BadFlags(t *testing.T) {
	for _, args := range [][]string{{"--color", "rainbow"}, {"--name-only", "--patch"}, {"1", "2"}} {
		reg := httpmock.New(t)
		f, _, _, _ := prtest.NewFactory(reg)
		err := prtest.Run(NewCmdDiff(f, nil), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
