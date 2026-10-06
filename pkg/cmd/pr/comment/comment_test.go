package comment

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const commentURL = "https://bitbucket.org/acme/widgets/pull-requests/42/_/diff#comment-101"

func newReg(t *testing.T) *httpmock.Registry {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	return reg
}

func diffstat(paths ...string) httpmock.Responder {
	items := make([]string, len(paths))
	for i, p := range paths {
		items[i] = `{"status":"modified","lines_added":1,"lines_removed":1,"old":{"path":"` + p + `"},"new":{"path":"` + p + `"}}`
	}
	return httpmock.JSONResponse(200, prtest.Page(items...))
}

func TestComment_General(t *testing.T) {
	reg := newReg(t)
	reg.Register("POST", prtest.PRs+"/42/comments", httpmock.JSONResponse(201, prtest.CommentInline))
	f, _, out, _ := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdComment(f, nil), "42", "--body", "Looks good"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[1], `{"content":{"raw":"Looks good"}}`)
	if out.String() != commentURL+"\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestComment_InlineChecksTheFile(t *testing.T) {
	reg := newReg(t)
	reg.Register("GET", prtest.PRs+"/42/diffstat", diffstat("README.md", "src/widget.go"))
	reg.Register("POST", prtest.PRs+"/42/comments", httpmock.JSONResponse(201, prtest.CommentInline))
	f, _, _, _ := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdComment(f, nil), "42", "--file", "src/widget.go", "--line", "12", "-b", "Rename this"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[2], `{"content":{"raw":"Rename this"},"inline":{"path":"src/widget.go","to":12}}`)
}

func TestComment_FileNotInThePullRequest(t *testing.T) {
	reg := newReg(t)
	reg.Register("GET", prtest.PRs+"/42/diffstat", diffstat("src/widget.go"))
	f, _, _, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdComment(f, nil), "42", "--file", "docs/other.md", "-b", "x")
	var notFound *cmdutil.NotFoundError
	if !errors.As(err, &notFound) || err.Error() != "docs/other.md is not changed in pull request #42" {
		t.Errorf("err = %v", err)
	}
	if len(prtest.Writes(reg)) != 0 {
		t.Error("nothing may be posted")
	}
}

func TestComment_ReplyWithJSON(t *testing.T) {
	reg := newReg(t)
	reg.Register("POST", prtest.PRs+"/42/comments", httpmock.JSONResponse(201, prtest.CommentReply))
	f, _, out, _ := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdComment(f, nil), "42", "--reply-to", "101", "-b", "Done.", "--json", "id,parentId"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[1], `{"content":{"raw":"Done."},"parent":{"id":101}}`)
	if out.String() != `{"id":102,"parentId":101}`+"\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestComment_BodyFromStandardInput(t *testing.T) {
	reg := newReg(t)
	reg.Register("POST", prtest.PRs+"/42/comments", httpmock.JSONResponse(201, prtest.CommentInline))
	f, ios, _, _ := prtest.NewFactory(reg)
	ios.In.(*bytes.Buffer).WriteString("From stdin\n")

	if err := prtest.Run(NewCmdComment(f, nil), "42", "--body-file", "-"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[1], `{"content":{"raw":"From stdin\n"}}`)
}

func TestComment_PromptsOnATerminal(t *testing.T) {
	reg := newReg(t)
	reg.Register("POST", prtest.PRs+"/42/comments", httpmock.JSONResponse(201, prtest.CommentInline))
	f, ios, _, _ := prtest.NewFactory(reg)
	prtest.SetTTY(ios)
	pm := prompter.NewMock(t)
	pm.RegisterInput("Comment", func(_, _ string) (string, error) { return "Typed", nil })
	f.Prompter = pm

	if err := prtest.Run(NewCmdComment(f, nil), "42"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[1], `{"content":{"raw":"Typed"}}`)
}

func TestComment_FlagErrors(t *testing.T) {
	for _, args := range [][]string{
		{"42"},                          // no body without a terminal
		{"42", "-b", " "},               // blank body
		{"42", "-b", "x", "-F", "y.md"}, // two bodies
		{"42", "-b", "x", "--line", "3"},
		{"42", "-b", "x", "--file", "a.go", "--line", "0"},
		{"42", "-b", "x", "--reply-to", "0"},
		{"42", "-b", "x", "--reply-to", "5", "--file", "a.go"},
	} {
		f, _, _, _ := prtest.NewFactory(httpmock.New(t))
		err := prtest.Run(NewCmdComment(f, nil), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

func TestComment_DryRun(t *testing.T) {
	reg := newReg(t)
	f, _, out, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdComment(f, nil), "42", "-b", "x", "--dry-run")
	if !errors.Is(err, bitbucket.ErrDryRun) || !strings.Contains(out.String(), "/pullrequests/42/comments") {
		t.Errorf("err %v out %q", err, out.String())
	}
}
