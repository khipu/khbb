package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/internal/iostreams"
)

const prs = "/2.0/repositories/acme/widgets/pullrequests"

func newOpts(t *testing.T) (*APIOptions, *httpmock.Registry, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	ios, stdin, out, _ := iostreams.Test()
	reg := httpmock.New(t)
	return &APIOptions{
		IO: ios,
		HTTPClient: func() (*bitbucket.Client, error) {
			return bitbucket.New(bitbucket.Options{Email: "dev@example.com", Token: "s3cret", HTTPClient: reg.Client()}), nil
		},
		BaseRepo: func() (gitctx.Repo, error) { return gitctx.Repo{Workspace: "acme", Slug: "widgets"}, nil },
		Branch:   func() (string, error) { return "feature/x", nil },
		Method:   "GET",
	}, reg, stdin, out
}

func TestAPI_GETPrintsRawBody(t *testing.T) {
	opts, reg, _, out := newOpts(t)
	opts.Path = "user"
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, `{"nickname":"ada"}`))
	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if out.String() != `{"nickname":"ada"}` {
		t.Errorf("out = %q", out.String())
	}
}

func TestAPI_FillsPlaceholders(t *testing.T) {
	opts, reg, _, _ := newOpts(t)
	opts.Path = "repositories/{workspace}/{repo}/refs/branches/{branch}"
	reg.Register("GET", "/2.0/repositories/acme/widgets/refs/branches/feature/x", httpmock.JSONResponse(200, `{}`))
	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
}

func TestAPI_PlaceholderWithoutRepoFails(t *testing.T) {
	opts, _, _, _ := newOpts(t)
	opts.Path = "repositories/{workspace}/{repo}"
	opts.BaseRepo = func() (gitctx.Repo, error) { return gitctx.Repo{}, gitctx.ErrNoRepo }
	if err := apiRun(context.Background(), opts); !errors.Is(err, gitctx.ErrNoRepo) {
		t.Errorf("err = %v", err)
	}
}

