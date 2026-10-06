package create

import (
	"errors"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const prURL = "https://bitbucket.org/acme/widgets/pull-requests/42"

func mainBranch(reg *httpmock.Registry) {
	reg.Register("GET", prtest.Repo, httpmock.JSONResponse(200, `{"mainbranch":{"name":"main","type":"branch"}}`))
}

func noOpenPR(reg *httpmock.Registry) {
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page()))
}

func defaultReviewers(reg *httpmock.Registry, users ...string) {
	items := make([]string, len(users))
	for i, u := range users {
		items[i] = `{"type":"default_reviewer","reviewer_type":"repository","user":` + u + `}`
	}
	reg.Register("GET", prtest.Repo+"/effective-default-reviewers", httpmock.JSONResponse(200, prtest.Page(items...)))
}

func lastCall(reg *httpmock.Registry) httpmock.Call { return reg.Calls[len(reg.Calls)-1] }

func TestCreate_NonInteractive(t *testing.T) {
	reg := httpmock.New(t)
	mainBranch(reg)
	noOpenPR(reg)
	reg.Register("GET", prtest.Members, httpmock.JSONResponse(200, prtest.Page(prtest.Member(prtest.Bob))))
	defaultReviewers(reg, prtest.Cy, prtest.Ada, prtest.Bob)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))
	reg.Register("POST", prtest.PRs, httpmock.JSONResponse(201, prtest.PR42))
	f, _, out, errOut := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")

	err := prtest.Run(NewCmdCreate(f, nil), "--title", "Add widgets", "--body", "Adds the widget factory.", "--reviewer", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != prURL+"\n" || errOut.String() != "" {
		t.Errorf("out %q stderr %q", out.String(), errOut.String())
	}
	// Bob comes from --reviewer, Cy from the default reviewers; Ada is the author; Bob is not repeated.
	prtest.AssertJSONBody(t, lastCall(reg), `{"title":"Add widgets","description":"Adds the widget factory.",
		"source":{"branch":{"name":"feature/widgets"}},"destination":{"branch":{"name":"main"}},
		"draft":false,"close_source_branch":false,
		"reviewers":[{"uuid":"`+prtest.BobUUID+`"},{"uuid":"`+prtest.CyUUID+`"}]}`)
	if q := reg.Calls[1].URL.Query().Get("q"); q != `state = "OPEN" AND (source.branch.name = "feature/widgets" AND destination.branch.name = "main")` {
		t.Errorf("duplicate check q = %q", q)
	}
}

func TestCreate_ExistingPullRequest(t *testing.T) {
	reg := httpmock.New(t)
	mainBranch(reg)
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page(prtest.PR42)))
	f, _, _, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")

	err := prtest.Run(NewCmdCreate(f, nil), "-t", "Add widgets", "-b", "")
	var conflict *cmdutil.ConflictError
	if !errors.As(err, &conflict) || err.Error() != "a pull request for feature/widgets into main already exists: "+prURL {
		t.Errorf("err = %v", err)
	}
	if len(prtest.Writes(reg)) != 0 {
		t.Error("nothing may be created")
	}
}

func TestCreate_HeadNotPushed(t *testing.T) {
	reg := httpmock.New(t)
	noOpenPR(reg)
	reg.Register("POST", prtest.PRs, httpmock.JSONResponse(400, `{"type":"error","error":{"message":"source: branch not found: feature/widgets",`+
		`"fields":{"source":["branch not found: feature/widgets"]}}}`))
	f, _, _, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")

	err := prtest.Run(NewCmdCreate(f, nil), "-B", "main", "-t", "Add widgets", "-b", "", "--no-default-reviewers")
	var notFound *cmdutil.NotFoundError
	if !errors.As(err, &notFound) || !strings.Contains(err.Error(), `branch "feature/widgets" is not on Bitbucket; push it first`) {
		t.Errorf("err = %v", err)
	}
}

