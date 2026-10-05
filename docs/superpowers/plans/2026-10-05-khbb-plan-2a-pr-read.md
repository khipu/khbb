# khbb Plan 2a — Foundation follow-ups and read-only PR commands

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the foundation items deferred by Plan 1's final review and ship the read-only pull request commands `khbb pr list`, `pr view`, `pr status`, `pr diff` and `pr checks`.

**Architecture:** Same patterns as Plan 1: one package per command under `pkg/cmd/pr/`, `XxxOptions` + injectable `runF`, the Bitbucket client in `internal/bitbucket` returns raw API structs, and `pkg/cmd/pr/shared` maps them to the stable JSON shapes of spec §8.1. Tests use `internal/httpmock` and shared fixtures in `pkg/cmd/pr/shared/prtest`.

**Tech Stack:** Go 1.26, cobra v1.10.2, go-gh v2.16.1 (`tableprinter`, `browser`, `template`, `jq`), go-keyring v0.2.8, yaml.v3, safeexec. Already in the module graph and promoted to direct dependencies here: `github.com/AlecAivazis/survey/v2` (interrupt detection), `github.com/itchyny/gojq` (jq syntax check), `github.com/cli/browser` (system browser).

**Spec:** `docs/superpowers/specs/2026-10-05-khbb-cli-design.md`. API facts: `docs/superpowers/specs/2026-10-05-bitbucket-api-findings.md` (sections "Resolved with live requests" and "Pull request API").

**Roadmap:** Plan 1 (done) → **Plan 2a (this)** → Plan 2b: `pr create/edit/comment/approve/unapprove/request-changes/merge/decline/checkout` (needs a sandbox repo to verify findings #1) → Plan 3: pipelines → Plan 4: release + agent skill.

## Global Constraints

Everything in Plan 1's Global Constraints still applies. The ones that matter most here:

- Module `github.com/khipu/khbb`; all code, help text, messages and docs in English.
- Must build and pass `go test -race ./...` on macOS and Linux and `go test ./...` on Windows; CI runs `golangci-lint`.
- stdout carries data only; prompts, progress, notices, warnings and errors go to stderr.
- Exit codes: `0` success, `1` error, `2` cancelled, `4` auth required, `8` checks pending.
- Flag shorthands reserved on commands that have the flag: `-R` = `--repo`, `-q` = `--jq`, `-t` = `--template`. `pr list` uses a long-only `--query` for BBQL.
- JSON field names are camelCase and stable; slices are always `[]`, never `null`.
- PullRequest JSON fields (spec §8.1): `id, title, body, state, draft, author, sourceBranch, sourceCommit, sourceRepo, destinationBranch, destinationCommit, mergeCommit, reviewers, participants, commentCount, taskCount, closeSourceBranch, url, createdOn, updatedOn, closedBy`. Reviewer `state` ∈ `approved | changes_requested | pending`.
- Comment JSON fields: `id, author, body, path, line, parentId, deleted, url, createdOn, updatedOn`.
- Check JSON fields: `key, name, state, description, url, updatedOn`, with `state` ∈ `successful | failed | inprogress | stopped`.
- User JSON fields: `displayName, nickname, uuid, accountId`.
- `pr status` JSON fields: `currentBranch` (PullRequest or null), `createdByMe` ([]), `needsMyReview` ([]).
- Timestamps are RFC 3339 UTC strings.
- `pr checks` exit status: 1 if any check failed or stopped (takes precedence), else 8 if any is in progress, else 0; no checks → 0 with a notice on stderr.
- Never request `pagelen` above 50 (pull requests cap at 50).
- Non-JSON resources (diff, patch) are fetched with `Accept: */*`.
- Test fixtures use fake identities only (`@example.com`, "Ada Example", `{00000000-…}` UUIDs, workspace `acme`).
- No new dependencies beyond the Tech Stack line.

## Decisions taken for this plan (rulings)

- Plan 2 is split: 2a (this, read-only) and 2b (write commands). — Each half ships working software; write commands need sandbox verification first.
- `KHBB_PAGER` / pager support is deferred to the backlog. — Agents never use a pager; humans can pipe to `less`. Cost if wrong: humans scroll long diffs.
- `pr view --json` also offers a `comments` field (array of Comment). — Agents need review comments as data (spec §13 recipe). Cost if wrong: one extra field.
- PR `state` keeps Bitbucket's uppercase values (`OPEN`, `MERGED`, `DECLINED`, `SUPERSEDED`), as `gh` does.
- `pr checks --interval` is an integer number of seconds (gh parity), default 5.
- When the current branch has several open PRs, commands fail with a usage error listing them instead of guessing.
- `--author` / `--reviewer` accept `@me`, `{uuid}`, an account ID (contains `:`) or a nickname, mapped straight to BBQL (no workspace-member lookup; that arrives with reviewer assignment in Plan 2b).
- `--web` uses the config `browser`, then `$BROWSER`, then the system default — never GitHub CLI settings.
- `khbb api`'s `{branch}` placeholder stays unescaped (gh parity); our own commands never put branch names in URL paths.

## Review Focus

1. **Branch names with `/` or `"`** in the current-branch lookup and `--head/--base` must be quoted as BBQL string literals — tests `TestFind_CurrentBranchQuotesTheName` (Task 7) and `TestList_FiltersAndTTYTable` (Task 8).
2. **Empty results** print `[]` (or a stderr notice) and exit 0, never `null` — tests `TestList_EmptyResults` (Task 8), `TestStatus_JSONEmptyArrays` (Task 10), `TestChecks_NoChecks` (Task 12).
3. **Pull requests without author or participants** (list items, deleted accounts) render without panics — tests `TestNewPullRequest_WithoutAuthor`, `TestNewPullRequest_ListItemHasEmptyArrays` (Task 6), `TestAuthorAndStateLabel` (Task 8).
4. **Current branch with zero or several open PRs** fails with a message naming the branch or the PR numbers — tests `TestFind_NoOpenPullRequest`, `TestFind_SeveralOpenPullRequests` (Task 7).
5. **`pr checks --watch` and transient failures**: keep polling through 5xx/429/network errors, stop at once on 4xx — tests `TestChecks_WatchRetriesTransientErrors`, `TestChecks_WatchStopsOnNotFound` (Task 12).

---

## File Map

| File | Responsibility | Task |
|---|---|---|
| `internal/cmdutil/args.go` | Positional-argument validators that return usage errors | 1 |
| `internal/cmdutil/errors.go` | Ctrl-C → cancelled; 404 hint only for repositories; field errors as hints | 1, 3 |
| `internal/cmdutil/filters.go` | `--jq` / `--template` syntax validation | 2 |
| `internal/cmdutil/json.go` | Validate filters in `AddJSONFlags` | 2 |
| `internal/cmdutil/factory.go` | `Browser` interface and field | 9 |
| `internal/bitbucket/errors.go` | `HTTPError.Fields` | 3 |
| `internal/bitbucket/paths.go` | `RepoPath`, `QuoteBBQL` | 3 |
| `internal/bitbucket/users.go` | `WhoAmI`, `MissingScopes` | 4 |
| `internal/bitbucket/text.go` | `GetText` for diff/patch | 5 |
| `internal/bitbucket/pullrequests.go` | PR, comment, status, diffstat API | 5 |
| `internal/gitctx/repo.go` | Strict workspace/slug characters | 3 |
| `internal/iostreams/color.go` | ANSI color helpers | 8 |
| `pkg/cmd/root/root.go` | Unknown command → usage error; register `pr` | 1, 8 |
| `pkg/cmd/api/api.go`, `pkg/cmd/version/version.go`, `pkg/cmd/auth/{login,logout,status}/*.go` | Use the new arg validators | 1 |
| `pkg/cmd/auth/status/status.go` | Show token scopes and missing ones | 4 |
| `pkg/cmd/auth/login/login.go` | Show token instructions before the email prompt | 4 |
| `pkg/cmd/factory/default.go` | Browser launcher | 9 |
| `pkg/cmd/pr/pr.go` | `khbb pr` group | 8–12 |
| `pkg/cmd/pr/shared/export.go` | JSON shapes and mapping | 6 |
| `pkg/cmd/pr/shared/prtest/prtest.go` | Test fixtures and helpers | 6 |
| `pkg/cmd/pr/shared/finder.go` | PR selector parsing and lookup | 7 |
| `pkg/cmd/pr/shared/query.go` | `@me`/uuid/account/nickname → BBQL | 7 |
| `pkg/cmd/pr/shared/display.go` | State labels, author names, review summaries | 8 |
| `pkg/cmd/pr/list/list.go` | `khbb pr list` | 8 |
| `pkg/cmd/pr/view/view.go` | `khbb pr view` | 9 |
| `pkg/cmd/pr/status/status.go` | `khbb pr status` | 10 |
| `pkg/cmd/pr/diff/diff.go` | `khbb pr diff` | 11 |
| `pkg/cmd/pr/checks/checks.go` | `khbb pr checks` | 12 |

---

### Task 1: Usage errors for arguments and unknown commands; Ctrl-C exits 2; scoped 404 hint

**Files:**
- Create: `internal/cmdutil/args.go`, `internal/cmdutil/args_test.go`
- Modify: `internal/cmdutil/errors.go`, `internal/cmdutil/errors_test.go`
- Modify: `pkg/cmd/root/root.go`, `pkg/cmd/root/root_test.go`
- Modify: `pkg/cmd/version/version.go`, `pkg/cmd/auth/login/login.go`, `pkg/cmd/auth/logout/logout.go`, `pkg/cmd/auth/status/status.go`, `pkg/cmd/api/api.go` (only their `Args:` lines)

**Interfaces:**
- Consumes: `cmdutil.FlagErrorf`, `cmdutil.GroupRunE`, `cmdutil.Classify`.
- Produces: `cmdutil.NoArgs(cmd *cobra.Command, args []string) error`; `cmdutil.ExactArgs(n int, what string) cobra.PositionalArgs`; `cmdutil.MaximumNArgs(n int, what string) cobra.PositionalArgs` — all return `*FlagError`. `Classify` maps `terminal.InterruptErr` (survey) to code `cancelled`, exit 2. A 404 gets the private-repository hint only when its URL contains `/repositories/`.

- [ ] **Step 1: Write the failing tests**

`internal/cmdutil/args_test.go`:

```go
package cmdutil_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
)

func runArgs(validator cobra.PositionalArgs, args ...string) error {
	cmd := &cobra.Command{Use: "thing", Args: validator, RunE: func(*cobra.Command, []string) error { return nil }}
	// A non-nil slice: cobra falls back to os.Args when SetArgs receives nil.
	cmd.SetArgs(append([]string{}, args...))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestArgValidatorsReturnUsageErrors(t *testing.T) {
	cases := []struct {
		name      string
		validator cobra.PositionalArgs
		args      []string
		wantErr   string // substring; "" means success
	}{
		{"no args ok", cmdutil.NoArgs, nil, ""},
		{"no args extra", cmdutil.NoArgs, []string{"x"}, `unexpected argument "x" for "thing"`},
		{"exact ok", cmdutil.ExactArgs(1, "<path>"), []string{"user"}, ""},
		{"exact missing", cmdutil.ExactArgs(1, "<path>"), nil, "thing requires <path>"},
		{"exact extra", cmdutil.ExactArgs(1, "<path>"), []string{"a", "b"}, "thing accepts 1 argument(s) (<path>), received 2"},
		{"max ok", cmdutil.MaximumNArgs(1, "[<number>]"), nil, ""},
		{"max extra", cmdutil.MaximumNArgs(1, "[<number>]"), []string{"1", "2"}, "thing accepts at most 1 argument(s) ([<number>]), received 2"},
	}
	for _, tc := range cases {
		err := runArgs(tc.validator, tc.args...)
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", tc.name, err)
			}
			continue
		}
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: err = %v, want a FlagError containing %q", tc.name, err, tc.wantErr)
		}
	}
}
```

In `internal/cmdutil/errors_test.go`:

1. Add `"github.com/AlecAivazis/survey/v2/terminal"` to the imports.
2. In `TestClassify`'s table, add after the `"cancel"` row:

```go
		{"interrupt", fmt.Errorf("could not prompt: %w", terminal.InterruptErr), "cancelled", 2, 0, "", false},
```

3. Replace the `"404"` row with:

```go
		{"404 repository", &bitbucket.HTTPError{StatusCode: 404, URL: "https://api.bitbucket.org/2.0/repositories/acme/nope"}, "not_found", 1, 404, "private repositories", false},
```

4. Add this test:

```go
func TestClassify_404HintOnlyForRepositories(t *testing.T) {
	info := cmdutil.Classify(&bitbucket.HTTPError{StatusCode: 404, URL: "https://api.bitbucket.org/2.0/user"})
	if info.Code != "not_found" || info.Hint != "" {
		t.Errorf("Classify = %+v, want not_found without a hint", info)
	}
}
```

5. Replace `TestPrintError_Human` and `TestPrintError_JSON` with:

```go
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
```

Append to `pkg/cmd/root/root_test.go`:

```go
func TestRootUnknownCommandIsUsageError(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{"nope"})
	err := cmd.Execute()
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), `unknown command "nope" for "khbb"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestRootWithoutArgumentsPrintsHelp(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("out = %q", out.String())
	}
}

func TestArgumentCountErrorsAreUsageErrors(t *testing.T) {
	for _, args := range [][]string{{"version", "extra"}, {"api"}, {"auth", "status", "extra"}} {
		ios, _, _, _ := iostreams.Test()
		cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
		cmd.SetArgs(args)
		var flagErr *cmdutil.FlagError
		if err := cmd.Execute(); !errors.As(err, &flagErr) {
			t.Errorf("%v: expected FlagError, got %T: %v", args, err, err)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cmdutil/ ./pkg/cmd/root/`
Expected: FAIL — `undefined: cmdutil.NoArgs` (build failure of the cmdutil tests); once that compiles, the root tests fail because `nope` and `version extra` return plain cobra errors.

- [ ] **Step 3: Implement**

`internal/cmdutil/args.go`:

```go
package cmdutil

import "github.com/spf13/cobra"

// NoArgs rejects positional arguments as a usage error.
func NoArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return FlagErrorf("unexpected argument %q for %q", args[0], cmd.CommandPath())
	}
	return nil
}

// ExactArgs requires exactly n positional arguments; what names them in messages (for example "<path>").
func ExactArgs(n int, what string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		switch {
		case len(args) < n:
			return FlagErrorf("%s requires %s", cmd.CommandPath(), what)
		case len(args) > n:
			return FlagErrorf("%s accepts %d argument(s) (%s), received %d", cmd.CommandPath(), n, what, len(args))
		}
		return nil
	}
}

// MaximumNArgs allows at most n positional arguments; what names them in messages.
func MaximumNArgs(n int, what string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return FlagErrorf("%s accepts at most %d argument(s) (%s), received %d", cmd.CommandPath(), n, what, len(args))
		}
		return nil
	}
}
```

In `internal/cmdutil/errors.go`:

- Add `"github.com/AlecAivazis/survey/v2/terminal"` to the imports.
- Replace `case errors.Is(err, ErrCancel):` with:

```go
	case errors.Is(err, ErrCancel), errors.Is(err, terminal.InterruptErr):
```

- Replace the 404 case in `classifyHTTP` with:

```go
	case e.StatusCode == 404:
		info.Code = "not_found"
		if strings.Contains(e.URL, "/repositories/") {
			info.Hint = "check the repository name and your access: private repositories return 404 when you lack access"
		}
```

In `pkg/cmd/root/root.go`, add these two fields to the root `cobra.Command` literal (after `Version: f.AppVersion,`):

```go
		Args:          cobra.ArbitraryArgs,
		RunE:          cmdutil.GroupRunE,
```

Change the `Args:` line of these commands:

| File | New line |
|---|---|
| `pkg/cmd/version/version.go` | `Args:  cmdutil.NoArgs,` |
| `pkg/cmd/auth/login/login.go` | `Args: cmdutil.NoArgs,` |
| `pkg/cmd/auth/logout/logout.go` | `Args:  cmdutil.NoArgs,` |
| `pkg/cmd/auth/status/status.go` | `Args:  cmdutil.NoArgs,` |
| `pkg/cmd/api/api.go` | `Args: cmdutil.ExactArgs(1, "<path>"),` |

Then run `go mod tidy` (survey becomes a direct requirement).

- [ ] **Step 4: Run the tests**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: all `ok`; vet silent; gofmt prints nothing.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/cmdutil pkg/cmd
git commit -m "fix(cmdutil): usage errors for arguments, exit 2 on Ctrl-C, scoped 404 hint"
```

---

### Task 2: Validate `--jq` and `--template` before anything runs

**Files:**
- Create: `internal/cmdutil/filters.go`, `internal/cmdutil/filters_test.go`
- Modify: `internal/cmdutil/json.go`, `internal/cmdutil/json_test.go`, `pkg/cmd/api/api.go`, `pkg/cmd/api/api_test.go`