func TestAPI_FieldsMakeJSONPost(t *testing.T) {
	opts, reg, _, _ := newOpts(t)
	opts.Path = "repositories/{workspace}/{repo}/pullrequests"
	opts.RawFields = []string{"title=Add widgets"}
	opts.TypedFields = []string{"draft=true", "count=3", "parent=null"}
	reg.Register("POST", prs, httpmock.JSONResponse(201, `{"id":1}`))

	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(reg.Calls[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["title"] != "Add widgets" || body["draft"] != true || body["count"] != float64(3) || body["parent"] != nil {
		t.Errorf("body = %v", body)
	}
	if _, ok := body["parent"]; !ok {
		t.Error("null field must be sent")
	}
}

func TestAPI_TypedFieldNumbers(t *testing.T) {
	opts, reg, _, _ := newOpts(t)
	opts.Path = "repositories/{workspace}/{repo}/pullrequests"
	opts.TypedFields = []string{"ratio=1.5", "count=3", "code=007"}
	reg.Register("POST", prs, httpmock.JSONResponse(201, `{}`))

	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(reg.Calls[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if ratio, ok := body["ratio"].(float64); !ok || ratio != 1.5 {
		t.Errorf("ratio = %v (expected float64 1.5)", body["ratio"])
	}
	if count, ok := body["count"].(float64); !ok || count != 3 {
		t.Errorf("count = %v (expected float64 3)", body["count"])
	}
	if code, ok := body["code"].(string); !ok || code != "007" {
		t.Errorf("code = %v (expected string \"007\")", body["code"])
	}
}

func TestAPI_GETFieldsBecomeQuery(t *testing.T) {
	opts, reg, _, _ := newOpts(t)
	opts.Path = "repositories/{workspace}/{repo}/pullrequests"
	opts.MethodSet = true
	opts.RawFields = []string{`q=state="OPEN"`}
	reg.Register("GET", prs, httpmock.JSONResponse(200, `{"values":[]}`))

	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if got := reg.Calls[0].URL.Query().Get("q"); got != `state="OPEN"` {
		t.Errorf("q = %q", got)
	}
}

func TestAPI_PaginateSendsFieldsAsQuery(t *testing.T) {
	opts, reg, _, out := newOpts(t)
	opts.Path = "repositories/{workspace}/{repo}/pullrequests"
	opts.Paginate = true
	opts.RawFields = []string{`q=state="OPEN"`}
	reg.Register("GET", prs, httpmock.JSONResponse(200, `{"values":[{"id":1}]}`))

	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if got := reg.Calls[0].URL.Query().Get("q"); got != `state="OPEN"` {
		t.Errorf("q = %q", got)
	}
	if out.String() != `[{"id":1}]`+"\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestAPI_TypedFieldFromFile(t *testing.T) {
	opts, reg, _, _ := newOpts(t)
	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte("Long description"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts.Path = "repositories/{workspace}/{repo}/pullrequests"
	opts.TypedFields = []string{"description=@" + path}
	reg.Register("POST", prs, httpmock.JSONResponse(201, `{}`))

	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reg.Calls[0].Body), `"description":"Long description"`) {
		t.Errorf("body = %s", reg.Calls[0].Body)
	}
}

func TestAPI_InputFromStdin(t *testing.T) {
	opts, reg, stdin, _ := newOpts(t)
	stdin.WriteString(`{"title":"Renamed"}`)
	opts.Path = "repositories/{workspace}/{repo}/pullrequests/1"
	opts.Method, opts.MethodSet = "PUT", true
	opts.Input = "-"
	reg.Register("PUT", prs+"/1", httpmock.JSONResponse(200, `{}`))

	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if string(reg.Calls[0].Body) != `{"title":"Renamed"}` {
		t.Errorf("body = %s", reg.Calls[0].Body)
	}
}

func TestAPI_PaginateMergesValues(t *testing.T) {
	opts, reg, _, out := newOpts(t)
	opts.Path = "repositories/{workspace}/{repo}/pullrequests"
	opts.Paginate = true
	reg.Register("GET", prs, httpmock.JSONResponse(200, `{"values":[{"id":1}],"next":"https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests?page=2"}`))
	reg.Register("GET", prs, httpmock.JSONResponse(200, `{"values":[{"id":2}]}`))

	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if out.String() != `[{"id":1},{"id":2}]`+"\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestAPI_PaginateEmptyIsArray(t *testing.T) {
	opts, reg, _, out := newOpts(t)
	opts.Path = "repositories/{workspace}/{repo}/pullrequests"
	opts.Paginate = true
	reg.Register("GET", prs, httpmock.JSONResponse(200, `{"values":[]}`))

	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if out.String() != "[]\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestAPI_PaginateNonPaginatedPassesThrough(t *testing.T) {
	opts, reg, _, out := newOpts(t)
	opts.Path = "user"
	opts.Paginate = true
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, `{"nickname":"ada"}`))

	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if out.String() != `{"nickname":"ada"}` {
		t.Errorf("out = %q", out.String())
	}
}

func TestAPI_PaginateRejectsNonCollectionLaterPage(t *testing.T) {
	opts, reg, _, out := newOpts(t)
	opts.Path = "repositories/{workspace}/{repo}/pullrequests"
	opts.Paginate = true
	reg.Register("GET", prs, httpmock.JSONResponse(200, `{"values":[{"id":1}],"next":"https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests?page=2"}`))
	reg.Register("GET", prs, httpmock.JSONResponse(200, `{"nickname":"ada"}`))

	err := apiRun(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for non-collection on page 2")
	}
	if !strings.Contains(err.Error(), "page 2") {
		t.Errorf("error should mention page 2, got: %v", err)
	}
	if out.String() != "" {
		t.Errorf("stdout should be empty, got %q", out.String())
	}
}

func TestAPI_JQ(t *testing.T) {
	opts, reg, _, out := newOpts(t)
	opts.Path = "repositories/{workspace}/{repo}/pullrequests"
	opts.JQ = ".values[].title"
	reg.Register("GET", prs, httpmock.JSONResponse(200, `{"values":[{"title":"A"},{"title":"B"}]}`))

	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if out.String() != "A\nB\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestAPI_ErrorStatusPrintsBodyAndFails(t *testing.T) {
	opts, reg, _, out := newOpts(t)
	opts.Path = "repositories/acme/nope"
	opts.JQ = ".values"
	body := `{"type":"error","error":{"message":"Repository acme/nope not found"}}`
	reg.Register("GET", "/2.0/repositories/acme/nope", httpmock.JSONResponse(404, body))

	err := apiRun(context.Background(), opts)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 {
		t.Fatalf("err = %v", err)
	}
	if out.String() != body {
		t.Errorf("error body must be printed unfiltered, got %q", out.String())
	}
}

func TestAPI_IncludeWritesStatusAndHeaders(t *testing.T) {
	opts, reg, _, out := newOpts(t)
	opts.Path = "user"
	opts.Include = true
	opts.Silent = true
	reg.Register("GET", "/2.0/user", httpmock.WithHeader(httpmock.JSONResponse(200, `{}`), "X-Request-Id", "abc"))

	if err := apiRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.HasPrefix(s, "HTTP/1.1 200 OK\n") || !strings.Contains(s, "X-Request-Id: abc\n") || strings.Contains(s, "{}") {
		t.Errorf("out = %q", s)
	}
}

func TestAPI_DryRunDoesNotSend(t *testing.T) {
	opts, _, _, out := newOpts(t)
	opts.HTTPClient = func() (*bitbucket.Client, error) {
		return bitbucket.New(bitbucket.Options{Token: "s3cret", DryRun: true, DryRunOut: opts.IO.Out}), nil
	}
	opts.Path = "repositories/{workspace}/{repo}/pullrequests"
	opts.RawFields = []string{"title=Add widgets"}

	if err := apiRun(context.Background(), opts); !errors.Is(err, bitbucket.ErrDryRun) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out.String(), `"method": "POST"`) {
		t.Errorf("out = %q", out.String())
	}
}

func TestNewCmdAPI_Validation(t *testing.T) {
	cases := [][]string{
		{"--paginate", "-X", "POST", "user"},
		{"--paginate", "--input", "body.json", "user"},
		{"--input", "body.json", "-f", "a=b", "user"},
		{"--jq", ".", "--template", "{{.}}", "user"},
		{"--include", "--paginate", "user"},
		{"-f", "novalue", "user"},
		{"--jq", ".[", "user"},
		{"--jq", "nosuchfn", "user"},
		{"-X", "POST", "--template", "{{", "user"},
	}
	for _, args := range cases {
		ios, _, _, _ := iostreams.Test()
		cmd := NewCmdAPI(&cmdutil.Factory{IOStreams: ios}, func(*APIOptions) error { return nil })
		cmd.SetArgs(args)
		var flagErr *cmdutil.FlagError
		if err := cmd.Execute(); !errors.As(err, &flagErr) {
			t.Errorf("%v: expected FlagError, got %v", args, err)
		}
	}
}
