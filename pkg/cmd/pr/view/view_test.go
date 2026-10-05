package view

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

type fakeBrowser struct{ urls []string }

func (b *fakeBrowser) Browse(u string) error {
	b.urls = append(b.urls, u)
	return nil
}

const prURL = "https://bitbucket.org/acme/widgets/pull-requests/42"

func TestView_RawOutputWithComments(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	reg.Register("GET", prtest.PRs+"/42/comments", httpmock.JSONResponse(200, prtest.Page(prtest.CommentInline, prtest.CommentReply)))
	f, _, out, _ := prtest.NewFactory(reg)
	if err := prtest.Run(NewCmdView(f, nil), "42", "--comments"); err != nil {
		t.Fatal(err)
	}
	want := "title:\tAdd widgets\nnumber:\t42\nstate:\tOPEN\ndraft:\tfalse\nauthor:\tada\n" +
		"source:\tfeature/widgets\ndestination:\tmain\nreviewers:\tbob (approved), cy (pending)\n" +
		"comments:\t2\ntasks:\t1\nurl:\t" + prURL + "\n--\nAdds the widget factory.\n" +
		"--\ncomment:\t101\nauthor:\tbob\ncreated:\t2026-10-01T13:00:00Z\nlocation:\tsrc/widget.go:12\n--\nPlease rename this.\n" +
		"--\ncomment:\t102\nauthor:\tada\ncreated:\t2026-10-01T14:00:00Z\nreply to:\t101\n--\nDone.\n"
	if out.String() != want {
		t.Errorf("out = %q\nwant %q", out.String(), want)
	}
}

func TestView_TTY(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	f, ios, out, _ := prtest.NewFactory(reg)
	ios.SetStdoutTTY(true)
	if err := prtest.Run(NewCmdView(f, nil), "42"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Add widgets #42\n",
		"OPEN • ada wants to merge feature/widgets into main • updated 2026-10-02\n",
		"Reviewers: bob (approved), cy (pending)\n",
		"Comments: 2 • Tasks: 1\n",
		"Adds the widget factory.\n",
		"View this pull request on Bitbucket: " + prURL + "\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
}

func TestView_JSONWithComments(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	reg.Register("GET", prtest.PRs+"/42/comments", httpmock.JSONResponse(200, prtest.Page(prtest.CommentInline, prtest.CommentReply)))
	f, _, out, _ := prtest.NewFactory(reg)
	if err := prtest.Run(NewCmdView(f, nil), "42", "--json", "id,comments"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		ID       int `json:"id"`
		Comments []struct {
			ID   int  `json:"id"`
			Line *int `json:"line"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if got.ID != 42 || len(got.Comments) != 2 || got.Comments[0].Line == nil || *got.Comments[0].Line != 12 {
		t.Errorf("got %+v", got)
	}
}

func TestView_JSONWithoutCommentsSkipsThatRequest(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	f, _, out, _ := prtest.NewFactory(reg)
	if err := prtest.Run(NewCmdView(f, nil), "42", "--json", "id,title"); err != nil {
		t.Fatal(err)
	}
	if out.String() != `{"id":42,"title":"Add widgets"}`+"\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestView_CurrentBranch(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page(prtest.PR42)))
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")
	if err := prtest.Run(NewCmdView(f, nil), "--json", "id"); err != nil {
		t.Fatal(err)
	}
	if out.String() != `{"id":42}`+"\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestView_Web(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	f, ios, out, errOut := prtest.NewFactory(reg)
	ios.SetStderrTTY(true)
	b := &fakeBrowser{}
	f.Browser = b
	if err := prtest.Run(NewCmdView(f, nil), "42", "--web"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.urls, []string{prURL}) || out.Len() != 0 || errOut.String() != "Opening "+prURL+" in your browser.\n" {
		t.Errorf("urls %v out %q stderr %q", b.urls, out.String(), errOut.String())
	}
}

func TestView_WebWithJSONIsUsageError(t *testing.T) {
	reg := httpmock.New(t)
	f, _, _, _ := prtest.NewFactory(reg)
	f.Browser = &fakeBrowser{}
	err := prtest.Run(NewCmdView(f, nil), "42", "--web", "--json", "id")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) {
		t.Errorf("err = %v", err)
	}
}