**Interfaces:**
- Consumes: `cmdutil.FlagErrorf`; go-gh `template.New(w, width, color).Parse`; `gojq.Parse`.
- Produces: `cmdutil.ValidateJQ(expr string) error`, `cmdutil.ValidateTemplate(tmpl string) error` (nil for ""; `*FlagError` otherwise). `AddJSONFlags`' PreRunE and `khbb api` call them before any request.

- [ ] **Step 1: Write the failing tests**

`internal/cmdutil/filters_test.go`:

```go
package cmdutil_test

import (
	"errors"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
)

func TestValidateJQ(t *testing.T) {
	for _, ok := range []string{"", ".", ".values[].id", `.[] | select(.state == "OPEN") | .title`} {
		if err := cmdutil.ValidateJQ(ok); err != nil {
			t.Errorf("ValidateJQ(%q) = %v", ok, err)
		}
	}
	var flagErr *cmdutil.FlagError
	if err := cmdutil.ValidateJQ(".["); !errors.As(err, &flagErr) {
		t.Errorf("ValidateJQ(.[) = %v, want FlagError", err)
	}
}

func TestValidateTemplate(t *testing.T) {
	for _, ok := range []string{"", `{{range .}}{{.id}}{{"\n"}}{{end}}`, `{{tablerow "a" "b"}}{{tablerender}}`} {
		if err := cmdutil.ValidateTemplate(ok); err != nil {
			t.Errorf("ValidateTemplate(%q) = %v", ok, err)
		}
	}
	var flagErr *cmdutil.FlagError
	if err := cmdutil.ValidateTemplate("{{"); !errors.As(err, &flagErr) {
		t.Errorf("ValidateTemplate({{) = %v, want FlagError", err)
	}
}
```

Append to `internal/cmdutil/json_test.go`:

```go
func TestJSON_InvalidFiltersAreRejectedBeforeRunning(t *testing.T) {
	for _, tc := range []struct{ args []string; want string }{
		{[]string{"--json", "id", "--jq", ".["}, "invalid --jq expression"},
		{[]string{"--json", "id", "--template", "{{"}, "invalid --template"},
	} {
		_, err := runJSON(t, samples, tc.args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v", tc.args, err)
		}
	}
}
```

In `pkg/cmd/api/api_test.go`, add two rows to the `cases` slice of `TestNewCmdAPI_Validation`:

```go
		{"--jq", ".[", "user"},
		{"-X", "POST", "--template", "{{", "user"},
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cmdutil/ ./pkg/cmd/api/`
Expected: FAIL — `undefined: cmdutil.ValidateJQ`.

- [ ] **Step 3: Implement**

`internal/cmdutil/filters.go`:

```go
package cmdutil

import (
	"io"

	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/itchyny/gojq"
)

// ValidateJQ reports a usage error when expr is not a valid jq expression ("" is valid).
func ValidateJQ(expr string) error {
	if expr == "" {
		return nil
	}
	if _, err := gojq.Parse(expr); err != nil {
		return FlagErrorf("invalid --jq expression: %v", err)
	}
	return nil
}

// ValidateTemplate reports a usage error when tmpl is not a valid Go template ("" is valid).
func ValidateTemplate(tmpl string) error {
	if tmpl == "" {
		return nil
	}
	if err := template.New(io.Discard, 80, false).Parse(tmpl); err != nil {
		return FlagErrorf("invalid --template: %v", err)
	}
	return nil
}
```

In `internal/cmdutil/json.go`, inside the PreRunE, right after the `cannot use --jq and --template together` check, add:

```go
		if err := ValidateJQ(jqExpr); err != nil {
			return err
		}
		if err := ValidateTemplate(tmpl); err != nil {
			return err
		}
```

In `pkg/cmd/api/api.go`, in `validateFlags`, replace the final `return validateFields(opts)` with:

```go
	if err := cmdutil.ValidateJQ(opts.JQ); err != nil {
		return err
	}
	if err := cmdutil.ValidateTemplate(opts.Template); err != nil {
		return err
	}
	return validateFields(opts)
```

Run `go mod tidy` (gojq becomes direct).

- [ ] **Step 4: Run the tests**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: all `ok`, nothing printed by vet/gofmt.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/cmdutil pkg/cmd/api
git commit -m "fix(cmdutil): validate --jq and --template before running"
```

---

### Task 3: Bitbucket field errors, safe path building, strict repository names

**Files:**
- Create: `internal/bitbucket/paths.go`, `internal/bitbucket/paths_test.go`, `internal/bitbucket/errors_test.go`
- Modify: `internal/bitbucket/errors.go`, `internal/cmdutil/errors.go`, `internal/cmdutil/errors_test.go`, `internal/gitctx/repo.go`, `internal/gitctx/remote_test.go`

**Interfaces:**
- Consumes: `bitbucket.ParseHTTPError`, `cmdutil.Classify`.
- Produces: `bitbucket.HTTPError.Fields map[string][]string` (from Bitbucket's `error.fields`); `bitbucket.RepoPath(workspace, slug string, segments ...string) string` (every element path-escaped, no leading slash); `bitbucket.QuoteBBQL(s string) string`; `gitctx.ParseRepo` accepts only `[A-Za-z0-9._-]` parts (not `.`/`..`). `Classify` uses Fields as the hint when there is no other hint.

- [ ] **Step 1: Write the failing tests**

`internal/bitbucket/errors_test.go`:

```go
package bitbucket_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
)

func TestParseHTTPError_Fields(t *testing.T) {
	resp := &http.Response{StatusCode: 400, Request: httptest.NewRequest("POST", "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests", nil)}
	body := `{"type":"error","error":{"message":"Bad request","fields":{"source":["source branch not found"],"title":"too long"}}}`
	e := bitbucket.ParseHTTPError(resp, []byte(body))
	if e.Message != "Bad request" || e.Method != "POST" {
		t.Errorf("unexpected error: %+v", e)
	}
	if !slices.Equal(e.Fields["source"], []string{"source branch not found"}) || !slices.Equal(e.Fields["title"], []string{"too long"}) {
		t.Errorf("Fields = %v", e.Fields)
	}
}
```

`internal/bitbucket/paths_test.go`:

```go
package bitbucket_test

import (
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
)

func TestRepoPath(t *testing.T) {
	cases := map[string]string{
		bitbucket.RepoPath("acme", "widgets", "pullrequests", "12"):         "repositories/acme/widgets/pullrequests/12",
		bitbucket.RepoPath("acme", "widgets", "pipelines", "{abc}", "steps"): "repositories/acme/widgets/pipelines/%7Babc%7D/steps",
		bitbucket.RepoPath("acme", "widgets"):                               "repositories/acme/widgets",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("RepoPath = %q, want %q", got, want)
		}
	}
}

func TestQuoteBBQL(t *testing.T) {
	if got, want := bitbucket.QuoteBBQL(`fix/"quoted"\x`), `"fix/\"quoted\"\\x"`; got != want {
		t.Errorf("QuoteBBQL = %s, want %s", got, want)
	}
}
```

In `internal/cmdutil/errors_test.go`, add a row to `TestClassify`:

```go
		{"400 fields", &bitbucket.HTTPError{StatusCode: 400, Fields: map[string][]string{"title": {"too long"}, "source": {"branch not found"}}}, "validation", 1, 400, "source: branch not found; title: too long", false},
```

In `internal/gitctx/remote_test.go`, extend the bad list in `TestParseRepo` to:

```go
	for _, bad := range []string{"", "acme", "acme/", "/widgets", "a/b/c", "acme/wid gets", "acme/..", "acme/w?x", "acme/w#x", "../widgets"} {
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/bitbucket/ ./internal/cmdutil/ ./internal/gitctx/`
Expected: FAIL — `e.Fields undefined`, `undefined: bitbucket.RepoPath`, and `ParseRepo("acme/wid gets"): expected error`.

- [ ] **Step 3: Implement**

In `internal/bitbucket/errors.go`:

- Add to `HTTPError` (after `RequiredScopes`):

```go
	// Fields holds per-field validation messages from Bitbucket's error.fields.
	Fields map[string][]string
```

- In `ParseHTTPError`, add `Fields map[string]json.RawMessage `json:"fields"`` to the anonymous `Error` struct, and after `e.Detail, e.RequiredScopes = parseDetail(...)` add `e.Fields = parseFields(payload.Error.Fields)`.
- Add:

```go
// parseFields accepts each field's messages as a list of strings or a single string.
func parseFields(raw map[string]json.RawMessage) map[string][]string {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string][]string, len(raw))
	for k, v := range raw {
		var list []string
		if json.Unmarshal(v, &list) == nil {
			out[k] = list
			continue
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			out[k] = []string{s}
			continue
		}
		out[k] = []string{string(v)}
	}
	return out
}
```

`internal/bitbucket/paths.go`:

```go
package bitbucket

import (
	"net/url"
	"strings"
)

// RepoPath builds "repositories/{workspace}/{slug}/{segments...}" with every element path-escaped.
func RepoPath(workspace, slug string, segments ...string) string {
	parts := []string{"repositories", url.PathEscape(workspace), url.PathEscape(slug)}
	for _, s := range segments {
		parts = append(parts, url.PathEscape(s))
	}
	return strings.Join(parts, "/")
}

// QuoteBBQL returns s as a double-quoted BBQL string literal.
func QuoteBBQL(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
```

In `internal/cmdutil/errors.go`, add `"maps"` and `"slices"` to the imports, append to `classifyHTTP` (after the existing `e.Detail` fallback):

```go
	if info.Hint == "" && len(e.Fields) > 0 {
		info.Hint = formatFields(e.Fields)
	}
```

and add:

```go
// formatFields renders Bitbucket field errors as "field: msg, msg; other: msg", sorted by field.
func formatFields(fields map[string][]string) string {
	keys := slices.Sorted(maps.Keys(fields))
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+strings.Join(fields[k], ", "))
	}
	return strings.Join(parts, "; ")
}
```

Replace `internal/gitctx/repo.go`'s `ParseRepo` with (add `"regexp"` to the imports):

```go
var repoPartRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ParseRepo parses WORKSPACE/REPO. Each part may contain letters, digits, '.', '_' and '-'.
func ParseRepo(s string) (Repo, error) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 2 || !validRepoPart(parts[0]) || !validRepoPart(parts[1]) {
		return Repo{}, fmt.Errorf("invalid repository %q: expected WORKSPACE/REPO", s)
	}
	return Repo{Workspace: parts[0], Slug: parts[1]}, nil
}

func validRepoPart(p string) bool {
	return p != "." && p != ".." && repoPartRE.MatchString(p)
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal
git commit -m "feat(bitbucket): field errors, escaped repo paths, BBQL quoting, strict repo names"
```

---

### Task 4: `auth status` shows token scopes; `auth login` shows instructions first

**Files:**
- Create: `internal/bitbucket/users_test.go`
- Modify: `internal/bitbucket/users.go`, `pkg/cmd/auth/status/status.go`, `pkg/cmd/auth/status/status_test.go`, `pkg/cmd/auth/login/login.go`, `pkg/cmd/auth/login/login_test.go`

**Interfaces:**
- Consumes: `(*Client).Request`, `readHTTPError`, `RequiredScopes`; test helpers `newTestClient`, `userJSON` from `internal/bitbucket/client_test.go`.
- Produces: `(*Client).WhoAmI(ctx) (*User, []string, error)` — scopes come from the `X-Oauth-Scopes` response header (nil when absent); `bitbucket.MissingScopes(granted []string) []string`.

- [ ] **Step 1: Write the failing tests**

`internal/bitbucket/users_test.go`:

```go
package bitbucket_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

func TestWhoAmI_ReturnsGrantedScopes(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/user", httpmock.WithHeader(httpmock.JSONResponse(200, userJSON), "X-Oauth-Scopes", "read:user:bitbucket, read:pullrequest:bitbucket"))
	u, scopes, err := c.WhoAmI(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Nickname != "ada" || !slices.Equal(scopes, []string{"read:user:bitbucket", "read:pullrequest:bitbucket"}) {
		t.Errorf("user %+v scopes %v", u, scopes)
	}
}

func TestWhoAmI_WithoutScopeHeader(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))
	_, scopes, err := c.WhoAmI(context.Background())
	if err != nil || scopes != nil {
		t.Errorf("scopes %v err %v", scopes, err)
	}
}

func TestWhoAmI_Unauthorized(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(401, `{}`))
	_, _, err := c.WhoAmI(context.Background())
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 401 {
		t.Errorf("err = %v", err)
	}
}

func TestMissingScopes(t *testing.T) {
	if got := bitbucket.MissingScopes([]string{"read:user:bitbucket"}); !slices.Equal(got, bitbucket.RequiredScopes[1:]) {
		t.Errorf("MissingScopes = %v", got)
	}
	if got := bitbucket.MissingScopes(bitbucket.RequiredScopes); len(got) != 0 {
		t.Errorf("MissingScopes(all) = %v", got)
	}
}
```

Append to `pkg/cmd/auth/status/status_test.go`:

```go
func TestStatus_ShowsScopesAndMissingOnes(t *testing.T) {
	opts, reg, out := newOpts(t, keyringCreds, nil)
	reg.Register("GET", "/2.0/user", httpmock.WithHeader(
		httpmock.JSONResponse(200, `{"display_name":"Ada Example","nickname":"ada"}`),
		"X-Oauth-Scopes", "read:user:bitbucket, read:pullrequest:bitbucket"))

	if err := statusRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	want := "bitbucket.org\n  Logged in as ada (Ada Example)\n  Email: dev@example.com\n  Token source: keyring\n" +
		"  Token scopes: read:user:bitbucket, read:pullrequest:bitbucket\n" +
		"  Missing scopes: read:workspace:bitbucket, read:repository:bitbucket, write:pullrequest:bitbucket, read:pipeline:bitbucket, write:pipeline:bitbucket\n"
	if out() != want {
		t.Errorf("out = %q, want %q", out(), want)
	}
}
```

In `pkg/cmd/auth/login/login_test.go`, inside `TestLogin_InteractivePromptsAndListsScopes`, replace the `pm.RegisterInput(...)` line with:

```go
	pm.RegisterInput("Atlassian account email:", func(_, _ string) (string, error) {
		if !strings.Contains(fx.stderr.String(), tokenURL) {
			t.Error("the token instructions must be shown before the email prompt")
		}
		return " dev@example.com ", nil
	})
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/bitbucket/ ./pkg/cmd/auth/...`
Expected: FAIL — `c.WhoAmI undefined`; then the status test misses the scope lines and the login test reports the instructions came too late.

- [ ] **Step 3: Implement**

Append to `internal/bitbucket/users.go` (imports become `context`, `encoding/json`, `fmt`, `net/http`, `slices`, `strings`):

```go
// WhoAmI returns the authenticated account and the scopes granted to its token, read from the
// X-Oauth-Scopes response header (nil when Bitbucket does not send it).
func (c *Client) WhoAmI(ctx context.Context) (*User, []string, error) {
	resp, err := c.Request(ctx, http.MethodGet, "user", nil, nil)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, nil, readHTTPError(resp)
	}
	var u User
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, nil, fmt.Errorf("decoding the current user: %w", err)
	}
	return &u, splitScopes(resp.Header.Get("X-Oauth-Scopes")), nil
}

func splitScopes(header string) []string {
	var scopes []string
	for _, s := range strings.Split(header, ",") {
		if s = strings.TrimSpace(s); s != "" {
			scopes = append(scopes, s)
		}
	}
	return scopes
}

// MissingScopes returns the RequiredScopes that granted lacks, in RequiredScopes order.
func MissingScopes(granted []string) []string {
	var missing []string
	for _, s := range RequiredScopes {
		if !slices.Contains(granted, s) {
			missing = append(missing, s)
		}
	}
	return missing
}
```

In `pkg/cmd/auth/status/status.go` (add `"strings"` to the imports), replace from `user, err := opts.NewClient(creds.Email, creds.Token).CurrentUser(ctx)` to the end of `statusRun` with:

```go
	user, scopes, err := opts.NewClient(creds.Email, creds.Token).WhoAmI(ctx)
	if err != nil {
		var httpErr *bitbucket.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized {
			return &cmdutil.AuthError{Msg: fmt.Sprintf("the token for %s (from %s) is invalid or expired", creds.Email, creds.Source)}
		}
		return err
	}
	fmt.Fprintf(opts.IO.Out, "bitbucket.org\n  Logged in as %s (%s)\n  Email: %s\n  Token source: %s\n",
		user.Nickname, user.DisplayName, creds.Email, creds.Source)
	if scopes != nil {
		fmt.Fprintf(opts.IO.Out, "  Token scopes: %s\n", strings.Join(scopes, ", "))
		if missing := bitbucket.MissingScopes(scopes); len(missing) > 0 {
			fmt.Fprintf(opts.IO.Out, "  Missing scopes: %s\n", strings.Join(missing, ", "))
		}
	}
	return nil
}
```

In `pkg/cmd/auth/login/login.go`:

- In `loginRun`, insert before `email, err := readEmail(opts, cfg.Email)`:

```go
	if !opts.WithToken && opts.IO.CanPrompt() {
		printTokenInstructions(opts.IO.ErrOut)
	}