func TestCreate_DraftDeleteBranchBaseAndJSON(t *testing.T) {
	reg := httpmock.New(t)
	noOpenPR(reg)
	reg.Register("POST", prtest.PRs, httpmock.JSONResponse(201, prtest.PR42))
	f, _, out, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdCreate(f, nil), "-H", "feature/widgets", "-B", "develop", "--draft", "-d",
		"--no-default-reviewers", "-t", "Add widgets", "-b", "", "--json", "id,draft")
	if err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, lastCall(reg), `{"title":"Add widgets","description":"",
		"source":{"branch":{"name":"feature/widgets"}},"destination":{"branch":{"name":"develop"}},
		"draft":true,"close_source_branch":true,"reviewers":[]}`)
	if out.String() != `{"draft":false,"id":42}`+"\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestCreate_PromptsOnATerminal(t *testing.T) {
	reg := httpmock.New(t)
	mainBranch(reg)
	noOpenPR(reg)
	reg.Register("POST", prtest.PRs, httpmock.JSONResponse(201, prtest.PR42))
	f, ios, _, errOut := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")
	prtest.SetTTY(ios)
	pm := prompter.NewMock(t)
	pm.RegisterInput("Title", func(_, _ string) (string, error) { return "Typed title", nil })
	pm.RegisterInput("Body", func(_, _ string) (string, error) { return "Typed body", nil })
	f.Prompter = pm

	if err := prtest.Run(NewCmdCreate(f, nil), "--no-default-reviewers"); err != nil {
		t.Fatal(err)
	}
	body := prtest.JSONBodyOf(t, lastCall(reg))
	if body["title"] != "Typed title" || body["description"] != "Typed body" {
		t.Errorf("body = %v", body)
	}
	if errOut.String() != "Creating pull request for feature/widgets into main in acme/widgets\n\n" {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestCreate_UnknownReviewer(t *testing.T) {
	reg := httpmock.New(t)
	noOpenPR(reg)
	reg.Register("GET", prtest.Members, httpmock.JSONResponse(200, prtest.Page()))
	reg.Register("GET", prtest.Members, httpmock.JSONResponse(200, prtest.Page(prtest.Member(prtest.Bob))))
	f, _, _, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")

	err := prtest.Run(NewCmdCreate(f, nil), "-B", "main", "-t", "x", "-b", "", "-r", "zed")
	var notFound *cmdutil.NotFoundError
	if !errors.As(err, &notFound) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err %v writes %d", err, len(prtest.Writes(reg)))
	}
}

func TestCreate_DryRun(t *testing.T) {
	reg := httpmock.New(t)
	mainBranch(reg)
	noOpenPR(reg)
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")

	err := prtest.Run(NewCmdCreate(f, nil), "--dry-run", "--no-default-reviewers", "-t", "Add widgets", "-b", "")
	if !errors.Is(err, bitbucket.ErrDryRun) || !strings.Contains(out.String(), `"method": "POST"`) ||
		!strings.Contains(out.String(), `"title": "Add widgets"`) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err %v out %q", err, out.String())
	}
}

func TestCreate_FlagErrors(t *testing.T) {
	for _, args := range [][]string{
		{},                                   // no title without a terminal
		{"-t", "x"},                          // no body without a terminal
		{"-t", "x", "-b", "y", "-F", "z"},    // two bodies
		{"-t", " ", "-b", "y", "-B", "main"}, // blank title
		{"-H", "main", "-B", "main", "-t", "x", "-b", "y"}, // head is base
		{"extra", "-t", "x", "-b", "y"},                    // no positional arguments
	} {
		f, _, _, _ := prtest.NewFactory(httpmock.New(t))
		prtest.SetBranch(f, "feature/widgets")
		err := prtest.Run(NewCmdCreate(f, nil), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

func TestCreate_RepoFlagNeedsHead(t *testing.T) {
	f, _, _, _ := prtest.NewFactory(httpmock.New(t))
	cmd := NewCmdCreate(f, nil)
	cmdutil.EnableRepoOverride(cmd, f) // the pr group normally adds --repo
	err := prtest.Run(cmd, "-R", "acme/widgets", "-t", "x", "-b", "y")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "--head") {
		t.Errorf("err = %v", err)
	}
}
