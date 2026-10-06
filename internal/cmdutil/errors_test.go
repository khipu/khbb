package cmdutil_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		code   string
		exit   int
		status int
		hint   string // substring
		silent bool
	}{
		{"dry run", fmt.Errorf("wrapped: %w", bitbucket.ErrDryRun), "", 0, 0, "", true},
		{"exit code", &cmdutil.ExitError{Code: 8}, "", 8, 0, "", true},
		{"cancel", cmdutil.ErrCancel, "cancelled", 2, 0, "", false},
		{"interrupt", fmt.Errorf("could not prompt: %w", terminal.InterruptErr), "cancelled", 2, 0, "", false},
		{"confirm", cmdutil.ErrConfirmationRequired, "confirmation_required", 1, 0, "", false},
		{"flag", cmdutil.FlagErrorf("required flag --title not set"), "usage", 1, 0, "--help", false},
		{"auth", &cmdutil.AuthError{Msg: "not logged in to bitbucket.org"}, "auth_required", 4, 0, "khbb auth login", false},
		{"401", &bitbucket.HTTPError{StatusCode: 401}, "auth_required", 4, 401, "khbb auth login", false},
		{"403 reported", &bitbucket.HTTPError{StatusCode: 403, RequiredScopes: []string{"read:pipeline:bitbucket"}}, "forbidden", 1, 403, "read:pipeline:bitbucket", false},
		{"403 inferred", &bitbucket.HTTPError{StatusCode: 403, Method: "POST", URL: "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests/1/merge"}, "forbidden", 1, 403, "write:pullrequest:bitbucket", false},
		{"404 repository", &bitbucket.HTTPError{StatusCode: 404, URL: "https://api.bitbucket.org/2.0/repositories/acme/nope"}, "not_found", 1, 404, "private repositories", false},
		{"404 pull request", &bitbucket.HTTPError{StatusCode: 404, URL: "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests/99999"}, "not_found", 1, 404, "pull request number", false},
		{"409", &bitbucket.HTTPError{StatusCode: 409}, "conflict", 1, 409, "", false},
		{"400", &bitbucket.HTTPError{StatusCode: 400, Detail: "title is required"}, "validation", 1, 400, "title is required", false},
		{"400 fields", &bitbucket.HTTPError{StatusCode: 400, Fields: map[string][]string{"title": {"too long"}, "source": {"branch not found"}}}, "validation", 1, 400, "source: branch not found; title: too long", false},
		{"429", &bitbucket.HTTPError{StatusCode: 429}, "rate_limited", 1, 429, "", false},
		{"555", &bitbucket.HTTPError{StatusCode: 555}, "server_error", 1, 555, "retry later", false},
		{"not found", &cmdutil.NotFoundError{Msg: `no open pull request found for branch "feature/widgets" in acme/widgets`}, "not_found", 1, 0, "", false},
		{"network", &bitbucket.NetworkError{Err: errors.New("dial tcp: i/o timeout")}, "network", 1, 0, "", false},
		{"other", errors.New("boom"), "error", 1, 0, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := cmdutil.Classify(tc.err)
			if info.Code != tc.code || info.Exit != tc.exit || info.Status != tc.status || info.Silent != tc.silent {
				t.Errorf("Classify = %+v", info)
			}
			if !strings.Contains(info.Hint, tc.hint) {
				t.Errorf("Hint = %q, want it to contain %q", info.Hint, tc.hint)
			}
		})
	}
}

func TestClassify_404HintOnlyForRepositories(t *testing.T) {
	info := cmdutil.Classify(&bitbucket.HTTPError{StatusCode: 404, URL: "https://api.bitbucket.org/2.0/user"})
	if info.Code != "not_found" || info.Hint != "" {
		t.Errorf("Classify = %+v, want not_found without a hint", info)
	}
}

const repoNotFoundHint = "check the repository name and your access: private repositories return 404 when you lack access"

func repoNotFound() *bitbucket.HTTPError {
	return &bitbucket.HTTPError{StatusCode: 404, Message: "Repository acme/nope not found", URL: "https://api.bitbucket.org/2.0/repositories/acme/nope"}
}

func TestPrintError_Human(t *testing.T) {
	ios, _, out, errOut := iostreams.Test()
	code := cmdutil.PrintError(ios, repoNotFound(), false)
	if code != 1 {
		t.Errorf("exit = %d", code)
	}
	want := "error: Repository acme/nope not found (HTTP 404)\nhint: " + repoNotFoundHint + "\n"
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", out.String())
	}
}

func TestPrintError_JSON(t *testing.T) {
	ios, _, _, errOut := iostreams.Test()
	cmdutil.PrintError(ios, repoNotFound(), true)
	want := `{"error":{"code":"not_found","status":404,"message":"Repository acme/nope not found (HTTP 404)","hint":"` + repoNotFoundHint + `"}}` + "\n"
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

func TestPrintError_SilentErrors(t *testing.T) {
	ios, _, _, errOut := iostreams.Test()
	if code := cmdutil.PrintError(ios, bitbucket.ErrDryRun, false); code != 0 {
		t.Errorf("dry run exit = %d", code)
	}
	if code := cmdutil.PrintError(ios, &cmdutil.ExitError{Code: 8}, true); code != 8 {
		t.Errorf("exit error code = %d", code)
	}
	if errOut.Len() != 0 {
		t.Errorf("silent errors printed %q", errOut.String())
	}
}

func TestClassify_FieldHintThatRepeatsTheMessage(t *testing.T) {
	err := &bitbucket.HTTPError{
		StatusCode: 400,
		Message:    "source: branch not found: feature/x",
		Fields:     map[string][]string{"source": {"branch not found: feature/x"}},
	}
	if info := cmdutil.Classify(err); info.Code != "validation" || info.Hint != "" {
		t.Errorf("Classify = %+v, want validation without a hint that repeats the message", info)
	}
}