```

- In `readToken`, delete the four lines that print the token URL and the scopes from the `case opts.IO.CanPrompt():` branch (keep the `Password` prompt).
- Add:

```go
func printTokenInstructions(w io.Writer) {
	fmt.Fprintf(w, "Create an API token at %s\nwith these scopes:\n", tokenURL)
	for _, s := range bitbucket.RequiredScopes {
		fmt.Fprintf(w, "  - %s\n", s)
	}
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/bitbucket pkg/cmd/auth
git commit -m "feat(auth): show granted and missing token scopes; instructions before prompts"
```

---

### Task 5: Pull request API in the Bitbucket client

**Files:**
- Create: `internal/bitbucket/text.go`, `internal/bitbucket/pullrequests.go`, `internal/bitbucket/pullrequests_test.go`

**Interfaces:**
- Consumes: `Client.Request`, `Client.Do`, `List[T]`, `RepoPath`, `readHTTPError`, `User`; test helper `newTestClient`.
- Produces:
  - `(*Client).GetText(ctx, path string) (string, error)` — GET with `Accept: */*`; non-2xx → `*HTTPError`.
  - Types `PullRequest`, `PREndpoint`, `Commit{Hash}`, `Link{Href}`, `Participant{User, Role, Approved, State *string}`, `Comment`, `CommitStatus`, `DiffStat` (fields below).
  - `PRListOptions{States []string; Query string; WithParticipants bool}`.
  - `(*Client).ListPullRequests(ctx, workspace, slug string, opts PRListOptions, limit int) ([]PullRequest, error)`, `GetPullRequest(ctx, workspace, slug string, id int) (*PullRequest, error)`, `ListPRComments(ctx, workspace, slug string, id int) ([]Comment, error)`, `ListPRStatuses(...) ([]CommitStatus, error)`, `PRDiffStat(...) ([]DiffStat, error)`, `PRDiff(...) (string, error)`, `PRPatch(...) (string, error)`.

- [ ] **Step 1: Write the failing tests**

`internal/bitbucket/pullrequests_test.go`:

```go
package bitbucket_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

const prPath = "/2.0/repositories/acme/widgets/pullrequests"

const prJSON = `{"id":42,"title":"Add widgets","description":"Adds the widget factory.","state":"OPEN","draft":true,` +
	`"author":{"display_name":"Ada Example","nickname":"ada","uuid":"{00000000-0000-0000-0000-000000000001}","account_id":"000000:aaaa"},` +
	`"source":{"branch":{"name":"feature/widgets"},"commit":{"hash":"abc1234"},"repository":{"full_name":"acme/widgets"}},` +
	`"destination":{"branch":{"name":"main"},"commit":{"hash":"def5678"},"repository":{"full_name":"acme/widgets"}},` +
	`"merge_commit":null,` +
	`"reviewers":[{"display_name":"Bob Example","nickname":"bob","uuid":"{00000000-0000-0000-0000-000000000002}","account_id":"000000:bbbb"}],` +
	`"participants":[{"user":{"display_name":"Bob Example","nickname":"bob","uuid":"{00000000-0000-0000-0000-000000000002}","account_id":"000000:bbbb"},"role":"REVIEWER","approved":true,"state":"approved"}],` +
	`"comment_count":2,"task_count":1,"close_source_branch":true,"closed_by":null,` +
	`"created_on":"2026-10-01T12:00:00.000000+00:00","updated_on":"2026-10-02T08:30:00.000000+00:00",` +
	`"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/42"}}}`

func TestListPullRequests_BuildsQuery(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath, httpmock.JSONResponse(200, `{"values":[`+prJSON+`]}`))

	prs, err := c.ListPullRequests(context.Background(), "acme", "widgets", bitbucket.PRListOptions{
		States: []string{"OPEN", "MERGED"}, Query: `author.uuid = "{x}"`, WithParticipants: true,
	}, 30)
	if err != nil {
		t.Fatal(err)
	}
	q := reg.Calls[0].URL.Query()
	if !slices.Equal(q["state"], []string{"OPEN", "MERGED"}) || q.Get("q") != `author.uuid = "{x}"` ||
		q.Get("fields") != "+values.participants,+values.reviewers" || q.Get("pagelen") != "30" {
		t.Errorf("query = %v", q)
	}
	if !strings.Contains(reg.Calls[0].URL.RawQuery, "fields=%2Bvalues.participants") {
		t.Errorf("'+' must be percent-encoded: %s", reg.Calls[0].URL.RawQuery)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d pull requests", len(prs))
	}
	pr := prs[0]
	if pr.ID != 42 || !pr.Draft || pr.Source.Branch.Name != "feature/widgets" || pr.Source.Commit.Hash != "abc1234" ||
		pr.Destination.Repository.FullName != "acme/widgets" || pr.Author.Nickname != "ada" || pr.MergeCommit != nil {
		t.Errorf("decoded %+v", pr)
	}
	if len(pr.Participants) != 1 || pr.Participants[0].State == nil || *pr.Participants[0].State != "approved" {
		t.Errorf("participants = %+v", pr.Participants)
	}
	if !pr.CreatedOn.Equal(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)) || pr.Links.HTML.Href != "https://bitbucket.org/acme/widgets/pull-requests/42" {
		t.Errorf("created %v url %q", pr.CreatedOn, pr.Links.HTML.Href)
	}
}

func TestListPullRequests_DefaultsToPagelenOnly(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath, httpmock.JSONResponse(200, `{"values":[]}`))
	prs, err := c.ListPullRequests(context.Background(), "acme", "widgets", bitbucket.PRListOptions{}, 0)
	if err != nil || len(prs) != 0 {
		t.Fatalf("prs %v err %v", prs, err)
	}
	if got := reg.Calls[0].URL.RawQuery; got != "pagelen=50" {
		t.Errorf("query = %q", got)
	}
}

func TestGetPullRequest(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath+"/42", httpmock.JSONResponse(200, prJSON))
	pr, err := c.GetPullRequest(context.Background(), "acme", "widgets", 42)
	if err != nil || pr.Title != "Add widgets" || len(pr.Reviewers) != 1 {
		t.Fatalf("pr %+v err %v", pr, err)
	}
}

func TestPullRequestSubresources(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath+"/42/comments", httpmock.JSONResponse(200,
		`{"values":[{"id":101,"content":{"raw":"Please rename this."},"inline":{"path":"src/widget.go","from":null,"to":12},"deleted":false},`+
			`{"id":102,"content":{"raw":"Done."},"parent":{"id":101},"deleted":false}]}`))
	reg.Register("GET", prPath+"/42/statuses", httpmock.JSONResponse(200,
		`{"values":[{"key":"build","name":"Build","state":"SUCCESSFUL","description":"ok","url":"https://ci.example.com/build","updated_on":"2026-10-02T09:00:00.000000+00:00"}]}`))
	reg.Register("GET", prPath+"/42/diffstat", httpmock.JSONResponse(200,
		`{"values":[{"status":"removed","lines_added":0,"lines_removed":3,"old":{"path":"docs/old.md"},"new":null}]}`))
	reg.Register("GET", prPath+"/42/diff", httpmock.StringResponse(200, "diff --git a/x b/x\n"))
	reg.Register("GET", prPath+"/42/patch", httpmock.StringResponse(200, "From abc\n"))

	ctx := context.Background()
	comments, err := c.ListPRComments(ctx, "acme", "widgets", 42)
	if err != nil || len(comments) != 2 || *comments[0].Inline.To != 12 || comments[1].Parent.ID != 101 {
		t.Fatalf("comments %+v err %v", comments, err)
	}
	statuses, err := c.ListPRStatuses(ctx, "acme", "widgets", 42)
	if err != nil || len(statuses) != 1 || statuses[0].State != "SUCCESSFUL" || statuses[0].URL != "https://ci.example.com/build" {
		t.Fatalf("statuses %+v err %v", statuses, err)
	}
	stats, err := c.PRDiffStat(ctx, "acme", "widgets", 42)
	if err != nil || len(stats) != 1 || stats[0].New != nil || stats[0].Old.Path != "docs/old.md" {
		t.Fatalf("stats %+v err %v", stats, err)
	}
	diff, err := c.PRDiff(ctx, "acme", "widgets", 42)
	if err != nil || diff != "diff --git a/x b/x\n" {
		t.Fatalf("diff %q err %v", diff, err)
	}
	patch, err := c.PRPatch(ctx, "acme", "widgets", 42)
	if err != nil || patch != "From abc\n" {
		t.Fatalf("patch %q err %v", patch, err)
	}
	for _, call := range reg.Calls[3:] {
		if got := call.Header.Get("Accept"); got != "*/*" {
			t.Errorf("%s: Accept = %q, want */*", call.URL.Path, got)
		}
	}
}

func TestGetText_ErrorStatus(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath+"/99/diff", httpmock.JSONResponse(404, `{"type":"error","error":{"message":"Pull request not found"}}`))
	_, err := c.PRDiff(context.Background(), "acme", "widgets", 99)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 || httpErr.Message != "Pull request not found" {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/bitbucket/`
Expected: FAIL — `c.ListPullRequests undefined`.

- [ ] **Step 3: Implement**

`internal/bitbucket/text.go`:

```go
package bitbucket

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// GetText fetches a non-JSON resource (diffs, patches, logs). It sends Accept: */* because some
// of these endpoints answer 406 to Accept: application/json.
func (c *Client) GetText(ctx context.Context, path string) (string, error) {
	resp, err := c.Request(ctx, http.MethodGet, path, http.Header{"Accept": []string{"*/*"}}, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", readHTTPError(resp)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return string(b), nil
}
```

`internal/bitbucket/pullrequests.go`:

```go
package bitbucket

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// PullRequest is a Bitbucket pull request as returned by the API. List responses omit
// Reviewers and Participants unless PRListOptions.WithParticipants is set.
type PullRequest struct {
	ID                int           `json:"id"`
	Title             string        `json:"title"`
	Description       string        `json:"description"`
	State             string        `json:"state"`
	Draft             bool          `json:"draft"`
	Author            *User         `json:"author"`
	Source            PREndpoint    `json:"source"`
	Destination       PREndpoint    `json:"destination"`
	MergeCommit       *Commit       `json:"merge_commit"`
	Reviewers         []User        `json:"reviewers"`
	Participants      []Participant `json:"participants"`
	CommentCount      int           `json:"comment_count"`
	TaskCount         int           `json:"task_count"`
	CloseSourceBranch bool          `json:"close_source_branch"`
	ClosedBy          *User         `json:"closed_by"`
	CreatedOn         time.Time     `json:"created_on"`
	UpdatedOn         time.Time     `json:"updated_on"`
	Links             struct {
		HTML Link `json:"html"`
	} `json:"links"`
}

// PREndpoint is the source or destination of a pull request.
type PREndpoint struct {
	Branch struct {
		Name string `json:"name"`
	} `json:"branch"`
	Commit     *Commit `json:"commit"`
	Repository *struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// Commit identifies a commit.
type Commit struct {
	Hash string `json:"hash"`
}

// Link is a hyperlink in an API response.
type Link struct {
	Href string `json:"href"`
}

// Participant is a user's involvement in a pull request. State is "approved",
// "changes_requested" or nil.
type Participant struct {
	User     User    `json:"user"`
	Role     string  `json:"role"`
	Approved bool    `json:"approved"`
	State    *string `json:"state"`
}

// Comment is a pull request comment. Inline is set for comments on a file; Parent for replies.
type Comment struct {
	ID      int `json:"id"`
	Content struct {
		Raw string `json:"raw"`
	} `json:"content"`
	User   *User `json:"user"`
	Inline *struct {
		Path string `json:"path"`
		From *int   `json:"from"`
		To   *int   `json:"to"`
	} `json:"inline"`
	Parent *struct {
		ID int `json:"id"`
	} `json:"parent"`
	Deleted   bool      `json:"deleted"`
	CreatedOn time.Time `json:"created_on"`
	UpdatedOn time.Time `json:"updated_on"`
	Links     struct {
		HTML Link `json:"html"`
	} `json:"links"`
}

// CommitStatus is a build status reported on a commit (pipelines and external CI).
// State is SUCCESSFUL, FAILED, INPROGRESS or STOPPED.
type CommitStatus struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	State       string    `json:"state"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	UpdatedOn   time.Time `json:"updated_on"`
}

// DiffStat summarizes the change to one file. Old is nil for added files, New for removed ones.
type DiffStat struct {
	Status       string `json:"status"`
	LinesAdded   int    `json:"lines_added"`
	LinesRemoved int    `json:"lines_removed"`
	Old          *struct {
		Path string `json:"path"`
	} `json:"old"`
	New *struct {
		Path string `json:"path"`
	} `json:"new"`
}

// PRListOptions filters ListPullRequests.
type PRListOptions struct {
	States           []string // OPEN, MERGED, DECLINED, SUPERSEDED; empty means Bitbucket's default (OPEN)
	Query            string   // BBQL expression
	WithParticipants bool     // include reviewers and participants in each item
}

// ListPullRequests lists a repository's pull requests (limit <= 0 means all).
func (c *Client) ListPullRequests(ctx context.Context, workspace, slug string, opts PRListOptions, limit int) ([]PullRequest, error) {
	q := url.Values{}
	for _, s := range opts.States {
		q.Add("state", s)
	}
	if opts.Query != "" {
		q.Set("q", opts.Query)
	}
	if opts.WithParticipants {
		q.Set("fields", "+values.participants,+values.reviewers")
	}
	path := RepoPath(workspace, slug, "pullrequests")
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	return List[PullRequest](ctx, c, path, limit)
}

// GetPullRequest returns one pull request, including reviewers and participants.
func (c *Client) GetPullRequest(ctx context.Context, workspace, slug string, id int) (*PullRequest, error) {
	var pr PullRequest
	if err := c.Do(ctx, http.MethodGet, RepoPath(workspace, slug, "pullrequests", strconv.Itoa(id)), nil, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// ListPRComments returns every comment on a pull request.
func (c *Client) ListPRComments(ctx context.Context, workspace, slug string, id int) ([]Comment, error) {
	return List[Comment](ctx, c, RepoPath(workspace, slug, "pullrequests", strconv.Itoa(id), "comments"), 0)
}

// ListPRStatuses returns the commit statuses reported on a pull request.
func (c *Client) ListPRStatuses(ctx context.Context, workspace, slug string, id int) ([]CommitStatus, error) {
	return List[CommitStatus](ctx, c, RepoPath(workspace, slug, "pullrequests", strconv.Itoa(id), "statuses"), 0)
}

// PRDiffStat returns the per-file summary of a pull request's changes.
func (c *Client) PRDiffStat(ctx context.Context, workspace, slug string, id int) ([]DiffStat, error) {
	return List[DiffStat](ctx, c, RepoPath(workspace, slug, "pullrequests", strconv.Itoa(id), "diffstat"), 0)
}

// PRDiff returns a pull request's unified diff.
func (c *Client) PRDiff(ctx context.Context, workspace, slug string, id int) (string, error) {
	return c.GetText(ctx, RepoPath(workspace, slug, "pullrequests", strconv.Itoa(id), "diff"))
}

// PRPatch returns a pull request's changes as git patches.
func (c *Client) PRPatch(ctx context.Context, workspace, slug string, id int) (string, error) {
	return c.GetText(ctx, RepoPath(workspace, slug, "pullrequests", strconv.Itoa(id), "patch"))
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/bitbucket/ && go vet ./... && gofmt -l .`
Expected: `ok`, nothing else printed.

- [ ] **Step 5: Commit**

```bash
git add internal/bitbucket
git commit -m "feat(bitbucket): pull request, comment, status and diff API"
```

---

### Task 6: Pull request JSON shapes and test fixtures

**Files:**
- Create: `pkg/cmd/pr/shared/export.go`, `pkg/cmd/pr/shared/export_test.go`, `pkg/cmd/pr/shared/prtest/prtest.go`

**Interfaces:**
- Consumes: `bitbucket.PullRequest`, `bitbucket.Comment`, `bitbucket.CommitStatus`, `bitbucket.User`; `httpmock`, `iostreams.Test`, `cmdutil.Factory`, `gitctx.Resolver`.
- Produces:
  - `shared.User`, `shared.Reviewer{User, State}`, `shared.Participant{User, Role, Approved, State}`, `shared.PullRequest`, `shared.Comment`, `shared.Check` (JSON tags exactly as listed in Global Constraints).
  - `shared.PullRequestFields`, `shared.CommentFields`, `shared.CheckFields []string`.
  - `shared.NewUser(*bitbucket.User) *User`, `shared.NewPullRequest(*bitbucket.PullRequest) PullRequest`, `shared.NewComment(*bitbucket.Comment) Comment`, `shared.NewCheck(*bitbucket.CommitStatus) Check`.
  - Package `prtest`: constants `AdaUUID`, `BobUUID`, `CyUUID`, `Ada`, `Bob`, `Cy`, `PRs`, `PR42`, `PR9`, `PR7`, `CommentInline`, `CommentReply`; funcs `Status(key, state string) string`, `Page(items ...string) string`, `NewFactory(reg) (*cmdutil.Factory, *iostreams.IOStreams, *bytes.Buffer /*out*/, *bytes.Buffer /*errOut*/)`, `SetBranch(f, branch string)`, `Run(cmd *cobra.Command, args ...string) error`.

- [ ] **Step 1: Write the fixtures and the failing tests**

`pkg/cmd/pr/shared/prtest/prtest.go` (test support, no test of its own):

```go
// Package prtest holds Bitbucket fixtures and helpers for `khbb pr` tests. Every identity is fake.
package prtest

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/internal/iostreams"
)

// Fake accounts. Ada is the authenticated user wherever a test serves GET /user.
const (
	AdaUUID = "{00000000-0000-0000-0000-000000000001}"
	BobUUID = "{00000000-0000-0000-0000-000000000002}"
	CyUUID  = "{00000000-0000-0000-0000-000000000003}"

	Ada = `{"display_name":"Ada Example","nickname":"ada","uuid":"` + AdaUUID + `","account_id":"000000:aaaa"}`
	Bob = `{"display_name":"Bob Example","nickname":"bob","uuid":"` + BobUUID + `","account_id":"000000:bbbb"}`
	Cy  = `{"display_name":"Cy Example","nickname":"cy","uuid":"` + CyUUID + `","account_id":"000000:cccc"}`
)

// PRs is the request path of acme/widgets pull requests.
const PRs = "/2.0/repositories/acme/widgets/pullrequests"

// PR42 is an open pull request by ada from feature/widgets into main; bob approved, cy has not reviewed.
const PR42 = `{"id":42,"title":"Add widgets","description":"Adds the widget factory.","state":"OPEN","draft":false,"author":` + Ada +
	`,"source":{"branch":{"name":"feature/widgets"},"commit":{"hash":"abc1234"},"repository":{"full_name":"acme/widgets"}}` +
	`,"destination":{"branch":{"name":"main"},"commit":{"hash":"def5678"},"repository":{"full_name":"acme/widgets"}}` +
	`,"merge_commit":null,"reviewers":[` + Bob + `,` + Cy + `]` +
	`,"participants":[{"user":` + Bob + `,"role":"REVIEWER","approved":true,"state":"approved"},` +
	`{"user":` + Cy + `,"role":"REVIEWER","approved":false,"state":null},` +
	`{"user":` + Ada + `,"role":"PARTICIPANT","approved":false,"state":null}]` +
	`,"comment_count":2,"task_count":1,"close_source_branch":true,"closed_by":null` +
	`,"created_on":"2026-10-01T12:00:00.000000+00:00","updated_on":"2026-10-02T08:30:00.000000+00:00"` +
	`,"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/42"}}}`

// PR9 is an open pull request by bob that asks ada (pending) and cy (approved) for review.
const PR9 = `{"id":9,"title":"Update docs","description":"","state":"OPEN","draft":false,"author":` + Bob +
	`,"source":{"branch":{"name":"feature/docs"},"commit":{"hash":"999aaaa"},"repository":{"full_name":"acme/widgets"}}` +
	`,"destination":{"branch":{"name":"main"},"commit":{"hash":"def5678"},"repository":{"full_name":"acme/widgets"}}` +
	`,"merge_commit":null,"reviewers":[` + Ada + `,` + Cy + `]` +
	`,"participants":[{"user":` + Cy + `,"role":"REVIEWER","approved":true,"state":"approved"}]` +
	`,"comment_count":0,"task_count":0,"close_source_branch":false,"closed_by":null` +
	`,"created_on":"2026-09-30T09:00:00.000000+00:00","updated_on":"2026-10-01T09:00:00.000000+00:00"` +
	`,"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/9"}}}`

// PR7 is a merged pull request by bob, shaped like a list item (no reviewers or participants).
const PR7 = `{"id":7,"title":"Fix gears","description":"","state":"MERGED","draft":false,"author":` + Bob +
	`,"source":{"branch":{"name":"fix/gears"},"commit":{"hash":"aaa1111"},"repository":{"full_name":"acme/widgets"}}` +
	`,"destination":{"branch":{"name":"main"},"commit":{"hash":"bbb2222"},"repository":{"full_name":"acme/widgets"}}` +
	`,"merge_commit":{"hash":"ccc3333"},"comment_count":0,"task_count":0,"close_source_branch":false,"closed_by":` + Bob +
	`,"created_on":"2026-09-20T10:00:00.000000+00:00","updated_on":"2026-09-21T10:00:00.000000+00:00"` +
	`,"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/7"}}}`

// CommentInline is bob's comment on src/widget.go line 12 of PR 42; CommentReply is ada's reply to it.
const (
	CommentInline = `{"id":101,"content":{"raw":"Please rename this."},"user":` + Bob +
		`,"inline":{"path":"src/widget.go","from":null,"to":12},"deleted":false` +
		`,"created_on":"2026-10-01T13:00:00.000000+00:00","updated_on":"2026-10-01T13:00:00.000000+00:00"` +
		`,"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/42/_/diff#comment-101"}}}`
	CommentReply = `{"id":102,"content":{"raw":"Done."},"user":` + Ada + `,"parent":{"id":101},"deleted":false` +
		`,"created_on":"2026-10-01T14:00:00.000000+00:00","updated_on":"2026-10-01T14:00:00.000000+00:00"` +
		`,"links":{"html":{"href":"https://bitbucket.org/acme/widgets/pull-requests/42/_/diff#comment-102"}}}`
)

// Status returns a commit status whose key and name are key and whose Bitbucket state is state.
func Status(key, state string) string {
	return `{"key":"` + key + `","name":"` + key + `","state":"` + state + `","description":"` + key + ` ` + strings.ToLower(state) +
		`","url":"https://ci.example.com/` + key + `","updated_on":"2026-10-02T09:00:00.000000+00:00"}`
}

// Page wraps items in a single-page Bitbucket collection.
func Page(items ...string) string {
	return `{"values":[` + strings.Join(items, ",") + `],"pagelen":50}`
}

// NewFactory returns a non-TTY Factory bound to acme/widgets whose client talks to reg without retries.
func NewFactory(reg *httpmock.Registry) (*cmdutil.Factory, *iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	ios, _, out, errOut := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams:    ios,
		RepoOverride: "acme/widgets",
		HTTPClient: func() (*bitbucket.Client, error) {
			return bitbucket.New(bitbucket.Options{Email: "dev@example.com", Token: "t", HTTPClient: reg.Client(), MaxAttempts: 1}), nil
		},
	}
	return f, ios, out, errOut
}

// SetBranch makes f report branch as the checked-out branch; "" simulates a detached HEAD.
func SetBranch(f *cmdutil.Factory, branch string) {
	f.Git = &gitctx.Resolver{Git: func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "symbolic-ref" && branch != "" {
			return branch, nil
		}
		return "", errors.New("not on a branch")
	}}
}

// Run executes cmd with args (never falling back to os.Args) and returns its error.
func Run(cmd *cobra.Command, args ...string) error {
	cmd.SetArgs(append([]string{}, args...))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}
```

`pkg/cmd/pr/shared/export_test.go`:

```go
package shared_test

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func decode[T any](t *testing.T, raw string) *T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	return &v
}

// assertJSON compares got's JSON encoding with want by value (key order does not matter).
func assertJSON(t *testing.T, got any, want string) {
	t.Helper()
	gotBytes, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var g, w any
	if err := json.Unmarshal(gotBytes, &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("bad expectation: %v", err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("JSON mismatch\n got: %s\nwant: %s", gotBytes, want)
	}
}

const (
	adaJSON = `{"displayName":"Ada Example","nickname":"ada","uuid":"{00000000-0000-0000-0000-000000000001}","accountId":"000000:aaaa"}`
	bobJSON = `{"displayName":"Bob Example","nickname":"bob","uuid":"{00000000-0000-0000-0000-000000000002}","accountId":"000000:bbbb"}`
	cyJSON  = `{"displayName":"Cy Example","nickname":"cy","uuid":"{00000000-0000-0000-0000-000000000003}","accountId":"000000:cccc"}`
)

func TestFieldListsMatchJSONKeys(t *testing.T) {
	check := func(name string, v any, fields []string) {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		if keys := slices.Sorted(maps.Keys(m)); !slices.Equal(keys, slices.Sorted(slices.Values(fields))) {
			t.Errorf("%s: JSON keys %v != field list %v", name, keys, fields)
		}
	}
	check("PullRequest", shared.PullRequest{}, shared.PullRequestFields)
	check("Comment", shared.Comment{}, shared.CommentFields)
	check("Check", shared.Check{}, shared.CheckFields)
}

func TestNewPullRequest_Golden(t *testing.T) {
	got := shared.NewPullRequest(decode[bitbucket.PullRequest](t, prtest.PR42))
	assertJSON(t, got, `{"id":42,"title":"Add widgets","body":"Adds the widget factory.","state":"OPEN","draft":false,`+
		`"author":`+adaJSON+`,"sourceBranch":"feature/widgets","sourceCommit":"abc1234","sourceRepo":"acme/widgets",`+
		`"destinationBranch":"main","destinationCommit":"def5678","mergeCommit":"",`+
		`"reviewers":[{"user":`+bobJSON+`,"state":"approved"},{"user":`+cyJSON+`,"state":"pending"}],`+
		`"participants":[{"user":`+bobJSON+`,"role":"reviewer","approved":true,"state":"approved"},`+
		`{"user":`+cyJSON+`,"role":"reviewer","approved":false,"state":"pending"},`+
		`{"user":`+adaJSON+`,"role":"participant","approved":false,"state":"pending"}],`+
		`"commentCount":2,"taskCount":1,"closeSourceBranch":true,"url":"https://bitbucket.org/acme/widgets/pull-requests/42",`+
		`"createdOn":"2026-10-01T12:00:00Z","updatedOn":"2026-10-02T08:30:00Z","closedBy":null}`)
}

func TestNewPullRequest_ListItemHasEmptyArrays(t *testing.T) {
	b, err := json.Marshal(shared.NewPullRequest(decode[bitbucket.PullRequest](t, prtest.PR7)))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"reviewers":[]`, `"participants":[]`, `"mergeCommit":"ccc3333"`, `"closedBy":` + bobJSON} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
}

func TestNewPullRequest_WithoutAuthor(t *testing.T) {
	raw := decode[bitbucket.PullRequest](t, prtest.PR42)
	raw.Author = nil
	got := shared.NewPullRequest(raw)
	b, _ := json.Marshal(got)
	if got.Author != nil || !strings.Contains(string(b), `"author":null`) {
		t.Errorf("author = %+v, json %s", got.Author, b)
	}
}

func TestNewComment(t *testing.T) {
	inline := shared.NewComment(decode[bitbucket.Comment](t, prtest.CommentInline))
	if inline.Path != "src/widget.go" || inline.Line == nil || *inline.Line != 12 || inline.ParentID != nil || inline.Author.Nickname != "bob" {
		t.Errorf("inline = %+v", inline)
	}
	reply := shared.NewComment(decode[bitbucket.Comment](t, prtest.CommentReply))
	assertJSON(t, reply, `{"id":102,"author":`+adaJSON+`,"body":"Done.","path":"","line":null,"parentId":101,"deleted":false,`+
		`"url":"https://bitbucket.org/acme/widgets/pull-requests/42/_/diff#comment-102",`+
		`"createdOn":"2026-10-01T14:00:00Z","updatedOn":"2026-10-01T14:00:00Z"}`)
}

func TestNewCheck(t *testing.T) {
	c := shared.NewCheck(decode[bitbucket.CommitStatus](t, prtest.Status("build", "INPROGRESS")))
	assertJSON(t, c, `{"key":"build","name":"build","state":"inprogress","description":"build inprogress",`+
		`"url":"https://ci.example.com/build","updatedOn":"2026-10-02T09:00:00Z"}`)
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/...`
Expected: FAIL — `undefined: shared.PullRequest` (package `shared` has no non-test files yet).

- [ ] **Step 3: Implement**

`pkg/cmd/pr/shared/export.go`:

```go
// Package shared holds what the `khbb pr` commands have in common: JSON shapes, pull request
// lookup and display helpers.
package shared

import (
	"strings"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
)

// User is the stable JSON shape of a Bitbucket account.
type User struct {
	DisplayName string `json:"displayName"`
	Nickname    string `json:"nickname"`
	UUID        string `json:"uuid"`
	AccountID   string `json:"accountId"`
}

// Reviewer is a requested reviewer and their decision: approved, changes_requested or pending.
type Reviewer struct {
	User  User   `json:"user"`
	State string `json:"state"`
}

// Participant is anyone involved in a pull request. Role is reviewer or participant.
type Participant struct {
	User     User   `json:"user"`
	Role     string `json:"role"`
	Approved bool   `json:"approved"`
	State    string `json:"state"`
}

// PullRequest is the stable JSON shape of a pull request (spec §8.1).
type PullRequest struct {
	ID                int           `json:"id"`
	Title             string        `json:"title"`
	Body              string        `json:"body"`
	State             string        `json:"state"`
	Draft             bool          `json:"draft"`
	Author            *User         `json:"author"`
	SourceBranch      string        `json:"sourceBranch"`
	SourceCommit      string        `json:"sourceCommit"`
	SourceRepo        string        `json:"sourceRepo"`
	DestinationBranch string        `json:"destinationBranch"`
	DestinationCommit string        `json:"destinationCommit"`
	MergeCommit       string        `json:"mergeCommit"`
	Reviewers         []Reviewer    `json:"reviewers"`
	Participants      []Participant `json:"participants"`
	CommentCount      int           `json:"commentCount"`
	TaskCount         int           `json:"taskCount"`
	CloseSourceBranch bool          `json:"closeSourceBranch"`
	URL               string        `json:"url"`
	CreatedOn         time.Time     `json:"createdOn"`
	UpdatedOn         time.Time     `json:"updatedOn"`
	ClosedBy          *User         `json:"closedBy"`
}

// PullRequestFields lists PullRequest's JSON fields in spec order.
var PullRequestFields = []string{
	"id", "title", "body", "state", "draft", "author",
	"sourceBranch", "sourceCommit", "sourceRepo", "destinationBranch", "destinationCommit", "mergeCommit",
	"reviewers", "participants", "commentCount", "taskCount", "closeSourceBranch",
	"url", "createdOn", "updatedOn", "closedBy",
}

// Comment is the stable JSON shape of a pull request comment.
type Comment struct {
	ID        int       `json:"id"`
	Author    *User     `json:"author"`
	Body      string    `json:"body"`
	Path      string    `json:"path"`
	Line      *int      `json:"line"`
	ParentID  *int      `json:"parentId"`
	Deleted   bool      `json:"deleted"`
	URL       string    `json:"url"`
	CreatedOn time.Time `json:"createdOn"`
	UpdatedOn time.Time `json:"updatedOn"`
}

// CommentFields lists Comment's JSON fields.
var CommentFields = []string{"id", "author", "body", "path", "line", "parentId", "deleted", "url", "createdOn", "updatedOn"}

// Check is the stable JSON shape of a commit status. State is successful, failed, inprogress or stopped.
type Check struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	State       string    `json:"state"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	UpdatedOn   time.Time `json:"updatedOn"`
}

// CheckFields lists Check's JSON fields.
var CheckFields = []string{"key", "name", "state", "description", "url", "updatedOn"}

// NewUser maps an API account; nil stays nil.
func NewUser(u *bitbucket.User) *User {
	if u == nil {
		return nil
	}
	return &User{DisplayName: u.DisplayName, Nickname: u.Nickname, UUID: u.UUID, AccountID: u.AccountID}
}

// NewPullRequest maps an API pull request. Reviewers and Participants are never nil.
func NewPullRequest(pr *bitbucket.PullRequest) PullRequest {
	out := PullRequest{
		ID:                pr.ID,
		Title:             pr.Title,
		Body:              pr.Description,
		State:             pr.State,
		Draft:             pr.Draft,
		Author:            NewUser(pr.Author),
		SourceBranch:      pr.Source.Branch.Name,
		DestinationBranch: pr.Destination.Branch.Name,
		Reviewers:         []Reviewer{},
		Participants:      []Participant{},
		CommentCount:      pr.CommentCount,
		TaskCount:         pr.TaskCount,
		CloseSourceBranch: pr.CloseSourceBranch,
		URL:               pr.Links.HTML.Href,
		CreatedOn:         pr.CreatedOn.UTC(),
		UpdatedOn:         pr.UpdatedOn.UTC(),
		ClosedBy:          NewUser(pr.ClosedBy),
	}
	if pr.Source.Commit != nil {
		out.SourceCommit = pr.Source.Commit.Hash
	}
	if pr.Source.Repository != nil {
		out.SourceRepo = pr.Source.Repository.FullName
	}
	if pr.Destination.Commit != nil {
		out.DestinationCommit = pr.Destination.Commit.Hash
	}
	if pr.MergeCommit != nil {
		out.MergeCommit = pr.MergeCommit.Hash
	}
	states := map[string]string{}
	for _, p := range pr.Participants {
		state := participantState(p)
		states[p.User.UUID] = state
		out.Participants = append(out.Participants, Participant{
			User: *NewUser(&p.User), Role: strings.ToLower(p.Role), Approved: p.Approved, State: state,
		})
	}
	for _, r := range pr.Reviewers {
		state := states[r.UUID]
		if state == "" {
			state = "pending"
		}
		out.Reviewers = append(out.Reviewers, Reviewer{User: *NewUser(&r), State: state})
	}
	return out
}

func participantState(p bitbucket.Participant) string {
	switch {
	case p.State != nil && *p.State != "":
		return *p.State
	case p.Approved:
		return "approved"
	}
	return "pending"
}

// NewComment maps an API comment. Line is the new-side line, else the old-side line.
func NewComment(c *bitbucket.Comment) Comment {
	out := Comment{
		ID:        c.ID,
		Author:    NewUser(c.User),
		Body:      c.Content.Raw,
		Deleted:   c.Deleted,
		URL:       c.Links.HTML.Href,
		CreatedOn: c.CreatedOn.UTC(),
		UpdatedOn: c.UpdatedOn.UTC(),
	}
	if c.Inline != nil {
		out.Path = c.Inline.Path
		switch {
		case c.Inline.To != nil:
			out.Line = c.Inline.To
		case c.Inline.From != nil:
			out.Line = c.Inline.From
		}
	}
	if c.Parent != nil {
		id := c.Parent.ID
		out.ParentID = &id
	}
	return out
}

// NewCheck maps an API commit status, lower-casing its state.
func NewCheck(s *bitbucket.CommitStatus) Check {
	return Check{
		Key:         s.Key,
		Name:        s.Name,
		State:       strings.ToLower(s.State),
		Description: s.Description,
		URL:         s.URL,
		UpdatedOn:   s.UpdatedOn.UTC(),
	}
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./pkg/cmd/pr/... && go vet ./... && gofmt -l .`
Expected: `ok  github.com/khipu/khbb/pkg/cmd/pr/shared`, `prtest` has no test files, nothing else printed.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr
git commit -m "feat(pr): stable JSON shapes for pull requests, comments and checks"
```

---

### Task 7: Pull request lookup and user filters

**Files:**
- Create: `pkg/cmd/pr/shared/finder.go`, `pkg/cmd/pr/shared/finder_test.go`, `pkg/cmd/pr/shared/query.go`, `pkg/cmd/pr/shared/query_test.go`

**Interfaces:**
- Consumes: `bitbucket.Client` (`ListPullRequests`, `GetPullRequest`, `CurrentUser`), `bitbucket.QuoteBBQL`, `gitctx.ParseRepo`, `cmdutil.FlagErrorf`; `prtest` helpers.
- Produces:
  - `shared.ParseSelector(s string) (id int, repo *gitctx.Repo, err error)` — `""` → 0 (current branch); `"42"`, `"#42"`; `https://bitbucket.org/WS/REPO/pull-requests/42[/…]` → id and repo.
  - `shared.Finder{Client *bitbucket.Client; BaseRepo func() (gitctx.Repo, error); Branch func() (string, error)}` with `Find(ctx, selector string) (*bitbucket.PullRequest, gitctx.Repo, error)`.
  - `shared.UserClause(ctx, client *bitbucket.Client, field, who string) (string, error)`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pr/shared/finder_test.go`:

```go
package shared_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func newFinder(reg *httpmock.Registry, branch string) *shared.Finder {
	f, _, _, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, branch)
	client, _ := f.HTTPClient()
	return &shared.Finder{Client: client, BaseRepo: f.BaseRepo, Branch: f.Branch}
}

func TestParseSelector(t *testing.T) {
	cases := []struct {
		in   string
		id   int
		repo string
		bad  bool
	}{
		{in: "", id: 0},
		{in: "42", id: 42},
		{in: "#42", id: 42},
		{in: "https://bitbucket.org/other/tools/pull-requests/7", id: 7, repo: "other/tools"},
		{in: "https://bitbucket.org/other/tools/pull-requests/7/diff", id: 7, repo: "other/tools"},
		{in: "abc", bad: true},
		{in: "0", bad: true},
		{in: "-3", bad: true},
		{in: "https://github.com/a/b/pull/1", bad: true},
	}
	for _, tc := range cases {
		id, repo, err := shared.ParseSelector(tc.in)
		if tc.bad {
			var flagErr *cmdutil.FlagError
			if !errors.As(err, &flagErr) {
				t.Errorf("ParseSelector(%q): err = %v, want FlagError", tc.in, err)
			}
			continue
		}
		gotRepo := ""
		if repo != nil {
			gotRepo = repo.FullName()
		}
		if err != nil || id != tc.id || gotRepo != tc.repo {
			t.Errorf("ParseSelector(%q) = %d %q %v", tc.in, id, gotRepo, err)
		}
	}
}

func TestFind_ByNumber(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	pr, repo, err := newFinder(reg, "").Find(context.Background(), "#42")
	if err != nil || pr.ID != 42 || repo.FullName() != "acme/widgets" {
		t.Fatalf("pr %v repo %v err %v", pr, repo, err)
	}
}

func TestFind_ByURLUsesThatRepository(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/repositories/other/tools/pullrequests/7", httpmock.JSONResponse(200, prtest.PR7))
	pr, repo, err := newFinder(reg, "").Find(context.Background(), "https://bitbucket.org/other/tools/pull-requests/7")
	if err != nil || pr.ID != 7 || repo.FullName() != "other/tools" {
		t.Fatalf("pr %v repo %v err %v", pr, repo, err)
	}
}

func TestFind_CurrentBranchQuotesTheName(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page(prtest.PR42)))
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	pr, _, err := newFinder(reg, `feature/"quoted"`).Find(context.Background(), "")
	if err != nil || pr.ID != 42 {
		t.Fatalf("pr %v err %v", pr, err)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("q") != `source.branch.name = "feature/\"quoted\""` || q.Get("state") != "OPEN" {
		t.Errorf("query = %v", q)
	}
}

func TestFind_NoOpenPullRequest(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page()))
	_, _, err := newFinder(reg, "feature/widgets").Find(context.Background(), "")
	if err == nil || err.Error() != `no open pull request found for branch "feature/widgets" in acme/widgets` {
		t.Errorf("err = %v", err)
	}
}

func TestFind_SeveralOpenPullRequests(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page(prtest.PR42, prtest.PR9)))
	_, _, err := newFinder(reg, "feature/widgets").Find(context.Background(), "")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "#42, #9") {
		t.Errorf("err = %v", err)
	}
}

func TestFind_DetachedHEAD(t *testing.T) {
	reg := httpmock.New(t)
	_, _, err := newFinder(reg, "").Find(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "not on a branch") {
		t.Errorf("err = %v", err)
	}
}
```

`pkg/cmd/pr/shared/query_test.go`:

```go
package shared_test

import (
	"context"
	"testing"

	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func TestUserClause(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))
	f, _, _, _ := prtest.NewFactory(reg)
	client, _ := f.HTTPClient()
	cases := []struct{ who, want string }{
		{"@me", `author.uuid = "` + prtest.AdaUUID + `"`},
		{prtest.BobUUID, `author.uuid = "` + prtest.BobUUID + `"`},
		{"000000:bbbb", `author.account_id = "000000:bbbb"`},
		{"bob", `author.nickname = "bob"`},
	}
	for _, tc := range cases {
		got, err := shared.UserClause(context.Background(), client, "author", tc.who)
		if err != nil || got != tc.want {
			t.Errorf("UserClause(%q) = %q, %v; want %q", tc.who, got, err, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/shared/`
Expected: FAIL — `undefined: shared.Finder`, `undefined: shared.UserClause`.

- [ ] **Step 3: Implement**

`pkg/cmd/pr/shared/finder.go`:

```go
package shared

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
)

var prURLRE = regexp.MustCompile(`^https?://(?:www\.)?bitbucket\.org/([^/]+)/([^/]+)/pull-requests/(\d+)(?:[/?#].*)?$`)

// ParseSelector reads a pull request argument: "" (the current branch), "42", "#42" or a
// pull request URL. A URL also names the repository.
func ParseSelector(s string) (int, *gitctx.Repo, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil, nil
	}
	if m := prURLRE.FindStringSubmatch(s); m != nil {
		repo, err := gitctx.ParseRepo(m[1] + "/" + m[2])
		if err != nil {
			return 0, nil, cmdutil.FlagErrorf("invalid pull request URL %q", s)
		}
		id, _ := strconv.Atoi(m[3])
		return id, &repo, nil
	}
	id, err := strconv.Atoi(strings.TrimPrefix(s, "#"))
	if err != nil || id <= 0 {
		return 0, nil, cmdutil.FlagErrorf("invalid pull request %q: expected a number, #number or a pull request URL", s)
	}
	return id, nil, nil
}

// Finder locates the pull request a `pr` command acts on.
type Finder struct {
	Client   *bitbucket.Client
	BaseRepo func() (gitctx.Repo, error)
	Branch   func() (string, error)
}

// Find returns the pull request named by selector and its repository. An empty selector means
// the only open pull request whose source is the current branch.
func (f *Finder) Find(ctx context.Context, selector string) (*bitbucket.PullRequest, gitctx.Repo, error) {
	id, urlRepo, err := ParseSelector(selector)
	if err != nil {
		return nil, gitctx.Repo{}, err
	}
	var repo gitctx.Repo
	if urlRepo != nil {
		repo = *urlRepo
	} else if repo, err = f.BaseRepo(); err != nil {
		return nil, gitctx.Repo{}, err
	}
	if id == 0 {
		if id, err = f.idForCurrentBranch(ctx, repo); err != nil {
			return nil, repo, err
		}
	}
	pr, err := f.Client.GetPullRequest(ctx, repo.Workspace, repo.Slug, id)
	return pr, repo, err
}

func (f *Finder) idForCurrentBranch(ctx context.Context, repo gitctx.Repo) (int, error) {
	branch, err := f.Branch()
	if err != nil {
		return 0, err
	}
	prs, err := f.Client.ListPullRequests(ctx, repo.Workspace, repo.Slug, bitbucket.PRListOptions{
		States: []string{"OPEN"},
		Query:  "source.branch.name = " + bitbucket.QuoteBBQL(branch),
	}, 10)
	if err != nil {
		return 0, err
	}
	switch len(prs) {
	case 0:
		return 0, fmt.Errorf("no open pull request found for branch %q in %s", branch, repo.FullName())
	case 1:
		return prs[0].ID, nil
	}
	ids := make([]string, len(prs))
	for i, pr := range prs {
		ids[i] = "#" + strconv.Itoa(pr.ID)
	}
	return 0, cmdutil.FlagErrorf("branch %q has several open pull requests (%s); specify one", branch, strings.Join(ids, ", "))
}
```

`pkg/cmd/pr/shared/query.go`:

```go
package shared

import (
	"context"
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
)

// UserClause turns a user filter into a BBQL clause on field ("author" or "reviewers").
// who is @me, a {uuid}, an Atlassian account ID (it contains ':') or a nickname.
func UserClause(ctx context.Context, client *bitbucket.Client, field, who string) (string, error) {
	who = strings.TrimSpace(who)
	switch {
	case who == "@me":
		me, err := client.CurrentUser(ctx)
		if err != nil {
			return "", err
		}
		return field + ".uuid = " + bitbucket.QuoteBBQL(me.UUID), nil
	case strings.HasPrefix(who, "{") && strings.HasSuffix(who, "}"):
		return field + ".uuid = " + bitbucket.QuoteBBQL(who), nil
	case strings.Contains(who, ":"):
		return field + ".account_id = " + bitbucket.QuoteBBQL(who), nil
	}
	return field + ".nickname = " + bitbucket.QuoteBBQL(who), nil
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./pkg/cmd/pr/... && go vet ./... && gofmt -l .`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr/shared
git commit -m "feat(pr): pull request lookup by number, URL or current branch"
```

---

### Task 8: `khbb pr` group and `khbb pr list`

**Files:**
- Create: `internal/iostreams/color.go`, `internal/iostreams/color_test.go`
- Create: `pkg/cmd/pr/shared/display.go`, `pkg/cmd/pr/shared/display_test.go`
- Create: `pkg/cmd/pr/pr.go`, `pkg/cmd/pr/list/list.go`, `pkg/cmd/pr/list/list_test.go`
- Modify: `pkg/cmd/root/root.go`, `pkg/cmd/root/root_test.go`

**Interfaces:**
- Consumes: Tasks 5–7; `cmdutil.AddJSONFlags`, `cmdutil.EnableRepoOverride`, `cmdutil.GroupRunE`, `cmdutil.NoArgs`; go-gh `tableprinter.New(w, isTTY, width)` (headers are ignored when not a TTY; non-TTY rows are tab-separated).
- Produces:
  - `(*iostreams.IOStreams).Bold/Green/Red/Yellow/Cyan/Gray(string) string` — ANSI only when color is enabled.
  - `shared.StateColor(ios, pr PullRequest) func(string) string`, `shared.StateLabel(ios, pr) string`, `shared.AuthorName(pr) string`, `shared.ReviewSummary(pr) string`, `shared.ApprovalCount(pr) (approved, total int, changesRequested bool)`.
  - `pr.NewCmdPR(f *cmdutil.Factory) *cobra.Command` (registers `list`; Tasks 9–12 add their commands); `list.ListOptions`, `list.NewCmdList(f, runF)`.

- [ ] **Step 1: Write the failing tests**

`internal/iostreams/color_test.go`:

```go
package iostreams_test

import (
	"testing"

	"github.com/khipu/khbb/internal/iostreams"
)

func TestColors(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if got := ios.Green("ok"); got != "ok" {
		t.Errorf("disabled: %q", got)
	}
	ios.SetColorEnabled(true)
	if got := ios.Green("ok"); got != "\x1b[32mok\x1b[m" {
		t.Errorf("enabled: %q", got)
	}
	if got := ios.Bold(""); got != "" {
		t.Errorf("empty text must stay empty: %q", got)
	}
}
```

`pkg/cmd/pr/shared/display_test.go`:

```go
package shared_test

import (
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func TestReviewSummaryAndApprovals(t *testing.T) {
	pr := shared.NewPullRequest(decode[bitbucket.PullRequest](t, prtest.PR42))
	if got := shared.ReviewSummary(pr); got != "bob (approved), cy (pending)" {
		t.Errorf("ReviewSummary = %q", got)
	}
	if a, n, changes := shared.ApprovalCount(pr); a != 1 || n != 2 || changes {
		t.Errorf("ApprovalCount = %d %d %v", a, n, changes)
	}
	pr.Reviewers[1].State = "changes_requested"
	if got := shared.ReviewSummary(pr); got != "bob (approved), cy (changes requested)" {
		t.Errorf("ReviewSummary = %q", got)
	}
	if _, _, changes := shared.ApprovalCount(pr); !changes {
		t.Error("changes requested not reported")
	}
	if got := shared.ReviewSummary(shared.PullRequest{}); got != "none" {
		t.Errorf("empty ReviewSummary = %q", got)
	}
}

func TestAuthorAndStateLabel(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	pr := shared.NewPullRequest(decode[bitbucket.PullRequest](t, prtest.PR42))
	if shared.AuthorName(pr) != "ada" || shared.StateLabel(ios, pr) != "OPEN" {
		t.Errorf("author %q state %q", shared.AuthorName(pr), shared.StateLabel(ios, pr))
	}
	pr.Author = nil
	pr.Draft = true
	if shared.AuthorName(pr) != "unknown" || shared.StateLabel(ios, pr) != "DRAFT" {
		t.Errorf("author %q state %q", shared.AuthorName(pr), shared.StateLabel(ios, pr))
	}
}
```

`pkg/cmd/pr/list/list_test.go`:

```go
package list

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func TestNewCmdList_ParsesFlags(t *testing.T) {
	reg := httpmock.New(t)
	f, _, _, _ := prtest.NewFactory(reg)
	var got *ListOptions
	cmd := NewCmdList(f, func(o *ListOptions) error { got = o; return nil })
	err := prtest.Run(cmd, "-s", "MERGED", "-A", "@me", "--reviewer", "bob", "-B", "main", "-H", "feat",
		"--query", `title ~ "x"`, "-L", "5", "--json", "id,title")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "merged" || got.Author != "@me" || got.Reviewer != "bob" || got.Base != "main" || got.Head != "feat" ||
		got.Query != `title ~ "x"` || got.Limit != 5 || got.Exporter == nil {
		t.Errorf("parsed %+v", got)
	}
}

func TestNewCmdList_RejectsBadInput(t *testing.T) {
	for _, args := range [][]string{{"--state", "closed"}, {"--limit", "0"}, {"extra"}} {
		reg := httpmock.New(t)
		f, _, _, _ := prtest.NewFactory(reg)
		err := prtest.Run(NewCmdList(f, func(*ListOptions) error { return nil }), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v, want FlagError", args, err)
		}
	}
}

func TestList_FiltersAndTTYTable(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page(prtest.PR42)))
	f, ios, out, _ := prtest.NewFactory(reg)
	ios.SetStdoutTTY(true)

	err := prtest.Run(NewCmdList(f, nil), "--state", "merged", "--author", "@me", "--reviewer", "bob",
		"--base", "main", "--head", `feat/"x"`, "--query", `title ~ "x"`, "--limit", "5")
	if err != nil {
		t.Fatal(err)
	}
	q := reg.Calls[1].URL.Query()
	wantQ := `author.uuid = "` + prtest.AdaUUID + `" AND reviewers.nickname = "bob" AND destination.branch.name = "main"` +
		` AND source.branch.name = "feat/\"x\"" AND (title ~ "x")`
	if q.Get("q") != wantQ || q.Get("state") != "MERGED" || q.Get("pagelen") != "5" || q.Get("fields") != "" {
		t.Errorf("query = %v", q)
	}
	for _, want := range []string{"ID", "TITLE", "#42", "Add widgets", "feature/widgets → main", "ada", "2026-10-02"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestList_NonTTYIsTabSeparated(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page(prtest.PR42, prtest.PR7)))
	f, _, out, _ := prtest.NewFactory(reg)
	if err := prtest.Run(NewCmdList(f, nil), "--state", "all"); err != nil {
		t.Fatal(err)
	}
	q := reg.Calls[0].URL.Query()
	if !slices.Equal(q["state"], []string{"OPEN", "MERGED", "DECLINED", "SUPERSEDED"}) || q.Get("q") != "" {
		t.Errorf("query = %v", q)
	}
	want := "42\tAdd widgets\tfeature/widgets\tmain\tOPEN\tada\t2026-10-02T08:30:00Z\n" +
		"7\tFix gears\tfix/gears\tmain\tMERGED\tbob\t2026-09-21T10:00:00Z\n"
	if out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
}

func TestList_JSONAsksForParticipantsOnlyWhenNeeded(t *testing.T) {
	for _, tc := range []struct {
		fields string
		want   bool
	}{{"id,title", false}, {"id,reviewers", true}} {
		reg := httpmock.New(t)
		reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page(prtest.PR42)))
		f, _, out, _ := prtest.NewFactory(reg)
		if err := prtest.Run(NewCmdList(f, nil), "--json", tc.fields); err != nil {
			t.Fatal(err)
		}
		if got := reg.Calls[0].URL.Query().Get("fields") != ""; got != tc.want {
			t.Errorf("--json %s: participants requested = %v", tc.fields, got)
		}
		if !strings.HasPrefix(out.String(), `[{"id":42`) {
			t.Errorf("--json %s: out = %q", tc.fields, out.String())
		}
	}
}

func TestList_EmptyResults(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page()))
	reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, prtest.Page()))

	f, _, out, _ := prtest.NewFactory(reg)
	if err := prtest.Run(NewCmdList(f, nil), "--json", "id"); err != nil || out.String() != "[]\n" {
		t.Errorf("JSON: out %q err %v", out.String(), err)
	}

	f, ios, out, errOut := prtest.NewFactory(reg)
	ios.SetStdoutTTY(true)
	if err := prtest.Run(NewCmdList(f, nil)); err != nil || out.Len() != 0 ||
		errOut.String() != "No pull requests match your filters in acme/widgets\n" {
		t.Errorf("TTY: out %q stderr %q err %v", out.String(), errOut.String(), err)
	}
}
```

Append to `pkg/cmd/root/root_test.go`:

```go
func TestRootRegistersPRGroup(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{"pr", "lisst"})
	var flagErr *cmdutil.FlagError
	if err := cmd.Execute(); !errors.As(err, &flagErr) || !strings.Contains(err.Error(), `unknown command "lisst" for "khbb pr"`) {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/iostreams/ ./pkg/cmd/...`
Expected: FAIL — `ios.Green undefined`, `undefined: shared.ReviewSummary`, `undefined: NewCmdList`.

- [ ] **Step 3: Implement**

`internal/iostreams/color.go`:

```go
package iostreams

// colorize wraps text in an ANSI SGR sequence when color output is enabled.
func (s *IOStreams) colorize(code, text string) string {
	if !s.colorEnabled || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[m"
}

func (s *IOStreams) Bold(text string) string   { return s.colorize("1", text) }
func (s *IOStreams) Green(text string) string  { return s.colorize("32", text) }
func (s *IOStreams) Red(text string) string    { return s.colorize("31", text) }
func (s *IOStreams) Yellow(text string) string { return s.colorize("33", text) }
func (s *IOStreams) Cyan(text string) string   { return s.colorize("36", text) }
func (s *IOStreams) Gray(text string) string   { return s.colorize("90", text) }
```

`pkg/cmd/pr/shared/display.go`:

```go
package shared

import (
	"fmt"
	"strings"

	"github.com/khipu/khbb/internal/iostreams"
)

// StateColor returns the color for a pull request's state: green open, gray draft, cyan merged, red otherwise.
func StateColor(ios *iostreams.IOStreams, pr PullRequest) func(string) string {
	switch {
	case pr.State == "OPEN" && pr.Draft:
		return ios.Gray
	case pr.State == "OPEN":
		return ios.Green
	case pr.State == "MERGED":
		return ios.Cyan
	}
	return ios.Red
}

// StateLabel renders the state for humans; open drafts read DRAFT.
func StateLabel(ios *iostreams.IOStreams, pr PullRequest) string {
	label := pr.State
	if pr.State == "OPEN" && pr.Draft {
		label = "DRAFT"
	}
	return StateColor(ios, pr)(label)
}

// AuthorName returns the author's nickname, or "unknown" when Bitbucket sends no author.
func AuthorName(pr PullRequest) string {
	if pr.Author == nil || pr.Author.Nickname == "" {
		return "unknown"
	}
	return pr.Author.Nickname
}

// ReviewSummary lists reviewers and their decisions, e.g. "bob (approved), cy (pending)".
func ReviewSummary(pr PullRequest) string {
	if len(pr.Reviewers) == 0 {
		return "none"
	}
	parts := make([]string, len(pr.Reviewers))
	for i, r := range pr.Reviewers {
		name := r.User.Nickname
		if name == "" {
			name = r.User.DisplayName
		}
		parts[i] = fmt.Sprintf("%s (%s)", name, strings.ReplaceAll(r.State, "_", " "))
	}
	return strings.Join(parts, ", ")
}

// ApprovalCount counts approving reviewers and reports whether any requested changes.
func ApprovalCount(pr PullRequest) (approved, total int, changesRequested bool) {
	for _, r := range pr.Reviewers {
		switch r.State {
		case "approved":
			approved++
		case "changes_requested":
			changesRequested = true
		}
	}
	return approved, len(pr.Reviewers), changesRequested
}
```

`pkg/cmd/pr/pr.go`:

```go
// Package pr groups the `khbb pr` commands.
package pr

import (
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/pr/list"
)

// NewCmdPR returns `khbb pr`.
func NewCmdPR(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <command>",
		Short: "Work with Bitbucket pull requests",
		Long:  "List, inspect and check pull requests. Commands that take a pull request default to the open pull request of the current branch.",
		Args:  cobra.ArbitraryArgs,
		RunE:  cmdutil.GroupRunE,
	}
	cmdutil.EnableRepoOverride(cmd, f)
	cmd.AddCommand(
		list.NewCmdList(f, nil),
	)
	return cmd
}
```

`pkg/cmd/pr/list/list.go`:

```go
// Package list implements `khbb pr list`.
package list

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// ListOptions holds the inputs and dependencies of `khbb pr list`.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Exporter   cmdutil.Exporter

	State    string
	Author   string
	Reviewer string
	Base     string
	Head     string
	Query    string
	Limit    int
}

var stateValues = map[string][]string{
	"open":       {"OPEN"},
	"merged":     {"MERGED"},
	"declined":   {"DECLINED"},
	"superseded": {"SUPERSEDED"},
	"all":        {"OPEN", "MERGED", "DECLINED", "SUPERSEDED"},
}

// NewCmdList returns `khbb pr list`.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo}
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List pull requests in a repository",
		Example: `  $ khbb pr list
  $ khbb pr list --state merged --author @me --limit 10
  $ khbb pr list --reviewer @me --json id,title,url
  $ khbb pr list --query 'title ~ "hotfix"'`,
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.State = strings.ToLower(opts.State)
			if _, ok := stateValues[opts.State]; !ok {
				return cmdutil.FlagErrorf("invalid --state %q: use open, merged, declined, superseded or all", opts.State)
			}
			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid --limit %d: must be at least 1", opts.Limit)
			}
			if runF != nil {
				return runF(opts)
			}
			return listRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.State, "state", "s", "open", "Filter by `state`: open, merged, declined, superseded or all")
	fl.StringVarP(&opts.Author, "author", "A", "", "Filter by author: @me, a {uuid}, an account ID or a nickname")
	fl.StringVar(&opts.Reviewer, "reviewer", "", "Filter by reviewer: @me, a {uuid}, an account ID or a nickname")
	fl.StringVarP(&opts.Base, "base", "B", "", "Filter by destination `branch`")
	fl.StringVarP(&opts.Head, "head", "H", "", "Filter by source `branch`")
	fl.StringVar(&opts.Query, "query", "", "Extra BBQL `expression`, combined with the other filters using AND")
	fl.IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of pull requests to fetch")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PullRequestFields)
	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	query, err := buildQuery(ctx, client, opts)
	if err != nil {
		return err
	}
	raws, err := client.ListPullRequests(ctx, repo.Workspace, repo.Slug, bitbucket.PRListOptions{
		States:           stateValues[opts.State],
		Query:            query,
		WithParticipants: needsParticipants(opts.Exporter),
	}, opts.Limit)
	if err != nil {
		return err
	}
	prs := make([]shared.PullRequest, len(raws))
	for i := range raws {
		prs[i] = shared.NewPullRequest(&raws[i])
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, prs)
	}
	if len(prs) == 0 {
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "No pull requests match your filters in %s\n", repo.FullName())
		}
		return nil
	}
	return printTable(opts.IO, prs)
}

func needsParticipants(e cmdutil.Exporter) bool {
	if e == nil {
		return false
	}
	return slices.Contains(e.Fields(), "reviewers") || slices.Contains(e.Fields(), "participants")
}

func buildQuery(ctx context.Context, client *bitbucket.Client, opts *ListOptions) (string, error) {
	var clauses []string
	for _, filter := range []struct{ field, who string }{{"author", opts.Author}, {"reviewers", opts.Reviewer}} {
		if filter.who == "" {
			continue
		}
		clause, err := shared.UserClause(ctx, client, filter.field, filter.who)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, clause)
	}
	if opts.Base != "" {
		clauses = append(clauses, "destination.branch.name = "+bitbucket.QuoteBBQL(opts.Base))
	}
	if opts.Head != "" {
		clauses = append(clauses, "source.branch.name = "+bitbucket.QuoteBBQL(opts.Head))
	}
	if opts.Query != "" {
		clauses = append(clauses, "("+opts.Query+")")
	}
	return strings.Join(clauses, " AND "), nil
}

func printTable(ios *iostreams.IOStreams, prs []shared.PullRequest) error {
	tty := ios.IsStdoutTTY()
	tp := tableprinter.New(ios.Out, tty, ios.TerminalWidth())
	tp.AddHeader([]string{"ID", "TITLE", "BRANCH", "AUTHOR", "UPDATED"})
	for _, pr := range prs {
		if tty {
			tp.AddField("#"+strconv.Itoa(pr.ID), tableprinter.WithColor(shared.StateColor(ios, pr)))
			tp.AddField(pr.Title)
			tp.AddField(pr.SourceBranch + " → " + pr.DestinationBranch)
			tp.AddField(shared.AuthorName(pr))
			tp.AddField(pr.UpdatedOn.Format("2006-01-02"))
		} else {
			tp.AddField(strconv.Itoa(pr.ID))
			tp.AddField(pr.Title)
			tp.AddField(pr.SourceBranch)
			tp.AddField(pr.DestinationBranch)
			tp.AddField(pr.State)
			tp.AddField(shared.AuthorName(pr))
			tp.AddField(pr.UpdatedOn.Format(time.RFC3339))
		}
		tp.EndRow()
	}
	return tp.Render()
}
```

In `pkg/cmd/root/root.go`, add the import `prCmd "github.com/khipu/khbb/pkg/cmd/pr"` and `prCmd.NewCmdPR(f),` to the `AddCommand` list (after `authCmd.NewCmdAuth(f),`).

- [ ] **Step 4: Run tests and a smoke check**

Run: `go test ./... && go vet ./... && gofmt -l . && make build && ./bin/khbb pr --help | head -5 && ./bin/khbb pr lisst; echo "exit=$?"`
Expected: all `ok`; help text for `khbb pr`; then `error: unknown command "lisst" for "khbb pr"` with exit 1.

- [ ] **Step 5: Commit**

```bash
git add internal/iostreams pkg/cmd
git commit -m "feat(pr): add khbb pr list"
```

---

### Task 9: `khbb pr view` and `--web`

**Files:**
- Modify: `internal/cmdutil/factory.go` (add `Browser`), `pkg/cmd/factory/default.go`
- Create: `pkg/cmd/factory/browser_test.go`
- Create: `pkg/cmd/pr/view/view.go`, `pkg/cmd/pr/view/view_test.go`
- Modify: `pkg/cmd/pr/pr.go`

**Interfaces:**
- Consumes: Tasks 5–8; go-gh `browser.New(launcher, stdout, stderr)` (only with a non-empty launcher); `github.com/cli/browser` (`OpenURL`, package vars `Stdout`, `Stderr`).
- Produces: `cmdutil.Browser` interface `Browse(url string) error` and `Factory.Browser Browser`; `view.ViewOptions`, `view.NewCmdView(f, runF)`. `pr view --json` accepts `shared.PullRequestFields` plus `comments`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/factory/browser_test.go`:

```go
package factory

import "testing"

func TestResolveLauncher(t *testing.T) {
	t.Setenv("BROWSER", "firefox")
	if got := resolveLauncher(""); got != "firefox" {
		t.Errorf("from $BROWSER: %q", got)
	}
	if got := resolveLauncher("open -a Safari"); got != "open -a Safari" {
		t.Errorf("config wins: %q", got)
	}
	t.Setenv("BROWSER", "")
	if got := resolveLauncher(""); got != "" {
		t.Errorf("system default expected, got %q", got)
	}
}
```

`pkg/cmd/pr/view/view_test.go`:

```go
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
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/factory/ ./pkg/cmd/pr/...`
Expected: FAIL — `undefined: resolveLauncher`, `undefined: NewCmdView`.

- [ ] **Step 3: Implement**

In `internal/cmdutil/factory.go`, add after the `Prompter` interface:

```go
// Browser opens web pages for --web.
type Browser interface {
	Browse(url string) error
}
```

and add the field `Browser   Browser` to `Factory`, right after `Prompter  Prompter`.

In `pkg/cmd/factory/default.go`:

- Add the imports `cliBrowser "github.com/cli/browser"` and `"github.com/cli/go-gh/v2/pkg/browser"`.
- After `f.Config = cachedConfig()`, add `f.Browser = &browserLauncher{f: f}`.
- Append:

```go
// browserLauncher opens URLs with the config `browser`, then $BROWSER, then the system default.
// Unlike go-gh's default resolution, it never reads GitHub CLI settings.
type browserLauncher struct{ f *cmdutil.Factory }

func (b *browserLauncher) Browse(url string) error {
	var configured string
	if cfg, err := b.f.Config(); err == nil {
		configured = cfg.Browser
	}
	launcher := resolveLauncher(configured)
	if launcher == "" {
		cliBrowser.Stdout = b.f.IOStreams.ErrOut
		cliBrowser.Stderr = b.f.IOStreams.ErrOut
		return cliBrowser.OpenURL(url)
	}
	return browser.New(launcher, b.f.IOStreams.ErrOut, b.f.IOStreams.ErrOut).Browse(url)
}

func resolveLauncher(configured string) string {
	if configured != "" {
		return configured
	}
	return os.Getenv("BROWSER")
}
```

`pkg/cmd/pr/view/view.go`:

```go
// Package view implements `khbb pr view`.
package view

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// ViewOptions holds the inputs and dependencies of `khbb pr view`.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Browser    cmdutil.Browser
	Exporter   cmdutil.Exporter

	Selector string
	Comments bool
	Web      bool
}

// viewFields are the pull request fields plus the PR's comments.
var viewFields = append(slices.Clone(shared.PullRequestFields), "comments")

type viewExport struct {
	shared.PullRequest
	Comments []shared.Comment `json:"comments"`
}

// NewCmdView returns `khbb pr view`.
func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Browser: f.Browser}
	cmd := &cobra.Command{
		Use:   "view [<number> | <url>]",
		Short: "Show a pull request",
		Long:  "Show a pull request's title, state, reviewers and description. Without an argument, show the open pull request of the current branch.",
		Example: `  $ khbb pr view 42
  $ khbb pr view --comments
  $ khbb pr view 42 --json title,state,reviewers,comments`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			if opts.Web && opts.Exporter != nil {
				return cmdutil.FlagErrorf("--web cannot be combined with --json")
			}
			if runF != nil {
				return runF(opts)
			}
			return viewRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.Comments, "comments", "c", false, "Show the pull request's comments")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open the pull request in the browser")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, viewFields)
	return cmd
}

func viewRun(ctx context.Context, opts *ViewOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	raw, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	pr := shared.NewPullRequest(raw)
	if opts.Web {
		if opts.IO.IsStderrTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", pr.URL)
		}
		return opts.Browser.Browse(pr.URL)
	}

	var comments []shared.Comment
	if opts.Comments || (opts.Exporter != nil && slices.Contains(opts.Exporter.Fields(), "comments")) {
		raws, err := client.ListPRComments(ctx, repo.Workspace, repo.Slug, pr.ID)
		if err != nil {
			return err
		}
		comments = make([]shared.Comment, len(raws))
		for i := range raws {
			comments[i] = shared.NewComment(&raws[i])
		}
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, viewExport{PullRequest: pr, Comments: comments})
	}
	if opts.IO.IsStdoutTTY() {
		printHuman(opts.IO, pr, comments)
	} else {
		printRaw(opts.IO.Out, pr, comments)
	}
	return nil
}

func printHuman(ios *iostreams.IOStreams, pr shared.PullRequest, comments []shared.Comment) {
	w := ios.Out
	fmt.Fprintf(w, "%s #%d\n", ios.Bold(pr.Title), pr.ID)
	fmt.Fprintf(w, "%s • %s wants to merge %s into %s • updated %s\n",
		shared.StateLabel(ios, pr), shared.AuthorName(pr), ios.Cyan(pr.SourceBranch), ios.Cyan(pr.DestinationBranch),
		pr.UpdatedOn.Format("2006-01-02"))
	fmt.Fprintf(w, "Reviewers: %s\n", shared.ReviewSummary(pr))
	fmt.Fprintf(w, "Comments: %d • Tasks: %d\n\n", pr.CommentCount, pr.TaskCount)
	body := strings.TrimSpace(pr.Body)
	if body == "" {
		body = ios.Gray("No description provided")
	}
	fmt.Fprintf(w, "%s\n", body)
	if comments != nil {
		fmt.Fprintf(w, "\n%s\n", ios.Bold("Comments"))
		for _, c := range comments {
			if c.Deleted {
				continue
			}
			fmt.Fprintf(w, "%s • %s%s\n", commentAuthor(c), c.CreatedOn.Format("2006-01-02"), commentContext(c))
			for _, line := range strings.Split(strings.TrimRight(c.Body, "\n"), "\n") {
				fmt.Fprintf(w, "  %s\n", line)
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintf(w, "\n%s\n", ios.Gray("View this pull request on Bitbucket: "+pr.URL))
}

func printRaw(w io.Writer, pr shared.PullRequest, comments []shared.Comment) {
	fmt.Fprintf(w, "title:\t%s\nnumber:\t%d\nstate:\t%s\ndraft:\t%t\nauthor:\t%s\nsource:\t%s\ndestination:\t%s\n"+
		"reviewers:\t%s\ncomments:\t%d\ntasks:\t%d\nurl:\t%s\n--\n%s\n",
		pr.Title, pr.ID, pr.State, pr.Draft, shared.AuthorName(pr), pr.SourceBranch, pr.DestinationBranch,
		shared.ReviewSummary(pr), pr.CommentCount, pr.TaskCount, pr.URL, pr.Body)
	for _, c := range comments {
		if c.Deleted {
			continue
		}
		fmt.Fprintf(w, "--\ncomment:\t%d\nauthor:\t%s\ncreated:\t%s\n", c.ID, commentAuthor(c), c.CreatedOn.Format(time.RFC3339))
		if c.Path != "" {
			fmt.Fprintf(w, "location:\t%s\n", location(c))
		}
		if c.ParentID != nil {
			fmt.Fprintf(w, "reply to:\t%d\n", *c.ParentID)
		}
		fmt.Fprintf(w, "--\n%s\n", c.Body)
	}
}

func commentAuthor(c shared.Comment) string {
	if c.Author == nil || c.Author.Nickname == "" {
		return "unknown"
	}
	return c.Author.Nickname
}

func location(c shared.Comment) string {
	if c.Line != nil {
		return fmt.Sprintf("%s:%d", c.Path, *c.Line)
	}
	return c.Path
}

func commentContext(c shared.Comment) string {
	switch {
	case c.Path != "":
		return " • " + location(c)
	case c.ParentID != nil:
		return fmt.Sprintf(" • reply to %d", *c.ParentID)
	}
	return ""
}
```

In `pkg/cmd/pr/pr.go`, import `"github.com/khipu/khbb/pkg/cmd/pr/view"` and add `view.NewCmdView(f, nil),` to `AddCommand`.

Run `go mod tidy` (`github.com/cli/browser` becomes direct).

- [ ] **Step 4: Run the tests**

Run: `go test ./... && go vet ./... && GOOS=windows go vet ./... && gofmt -l .`
Expected: all `ok`, nothing printed.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/cmdutil pkg/cmd
git commit -m "feat(pr): add khbb pr view with --comments and --web"
```

---

### Task 10: `khbb pr status`

**Files:**
- Create: `pkg/cmd/pr/status/status.go`, `pkg/cmd/pr/status/status_test.go`
- Modify: `pkg/cmd/pr/pr.go`

**Interfaces:**
- Consumes: `Client.CurrentUser`, `Client.ListPullRequests`, `bitbucket.QuoteBBQL`, `shared.NewPullRequest`, `shared.ApprovalCount`; `prtest`.
- Produces: `status.StatusOptions`, `status.NewCmdStatus(f, runF)`; JSON fields `currentBranch`, `createdByMe`, `needsMyReview`.
- Request order (tests rely on it): `GET /user`, then the current-branch list (skipped when not on a branch), then author list, then reviewer list.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pr/status/status_test.go`:

```go
package status

import (
	"encoding/json"
	"testing"

	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func registerLists(reg *httpmock.Registry, pages ...string) {
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))
	for _, p := range pages {
		reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, p))
	}
}

func TestStatus_HumanOutput(t *testing.T) {
	reg := httpmock.New(t)
	registerLists(reg, prtest.Page(prtest.PR42), prtest.Page(prtest.PR42), prtest.Page(prtest.PR9, prtest.PR42))
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")

	if err := prtest.Run(NewCmdStatus(f, nil)); err != nil {
		t.Fatal(err)
	}
	want := "Relevant pull requests in acme/widgets\n\n" +
		"Current branch\n  #42  Add widgets [feature/widgets]  1/2 approved\n\n" +
		"Created by you\n  #42  Add widgets [feature/widgets]  1/2 approved\n\n" +
		"Requesting a code review from you\n  #9  Update docs [feature/docs]  1/2 approved\n"
	if out.String() != want {
		t.Errorf("out = %q\nwant %q", out.String(), want)
	}
	wantQueries := []string{
		`source.branch.name = "feature/widgets"`,
		`author.uuid = "` + prtest.AdaUUID + `"`,
		`reviewers.uuid = "` + prtest.AdaUUID + `"`,
	}
	for i, want := range wantQueries {
		q := reg.Calls[i+1].URL.Query()
		if q.Get("q") != want || q.Get("state") != "OPEN" || q.Get("fields") == "" {
			t.Errorf("call %d query = %v", i+1, q)
		}
	}
}

func TestStatus_NotOnBranchAndNothingToShow(t *testing.T) {
	reg := httpmock.New(t)
	registerLists(reg, prtest.Page(), prtest.Page())
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "")

	if err := prtest.Run(NewCmdStatus(f, nil)); err != nil {
		t.Fatal(err)
	}
	want := "Relevant pull requests in acme/widgets\n\n" +
		"Current branch\n  Not on a branch\n\n" +
		"Created by you\n  You have no open pull requests\n\n" +
		"Requesting a code review from you\n  You have no pull requests to review\n"
	if out.String() != want {
		t.Errorf("out = %q\nwant %q", out.String(), want)
	}
}

func TestStatus_JSON(t *testing.T) {
	reg := httpmock.New(t)
	registerLists(reg, prtest.Page(prtest.PR42), prtest.Page(prtest.PR42), prtest.Page(prtest.PR9, prtest.PR42))
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")

	if err := prtest.Run(NewCmdStatus(f, nil), "--json", "currentBranch,createdByMe,needsMyReview"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		CurrentBranch *struct{ ID int } `json:"currentBranch"`
		CreatedByMe   []struct{ ID int } `json:"createdByMe"`
		NeedsMyReview []struct{ ID int } `json:"needsMyReview"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if got.CurrentBranch == nil || got.CurrentBranch.ID != 42 || len(got.CreatedByMe) != 1 ||
		len(got.NeedsMyReview) != 1 || got.NeedsMyReview[0].ID != 9 {
		t.Errorf("got %+v", got)
	}
}

func TestStatus_JSONEmptyArrays(t *testing.T) {
	reg := httpmock.New(t)
	registerLists(reg, prtest.Page(), prtest.Page())
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "")

	if err := prtest.Run(NewCmdStatus(f, nil), "--json", "currentBranch,createdByMe,needsMyReview"); err != nil {
		t.Fatal(err)
	}
	if want := `{"createdByMe":[],"currentBranch":null,"needsMyReview":[]}` + "\n"; out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/status/`
Expected: FAIL — `undefined: NewCmdStatus`.

- [ ] **Step 3: Implement**

`pkg/cmd/pr/status/status.go`:

```go
// Package status implements `khbb pr status`.
package status

import (
	"context"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// StatusOptions holds the dependencies of `khbb pr status`.
type StatusOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Exporter   cmdutil.Exporter
}

var statusFields = []string{"currentBranch", "createdByMe", "needsMyReview"}

type statusExport struct {
	CurrentBranch *shared.PullRequest  `json:"currentBranch"`
	CreatedByMe   []shared.PullRequest `json:"createdByMe"`
	NeedsMyReview []shared.PullRequest `json:"needsMyReview"`
}

// NewCmdStatus returns `khbb pr status`.
func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the pull requests that involve you",
		Long:  "Show the open pull request of the current branch, your open pull requests, and those waiting for your review, in the current repository.",
		Args:  cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return statusRun(cmd.Context(), opts)
		},
	}
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, statusFields)
	return cmd
}

func statusRun(ctx context.Context, opts *StatusOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	me, err := client.CurrentUser(ctx)
	if err != nil {
		return err
	}
	branch, branchErr := opts.Branch()

	var current *shared.PullRequest
	if branchErr == nil {
		prs, err := openPRs(ctx, client, repo, "source.branch.name = "+bitbucket.QuoteBBQL(branch), 1)
		if err != nil {
			return err
		}
		if len(prs) > 0 {
			current = &prs[0]
		}
	}
	mine, err := openPRs(ctx, client, repo, "author.uuid = "+bitbucket.QuoteBBQL(me.UUID), 30)
	if err != nil {
		return err
	}
	toReview, err := openPRs(ctx, client, repo, "reviewers.uuid = "+bitbucket.QuoteBBQL(me.UUID), 30)
	if err != nil {
		return err
	}
	toReview = slices.DeleteFunc(toReview, func(pr shared.PullRequest) bool { return !awaitingReviewFrom(pr, me.UUID) })

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, statusExport{CurrentBranch: current, CreatedByMe: mine, NeedsMyReview: toReview})
	}
	printStatus(opts.IO, repo, branch, branchErr, current, mine, toReview)
	return nil
}

func openPRs(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, query string, limit int) ([]shared.PullRequest, error) {
	raws, err := client.ListPullRequests(ctx, repo.Workspace, repo.Slug, bitbucket.PRListOptions{
		States: []string{"OPEN"}, Query: query, WithParticipants: true,
	}, limit)
	if err != nil {
		return nil, err
	}
	prs := make([]shared.PullRequest, len(raws))
	for i := range raws {
		prs[i] = shared.NewPullRequest(&raws[i])
	}
	return prs, nil
}

func awaitingReviewFrom(pr shared.PullRequest, uuid string) bool {
	for _, r := range pr.Reviewers {
		if r.User.UUID == uuid {
			return r.State == "pending"
		}
	}
	return false
}

func printStatus(ios *iostreams.IOStreams, repo gitctx.Repo, branch string, branchErr error, current *shared.PullRequest, mine, toReview []shared.PullRequest) {
	w := ios.Out
	fmt.Fprintf(w, "Relevant pull requests in %s\n\n", repo.FullName())

	fmt.Fprintln(w, ios.Bold("Current branch"))
	switch {
	case branchErr != nil:
		fmt.Fprintln(w, ios.Gray("  Not on a branch"))
	case current == nil:
		fmt.Fprintln(w, ios.Gray("  There is no open pull request for "+branch))
	default:
		printRow(ios, *current)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, ios.Bold("Created by you"))
	if len(mine) == 0 {
		fmt.Fprintln(w, ios.Gray("  You have no open pull requests"))
	}
	for _, pr := range mine {
		printRow(ios, pr)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, ios.Bold("Requesting a code review from you"))
	if len(toReview) == 0 {
		fmt.Fprintln(w, ios.Gray("  You have no pull requests to review"))
	}
	for _, pr := range toReview {
		printRow(ios, pr)
	}
}

func printRow(ios *iostreams.IOStreams, pr shared.PullRequest) {
	fmt.Fprintf(ios.Out, "  %s  %s [%s]  %s\n", ios.Green(fmt.Sprintf("#%d", pr.ID)), pr.Title, ios.Cyan(pr.SourceBranch), reviewText(ios, pr))
}

func reviewText(ios *iostreams.IOStreams, pr shared.PullRequest) string {
	approved, total, changes := shared.ApprovalCount(pr)
	switch {
	case changes:
		return ios.Red("changes requested")
	case total == 0:
		return ios.Gray("no reviewers")
	case approved == total:
		return ios.Green(fmt.Sprintf("%d/%d approved", approved, total))
	}
	return ios.Yellow(fmt.Sprintf("%d/%d approved", approved, total))
}
```

In `pkg/cmd/pr/pr.go`, import `prStatus "github.com/khipu/khbb/pkg/cmd/pr/status"` and add `prStatus.NewCmdStatus(f, nil),` to `AddCommand`.

- [ ] **Step 4: Run the tests**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr
git commit -m "feat(pr): add khbb pr status"
```

---

### Task 11: `khbb pr diff`

**Files:**
- Create: `pkg/cmd/pr/diff/diff.go`, `pkg/cmd/pr/diff/diff_test.go`
- Modify: `pkg/cmd/pr/pr.go`

**Interfaces:**
- Consumes: `shared.Finder`, `Client.PRDiff`, `Client.PRPatch`, `Client.PRDiffStat`; `prtest`.
- Produces: `diff.DiffOptions`, `diff.NewCmdDiff(f, runF)`. `--color always` forces ANSI colors (`+` green `\x1b[32m`, `-` red `\x1b[31m`, `@@` cyan `\x1b[36m`, headers bold `\x1b[1m`, reset `\x1b[m`); `auto` follows `ios.ColorEnabled()`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pr/diff/diff_test.go`:

```go
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
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/diff/`
Expected: FAIL — `undefined: NewCmdDiff`.

- [ ] **Step 3: Implement**

`pkg/cmd/pr/diff/diff.go`:

```go
// Package diff implements `khbb pr diff`.
package diff

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// DiffOptions holds the inputs and dependencies of `khbb pr diff`.
type DiffOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)

	Selector string
	NameOnly bool
	Patch    bool
	Color    string
}

// NewCmdDiff returns `khbb pr diff`.
func NewCmdDiff(f *cmdutil.Factory, runF func(*DiffOptions) error) *cobra.Command {
	opts := &DiffOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch}
	cmd := &cobra.Command{
		Use:   "diff [<number> | <url>]",
		Short: "Show the changes in a pull request",
		Example: `  $ khbb pr diff 42
  $ khbb pr diff --name-only
  $ khbb pr diff 42 --patch > changes.patch`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			switch opts.Color {
			case "auto", "always", "never":
			default:
				return cmdutil.FlagErrorf("invalid --color %q: use auto, always or never", opts.Color)
			}
			if opts.NameOnly && opts.Patch {
				return cmdutil.FlagErrorf("--name-only cannot be combined with --patch")
			}
			if runF != nil {
				return runF(opts)
			}
			return diffRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.NameOnly, "name-only", false, "Show only the names of changed files")
	cmd.Flags().BoolVar(&opts.Patch, "patch", false, "Show the changes as git patches")
	cmd.Flags().StringVar(&opts.Color, "color", "auto", "Use color in diff output: auto, always or never")
	return cmd
}

func diffRun(ctx context.Context, opts *DiffOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if opts.NameOnly {
		stats, err := client.PRDiffStat(ctx, repo.Workspace, repo.Slug, pr.ID)
		if err != nil {
			return err
		}
		for _, s := range stats {
			switch {
			case s.New != nil:
				fmt.Fprintln(opts.IO.Out, s.New.Path)
			case s.Old != nil:
				fmt.Fprintln(opts.IO.Out, s.Old.Path)
			}
		}
		return nil
	}

	var text string
	if opts.Patch {
		text, err = client.PRPatch(ctx, repo.Workspace, repo.Slug, pr.ID)
	} else {
		text, err = client.PRDiff(ctx, repo.Workspace, repo.Slug, pr.ID)
	}
	if err != nil {
		return err
	}
	if !(opts.Color == "always" || (opts.Color == "auto" && opts.IO.ColorEnabled())) {
		_, err := io.WriteString(opts.IO.Out, text)
		return err
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		content := strings.TrimSuffix(line, "\n")
		if _, err := io.WriteString(opts.IO.Out, colorLine(content)+line[len(content):]); err != nil {
			return err
		}
	}
	return nil
}

const (
	ansiBold  = "\x1b[1m"
	ansiRed   = "\x1b[31m"
	ansiGreen = "\x1b[32m"
	ansiCyan  = "\x1b[36m"
	ansiReset = "\x1b[m"
)

func colorLine(line string) string {
	switch {
	case strings.HasPrefix(line, "diff --git"), strings.HasPrefix(line, "index "),
		strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return ansiBold + line + ansiReset
	case strings.HasPrefix(line, "@@"):
		return ansiCyan + line + ansiReset
	case strings.HasPrefix(line, "+"):
		return ansiGreen + line + ansiReset
	case strings.HasPrefix(line, "-"):
		return ansiRed + line + ansiReset
	}
	return line
}
```

In `pkg/cmd/pr/pr.go`, import `"github.com/khipu/khbb/pkg/cmd/pr/diff"` and add `diff.NewCmdDiff(f, nil),` to `AddCommand`.

- [ ] **Step 4: Run the tests**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr
git commit -m "feat(pr): add khbb pr diff"
```

---

### Task 12: `khbb pr checks`

**Files:**
- Create: `pkg/cmd/pr/checks/checks.go`, `pkg/cmd/pr/checks/checks_test.go`
- Modify: `pkg/cmd/pr/pr.go`

**Interfaces:**
- Consumes: `shared.Finder`, `Client.ListPRStatuses`, `shared.NewCheck`, `shared.CheckFields`, `cmdutil.ExitError`, `bitbucket.HTTPError`, `bitbucket.NetworkError`; `prtest`.
- Produces: `checks.ChecksOptions` (with `Sleep func(time.Duration)`, default `time.Sleep`), `checks.NewCmdChecks(f, runF)`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pr/checks/checks_test.go`:

```go
package checks

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const statusesPath = prtest.PRs + "/42/statuses"

func newReg(t *testing.T, responses ...httpmock.Responder) *httpmock.Registry {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	for _, r := range responses {
		reg.Register("GET", statusesPath, r)
	}
	return reg
}

func page(items ...string) httpmock.Responder {
	return httpmock.JSONResponse(200, prtest.Page(items...))
}

func run(t *testing.T, reg *httpmock.Registry, tty bool, args ...string) (string, string, []time.Duration, error) {
	t.Helper()
	f, ios, out, errOut := prtest.NewFactory(reg)
	ios.SetStdoutTTY(tty)
	var slept []time.Duration
	cmd := NewCmdChecks(f, func(o *ChecksOptions) error {
		o.Sleep = func(d time.Duration) { slept = append(slept, d) }
		return checksRun(context.Background(), o)
	})
	err := prtest.Run(cmd, append([]string{"42"}, args...)...)
	return out.String(), errOut.String(), slept, err
}

func exitCode(err error) int {
	var exitErr *cmdutil.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	if err != nil {
		return -1
	}
	return 0
}

func TestChecks_AllSuccessful(t *testing.T) {
	out, _, _, err := run(t, newReg(t, page(prtest.Status("lint", "SUCCESSFUL"), prtest.Status("build", "SUCCESSFUL"))), false)
	want := "build\tsuccessful\thttps://ci.example.com/build\nlint\tsuccessful\thttps://ci.example.com/lint\n"
	if err != nil || out != want {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestChecks_FailureExitsOne(t *testing.T) {
	out, _, _, err := run(t, newReg(t, page(prtest.Status("lint", "SUCCESSFUL"), prtest.Status("build", "FAILED"), prtest.Status("deploy", "STOPPED"))), false)
	want := "build\tfailed\thttps://ci.example.com/build\ndeploy\tstopped\thttps://ci.example.com/deploy\nlint\tsuccessful\thttps://ci.example.com/lint\n"
	if exitCode(err) != 1 || out != want {
		t.Errorf("exit %d out %q", exitCode(err), out)
	}
}

func TestChecks_PendingExitsEight(t *testing.T) {
	_, _, _, err := run(t, newReg(t, page(prtest.Status("build", "INPROGRESS"))), false)
	if exitCode(err) != 8 {
		t.Errorf("exit %d (%v)", exitCode(err), err)
	}
}

func TestChecks_NoChecks(t *testing.T) {
	out, errOut, _, err := run(t, newReg(t, page()), false)
	if err != nil || out != "" || errOut != "no checks reported on pull request #42\n" {
		t.Errorf("out %q stderr %q err %v", out, errOut, err)
	}
}

func TestChecks_TTYSummary(t *testing.T) {
	out, _, _, err := run(t, newReg(t, page(prtest.Status("build", "FAILED"), prtest.Status("lint", "SUCCESSFUL"))), true)
	if exitCode(err) != 1 {
		t.Errorf("exit %d", exitCode(err))
	}
	for _, want := range []string{"Some checks were not successful", "1 failed, 1 successful, 0 pending", "X", "build", "✓", "lint"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestChecks_WatchUntilFinished(t *testing.T) {
	reg := newReg(t, page(prtest.Status("build", "INPROGRESS")), page(prtest.Status("build", "SUCCESSFUL")))
	out, _, slept, err := run(t, reg, false, "--watch", "--interval", "2")
	if err != nil || out != "build\tsuccessful\thttps://ci.example.com/build\n" || !slices.Equal(slept, []time.Duration{2 * time.Second}) {
		t.Errorf("out %q slept %v err %v", out, slept, err)
	}
}

func TestChecks_FailFast(t *testing.T) {
	reg := newReg(t, page(prtest.Status("build", "FAILED"), prtest.Status("lint", "INPROGRESS")))
	_, _, slept, err := run(t, reg, false, "--watch", "--fail-fast")
	if exitCode(err) != 1 || len(slept) != 0 {
		t.Errorf("exit %d slept %v", exitCode(err), slept)
	}
}

func TestChecks_WatchRetriesTransientErrors(t *testing.T) {
	reg := newReg(t, httpmock.StringResponse(503, "busy"), page(prtest.Status("build", "SUCCESSFUL")))
	_, _, slept, err := run(t, reg, false, "--watch")
	if err != nil || !slices.Equal(slept, []time.Duration{5 * time.Second}) {
		t.Errorf("slept %v err %v", slept, err)
	}
}

func TestChecks_WatchStopsOnNotFound(t *testing.T) {
	reg := newReg(t, httpmock.JSONResponse(404, `{"type":"error","error":{"message":"Not found"}}`))
	_, _, slept, err := run(t, reg, false, "--watch")
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 || len(slept) != 0 {
		t.Errorf("err %v slept %v", err, slept)
	}
}

func TestChecks_JSON(t *testing.T) {
	out, _, _, err := run(t, newReg(t, page(prtest.Status("build", "FAILED"))), false, "--json", "name,state")
	if exitCode(err) != 1 || out != `[{"name":"build","state":"failed"}]`+"\n" {
		t.Errorf("exit %d out %q", exitCode(err), out)
	}
}

func TestNewCmdChecks_BadFlags(t *testing.T) {
	for _, args := range [][]string{{"--fail-fast"}, {"--watch", "--interval", "0"}} {
		reg := httpmock.New(t)
		f, _, _, _ := prtest.NewFactory(reg)
		err := prtest.Run(NewCmdChecks(f, nil), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/checks/`
Expected: FAIL — `undefined: NewCmdChecks`.

- [ ] **Step 3: Implement**

`pkg/cmd/pr/checks/checks.go`:

```go
// Package checks implements `khbb pr checks`.
package checks

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// ChecksOptions holds the inputs and dependencies of `khbb pr checks`.
type ChecksOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Exporter   cmdutil.Exporter
	Sleep      func(time.Duration)

	Selector string
	Watch    bool
	Interval int
	FailFast bool
}

// maxConsecutiveErrors bounds how many transient failures in a row --watch tolerates.
const maxConsecutiveErrors = 5

const clearScreen = "\x1b[H\x1b[2J"

// NewCmdChecks returns `khbb pr checks`.
func NewCmdChecks(f *cmdutil.Factory, runF func(*ChecksOptions) error) *cobra.Command {
	opts := &ChecksOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Sleep: time.Sleep}
	cmd := &cobra.Command{
		Use:   "checks [<number> | <url>]",
		Short: "Show the build statuses of a pull request",
		Long: `Show the commit statuses (pipelines and external builds) reported on a pull request.

Exit status: 0 when every check passed or none are reported, 1 when any check failed or was
stopped, 8 when any check is still in progress.`,
		Example: `  $ khbb pr checks
  $ khbb pr checks 42 --watch --fail-fast
  $ khbb pr checks 42 --json name,state,url`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			if opts.Interval < 1 {
				return cmdutil.FlagErrorf("invalid --interval %d: must be at least 1 second", opts.Interval)
			}
			if opts.FailFast && !opts.Watch {
				return cmdutil.FlagErrorf("--fail-fast requires --watch")
			}
			if runF != nil {
				return runF(opts)
			}
			return checksRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Watch, "watch", false, "Wait until no check is in progress")
	cmd.Flags().IntVarP(&opts.Interval, "interval", "i", 5, "Refresh interval in seconds while watching")
	cmd.Flags().BoolVar(&opts.FailFast, "fail-fast", false, "Stop watching as soon as a check fails")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.CheckFields)
	return cmd
}

type summary struct{ passed, failed, pending int }

func checksRun(ctx context.Context, opts *ChecksOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	interval := time.Duration(opts.Interval) * time.Second
	live := opts.Watch && opts.IO.IsStdoutTTY() && opts.Exporter == nil
	errorsInARow := 0
	for {
		statuses, err := client.ListPRStatuses(ctx, repo.Workspace, repo.Slug, pr.ID)
		if err != nil {
			errorsInARow++
			if !opts.Watch || !transient(err) || errorsInARow >= maxConsecutiveErrors {
				return err
			}
			opts.Sleep(interval)
			continue
		}
		errorsInARow = 0
		checks := make([]shared.Check, len(statuses))
		for i := range statuses {
			checks[i] = shared.NewCheck(&statuses[i])
		}
		sortChecks(checks)
		sum := summarize(checks)
		done := !opts.Watch || sum.pending == 0 || (opts.FailFast && sum.failed > 0)
		if live {
			fmt.Fprint(opts.IO.Out, clearScreen)
		}
		if done {
			if err := render(opts, pr.ID, checks, sum); err != nil {
				return err
			}
			return exitStatus(sum)
		}
		if live {
			if err := render(opts, pr.ID, checks, sum); err != nil {
				return err
			}
			fmt.Fprintf(opts.IO.Out, "\nRefreshing every %ds; press Ctrl-C to stop.\n", opts.Interval)
		}
		opts.Sleep(interval)
	}
}

func transient(err error) bool {
	var httpErr *bitbucket.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == 429 || httpErr.StatusCode >= 500
	}
	var netErr *bitbucket.NetworkError
	return errors.As(err, &netErr)
}

func summarize(checks []shared.Check) summary {
	var s summary
	for _, c := range checks {
		switch c.State {
		case "successful":
			s.passed++
		case "failed", "stopped":
			s.failed++
		default:
			s.pending++
		}
	}
	return s
}

func exitStatus(s summary) error {
	switch {
	case s.failed > 0:
		return &cmdutil.ExitError{Code: 1}
	case s.pending > 0:
		return &cmdutil.ExitError{Code: 8}
	}
	return nil
}

// sortChecks puts failures first, then checks in progress, then the rest; each group by name.
func sortChecks(checks []shared.Check) {
	rank := func(state string) int {
		switch state {
		case "failed", "stopped":
			return 0
		case "inprogress":
			return 1
		}
		return 2
	}
	slices.SortStableFunc(checks, func(a, b shared.Check) int {
		if d := rank(a.State) - rank(b.State); d != 0 {
			return d
		}
		return strings.Compare(a.Name, b.Name)
	})
}

func render(opts *ChecksOptions, id int, checks []shared.Check, s summary) error {
	if len(checks) == 0 {
		fmt.Fprintf(opts.IO.ErrOut, "no checks reported on pull request #%d\n", id)
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, checks)
	}
	if len(checks) == 0 {
		return nil
	}
	ios := opts.IO
	tty := ios.IsStdoutTTY()
	if tty {
		fmt.Fprintf(ios.Out, "%s\n%d failed, %d successful, %d pending\n\n", headline(ios, s), s.failed, s.passed, s.pending)
	}
	tp := tableprinter.New(ios.Out, tty, ios.TerminalWidth())
	for _, c := range checks {
		if tty {
			symbol, color := symbolFor(ios, c.State)
			tp.AddField(symbol, tableprinter.WithColor(color))
			tp.AddField(c.Name)
			tp.AddField(c.URL)
		} else {
			tp.AddField(c.Name)
			tp.AddField(c.State)
			tp.AddField(c.URL)
		}
		tp.EndRow()
	}
	return tp.Render()
}

func headline(ios *iostreams.IOStreams, s summary) string {
	switch {
	case s.failed > 0:
		return ios.Bold("Some checks were not successful")
	case s.pending > 0:
		return ios.Bold("Some checks are still in progress")
	}
	return ios.Bold("All checks were successful")
}

func symbolFor(ios *iostreams.IOStreams, state string) (string, func(string) string) {
	switch state {
	case "successful":
		return "✓", ios.Green
	case "failed":
		return "X", ios.Red
	case "stopped":
		return "-", ios.Gray
	}
	return "*", ios.Yellow
}
```

In `pkg/cmd/pr/pr.go`, import `"github.com/khipu/khbb/pkg/cmd/pr/checks"` and add `checks.NewCmdChecks(f, nil),` to `AddCommand`.

- [ ] **Step 4: Run tests, race detector and smoke checks**

Run: `go test -race -count=1 ./... && go vet ./... && GOOS=windows go vet ./... && gofmt -l . && make build && ./bin/khbb pr --help`
Expected: all `ok`; vet/gofmt silent; `khbb pr --help` lists `checks`, `diff`, `list`, `status`, `view`.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr
git commit -m "feat(pr): add khbb pr checks with --watch"
```

---

## Spec Coverage (Plan 2a)

| Spec item | Covered by |
|---|---|
| §7.1 `pr list` (state, author, reviewer, base, head, query, limit, pagination) | Tasks 5, 7, 8 |
| §7.1 `pr view` (`--comments`, `--web`, current-branch default) | Tasks 7, 9 |
| §7.1 `pr status` | Task 10 |
| §7.1 `pr diff` (`--name-only`, `--patch`, `--color`) | Task 11 |
| §7.1 `pr checks` (`--watch`, `-i`, `--fail-fast`, exit 0/1/8, notice when none) | Task 12 |
| §8.1 User, PullRequest, Comment, Check, pr status field sets | Tasks 6, 10 |
| §8.2 stdout/stderr, TTY vs. non-TTY | Tasks 8–12 |
| §8.4 exit 2 on cancel (incl. Ctrl-C) | Task 1 |
| §10 404 hint, validation details, watch transient errors | Tasks 1, 3, 12 |
| §5.2 `auth status` scopes | Task 4 |
| Plan 1 deferred: arity/usage errors, filter validation, error.fields, path/BBQL escaping, login prompt order | Tasks 1–4 |
| §7.1 write commands (`create`, `edit`, `comment`, `approve`, `unapprove`, `request-changes`, `merge`, `decline`, `checkout`), reviewer resolution against workspace members, findings #1 | Plan 2b |
| §5.3 `KHBB_PAGER` | Deferred (ruling above) |
