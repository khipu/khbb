# khbb Plan 2b — Pull request write commands

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the pull request write commands `khbb pr create`, `pr edit`, `pr comment`, `pr approve`, `pr unapprove`, `pr request-changes`, `pr merge`, `pr decline` and `pr checkout`, and stop `khbb api` from ever printing a non-JSON error page.

**Architecture:** Same patterns as Plans 1 and 2a: one package per command under `pkg/cmd/pr/`, `XxxOptions` + injectable `runF`, the Bitbucket client in `internal/bitbucket` sends requests and returns raw API structs, and `pkg/cmd/pr/shared` holds what several commands share (PR lookup, JSON shapes, reviewer resolution, body input, state checks). Mutating requests go through the client, so `--dry-run` is handled once there. Tests use `internal/httpmock`, the fixtures in `pkg/cmd/pr/shared/prtest`, go-gh's `prompter.NewMock` and a fake git runner.

**Tech Stack:** Go 1.26, cobra v1.10.2, go-gh v2.16.1 (`prompter`, `jq`, `template`, `jsonpretty`), go-keyring v0.2.8, yaml.v3, safeexec. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-10-05-khbb-cli-design.md` (§7.1, §8, §9, §10). API facts: `docs/superpowers/specs/2026-10-05-bitbucket-api-findings.md`, sections "Pull request API" and "Pull request write API (Source C, sandbox)" — every Bitbucket behavior this plan relies on was verified there against the sandbox repository `khipu/khipubb-sandbox`.

**Roadmap:** Plan 1 (done) → Plan 2a (done) → **Plan 2b (this)** → Plan 3: pipelines → Plan 4: release + agent skill.

## Global Constraints

Everything in Plan 1's and Plan 2a's Global Constraints still applies. The ones that matter most here:

- Module `github.com/khipu/khbb`; all code, help text, messages and docs in English.
- Must build and pass `go test -race ./...` on macOS and Linux and `go test ./...` on Windows; CI runs `golangci-lint` (`default: standard`, which includes staticcheck's ST1005: error strings start with a lowercase letter and do not end with punctuation).
- stdout carries data only; prompts, progress, notices, warnings, confirmations and errors go to stderr.
- Exit codes: `0` success (also after `--dry-run`), `1` error (including `confirmation_required`), `2` cancelled, `4` auth required, `8` checks pending.
- Error codes (spec §8.3): `auth_required`, `forbidden`, `not_found`, `conflict`, `validation`, `rate_limited`, `server_error`, `network`, `confirmation_required`, `cancelled`, `usage`.
- Every mutating command has `--dry-run`: the client prints `{"dryRun": true, "method", "url", "body"}` to stdout and sends nothing; GET requests still run.
- Destructive commands (`pr merge`, `pr decline`): on a terminal, a confirmation prompt that defaults to No; without a terminal, `--yes` is required, else exit 1 with `confirmation_required` and nothing is sent; `--yes` and `--dry-run` skip the prompt.
- No implicit side effects: no implicit `git push`, no implicit branch deletion (only with `-d`), no retries of mutating requests.
- Flag shorthands: `-R` = `--repo`, `-q` = `--jq`. `-t` = `--title` on `pr create`/`pr edit` (register `--title` before `cmdutil.AddJSONFlags`, which then leaves `--template` long-only); `-b` = `--body`, `-F` = `--body-file`, `-B` = `--base`, `-H` = `--head`, `-r` = `--reviewer`, `-d` = `--delete-branch`, `-m` = `--message`, `-w` = `--web`.
- JSON field names are camelCase and stable; slices are always `[]`, never `null`. PullRequest fields (spec §8.1): `id, title, body, state, draft, author, sourceBranch, sourceCommit, sourceRepo, destinationBranch, destinationCommit, mergeCommit, reviewers, participants, commentCount, taskCount, closeSourceBranch, url, createdOn, updatedOn, closedBy`. Comment fields: `id, author, body, path, line, parentId, deleted, url, createdOn, updatedOn`.
- Never print a non-JSON HTTP error body: Bitbucket serves some 404s as web pages that embed the caller's profile and a short-lived web token.
- Test fixtures use fake identities only (`@example.com`, "Ada Example", `{00000000-…}` UUIDs, workspace `acme`, repository `acme/widgets`).
- Tests that build commands through `pr.NewCmdPR(f)` must pass `-R`: registering the group's `--repo` flag resets `f.RepoOverride` to `""`. Tests in this plan build commands directly and keep `prtest.NewFactory`'s `acme/widgets`.

## Decisions taken for this plan (rulings)

- **`pr checks --watch` with no statuses yet** stays as spec §7.1 says (exit 0 with a notice). Agents are told in the skill (Plan 4) to gate fresh pushes on `pipeline watch`; repositories that require passing builds are still protected by Bitbucket's own merge checks, which `pr merge` surfaces. — Cost if wrong: on a repository without required builds, an agent could merge before CI registers.
- **`pr merge` always calls `POST …/merge?async=true`** and polls the task every 2 s for at most 2 minutes. — One code path for fast and slow merges; a synchronous 555 timeout would leave the merge state unknown, and an unknown task id answers PENDING forever (verified).
- **`pr merge` always sends `close_source_branch`** (`true` only with `-d`). When the pull request itself asks to close its branch and `-d` is absent, a note on stderr says the branch is kept. — Spec §9 forbids implicit deletion; Bitbucket would otherwise fall back to the pull request's setting.
- **Default merge strategy** = the destination branch's `default_merge_strategy` (read with `fields=destination.branch.*`); a strategy flag outside `merge_strategies` fails before anything is sent.
- **`pr merge` pre-checks `mergeability/checks`** and fails on any FAILED blocking check with Bitbucket's reason (`conflict` code).
- **New `cmdutil.ConflictError`** (code `conflict`, exit 1) for state conflicts: a pull request that is not open, an existing open pull request for the same head and base, blocking merge checks.
- **Every state-changing command except `pr comment` and `pr checkout` refuses a pull request that is not OPEN**, before sending anything. Bitbucket itself accepts approving a merged pull request (verified: 200). Comments on closed pull requests stay allowed (Bitbucket and `gh` both allow them).
- **Output of write commands** (gh parity): `pr create`, `pr edit` and `pr comment` print the URL of what they created or changed on stdout; `pr merge`, `pr decline`, `pr approve`, `pr unapprove`, `pr request-changes` and `pr checkout` print nothing on stdout. Each prints one confirmation line on stderr (with a ✓ on a terminal). `--json` is offered by `pr create`, `pr edit`, `pr merge`, `pr decline` (PullRequest) and `pr comment` (Comment).
- **Body prompts are one-line prompts.** No editor in v1; long bodies use `--body-file` (or `-F -` for standard input). — Cost if wrong: humans write multi-line bodies in a file.
- **`pr create` checks for an open pull request with the same head and base** and refuses with its URL: Bitbucket silently retitles that pull request instead of creating a new one (verified). Different bases may coexist (verified).
- **`pr create` always sends `destination`**: `-B`, else the repository's `mainbranch.name`.
- **`pr create -d`** sets `close_source_branch` on the pull request (honored by merges in the web UI); merges through `khbb` only delete the branch with `pr merge -d`.
- **`pr create --web`** opens `https://bitbucket.org/<ws>/<repo>/pull-requests/new?source=<head>&t=1` — the URL Bitbucket prints after a push — plus `dest=<base>` when `-B` is given. `dest` is unverified; Bitbucket ignores parameters it does not know, so the page opens either way.
- **Reviewer identifiers** (spec §7.1): `@me` → the current user; `{uuid}` → workspace members filtered by `user.uuid`; anything else → filtered by `(user.account_id = X OR user.nickname = X)` (nickname matching is case-insensitive, verified), then — only if that finds nobody — a case-insensitive match on the display names of all members, because Bitbucket cannot filter on `display_name` (verified: 400). Exactly one match is required; zero → `not_found`, several → `usage` with the candidates.
- **`pr unapprove` only withdraws an approval.** There is no command to withdraw a change request in v1 (`khbb api -X DELETE …/request-changes` covers it).
- **`pr checkout` requires a pull request argument** (gh parity; spec writes `[n]`, but checking out the pull request of the branch you are on is a no-op).
- **`git_protocol` unset means https** (gh default); `ssh` makes fork URLs `git@bitbucket.org:<ws>/<repo>.git`.
- **`pr checks` gets no code change** beyond reusing the client's new transient-error helper.

## Review Focus

1. **A pull request that is no longer open** (merged, declined, superseded) for `approve`, `unapprove`, `request-changes`, `edit`, `merge`, `decline` → refuse with a `conflict` error before any write; Bitbucket would accept an approval — tests `TestReview_RefusesClosedPullRequest` (Task 6), `TestEdit_RefusesClosedPullRequest` (Task 10), `TestMerge_RefusesClosedPullRequest` (Task 11), `TestDecline_RefusesClosedPullRequest` (Task 12).
2. **`--dry-run` on a destructive command without `--yes` and without a terminal** → print the request, exit 0, send no write, never fail with `confirmation_required` — tests `TestMerge_DryRun` (Task 11), `TestDecline_DryRun` (Task 12).
3. **Reviewer values that match nobody, several members, the author, or a default reviewer again** → clear errors for the first two, silent de-duplication for the others — tests `TestResolve_NoMatch`, `TestResolve_Ambiguous`, `TestUniqueUUIDs` (Task 5), `TestCreate_NonInteractive` (Task 8), `TestEdit_Reviewers` (Task 10).
4. **A merge task that never finishes or hits transient errors** → keep polling through 429/5xx/network errors, give up after 2 minutes with the task URL — tests `TestMerge_Timeout`, `TestMerge_TransientErrorWhilePolling` (Task 11).
5. **Branch names with `/` or `"`** in the duplicate-PR query, in `--fill`'s commit range and in checkout refs → quoted BBQL and literal refs — tests `TestCreate_NonInteractive` (Task 8), `TestCreate_FillSeveralCommits` (Task 9), `TestCheckout_NewBranchFromOrigin` (Task 13).

---

## File Map

| File | Responsibility | Task |
|---|---|---|
| `pkg/cmd/api/api.go` | Drop non-JSON error bodies | 1 |
| `internal/cmdutil/errors.go` | No hint that repeats the message; `ConflictError` | 1, 4 |
| `internal/bitbucket/prwrite.go` | Create, update, comment, review, decline | 2 |
| `internal/bitbucket/repos.go` | Main branch, default reviewers, workspace members | 2 |
| `internal/bitbucket/merge.go` | Merge, merge task, strategies, merge checks | 3 |
| `internal/bitbucket/errors.go` | `IsTransient` | 3 |
| `pkg/cmd/pr/checks/checks.go` | Use `bitbucket.IsTransient` | 3 |
| `pkg/cmd/pr/shared/write.go` | `RequireOpen`, `Describe`, `ReadBody`, `PrintSuccess` | 4 |
| `pkg/cmd/pr/shared/prtest/prtest.go` | Dry run in the test factory; write-test helpers; fake git | 4, 8, 9 |
| `pkg/cmd/pr/shared/reviewers.go` | Reviewer resolution against workspace members | 5 |
| `pkg/cmd/pr/review/review.go` | `pr approve`, `pr unapprove`, `pr request-changes` | 6 |
| `pkg/cmd/pr/comment/comment.go` | `pr comment` | 7 |
| `pkg/cmd/pr/create/create.go` | `pr create` | 8, 9 |
| `internal/gitctx/resolver.go` | `RemoteFor` | 9 |
| `pkg/cmd/pr/edit/edit.go` | `pr edit` | 10 |
| `pkg/cmd/pr/merge/merge.go` | `pr merge` | 11 |
| `pkg/cmd/pr/decline/decline.go` | `pr decline` | 12 |
| `pkg/cmd/pr/checkout/checkout.go` | `pr checkout` | 13 |
| `pkg/cmd/pr/pr.go` | Register the new commands | 6–8, 10–13 |
| — | Live smoke test in the sandbox (controller only) | 14 |

---

### Task 1: Never print a non-JSON error body; no hint that repeats the message

**Files:**
- Modify: `pkg/cmd/api/api.go` (error branches of `apiRun` and `paginate`; new `writeErrorBody`)
- Modify: `internal/cmdutil/errors.go` (`classifyHTTP`, field-error hint)
- Test: `pkg/cmd/api/api_test.go`, `internal/cmdutil/errors_test.go`

**Interfaces:**
- Consumes: `writeBody(opts *APIOptions, contentType string, data []byte, filter bool) error` (existing), `formatFields(map[string][]string) string` (existing).
- Produces: `writeErrorBody(opts *APIOptions, contentType string, data []byte)`. JSON error bodies are still printed unfiltered on stdout (gh parity); any other error body is replaced by one stderr line: `note: the <media type> error body (<n> bytes) was not printed`.

Why: `GET repositories/{ws}/{missing-repo}/commits/<rev>` answers `404 text/html` with a ~25 KB web page that embeds the caller's name, email, account ID and a short-lived web token, and `khbb api` printed it to stdout (findings, "HTML error pages").

- [ ] **Step 1: Write the failing tests**

Append to `pkg/cmd/api/api_test.go`:

```go
func TestAPI_NonJSONErrorBodyIsNotPrinted(t *testing.T) {
	opts, reg, _, out := newOpts(t)
	errOut := opts.IO.ErrOut.(*bytes.Buffer)
	opts.Path = "repositories/acme/nope/commits/main"
	page := "<!DOCTYPE html><html><body>ada@example.com apitoken=s3cret</body></html>"
	reg.Register("GET", "/2.0/repositories/acme/nope/commits/main",
		httpmock.WithHeader(httpmock.StringResponse(404, page), "Content-Type", "text/html; charset=utf-8"))

	err := apiRun(context.Background(), opts)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 {
		t.Fatalf("err = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("an HTML error page must never be printed, got %q", out.String())
	}
	if !strings.HasPrefix(errOut.String(), "note: the text/html error body (") || strings.Contains(errOut.String(), "s3cret") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestAPI_PaginateNonJSONErrorBodyIsNotPrinted(t *testing.T) {
	opts, reg, _, out := newOpts(t)
	opts.Path = "repositories/acme/nope/commits/main"
	opts.Paginate = true
	reg.Register("GET", "/2.0/repositories/acme/nope/commits/main",
		httpmock.WithHeader(httpmock.StringResponse(404, "<html>s3cret</html>"), "Content-Type", "text/html"))

	if err := apiRun(context.Background(), opts); err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(out.String(), "s3cret") {
		t.Errorf("out = %q", out.String())
	}
}

func TestAPI_SilentDropsTheNote(t *testing.T) {
	opts, reg, _, out := newOpts(t)
	errOut := opts.IO.ErrOut.(*bytes.Buffer)
	opts.Path = "repositories/acme/nope/commits/main"
	opts.Silent = true
	reg.Register("GET", "/2.0/repositories/acme/nope/commits/main",
		httpmock.WithHeader(httpmock.StringResponse(404, "<html></html>"), "Content-Type", "text/html"))

	if err := apiRun(context.Background(), opts); err == nil {
		t.Fatal("expected an error")
	}
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Errorf("out %q stderr %q", out.String(), errOut.String())
	}
}
```

Append to `internal/cmdutil/errors_test.go`:

```go
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
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/api/ ./internal/cmdutil/`
Expected: FAIL — `TestAPI_NonJSONErrorBodyIsNotPrinted` (out is the page), `TestAPI_PaginateNonJSONErrorBodyIsNotPrinted`, `TestClassify_FieldHintThatRepeatsTheMessage` (hint repeats the message). `TestAPI_SilentDropsTheNote` may already pass.

- [ ] **Step 3: Implement**

In `pkg/cmd/api/api.go`, in `apiRun`, replace

```go
	if !ok {
		_ = writeBody(opts, resp.Header.Get("Content-Type"), data, false)
		return bitbucket.ParseHTTPError(resp, data)
	}
```

with

```go
	if !ok {
		writeErrorBody(opts, resp.Header.Get("Content-Type"), data)
		return bitbucket.ParseHTTPError(resp, data)
	}
```

In `paginate`, replace

```go
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			_ = writeBody(opts, resp.Header.Get("Content-Type"), data, false)
			return bitbucket.ParseHTTPError(resp, data)
		}
```

with

```go
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			writeErrorBody(opts, resp.Header.Get("Content-Type"), data)
			return bitbucket.ParseHTTPError(resp, data)
		}
```

Add after `writeBody`:

```go
// writeErrorBody prints a JSON error body unfiltered, as gh does. Any other body is dropped: some
// Bitbucket errors are web pages that embed the caller's profile and a short-lived web token.
func writeErrorBody(opts *APIOptions, contentType string, data []byte) {
	if opts.Silent || len(data) == 0 {
		return
	}
	if strings.Contains(contentType, "json") || (contentType == "" && json.Valid(data)) {
		_ = writeBody(opts, contentType, data, false)
		return
	}
	kind, _, _ := strings.Cut(contentType, ";")
	if kind = strings.TrimSpace(kind); kind == "" {
		kind = "non-JSON"
	}
	fmt.Fprintf(opts.IO.ErrOut, "note: the %s error body (%d bytes) was not printed\n", kind, len(data))
}
```

In `internal/cmdutil/errors.go`, in `classifyHTTP`, replace

```go
	if info.Hint == "" && len(e.Fields) > 0 {
		info.Hint = formatFields(e.Fields)
	}
```

with

```go
	if info.Hint == "" && len(e.Fields) > 0 {
		// Bitbucket often repeats its only field error as the message; such a hint adds nothing.
		if hint := formatFields(e.Fields); hint != e.Message {
			info.Hint = hint
		}
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/cmd/api/ ./internal/cmdutil/`
Expected: PASS, including the existing `TestAPI_ErrorStatusPrintsBodyAndFails` (JSON error bodies are still printed).

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/api/api.go pkg/cmd/api/api_test.go internal/cmdutil/errors.go internal/cmdutil/errors_test.go
git commit -m "fix(api): never print non-JSON error bodies

Bitbucket serves some 404s as web pages that embed the caller's profile
and a short-lived web token. Also drop field hints that repeat the message."
```

---

### Task 2: Pull request write API in the Bitbucket client

**Files:**
- Create: `internal/bitbucket/prwrite.go`, `internal/bitbucket/repos.go`
- Test: `internal/bitbucket/prwrite_test.go`, `internal/bitbucket/repos_test.go`

**Interfaces:**
- Consumes: `Client.Do`, `RepoPath`, `List[T]`, `PullRequest`, `Comment`, `User` (existing). Test helpers in package `bitbucket_test`: `newTestClient(t, bitbucket.Options)`, `prPath` (`"/2.0/repositories/acme/widgets/pullrequests"`), `prJSON` (PR 42), `userJSON` (Ada).
- Produces (all on `*bitbucket.Client`, `ctx context.Context` first):
  - `type PRCreate struct { Title, Description, Source, Destination string; Draft, CloseSourceBranch bool; Reviewers []string }` — `Source`/`Destination` are branch names, `Reviewers` account UUIDs.
  - `CreatePullRequest(ctx, workspace, slug string, in PRCreate) (*PullRequest, error)` — `POST …/pullrequests`; always sends `title, description, source, destination, draft, close_source_branch, reviewers` (`[]` when none).
  - `type PRUpdate struct { Title, Description, Destination *string; Draft *bool; Reviewers []string }` — nil fields are not sent; `Reviewers` replaces the list when non-nil (an empty non-nil slice removes everyone).
  - `UpdatePullRequest(ctx, workspace, slug string, id int, in PRUpdate) (*PullRequest, error)` — `PUT …/pullrequests/{id}`.
  - `type PRCommentInput struct { Body, Path string; Line, ParentID int }` — `Path` alone: file comment; `Path`+`Line`: `inline.to`; `ParentID`: reply.
  - `CreatePRComment(ctx, workspace, slug string, id int, in PRCommentInput) (*Comment, error)` — `POST …/comments`.
  - `ApprovePR`, `UnapprovePR`, `RequestChangesPR(ctx, workspace, slug string, id int) error` — `POST …/approve`, `DELETE …/approve`, `POST …/request-changes`, no body.
  - `DeclinePR(ctx, workspace, slug string, id int, message string) (*PullRequest, error)` — `POST …/decline`, body `{"message": …}` only when `message != ""`.
  - `RepositoryMainBranch(ctx, workspace, slug string) (string, error)` — `GET repositories/{ws}/{slug}` → `mainbranch.name`.
  - `EffectiveDefaultReviewers(ctx, workspace, slug string) ([]User, error)` — `GET …/effective-default-reviewers`, items `{user}`.
  - `FindWorkspaceMembers(ctx, workspace, query string) ([]User, error)` — `GET workspaces/{ws}/members`, with `q=<query>` when non-empty; all pages.
  - Unexported `prPath(workspace, slug string, id int, segments ...string) string` for Task 3.

- [ ] **Step 1: Write the failing tests**

`internal/bitbucket/prwrite_test.go`:

```go
package bitbucket_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

// assertBody fails t unless the request body is the JSON value want (key order and spacing ignored).
func assertBody(t *testing.T, c httpmock.Call, want string) {
	t.Helper()
	var got, exp any
	if err := json.Unmarshal(c.Body, &got); err != nil {
		t.Fatalf("request body %q: %v", c.Body, err)
	}
	if err := json.Unmarshal([]byte(want), &exp); err != nil {
		t.Fatalf("bad expectation %q: %v", want, err)
	}
	if !reflect.DeepEqual(got, exp) {
		t.Errorf("request body = %s, want %s", c.Body, want)
	}
}

func TestCreatePullRequest(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", prPath, httpmock.JSONResponse(201, prJSON))

	pr, err := c.CreatePullRequest(context.Background(), "acme", "widgets", bitbucket.PRCreate{
		Title: "Add widgets", Description: "Adds the widget factory.", Source: "feature/widgets", Destination: "main",
		Draft: true, CloseSourceBranch: true, Reviewers: []string{"{b}", "{c}"},
	})
	if err != nil || pr.ID != 42 {
		t.Fatalf("pr %+v err %v", pr, err)
	}
	assertBody(t, reg.Calls[0], `{"title":"Add widgets","description":"Adds the widget factory.",
		"source":{"branch":{"name":"feature/widgets"}},"destination":{"branch":{"name":"main"}},
		"draft":true,"close_source_branch":true,"reviewers":[{"uuid":"{b}"},{"uuid":"{c}"}]}`)
}

func TestCreatePullRequest_NoReviewersIsAnEmptyList(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", prPath, httpmock.JSONResponse(201, prJSON))

	if _, err := c.CreatePullRequest(context.Background(), "acme", "widgets", bitbucket.PRCreate{
		Title: "Fix", Source: "fix/gears", Destination: "main",
	}); err != nil {
		t.Fatal(err)
	}
	assertBody(t, reg.Calls[0], `{"title":"Fix","description":"","source":{"branch":{"name":"fix/gears"}},
		"destination":{"branch":{"name":"main"}},"draft":false,"close_source_branch":false,"reviewers":[]}`)
}

func TestUpdatePullRequest_SendsOnlyChangedFields(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("PUT", prPath+"/42", httpmock.JSONResponse(200, prJSON))

	title, draft := "New title", false
	if _, err := c.UpdatePullRequest(context.Background(), "acme", "widgets", 42, bitbucket.PRUpdate{Title: &title, Draft: &draft}); err != nil {
		t.Fatal(err)
	}
	assertBody(t, reg.Calls[0], `{"title":"New title","draft":false}`)
}

func TestUpdatePullRequest_DescriptionDestinationAndNoReviewers(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("PUT", prPath+"/42", httpmock.JSONResponse(200, prJSON))

	body, base := "", "develop"
	if _, err := c.UpdatePullRequest(context.Background(), "acme", "widgets", 42, bitbucket.PRUpdate{
		Description: &body, Destination: &base, Reviewers: []string{},
	}); err != nil {
		t.Fatal(err)
	}
	assertBody(t, reg.Calls[0], `{"description":"","destination":{"branch":{"name":"develop"}},"reviewers":[]}`)
}

func TestCreatePRComment(t *testing.T) {
	cases := []struct {
		name string
		in   bitbucket.PRCommentInput
		want string
	}{
		{"general", bitbucket.PRCommentInput{Body: "LGTM"}, `{"content":{"raw":"LGTM"}}`},
		{"file", bitbucket.PRCommentInput{Body: "x", Path: "src/widget.go"}, `{"content":{"raw":"x"},"inline":{"path":"src/widget.go"}}`},
		{"line", bitbucket.PRCommentInput{Body: "x", Path: "src/widget.go", Line: 12}, `{"content":{"raw":"x"},"inline":{"path":"src/widget.go","to":12}}`},
		{"reply", bitbucket.PRCommentInput{Body: "Done.", ParentID: 101}, `{"content":{"raw":"Done."},"parent":{"id":101}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, reg := newTestClient(t, bitbucket.Options{})
			reg.Register("POST", prPath+"/42/comments", httpmock.JSONResponse(201, `{"id":7,"content":{"raw":"x"}}`))
			cm, err := c.CreatePRComment(context.Background(), "acme", "widgets", 42, tc.in)
			if err != nil || cm.ID != 7 {
				t.Fatalf("comment %+v err %v", cm, err)
			}
			assertBody(t, reg.Calls[0], tc.want)
		})
	}
}

func TestReviewActions(t *testing.T) {
	cases := []struct {
		name   string
		call   func(*bitbucket.Client) error
		method string
		path   string
		status int
	}{
		{"approve", func(c *bitbucket.Client) error { return c.ApprovePR(context.Background(), "acme", "widgets", 42) }, "POST", prPath + "/42/approve", 200},
		{"unapprove", func(c *bitbucket.Client) error { return c.UnapprovePR(context.Background(), "acme", "widgets", 42) }, "DELETE", prPath + "/42/approve", 204},
		{"request changes", func(c *bitbucket.Client) error { return c.RequestChangesPR(context.Background(), "acme", "widgets", 42) }, "POST", prPath + "/42/request-changes", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, reg := newTestClient(t, bitbucket.Options{})
			reg.Register(tc.method, tc.path, httpmock.JSONResponse(tc.status, `{"approved":true}`))
			if err := tc.call(c); err != nil {
				t.Fatal(err)
			}
			if len(reg.Calls[0].Body) != 0 {
				t.Errorf("body = %q, want none", reg.Calls[0].Body)
			}
		})
	}
}

func TestDeclinePR(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", prPath+"/42/decline", httpmock.JSONResponse(200, prJSON))
	reg.Register("POST", prPath+"/42/decline", httpmock.JSONResponse(200, prJSON))

	if pr, err := c.DeclinePR(context.Background(), "acme", "widgets", 42, "Superseded by #43"); err != nil || pr.ID != 42 {
		t.Fatalf("pr %+v err %v", pr, err)
	}
	assertBody(t, reg.Calls[0], `{"message":"Superseded by #43"}`)
	if _, err := c.DeclinePR(context.Background(), "acme", "widgets", 42, ""); err != nil {
		t.Fatal(err)
	}
	if len(reg.Calls[1].Body) != 0 {
		t.Errorf("body without a message = %q, want none", reg.Calls[1].Body)
	}
}
```

`internal/bitbucket/repos_test.go`:

```go
package bitbucket_test

import (
	"context"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

func TestRepositoryMainBranch(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/repositories/acme/widgets", httpmock.JSONResponse(200, `{"mainbranch":{"name":"main","type":"branch"}}`))
	reg.Register("GET", "/2.0/repositories/acme/widgets", httpmock.JSONResponse(200, `{"mainbranch":null}`))

	if name, err := c.RepositoryMainBranch(context.Background(), "acme", "widgets"); err != nil || name != "main" {
		t.Errorf("name %q err %v", name, err)
	}
	if _, err := c.RepositoryMainBranch(context.Background(), "acme", "widgets"); err == nil || !strings.Contains(err.Error(), "no main branch") {
		t.Errorf("err = %v", err)
	}
}

func TestEffectiveDefaultReviewers(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/repositories/acme/widgets/effective-default-reviewers", httpmock.JSONResponse(200,
		`{"values":[{"type":"default_reviewer","reviewer_type":"repository","user":`+userJSON+`}]}`))

	users, err := c.EffectiveDefaultReviewers(context.Background(), "acme", "widgets")
	if err != nil || len(users) != 1 || users[0].Nickname != "ada" {
		t.Errorf("users %+v err %v", users, err)
	}
}

func TestFindWorkspaceMembers(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/workspaces/acme/members", httpmock.JSONResponse(200, `{"values":[{"type":"workspace_membership","user":`+userJSON+`}]}`))
	reg.Register("GET", "/2.0/workspaces/acme/members", httpmock.JSONResponse(200, `{"values":[]}`))

	users, err := c.FindWorkspaceMembers(context.Background(), "acme", `user.nickname = "ada"`)
	if err != nil || len(users) != 1 || users[0].UUID != "{00000000-0000-0000-0000-000000000001}" {
		t.Fatalf("users %+v err %v", users, err)
	}
	if q := reg.Calls[0].URL.Query().Get("q"); q != `user.nickname = "ada"` {
		t.Errorf("q = %q", q)
	}
	if _, err := c.FindWorkspaceMembers(context.Background(), "acme", ""); err != nil {
		t.Fatal(err)
	}
	if reg.Calls[1].URL.Query().Has("q") {
		t.Errorf("an empty query must not send q: %s", reg.Calls[1].URL.RawQuery)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/bitbucket/`
Expected: FAIL — `undefined: bitbucket.PRCreate` (and the other new names).

- [ ] **Step 3: Implement**

`internal/bitbucket/prwrite.go`:

```go
package bitbucket

import (
	"context"
	"net/http"
	"strconv"
)

// PRCreate describes a new pull request. Source and Destination are branch names; Reviewers are
// account UUIDs.
type PRCreate struct {
	Title             string
	Description       string
	Source            string
	Destination       string
	Draft             bool
	CloseSourceBranch bool
	Reviewers         []string
}

// PRUpdate lists the fields of a pull request to change; nil fields keep their value.
type PRUpdate struct {
	Title       *string
	Description *string
	Destination *string
	Draft       *bool
	// Reviewers replaces the reviewer list when non-nil; an empty, non-nil slice removes everyone.
	Reviewers []string
}

// PRCommentInput describes a new comment. Path alone makes a file comment, Path with Line a comment
// on that line of the new version, and ParentID a reply.
type PRCommentInput struct {
	Body     string
	Path     string
	Line     int
	ParentID int
}

func prPath(workspace, slug string, id int, segments ...string) string {
	return RepoPath(workspace, slug, append([]string{"pullrequests", strconv.Itoa(id)}, segments...)...)
}

func branchRef(name string) map[string]any {
	return map[string]any{"branch": map[string]any{"name": name}}
}

func userRefs(uuids []string) []map[string]string {
	refs := make([]map[string]string, len(uuids))
	for i, u := range uuids {
		refs[i] = map[string]string{"uuid": u}
	}
	return refs
}

// CreatePullRequest opens a pull request. Bitbucket answers a second request for the same source
// and destination by retitling the open pull request, so callers check for one first.
func (c *Client) CreatePullRequest(ctx context.Context, workspace, slug string, in PRCreate) (*PullRequest, error) {
	body := map[string]any{
		"title":               in.Title,
		"description":         in.Description,
		"source":              branchRef(in.Source),
		"destination":         branchRef(in.Destination),
		"draft":               in.Draft,
		"close_source_branch": in.CloseSourceBranch,
		"reviewers":           userRefs(in.Reviewers),
	}
	var pr PullRequest
	if err := c.Do(ctx, http.MethodPost, RepoPath(workspace, slug, "pullrequests"), body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// UpdatePullRequest changes the given fields. Bitbucket's PUT is partial: fields left out keep their value.
func (c *Client) UpdatePullRequest(ctx context.Context, workspace, slug string, id int, in PRUpdate) (*PullRequest, error) {
	body := map[string]any{}
	if in.Title != nil {
		body["title"] = *in.Title
	}
	if in.Description != nil {
		body["description"] = *in.Description
	}
	if in.Destination != nil {
		body["destination"] = branchRef(*in.Destination)
	}
	if in.Draft != nil {
		body["draft"] = *in.Draft
	}
	if in.Reviewers != nil {
		body["reviewers"] = userRefs(in.Reviewers)
	}
	var pr PullRequest
	if err := c.Do(ctx, http.MethodPut, prPath(workspace, slug, id), body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// CreatePRComment adds a comment to a pull request.
func (c *Client) CreatePRComment(ctx context.Context, workspace, slug string, id int, in PRCommentInput) (*Comment, error) {
	body := map[string]any{"content": map[string]any{"raw": in.Body}}
	if in.Path != "" {
		inline := map[string]any{"path": in.Path}
		if in.Line > 0 {
			inline["to"] = in.Line
		}
		body["inline"] = inline
	}
	if in.ParentID > 0 {
		body["parent"] = map[string]any{"id": in.ParentID}
	}
	var cm Comment
	if err := c.Do(ctx, http.MethodPost, prPath(workspace, slug, id, "comments"), body, &cm); err != nil {
		return nil, err
	}
	return &cm, nil
}

// ApprovePR approves a pull request as the authenticated user.
func (c *Client) ApprovePR(ctx context.Context, workspace, slug string, id int) error {
	return c.Do(ctx, http.MethodPost, prPath(workspace, slug, id, "approve"), nil, nil)
}

// UnapprovePR withdraws the authenticated user's approval.
func (c *Client) UnapprovePR(ctx context.Context, workspace, slug string, id int) error {
	return c.Do(ctx, http.MethodDelete, prPath(workspace, slug, id, "approve"), nil, nil)
}

// RequestChangesPR requests changes on a pull request as the authenticated user.
func (c *Client) RequestChangesPR(ctx context.Context, workspace, slug string, id int) error {
	return c.Do(ctx, http.MethodPost, prPath(workspace, slug, id, "request-changes"), nil, nil)
}

// DeclinePR declines a pull request; a non-empty message becomes its reason. Declined pull requests
// cannot be reopened.
func (c *Client) DeclinePR(ctx context.Context, workspace, slug string, id int, message string) (*PullRequest, error) {
	var in any
	if message != "" {
		in = map[string]string{"message": message}
	}
	var pr PullRequest
	if err := c.Do(ctx, http.MethodPost, prPath(workspace, slug, id, "decline"), in, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}
```

`internal/bitbucket/repos.go`:

```go
package bitbucket

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// userEntry is the shape of workspace members and default reviewers: a user wrapped in an object.
type userEntry struct {
	User User `json:"user"`
}

func toUsers(entries []userEntry) []User {
	out := make([]User, len(entries))
	for i, e := range entries {
		out[i] = e.User
	}
	return out
}

// RepositoryMainBranch returns the name of a repository's main branch.
func (c *Client) RepositoryMainBranch(ctx context.Context, workspace, slug string) (string, error) {
	var repo struct {
		MainBranch *struct {
			Name string `json:"name"`
		} `json:"mainbranch"`
	}
	if err := c.Do(ctx, http.MethodGet, RepoPath(workspace, slug), nil, &repo); err != nil {
		return "", err
	}
	if repo.MainBranch == nil || repo.MainBranch.Name == "" {
		return "", fmt.Errorf("repository %s/%s has no main branch", workspace, slug)
	}
	return repo.MainBranch.Name, nil
}

// EffectiveDefaultReviewers returns the default reviewers of a repository, including its project's.
func (c *Client) EffectiveDefaultReviewers(ctx context.Context, workspace, slug string) ([]User, error) {
	entries, err := List[userEntry](ctx, c, RepoPath(workspace, slug, "effective-default-reviewers"), 0)
	if err != nil {
		return nil, err
	}
	return toUsers(entries), nil
}

// FindWorkspaceMembers returns the members of a workspace matching a BBQL query ("" means every
// member). Bitbucket filters on user.uuid, user.account_id and user.nickname, not on display_name.
func (c *Client) FindWorkspaceMembers(ctx context.Context, workspace, query string) ([]User, error) {
	path := "workspaces/" + url.PathEscape(workspace) + "/members"
	if query != "" {
		path += "?" + url.Values{"q": {query}}.Encode()
	}
	entries, err := List[userEntry](ctx, c, path, 0)
	if err != nil {
		return nil, err
	}
	return toUsers(entries), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/bitbucket/ && go vet ./internal/bitbucket/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bitbucket/prwrite.go internal/bitbucket/prwrite_test.go internal/bitbucket/repos.go internal/bitbucket/repos_test.go
git commit -m "feat(bitbucket): pull request write endpoints, default reviewers and members"
```

---

### Task 3: Merge API in the Bitbucket client

**Files:**
- Create: `internal/bitbucket/merge.go`
- Modify: `internal/bitbucket/errors.go` (add `IsTransient`), `pkg/cmd/pr/checks/checks.go` (use it)
- Test: `internal/bitbucket/merge_test.go`, `internal/bitbucket/errors_test.go`

**Interfaces:**
- Consumes: `prPath` (Task 2), `Client.Request`, `Client.Do`, `readHTTPError` (existing); test helpers `newTestClient`, `prPath`, `prJSON`, `assertBody` (Task 2).
- Produces:
  - `type PRMerge struct { Strategy, Message string; CloseSourceBranch bool }`.
  - `(*Client).StartMerge(ctx, workspace, slug string, id int, in PRMerge) (*PullRequest, string, error)` — `POST …/merge?async=true` with `{"type":"pullrequest","close_source_branch":…}` plus `merge_strategy`/`message` when non-empty. 202 → `(nil, <Location URL>, nil)`; other 2xx → `(<merged PR>, "", nil)`; else `*HTTPError`.
  - `(*Client).MergeTaskStatus(ctx, taskURL string) (*PullRequest, bool, error)` — `(nil, false, nil)` while the task has no `merge_result`; `(pr, true, nil)` when it has one; a failed merge is an `*HTTPError` (Bitbucket answers 400 with the merge error).
  - `type MergeStrategies struct { Allowed []string; Default string }`; `(*Client).PRMergeStrategies(ctx, workspace, slug string, id int) (MergeStrategies, error)` — `GET …/pullrequests/{id}?fields=destination.branch.*`.
  - `type MergeCheck struct { Type, Status, Reason string; Required, Blocking bool }`; `(*Client).PRMergeabilityChecks(ctx, workspace, slug string, id int) ([]MergeCheck, error)` — `GET …/mergeability/checks`.
  - `bitbucket.IsTransient(err error) bool` — true for HTTP 429 and ≥ 500 and for `*NetworkError`.

- [ ] **Step 1: Write the failing tests**

`internal/bitbucket/merge_test.go`:

```go
package bitbucket_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

const (
	taskURL  = "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests/42/merge/task-status/t1"
	taskPath = "/2.0/repositories/acme/widgets/pullrequests/42/merge/task-status/t1"
)

func TestStartMerge_Accepted(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", prPath+"/42/merge", httpmock.WithHeader(httpmock.JSONResponse(202, `""`), "Location", taskURL))

	pr, loc, err := c.StartMerge(context.Background(), "acme", "widgets", 42, bitbucket.PRMerge{Strategy: "squash", Message: "Add widgets"})
	if err != nil || pr != nil || loc != taskURL {
		t.Fatalf("pr %v loc %q err %v", pr, loc, err)
	}
	if q := reg.Calls[0].URL.Query().Get("async"); q != "true" {
		t.Errorf("async = %q", q)
	}
	assertBody(t, reg.Calls[0], `{"type":"pullrequest","merge_strategy":"squash","message":"Add widgets","close_source_branch":false}`)
}

func TestStartMerge_FinishedAtOnce(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", prPath+"/42/merge", httpmock.JSONResponse(200, prJSON))

	pr, loc, err := c.StartMerge(context.Background(), "acme", "widgets", 42, bitbucket.PRMerge{CloseSourceBranch: true})
	if err != nil || pr == nil || pr.ID != 42 || loc != "" {
		t.Fatalf("pr %v loc %q err %v", pr, loc, err)
	}
	assertBody(t, reg.Calls[0], `{"type":"pullrequest","close_source_branch":true}`)
}

func TestStartMerge_Errors(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", prPath+"/42/merge", httpmock.JSONResponse(400,
		`{"type":"error","error":{"message":"You can't merge until you resolve all merge conflicts."}}`))
	reg.Register("POST", prPath+"/42/merge", httpmock.JSONResponse(202, `""`))

	_, _, err := c.StartMerge(context.Background(), "acme", "widgets", 42, bitbucket.PRMerge{})
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || !strings.Contains(httpErr.Message, "merge conflicts") {
		t.Errorf("err = %v", err)
	}
	if _, _, err := c.StartMerge(context.Background(), "acme", "widgets", 42, bitbucket.PRMerge{}); err == nil || !strings.Contains(err.Error(), "task location") {
		t.Errorf("202 without Location: err = %v", err)
	}
}

func TestMergeTaskStatus(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", taskPath, httpmock.JSONResponse(200, `{"task_status":"PENDING","links":{}}`))
	reg.Register("GET", taskPath, httpmock.JSONResponse(200, `{"task_status":"SUCCESS","merge_result":`+prJSON+`}`))
	reg.Register("GET", taskPath, httpmock.JSONResponse(400,
		`{"type":"error","error":{"message":"You can't merge until you resolve all merge conflicts."}}`))

	if pr, done, err := c.MergeTaskStatus(context.Background(), taskURL); pr != nil || done || err != nil {
		t.Errorf("pending: pr %v done %v err %v", pr, done, err)
	}
	if pr, done, err := c.MergeTaskStatus(context.Background(), taskURL); pr == nil || pr.ID != 42 || !done || err != nil {
		t.Errorf("success: pr %v done %v err %v", pr, done, err)
	}
	var httpErr *bitbucket.HTTPError
	if _, _, err := c.MergeTaskStatus(context.Background(), taskURL); !errors.As(err, &httpErr) || httpErr.StatusCode != 400 {
		t.Errorf("failed task: err = %v", err)
	}
}

func TestPRMergeStrategies(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath+"/42", httpmock.JSONResponse(200,
		`{"destination":{"branch":{"name":"main","merge_strategies":["merge_commit","squash"],"default_merge_strategy":"squash"}}}`))

	got, err := c.PRMergeStrategies(context.Background(), "acme", "widgets", 42)
	if err != nil || got.Default != "squash" || !slices.Equal(got.Allowed, []string{"merge_commit", "squash"}) {
		t.Errorf("got %+v err %v", got, err)
	}
	if f := reg.Calls[0].URL.Query().Get("fields"); f != "destination.branch.*" {
		t.Errorf("fields = %q", f)
	}
}

func TestPRMergeabilityChecks(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", prPath+"/42/mergeability/checks", httpmock.JSONResponse(200, `{"size":2,"values":[`+
		`{"type":"pullrequest_state_check","status":"PASSED","required":true,"blocking":false,"reason":null,"state":"OPEN"},`+
		`{"type":"git_mergeability_check","status":"FAILED","required":true,"blocking":true,"reason":"conflicts"}]}`))

	checks, err := c.PRMergeabilityChecks(context.Background(), "acme", "widgets", 42)
	if err != nil || len(checks) != 2 {
		t.Fatalf("checks %+v err %v", checks, err)
	}
	want := bitbucket.MergeCheck{Type: "git_mergeability_check", Status: "FAILED", Reason: "conflicts", Required: true, Blocking: true}
	if checks[1] != want || checks[0].Reason != "" {
		t.Errorf("checks = %+v", checks)
	}
}
```

Append to `internal/bitbucket/errors_test.go` (package `bitbucket_test`; add the imports `errors` and `fmt` if missing):

```go
func TestIsTransient(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{&bitbucket.HTTPError{StatusCode: 429}, true},
		{&bitbucket.HTTPError{StatusCode: 503}, true},
		{&bitbucket.HTTPError{StatusCode: 555}, true},
		{fmt.Errorf("polling: %w", &bitbucket.HTTPError{StatusCode: 502}), true},
		{&bitbucket.NetworkError{Err: errors.New("connection reset")}, true},
		{&bitbucket.HTTPError{StatusCode: 404}, false},
		{&bitbucket.HTTPError{StatusCode: 400}, false},
		{errors.New("boom"), false},
	}
	for _, tc := range cases {
		if got := bitbucket.IsTransient(tc.err); got != tc.want {
			t.Errorf("IsTransient(%v) = %v", tc.err, got)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/bitbucket/`
Expected: FAIL — `undefined: bitbucket.PRMerge`, `undefined: bitbucket.IsTransient`.

- [ ] **Step 3: Implement**

`internal/bitbucket/merge.go`:

```go
package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// PRMerge describes how to merge a pull request. An empty Strategy lets Bitbucket use merge_commit.
type PRMerge struct {
	Strategy          string // merge_commit, squash, fast_forward, …
	Message           string
	CloseSourceBranch bool
}

// MergeStrategies are the merge strategies a pull request's destination branch allows, and its default.
type MergeStrategies struct {
	Allowed []string
	Default string
}

// MergeCheck is one of Bitbucket's merge checks. A FAILED check with Blocking set prevents the merge.
type MergeCheck struct {
	Type     string `json:"type"`
	Status   string `json:"status"` // PASSED or FAILED
	Reason   string `json:"reason"` // such as "clean" or "conflicts"; often null
	Required bool   `json:"required"`
	Blocking bool   `json:"blocking"`
}

// StartMerge asks Bitbucket to merge a pull request asynchronously. It returns the merged pull
// request when Bitbucket finishes at once, or else the URL of the merge task to poll with
// MergeTaskStatus. close_source_branch is always sent: when omitted, Bitbucket falls back to the
// pull request's own setting and may delete the branch.
func (c *Client) StartMerge(ctx context.Context, workspace, slug string, id int, in PRMerge) (*PullRequest, string, error) {
	body := map[string]any{"type": "pullrequest", "close_source_branch": in.CloseSourceBranch}
	if in.Strategy != "" {
		body["merge_strategy"] = in.Strategy
	}
	if in.Message != "" {
		body["message"] = in.Message
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("encoding request body: %w", err)
	}
	resp, err := c.Request(ctx, http.MethodPost, prPath(workspace, slug, id, "merge")+"?async=true", nil, b)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusAccepted:
		_, _ = io.Copy(io.Discard, resp.Body)
		loc := resp.Header.Get("Location")
		if loc == "" {
			return nil, "", fmt.Errorf("the merge of pull request #%d was accepted without a task location", id)
		}
		return nil, loc, nil
	case resp.StatusCode >= 200 && resp.StatusCode <= 299:
		var pr PullRequest
		if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
			return nil, "", fmt.Errorf("decoding the merged pull request: %w", err)
		}
		return &pr, "", nil
	}
	return nil, "", readHTTPError(resp)
}

// MergeTaskStatus polls a merge task. It reports done=false while the task has no result; note
// that Bitbucket answers PENDING forever for an unknown task, so callers need a deadline. A merge
// that fails inside the task comes back as an *HTTPError, like a failed synchronous merge.
func (c *Client) MergeTaskStatus(ctx context.Context, taskURL string) (*PullRequest, bool, error) {
	var st struct {
		TaskStatus  string       `json:"task_status"`
		MergeResult *PullRequest `json:"merge_result"`
	}
	if err := c.Do(ctx, http.MethodGet, taskURL, nil, &st); err != nil {
		return nil, false, err
	}
	if st.MergeResult == nil {
		return nil, false, nil
	}
	return st.MergeResult, true, nil
}

// PRMergeStrategies returns the merge strategies allowed into a pull request's destination branch
// and its default. Bitbucket includes them only when asked through the fields parameter.
func (c *Client) PRMergeStrategies(ctx context.Context, workspace, slug string, id int) (MergeStrategies, error) {
	var pr struct {
		Destination struct {
			Branch struct {
				MergeStrategies      []string `json:"merge_strategies"`
				DefaultMergeStrategy string   `json:"default_merge_strategy"`
			} `json:"branch"`
		} `json:"destination"`
	}
	path := prPath(workspace, slug, id) + "?" + url.Values{"fields": {"destination.branch.*"}}.Encode()
	if err := c.Do(ctx, http.MethodGet, path, nil, &pr); err != nil {
		return MergeStrategies{}, err
	}
	b := pr.Destination.Branch
	return MergeStrategies{Allowed: b.MergeStrategies, Default: b.DefaultMergeStrategy}, nil
}

// PRMergeabilityChecks returns the merge checks of a pull request.
func (c *Client) PRMergeabilityChecks(ctx context.Context, workspace, slug string, id int) ([]MergeCheck, error) {
	var res struct {
		Values []MergeCheck `json:"values"`
	}
	if err := c.Do(ctx, http.MethodGet, prPath(workspace, slug, id, "mergeability", "checks"), nil, &res); err != nil {
		return nil, err
	}
	return res.Values, nil
}
```

In `internal/bitbucket/errors.go`, add `"errors"` to the imports and append:

```go
// IsTransient reports whether err is worth retrying later: rate limiting, a server error or a
// network failure.
func IsTransient(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusTooManyRequests || httpErr.StatusCode >= 500
	}
	var netErr *NetworkError
	return errors.As(err, &netErr)
}
```

In `pkg/cmd/pr/checks/checks.go`, replace the call `transient(err)` with `bitbucket.IsTransient(err)`, delete the `transient` function, and remove the `"errors"` import (it was only used there).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/bitbucket/ ./pkg/cmd/pr/checks/ && go vet ./...`
Expected: PASS, including `TestChecks_WatchRetriesTransientErrors` and `TestChecks_WatchStopsOnNotFound`.

- [ ] **Step 5: Commit**

```bash
git add internal/bitbucket/merge.go internal/bitbucket/merge_test.go internal/bitbucket/errors.go internal/bitbucket/errors_test.go pkg/cmd/pr/checks/checks.go
git commit -m "feat(bitbucket): asynchronous merge, merge strategies and merge checks"
```

---
### Task 4: Shared helpers for write commands

**Files:**
- Modify: `internal/cmdutil/errors.go` (add `ConflictError`, classify it)
- Create: `pkg/cmd/pr/shared/write.go`
- Modify: `pkg/cmd/pr/shared/prtest/prtest.go` (dry run in `NewFactory`; write-test helpers)
- Test: `internal/cmdutil/errors_test.go`, `pkg/cmd/pr/shared/write_test.go`, `pkg/cmd/pr/shared/prtest/prtest_test.go`

**Interfaces:**
- Consumes: `bitbucket.PullRequest`, `iostreams.IOStreams` (`In`, `ErrOut`, `IsStderrTTY`, `Green`), `cmdutil.FlagErrorf`, `httpmock.Registry`/`Call`.
- Produces:
  - `cmdutil.ConflictError{Msg string}` — `Classify` → `{Code: "conflict", Exit: 1}`.
  - `shared.RequireOpen(pr *bitbucket.PullRequest, action string) error` — nil for `OPEN`, else `*cmdutil.ConflictError` "pull request #42 is merged; only open pull requests can be <action>".
  - `shared.Describe(pr *bitbucket.PullRequest) string` — `"#42 (feature/widgets → main)"`.
  - `shared.ReadBody(ios *iostreams.IOStreams, body string, bodySet bool, bodyFile string) (text string, provided bool, err error)` — `--body` wins when `bodySet` (an empty `--body ""` is a body); `bodyFile` `"-"` reads `ios.In`; both → usage error.
  - `shared.PrintSuccess(ios *iostreams.IOStreams, format string, args ...any)` — one line on stderr, prefixed with a green `✓ ` when stderr is a terminal.
  - `prtest.NewFactory` — the client now honors `f.DryRun`, printing to the returned stdout buffer.
  - `prtest.SetTTY(ios)`, `prtest.WithState(pr, state string) string`, `prtest.Member(user string) string`, `prtest.Writes(reg) []httpmock.Call` (every recorded request that is not a GET), `prtest.AssertJSONBody(t, call, want string)`, and the path constants `prtest.Repo` (`/2.0/repositories/acme/widgets`) and `prtest.Members` (`/2.0/workspaces/acme/members`).

- [ ] **Step 1: Write the failing tests**

In `internal/cmdutil/errors_test.go`, add this row to the `cases` table of `TestClassify`, after the `"not found"` row:

```go
		{"conflict state", &cmdutil.ConflictError{Msg: "pull request #42 is merged; only open pull requests can be approved"}, "conflict", 1, 0, "", false},
```

`pkg/cmd/pr/shared/write_test.go`:

```go
package shared_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

func prWithBranches(state string) *bitbucket.PullRequest {
	pr := &bitbucket.PullRequest{ID: 42, State: state}
	pr.Source.Branch.Name = "feature/widgets"
	pr.Destination.Branch.Name = "main"
	return pr
}

func TestRequireOpen(t *testing.T) {
	if err := shared.RequireOpen(prWithBranches("OPEN"), "merged"); err != nil {
		t.Errorf("open: %v", err)
	}
	err := shared.RequireOpen(prWithBranches("MERGED"), "approved")
	var conflict *cmdutil.ConflictError
	if !errors.As(err, &conflict) || err.Error() != "pull request #42 is merged; only open pull requests can be approved" {
		t.Errorf("err = %v", err)
	}
}

func TestDescribe(t *testing.T) {
	if got := shared.Describe(prWithBranches("OPEN")); got != "#42 (feature/widgets → main)" {
		t.Errorf("Describe = %q", got)
	}
}

func TestReadBody(t *testing.T) {
	ios, stdin, _, _ := iostreams.Test()
	stdin.WriteString("from stdin\n")
	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte("from file"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		body     string
		bodySet  bool
		file     string
		want     string
		provided bool
	}{
		{"flag", "inline", true, "", "inline", true},
		{"empty flag is a body", "", true, "", "", true},
		{"stdin", "", false, "-", "from stdin\n", true},
		{"file", "", false, path, "from file", true},
		{"nothing", "", false, "", "", false},
	}
	for _, tc := range cases {
		got, provided, err := shared.ReadBody(ios, tc.body, tc.bodySet, tc.file)
		if err != nil || got != tc.want || provided != tc.provided {
			t.Errorf("%s: got %q %v %v", tc.name, got, provided, err)
		}
	}
	var flagErr *cmdutil.FlagError
	if _, _, err := shared.ReadBody(ios, "x", true, path); !errors.As(err, &flagErr) {
		t.Errorf("both flags: err = %v", err)
	}
	if _, _, err := shared.ReadBody(ios, "", false, filepath.Join(t.TempDir(), "missing.md")); err == nil || !strings.Contains(err.Error(), "--body-file") {
		t.Errorf("missing file: err = %v", err)
	}
}

func TestPrintSuccess(t *testing.T) {
	ios, _, out, errOut := iostreams.Test()
	shared.PrintSuccess(ios, "Merged pull request %s", "#42")
	ios.SetStderrTTY(true)
	shared.PrintSuccess(ios, "Declined pull request %s", "#43")
	if want := "Merged pull request #42\n✓ Declined pull request #43\n"; errOut.String() != want || out.Len() != 0 {
		t.Errorf("stderr %q stdout %q", errOut.String(), out.String())
	}
}
```

`pkg/cmd/pr/shared/prtest/prtest_test.go`:

```go
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
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cmdutil/ ./pkg/cmd/pr/shared/...`
Expected: FAIL — `undefined: cmdutil.ConflictError`, `undefined: shared.RequireOpen`, `undefined: prtest.WithState`.

- [ ] **Step 3: Implement**

In `internal/cmdutil/errors.go`, add after `NotFoundError`:

```go
// ConflictError means the resource is in a state that does not allow the action, such as a pull
// request that is no longer open.
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }
```

In `Classify`, add `conflict *ConflictError` to the `var` block and this case right after the `notFound` case:

```go
	case errors.As(err, &conflict):
		return ErrorInfo{Code: "conflict", Message: err.Error(), Exit: 1}
```

`pkg/cmd/pr/shared/write.go`:

```go
package shared

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
)

// RequireOpen refuses to act on a pull request that is no longer open. Bitbucket itself accepts
// some of these actions, such as approving a merged pull request.
func RequireOpen(pr *bitbucket.PullRequest, action string) error {
	if pr.State == "OPEN" {
		return nil
	}
	return &cmdutil.ConflictError{Msg: fmt.Sprintf("pull request #%d is %s; only open pull requests can be %s",
		pr.ID, strings.ToLower(pr.State), action)}
}

// Describe names a pull request with its branches, such as "#42 (feature/widgets → main)".
func Describe(pr *bitbucket.PullRequest) string {
	return fmt.Sprintf("#%d (%s → %s)", pr.ID, pr.Source.Branch.Name, pr.Destination.Branch.Name)
}

// ReadBody returns the text given with --body (when bodySet, even if empty) or --body-file, where
// "-" reads standard input. provided is false when neither flag was used.
func ReadBody(ios *iostreams.IOStreams, body string, bodySet bool, bodyFile string) (text string, provided bool, err error) {
	switch {
	case bodySet && bodyFile != "":
		return "", false, cmdutil.FlagErrorf("specify only one of --body and --body-file")
	case bodySet:
		return body, true, nil
	case bodyFile == "-":
		b, err := io.ReadAll(ios.In)
		if err != nil {
			return "", false, fmt.Errorf("reading the body from standard input: %w", err)
		}
		return string(b), true, nil
	case bodyFile != "":
		b, err := os.ReadFile(bodyFile)
		if err != nil {
			return "", false, fmt.Errorf("reading --body-file: %w", err)
		}
		return string(b), true, nil
	}
	return "", false, nil
}

// PrintSuccess reports a completed action on stderr, with a check mark on a terminal.
func PrintSuccess(ios *iostreams.IOStreams, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if ios.IsStderrTTY() {
		msg = ios.Green("✓") + " " + msg
	}
	fmt.Fprintln(ios.ErrOut, msg)
}
```

In `pkg/cmd/pr/shared/prtest/prtest.go`:

1. Add the imports `"encoding/json"`, `"reflect"` and `"testing"`.
2. Add after the `PRs` constant:

```go
// Repo and Members are the request paths of the acme/widgets repository and the acme workspace's members.
const (
	Repo    = "/2.0/repositories/acme/widgets"
	Members = "/2.0/workspaces/acme/members"
)
```

3. Replace `NewFactory` with:

```go
// NewFactory returns a non-TTY Factory bound to acme/widgets whose client talks to reg without
// retries. The client honors f.DryRun (--dry-run), printing to the returned stdout buffer.
func NewFactory(reg *httpmock.Registry) (*cmdutil.Factory, *iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	ios, _, out, errOut := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, RepoOverride: "acme/widgets"}
	f.HTTPClient = func() (*bitbucket.Client, error) {
		return bitbucket.New(bitbucket.Options{
			Email: "dev@example.com", Token: "t", HTTPClient: reg.Client(), MaxAttempts: 1,
			DryRun: f.DryRun, DryRunOut: ios.Out,
		}), nil
	}
	return f, ios, out, errOut
}
```

4. Append:

```go
// SetTTY makes every stream of ios behave like a terminal.
func SetTTY(ios *iostreams.IOStreams) {
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)
}

// WithState returns a pull request fixture with the pull request's state replaced, such as
// WithState(PR42, "MERGED"). Participant states are left alone.
func WithState(pr, state string) string {
	return strings.Replace(pr, `"state":"OPEN"`, `"state":"`+state+`"`, 1)
}

// Member wraps an account fixture in a workspace membership, as GET /workspaces/{ws}/members returns it.
func Member(user string) string {
	return `{"type":"workspace_membership","user":` + user + `}`
}

// Writes returns the requests reg received that change something: every method but GET.
func Writes(reg *httpmock.Registry) []httpmock.Call {
	var writes []httpmock.Call
	for _, c := range reg.Calls {
		if c.Method != "GET" {
			writes = append(writes, c)
		}
	}
	return writes
}

// AssertJSONBody fails t unless the request body is the JSON value want (key order and spacing ignored).
func AssertJSONBody(t testing.TB, c httpmock.Call, want string) {
	t.Helper()
	var got, exp any
	if err := json.Unmarshal(c.Body, &got); err != nil {
		t.Fatalf("request body %q: %v", c.Body, err)
	}
	if err := json.Unmarshal([]byte(want), &exp); err != nil {
		t.Fatalf("bad expectation %q: %v", want, err)
	}
	if !reflect.DeepEqual(got, exp) {
		t.Errorf("request body = %s, want %s", c.Body, want)
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cmdutil/ ./pkg/cmd/pr/... && go vet ./...`
Expected: PASS — the existing `pr` command tests are unaffected by the `NewFactory` change.

- [ ] **Step 5: Commit**

```bash
git add internal/cmdutil/errors.go internal/cmdutil/errors_test.go pkg/cmd/pr/shared/write.go pkg/cmd/pr/shared/write_test.go pkg/cmd/pr/shared/prtest/
git commit -m "feat(pr): shared helpers for write commands; dry run in test factory"
```

---

### Task 5: Reviewer resolution against workspace members

**Files:**
- Create: `pkg/cmd/pr/shared/reviewers.go`
- Test: `pkg/cmd/pr/shared/reviewers_test.go`

**Interfaces:**
- Consumes: `Client.CurrentUser`, `Client.FindWorkspaceMembers` (Task 2), `bitbucket.QuoteBBQL`, `cmdutil.NotFoundError`, `cmdutil.FlagErrorf`; test helpers `prtest.NewFactory`, `prtest.Members`, `prtest.Member`, `prtest.Page`, `prtest.Ada/Bob/Cy`, `prtest.*UUID` (Task 4).
- Produces:
  - `type ReviewerResolver struct { Client *bitbucket.Client; Workspace string }` (plus an unexported cache of all members).
  - `(*ReviewerResolver).Resolve(ctx, who string) (bitbucket.User, error)` — `@me` → `GET /user`; `{uuid}` → `q=user.uuid = "…"`; else `q=(user.account_id = "X" OR user.nickname = "X")`, then — only when that finds nobody and `who` is not a `{uuid}` — every member once, matched on display name ignoring case. Exactly one match → the user; none → `*cmdutil.NotFoundError` `no member of workspace acme matches reviewer "zed"`; several → `*cmdutil.FlagError` listing `nickname (display name, {uuid})`.
  - `(*ReviewerResolver).ResolveAll(ctx, values []string) ([]string, error)` — UUIDs in order (duplicates kept; use `UniqueUUIDs`).
  - `shared.UniqueUUIDs(uuids []string, exclude ...string) []string` — order kept, duplicates and excluded UUIDs dropped (case-insensitive), never nil.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pr/shared/reviewers_test.go`:

```go
package shared_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func newResolver(reg *httpmock.Registry) *shared.ReviewerResolver {
	f, _, _, _ := prtest.NewFactory(reg)
	client, _ := f.HTTPClient()
	return &shared.ReviewerResolver{Client: client, Workspace: "acme"}
}

func members(users ...string) httpmock.Responder {
	items := make([]string, len(users))
	for i, u := range users {
		items[i] = prtest.Member(u)
	}
	return httpmock.JSONResponse(200, prtest.Page(items...))
}

func TestResolve_ByNicknameOrAccountID(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members(prtest.Bob))

	u, err := newResolver(reg).Resolve(context.Background(), "bob")
	if err != nil || u.UUID != prtest.BobUUID {
		t.Fatalf("user %+v err %v", u, err)
	}
	if q := reg.Calls[0].URL.Query().Get("q"); q != `(user.account_id = "bob" OR user.nickname = "bob")` {
		t.Errorf("q = %q", q)
	}
}

func TestResolve_ByUUID(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members(prtest.Cy))

	u, err := newResolver(reg).Resolve(context.Background(), prtest.CyUUID)
	if err != nil || u.Nickname != "cy" {
		t.Fatalf("user %+v err %v", u, err)
	}
	if q := reg.Calls[0].URL.Query().Get("q"); q != `user.uuid = "`+prtest.CyUUID+`"` {
		t.Errorf("q = %q", q)
	}
}

func TestResolve_Me(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))

	if u, err := newResolver(reg).Resolve(context.Background(), "@me"); err != nil || u.UUID != prtest.AdaUUID {
		t.Errorf("user %+v err %v", u, err)
	}
}

func TestResolve_ByDisplayNameLoadsMembersOnce(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members())                      // q for "cy example"
	reg.Register("GET", prtest.Members, members(prtest.Bob, prtest.Cy)) // every member
	reg.Register("GET", prtest.Members, members())                      // q for "Bob Example"
	r := newResolver(reg)

	cy, err := r.Resolve(context.Background(), "cy example")
	if err != nil || cy.UUID != prtest.CyUUID {
		t.Fatalf("cy %+v err %v", cy, err)
	}
	if reg.Calls[1].URL.Query().Has("q") {
		t.Errorf("the display-name fallback must list every member: %s", reg.Calls[1].URL.RawQuery)
	}
	bob, err := r.Resolve(context.Background(), "Bob Example")
	if err != nil || bob.UUID != prtest.BobUUID || len(reg.Calls) != 3 {
		t.Errorf("bob %+v err %v calls %d", bob, err, len(reg.Calls))
	}
}

func TestResolve_NoMatch(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members())
	reg.Register("GET", prtest.Members, members(prtest.Bob))

	_, err := newResolver(reg).Resolve(context.Background(), "zed")
	var notFound *cmdutil.NotFoundError
	if !errors.As(err, &notFound) || err.Error() != `no member of workspace acme matches reviewer "zed"` {
		t.Errorf("err = %v", err)
	}
}

func TestResolve_UUIDIsNeverMatchedByDisplayName(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members())

	_, err := newResolver(reg).Resolve(context.Background(), "{00000000-0000-0000-0000-000000000009}")
	var notFound *cmdutil.NotFoundError
	if !errors.As(err, &notFound) || len(reg.Calls) != 1 {
		t.Errorf("err %v calls %d", err, len(reg.Calls))
	}
}

func TestResolve_Ambiguous(t *testing.T) {
	sam1 := `{"display_name":"Sam Example","nickname":"sam1","uuid":"{00000000-0000-0000-0000-000000000011}","account_id":"000000:s1"}`
	sam2 := `{"display_name":"Sam Example","nickname":"sam2","uuid":"{00000000-0000-0000-0000-000000000012}","account_id":"000000:s2"}`
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members())
	reg.Register("GET", prtest.Members, members(sam1, sam2))

	_, err := newResolver(reg).Resolve(context.Background(), "Sam Example")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "sam1 (Sam Example,") || !strings.Contains(err.Error(), "sam2 (Sam Example,") {
		t.Errorf("err = %v", err)
	}
}

func TestResolveAll(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members(prtest.Bob))
	reg.Register("GET", prtest.Members, members(prtest.Cy))

	got, err := newResolver(reg).ResolveAll(context.Background(), []string{"bob", "cy"})
	if err != nil || !slices.Equal(got, []string{prtest.BobUUID, prtest.CyUUID}) {
		t.Errorf("got %v err %v", got, err)
	}
}

func TestUniqueUUIDs(t *testing.T) {
	got := shared.UniqueUUIDs([]string{"{b}", "{a}", "{B}", "{c}", "{d}"}, "{A}", "{d}")
	if !slices.Equal(got, []string{"{b}", "{c}"}) {
		t.Errorf("UniqueUUIDs = %v", got)
	}
	if got := shared.UniqueUUIDs(nil); got == nil || len(got) != 0 {
		t.Errorf("UniqueUUIDs(nil) = %#v, want an empty, non-nil slice", got)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/shared/`
Expected: FAIL — `undefined: shared.ReviewerResolver`.

- [ ] **Step 3: Implement**

`pkg/cmd/pr/shared/reviewers.go`:

```go
package shared

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
)

// ReviewerResolver turns --reviewer values into workspace members.
type ReviewerResolver struct {
	Client    *bitbucket.Client
	Workspace string

	members []bitbucket.User // every member, loaded once for display-name lookups
	loaded  bool
}

// Resolve returns the one workspace member named by who: @me, a {uuid}, an account ID, a nickname
// or a display name. Bitbucket cannot filter members by display name, so a name that matches no
// account ID or nickname is looked up in the full member list.
func (r *ReviewerResolver) Resolve(ctx context.Context, who string) (bitbucket.User, error) {
	who = strings.TrimSpace(who)
	if who == "" {
		return bitbucket.User{}, cmdutil.FlagErrorf("empty reviewer")
	}
	if who == "@me" {
		me, err := r.Client.CurrentUser(ctx)
		if err != nil {
			return bitbucket.User{}, err
		}
		return *me, nil
	}
	isUUID := strings.HasPrefix(who, "{") && strings.HasSuffix(who, "}")
	query := "user.uuid = " + bitbucket.QuoteBBQL(who)
	if !isUUID {
		q := bitbucket.QuoteBBQL(who)
		query = "(user.account_id = " + q + " OR user.nickname = " + q + ")"
	}
	found, err := r.Client.FindWorkspaceMembers(ctx, r.Workspace, query)
	if err != nil {
		return bitbucket.User{}, err
	}
	if len(found) == 0 && !isUUID {
		if found, err = r.byDisplayName(ctx, who); err != nil {
			return bitbucket.User{}, err
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return bitbucket.User{}, &cmdutil.NotFoundError{Msg: fmt.Sprintf("no member of workspace %s matches reviewer %q", r.Workspace, who)}
	}
	names := make([]string, len(found))
	for i, u := range found {
		names[i] = fmt.Sprintf("%s (%s, %s)", u.Nickname, u.DisplayName, u.UUID)
	}
	return bitbucket.User{}, cmdutil.FlagErrorf("reviewer %q matches several members of workspace %s: %s; use a nickname or {uuid}",
		who, r.Workspace, strings.Join(names, "; "))
}

// ResolveAll resolves every value and returns the members' UUIDs in order.
func (r *ReviewerResolver) ResolveAll(ctx context.Context, values []string) ([]string, error) {
	uuids := make([]string, 0, len(values))
	for _, v := range values {
		u, err := r.Resolve(ctx, v)
		if err != nil {
			return nil, err
		}
		uuids = append(uuids, u.UUID)
	}
	return uuids, nil
}

func (r *ReviewerResolver) byDisplayName(ctx context.Context, name string) ([]bitbucket.User, error) {
	if !r.loaded {
		all, err := r.Client.FindWorkspaceMembers(ctx, r.Workspace, "")
		if err != nil {
			return nil, err
		}
		r.members, r.loaded = all, true
	}
	var found []bitbucket.User
	for _, u := range r.members {
		if strings.EqualFold(u.DisplayName, name) {
			found = append(found, u)
		}
	}
	return found, nil
}

// UniqueUUIDs returns uuids in order without duplicates and without exclude, comparing case-insensitively.
// The result is never nil.
func UniqueUUIDs(uuids []string, exclude ...string) []string {
	out := []string{}
	seen := func(list []string, u string) bool {
		return slices.ContainsFunc(list, func(v string) bool { return strings.EqualFold(v, u) })
	}
	for _, u := range uuids {
		if seen(exclude, u) || seen(out, u) {
			continue
		}
		out = append(out, u)
	}
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/cmd/pr/shared/ && go vet ./pkg/cmd/pr/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr/shared/reviewers.go pkg/cmd/pr/shared/reviewers_test.go
git commit -m "feat(pr): resolve reviewers against workspace members"
```

---
### Task 6: `khbb pr approve`, `pr unapprove` and `pr request-changes`

**Files:**
- Create: `pkg/cmd/pr/review/review.go`, `pkg/cmd/pr/review/review_test.go`
- Modify: `pkg/cmd/pr/pr.go`

**Interfaces:**
- Consumes: `shared.Finder`, `shared.CheckRepoSelector`, `shared.RequireOpen`, `shared.Describe`, `shared.PrintSuccess` (Task 4); `Client.ApprovePR`, `UnapprovePR`, `RequestChangesPR` (Task 2); `cmdutil.AddDryRunFlag`; `prtest.NewFactory`, `prtest.Run`, `prtest.PR42`, `prtest.WithState`, `prtest.Writes`.
- Produces: `review.ReviewOptions`; `review.NewCmdApprove`, `review.NewCmdUnapprove`, `review.NewCmdRequestChanges`, each `func(f *cmdutil.Factory, runF func(*ReviewOptions) error) *cobra.Command`. stdout stays empty; stderr gets `Approved pull request #42 (feature/widgets → main)`, `Removed your approval from pull request #42 (…)` or `Requested changes on pull request #42 (…)`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pr/review/review_test.go`:

```go
package review

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

type newCmdFunc func(*cmdutil.Factory, func(*ReviewOptions) error) *cobra.Command

func run(reg *httpmock.Registry, newCmd newCmdFunc, args ...string) (string, string, error) {
	f, _, out, errOut := prtest.NewFactory(reg)
	err := prtest.Run(newCmd(f, nil), args...)
	return out.String(), errOut.String(), err
}

func TestReviewCommands(t *testing.T) {
	cases := []struct {
		name   string
		newCmd newCmdFunc
		method string
		path   string
		want   string
	}{
		{"approve", NewCmdApprove, "POST", "/42/approve", "Approved pull request #42 (feature/widgets → main)\n"},
		{"unapprove", NewCmdUnapprove, "DELETE", "/42/approve", "Removed your approval from pull request #42 (feature/widgets → main)\n"},
		{"request-changes", NewCmdRequestChanges, "POST", "/42/request-changes", "Requested changes on pull request #42 (feature/widgets → main)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := httpmock.New(t)
			reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
			reg.Register(tc.method, prtest.PRs+tc.path, httpmock.JSONResponse(200, `{"approved":true}`))
			out, errOut, err := run(reg, tc.newCmd, "42")
			if err != nil || out != "" || errOut != tc.want {
				t.Errorf("out %q stderr %q err %v", out, errOut, err)
			}
		})
	}
}

func TestReview_RefusesClosedPullRequest(t *testing.T) {
	for _, state := range []string{"MERGED", "DECLINED"} {
		reg := httpmock.New(t)
		reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.WithState(prtest.PR42, state)))
		_, _, err := run(reg, NewCmdApprove, "42")
		var conflict *cmdutil.ConflictError
		if !errors.As(err, &conflict) || !strings.Contains(err.Error(), "is "+strings.ToLower(state)) {
			t.Errorf("%s: err = %v", state, err)
		}
		if w := prtest.Writes(reg); len(w) != 0 {
			t.Errorf("%s: sent %d writes", state, len(w))
		}
	}
}

func TestReview_DryRun(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	out, errOut, err := run(reg, NewCmdRequestChanges, "42", "--dry-run")
	if !errors.Is(err, bitbucket.ErrDryRun) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, `"method": "POST"`) || !strings.Contains(out, "/pullrequests/42/request-changes") || errOut != "" {
		t.Errorf("out %q stderr %q", out, errOut)
	}
}

func TestReview_TooManyArguments(t *testing.T) {
	_, _, err := run(httpmock.New(t), NewCmdApprove, "1", "2")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) {
		t.Errorf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/review/`
Expected: FAIL — `undefined: NewCmdApprove`.

- [ ] **Step 3: Implement**

`pkg/cmd/pr/review/review.go`:

```go
// Package review implements `khbb pr approve`, `pr unapprove` and `pr request-changes`.
package review

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// ReviewOptions holds the inputs and dependencies of the review commands.
type ReviewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)

	Selector string
	Action   Action
}

// Action is a review decision: the API call and how to report it.
type Action struct {
	Name   string // command name
	Done   string // message prefix, such as "Approved"
	submit func(c *bitbucket.Client, ctx context.Context, workspace, slug string, id int) error
}

// The review decisions.
var (
	Approve        = Action{Name: "approve", Done: "Approved", submit: (*bitbucket.Client).ApprovePR}
	Unapprove      = Action{Name: "unapprove", Done: "Removed your approval from", submit: (*bitbucket.Client).UnapprovePR}
	RequestChanges = Action{Name: "request-changes", Done: "Requested changes on", submit: (*bitbucket.Client).RequestChangesPR}
)

// NewCmdApprove returns `khbb pr approve`.
func NewCmdApprove(f *cmdutil.Factory, runF func(*ReviewOptions) error) *cobra.Command {
	return newCmd(f, runF, Approve, "Approve a pull request", `  $ khbb pr approve 42
  $ khbb pr approve`)
}

// NewCmdUnapprove returns `khbb pr unapprove`.
func NewCmdUnapprove(f *cmdutil.Factory, runF func(*ReviewOptions) error) *cobra.Command {
	return newCmd(f, runF, Unapprove, "Remove your approval from a pull request", `  $ khbb pr unapprove 42`)
}

// NewCmdRequestChanges returns `khbb pr request-changes`.
func NewCmdRequestChanges(f *cmdutil.Factory, runF func(*ReviewOptions) error) *cobra.Command {
	return newCmd(f, runF, RequestChanges, "Request changes on a pull request", `  $ khbb pr request-changes 42
  $ khbb pr comment 42 --body "Please add tests" && khbb pr request-changes 42`)
}

func newCmd(f *cmdutil.Factory, runF func(*ReviewOptions) error, action Action, short, example string) *cobra.Command {
	opts := &ReviewOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Action: action}
	cmd := &cobra.Command{
		Use:     action.Name + " [<number> | <url>]",
		Short:   short,
		Long:    short + ". Without an argument, act on the open pull request of the current branch. Only open pull requests can be reviewed.",
		Example: example,
		Args:    cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			if runF != nil {
				return runF(opts)
			}
			return reviewRun(cmd.Context(), opts)
		},
	}
	cmdutil.AddDryRunFlag(cmd, f)
	return cmd
}

func reviewRun(ctx context.Context, opts *ReviewOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if err := shared.RequireOpen(pr, "reviewed"); err != nil {
		return err
	}
	if err := opts.Action.submit(client, ctx, repo.Workspace, repo.Slug, pr.ID); err != nil {
		return err
	}
	shared.PrintSuccess(opts.IO, "%s pull request %s", opts.Action.Done, shared.Describe(pr))
	return nil
}
```

Note: `go vet` accepts a `context.Context` that is not the first parameter of a func type; the receiver of a method expression comes first by definition.

In `pkg/cmd/pr/pr.go`, import `"github.com/khipu/khbb/pkg/cmd/pr/review"`, set the group's `Long` to `"Create, review, merge and inspect pull requests. Commands that take a pull request default to the open pull request of the current branch."`, and add to `cmd.AddCommand(...)`:

```go
		review.NewCmdApprove(f, nil),
		review.NewCmdUnapprove(f, nil),
		review.NewCmdRequestChanges(f, nil),
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/cmd/pr/... && go vet ./pkg/cmd/pr/... && go build -o bin/khbb ./cmd/khbb && ./bin/khbb pr approve --help`
Expected: PASS; the help shows `--dry-run` and `-R, --repo` (inherited).

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr/review/ pkg/cmd/pr/pr.go
git commit -m "feat(pr): approve, unapprove and request-changes"
```

---

### Task 7: `khbb pr comment`

**Files:**
- Create: `pkg/cmd/pr/comment/comment.go`, `pkg/cmd/pr/comment/comment_test.go`
- Modify: `pkg/cmd/pr/pr.go`

**Interfaces:**
- Consumes: `shared.Finder`, `shared.CheckRepoSelector`, `shared.ReadBody`, `shared.NewComment`, `shared.CommentFields`; `Client.CreatePRComment` (Task 2), `Client.PRDiffStat`; `cmdutil.AddJSONFlags`, `cmdutil.AddDryRunFlag`; `prtest` helpers; go-gh `prompter.NewMock`.
- Produces: `comment.CommentOptions`, `comment.NewCmdComment(f, runF)`. Prints the new comment's URL on stdout (or the Comment JSON with `--json`). Flags: `-b/--body`, `-F/--body-file`, `--file`, `--line`, `--reply-to`, `--dry-run`, `--json/-q/-t`.

Validation order: flag combinations (no network) → body (flags, else a one-line prompt on a terminal) → pull request lookup → `--file` checked against the diffstat (new or old path) → POST.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pr/comment/comment_test.go`:

```go
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
```

Note: the blank-body case (`-b " "`) is rejected before any request, so `httpmock.New(t)` without stubs is enough for every row.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/comment/`
Expected: FAIL — `undefined: NewCmdComment`.

- [ ] **Step 3: Implement**

`pkg/cmd/pr/comment/comment.go`:

```go
// Package comment implements `khbb pr comment`.
package comment

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// CommentOptions holds the inputs and dependencies of `khbb pr comment`.
type CommentOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Prompter   cmdutil.Prompter
	Exporter   cmdutil.Exporter

	Selector string
	Body     string
	BodySet  bool
	BodyFile string
	File     string
	Line     int
	ReplyTo  int
}

// NewCmdComment returns `khbb pr comment`.
func NewCmdComment(f *cmdutil.Factory, runF func(*CommentOptions) error) *cobra.Command {
	opts := &CommentOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Prompter: f.Prompter}
	cmd := &cobra.Command{
		Use:   "comment [<number> | <url>]",
		Short: "Comment on a pull request",
		Long: `Add a comment to a pull request: a general comment, a comment on a file (--file), on a line
of the new version of a file (--file with --line), or a reply to another comment (--reply-to).

Without an argument, comment on the open pull request of the current branch. Without --body or
--body-file, the comment is asked for on a terminal.`,
		Example: `  $ khbb pr comment 42 --body "Looks good"
  $ khbb pr comment 42 --file src/widget.go --line 12 --body "Rename this"
  $ khbb pr comment 42 --reply-to 101 --body-file reply.md`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			fl := cmd.Flags()
			opts.BodySet = fl.Changed("body")
			switch {
			case fl.Changed("line") && opts.File == "":
				return cmdutil.FlagErrorf("--line requires --file")
			case fl.Changed("line") && opts.Line < 1:
				return cmdutil.FlagErrorf("invalid --line %d: must be at least 1", opts.Line)
			case fl.Changed("reply-to") && opts.ReplyTo < 1:
				return cmdutil.FlagErrorf("invalid --reply-to %d: expected a comment ID", opts.ReplyTo)
			case opts.ReplyTo > 0 && opts.File != "":
				return cmdutil.FlagErrorf("--reply-to cannot be combined with --file or --line: a reply stays with its comment")
			}
			if runF != nil {
				return runF(opts)
			}
			return commentRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.Body, "body", "b", "", "The comment `text`")
	fl.StringVarP(&opts.BodyFile, "body-file", "F", "", "Read the comment from `file` (use \"-\" to read from standard input)")
	fl.StringVar(&opts.File, "file", "", "Comment on this `path` of the pull request")
	fl.IntVar(&opts.Line, "line", 0, "Comment on this `line` of the new version of --file")
	fl.IntVar(&opts.ReplyTo, "reply-to", 0, "Reply to the comment with this `id`")
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.CommentFields)
	return cmd
}

func commentRun(ctx context.Context, opts *CommentOptions) error {
	body, err := commentBody(opts)
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if opts.File != "" {
		if err := checkFile(ctx, client, repo, pr.ID, opts.File); err != nil {
			return err
		}
	}
	c, err := client.CreatePRComment(ctx, repo.Workspace, repo.Slug, pr.ID, bitbucket.PRCommentInput{
		Body: body, Path: opts.File, Line: opts.Line, ParentID: opts.ReplyTo,
	})
	if err != nil {
		return err
	}
	comment := shared.NewComment(c)
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, comment)
	}
	fmt.Fprintln(opts.IO.Out, comment.URL)
	return nil
}

// commentBody reads the comment from the flags, or asks for it on a terminal.
func commentBody(opts *CommentOptions) (string, error) {
	body, provided, err := shared.ReadBody(opts.IO, opts.Body, opts.BodySet, opts.BodyFile)
	if err != nil {
		return "", err
	}
	if !provided {
		if !opts.IO.CanPrompt() || opts.Prompter == nil {
			return "", cmdutil.FlagErrorf("--body or --body-file required when not running interactively")
		}
		if body, err = opts.Prompter.Input("Comment", ""); err != nil {
			return "", err
		}
	}
	if strings.TrimSpace(body) == "" {
		return "", cmdutil.FlagErrorf("the comment cannot be empty")
	}
	return body, nil
}

// checkFile makes sure path is part of the pull request: Bitbucket accepts comments on any path.
func checkFile(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, id int, path string) error {
	stats, err := client.PRDiffStat(ctx, repo.Workspace, repo.Slug, id)
	if err != nil {
		return err
	}
	for _, s := range stats {
		if (s.New != nil && s.New.Path == path) || (s.Old != nil && s.Old.Path == path) {
			return nil
		}
	}
	return &cmdutil.NotFoundError{Msg: fmt.Sprintf("%s is not changed in pull request #%d", path, id)}
}
```

In `pkg/cmd/pr/pr.go`, import `"github.com/khipu/khbb/pkg/cmd/pr/comment"` and add `comment.NewCmdComment(f, nil),` to `cmd.AddCommand(...)`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/cmd/pr/... && go vet ./pkg/cmd/pr/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr/comment/ pkg/cmd/pr/pr.go
git commit -m "feat(pr): pr comment with file, line and reply comments"
```

---
### Task 8: `khbb pr create`

**Files:**
- Create: `pkg/cmd/pr/create/create.go`, `pkg/cmd/pr/create/create_test.go`
- Modify: `pkg/cmd/pr/pr.go`

**Interfaces:**
- Consumes: `Client.RepositoryMainBranch`, `Client.EffectiveDefaultReviewers`, `Client.CreatePullRequest`, `bitbucket.PRCreate` (Task 2); `Client.ListPullRequests`, `Client.CurrentUser`, `bitbucket.QuoteBBQL`; `shared.ReviewerResolver`, `shared.UniqueUUIDs` (Task 5); `shared.ReadBody`, `cmdutil.ConflictError` (Task 4); `shared.NewPullRequest`, `shared.PullRequestFields`; `prtest` helpers.
- Produces: `create.CreateOptions`, `create.NewCmdCreate(f, runF)`, `createRun(ctx, opts)`, `titleAndBody(opts)` (replaced in Task 9). Prints the new pull request's URL on stdout (PullRequest JSON with `--json`). On a terminal, stderr gets `Creating pull request for <head> into <base> in <ws>/<repo>` and a blank line.

Request order (tests rely on it only through `reg.Calls[len-1]` being the POST): main branch (only without `-B`) → duplicate check (`GET …/pullrequests?q=…`) → member lookups for `-r` → default reviewers (unless `--no-default-reviewers`) → `GET /user` (only when there is at least one reviewer, to drop the author) → `POST …/pullrequests`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pr/create/create_test.go`:

```go
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
		{},                                 // no title without a terminal
		{"-t", "x"},                        // no body without a terminal
		{"-t", "x", "-b", "y", "-F", "z"},  // two bodies
		{"-t", " ", "-b", "y", "-B", "main"},   // blank title
		{"-H", "main", "-B", "main", "-t", "x", "-b", "y"}, // head is base
		{"extra", "-t", "x", "-b", "y"},    // no positional arguments
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
```

Add this helper to `pkg/cmd/pr/shared/prtest/prtest.go` (next to `AssertJSONBody`):

```go
// JSONBodyOf decodes the JSON object body of a recorded request.
func JSONBodyOf(t testing.TB, c httpmock.Call) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(c.Body, &m); err != nil {
		t.Fatalf("request body %q is not a JSON object: %v", c.Body, err)
	}
	return m
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/create/`
Expected: FAIL — `undefined: NewCmdCreate`.

- [ ] **Step 3: Implement**

`pkg/cmd/pr/create/create.go`:

```go
// Package create implements `khbb pr create`.
package create

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// CreateOptions holds the inputs and dependencies of `khbb pr create`.
type CreateOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Prompter   cmdutil.Prompter
	Exporter   cmdutil.Exporter

	Title              string
	Body               string
	BodySet            bool
	BodyFile           string
	Base               string
	Head               string
	Reviewers          []string
	Draft              bool
	NoDefaultReviewers bool
	DeleteBranch       bool
}

// NewCmdCreate returns `khbb pr create`.
func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Prompter: f.Prompter}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		Long: `Create a pull request from a branch that is already on Bitbucket. khbb never pushes:
run git push first.

The head branch defaults to the current branch and the base branch to the repository's main
branch. The repository's default reviewers are added unless --no-default-reviewers is given, and
you are never made a reviewer of your own pull request. --reviewer accepts a nickname, display
name, account ID, {uuid} or @me, matched against the members of the workspace.

On a terminal, a missing title or body is asked for. Otherwise --title and --body (or
--body-file) are required.`,
		Example: `  $ khbb pr create --title "Add widgets" --body "Adds the widget factory."
  $ khbb pr create -t "Fix gears" -b "" -r bob -r "Cy Example" --draft
  $ khbb pr create -R acme/widgets -H feature/widgets -B develop -t "Add widgets" -F body.md`,
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fl := cmd.Flags()
			opts.BodySet = fl.Changed("body")
			switch {
			case fl.Changed("repo") && opts.Head == "":
				return cmdutil.FlagErrorf("--head required when using the --repo flag")
			case opts.BodySet && opts.BodyFile != "":
				return cmdutil.FlagErrorf("specify only one of --body and --body-file")
			case opts.Head != "" && opts.Head == opts.Base:
				return cmdutil.FlagErrorf("the head and base branches are both %q", opts.Head)
			}
			if !opts.IO.CanPrompt() {
				if opts.Title == "" {
					return cmdutil.FlagErrorf("--title required when not running interactively")
				}
				if !opts.BodySet && opts.BodyFile == "" {
					return cmdutil.FlagErrorf("--body or --body-file required when not running interactively")
				}
			}
			if runF != nil {
				return runF(opts)
			}
			return createRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.Title, "title", "t", "", "Title of the pull request")
	fl.StringVarP(&opts.Body, "body", "b", "", "Description of the pull request")
	fl.StringVarP(&opts.BodyFile, "body-file", "F", "", "Read the description from `file` (use \"-\" to read from standard input)")
	fl.StringVarP(&opts.Base, "base", "B", "", "The `branch` to merge into (default: the repository's main branch)")
	fl.StringVarP(&opts.Head, "head", "H", "", "The `branch` that contains the changes (default: the current branch)")
	fl.StringSliceVarP(&opts.Reviewers, "reviewer", "r", nil, "Request a review from these `users`")
	fl.BoolVar(&opts.Draft, "draft", false, "Create a draft pull request")
	fl.BoolVar(&opts.NoDefaultReviewers, "no-default-reviewers", false, "Do not add the repository's default reviewers")
	fl.BoolVarP(&opts.DeleteBranch, "delete-branch", "d", false, "Close the head branch when the pull request is merged on Bitbucket")
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PullRequestFields)
	return cmd
}

func createRun(ctx context.Context, opts *CreateOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	head := opts.Head
	if head == "" {
		if head, err = opts.Branch(); err != nil {
			return fmt.Errorf("%w; name the branch with --head", err)
		}
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	base := opts.Base
	if base == "" {
		if base, err = client.RepositoryMainBranch(ctx, repo.Workspace, repo.Slug); err != nil {
			return err
		}
	}
	if head == base {
		return cmdutil.FlagErrorf("the head and base branches are both %q", head)
	}
	title, body, err := titleAndBody(opts)
	if err != nil {
		return err
	}
	if err := checkNoOpenPR(ctx, client, repo, head, base); err != nil {
		return err
	}
	reviewers, err := reviewerUUIDs(ctx, client, repo, opts)
	if err != nil {
		return err
	}
	if opts.IO.IsStderrTTY() {
		fmt.Fprintf(opts.IO.ErrOut, "Creating pull request for %s into %s in %s\n\n", head, base, repo.FullName())
	}
	pr, err := client.CreatePullRequest(ctx, repo.Workspace, repo.Slug, bitbucket.PRCreate{
		Title: title, Description: body, Source: head, Destination: base,
		Draft: opts.Draft, CloseSourceBranch: opts.DeleteBranch, Reviewers: reviewers,
	})
	if err != nil {
		return explainCreateError(err, head)
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, shared.NewPullRequest(pr))
	}
	fmt.Fprintln(opts.IO.Out, pr.Links.HTML.Href)
	return nil
}

// titleAndBody returns the title and body from the flags and asks on a terminal for what is missing.
func titleAndBody(opts *CreateOptions) (string, string, error) {
	body, bodyGiven, err := shared.ReadBody(opts.IO, opts.Body, opts.BodySet, opts.BodyFile)
	if err != nil {
		return "", "", err
	}
	title := opts.Title
	if title == "" || !bodyGiven {
		if !opts.IO.CanPrompt() || opts.Prompter == nil {
			return "", "", cmdutil.FlagErrorf("--title and --body required when not running interactively")
		}
		if title == "" {
			if title, err = opts.Prompter.Input("Title", ""); err != nil {
				return "", "", err
			}
		}
		if !bodyGiven {
			if body, err = opts.Prompter.Input("Body", ""); err != nil {
				return "", "", err
			}
		}
	}
	if strings.TrimSpace(title) == "" {
		return "", "", cmdutil.FlagErrorf("the title cannot be empty")
	}
	return title, body, nil
}

// checkNoOpenPR refuses to continue when head already has an open pull request into base:
// Bitbucket would retitle that pull request instead of creating a new one.
func checkNoOpenPR(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, head, base string) error {
	prs, err := client.ListPullRequests(ctx, repo.Workspace, repo.Slug, bitbucket.PRListOptions{
		Query: "source.branch.name = " + bitbucket.QuoteBBQL(head) + " AND destination.branch.name = " + bitbucket.QuoteBBQL(base),
	}, 1)
	if err != nil {
		return err
	}
	if len(prs) > 0 {
		return &cmdutil.ConflictError{Msg: fmt.Sprintf("a pull request for %s into %s already exists: %s", head, base, prs[0].Links.HTML.Href)}
	}
	return nil
}

// reviewerUUIDs resolves --reviewer and adds the default reviewers, without duplicates and without
// the author: Bitbucket rejects a pull request whose author is among its reviewers.
func reviewerUUIDs(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, opts *CreateOptions) ([]string, error) {
	resolver := &shared.ReviewerResolver{Client: client, Workspace: repo.Workspace}
	uuids, err := resolver.ResolveAll(ctx, opts.Reviewers)
	if err != nil {
		return nil, err
	}
	if !opts.NoDefaultReviewers {
		defaults, err := client.EffectiveDefaultReviewers(ctx, repo.Workspace, repo.Slug)
		if err != nil {
			return nil, err
		}
		for _, u := range defaults {
			uuids = append(uuids, u.UUID)
		}
	}
	if len(uuids) == 0 {
		return []string{}, nil
	}
	me, err := client.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	return shared.UniqueUUIDs(uuids, me.UUID), nil
}

// explainCreateError turns Bitbucket's "branch not found" for the source into advice: khbb never pushes.
func explainCreateError(err error, head string) error {
	var httpErr *bitbucket.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusBadRequest {
		for _, msg := range httpErr.Fields["source"] {
			if strings.Contains(msg, "branch not found") {
				return &cmdutil.NotFoundError{Msg: fmt.Sprintf("branch %q is not on Bitbucket; push it first, for example with `git push -u origin %s`", head, head)}
			}
		}
	}
	return err
}
```

Note: the `-t " "` row of `TestCreate_FlagErrors` passes `-B main`, so it fails in `titleAndBody` before any request; the `head is base` row fails in `RunE`.

In `pkg/cmd/pr/pr.go`, import `"github.com/khipu/khbb/pkg/cmd/pr/create"` and add `create.NewCmdCreate(f, nil),` to `cmd.AddCommand(...)`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/cmd/pr/... && go vet ./pkg/cmd/pr/... && go build -o bin/khbb ./cmd/khbb && ./bin/khbb pr create --help`
Expected: PASS; the help lists `-t, --title` and a long-only `--template`.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr/create/ pkg/cmd/pr/pr.go pkg/cmd/pr/shared/prtest/prtest.go
git commit -m "feat(pr): pr create with reviewers, default reviewers and duplicate check"
```

---

### Task 9: `pr create --fill`, `--web` and the unpushed-commits warning

**Files:**
- Modify: `internal/gitctx/resolver.go` (add `RemoteFor`, share the host check with `BaseRepo`)
- Modify: `pkg/cmd/pr/create/create.go`
- Modify: `pkg/cmd/pr/shared/prtest/prtest.go` (add `FakeGit`, `SetGit`)
- Test: `internal/gitctx/resolver_test.go`, `pkg/cmd/pr/create/create_test.go`

**Interfaces:**
- Consumes: Task 8's `create` package; `gitctx.Resolver` (`Git`, `Remotes`, `SSHHostname`, `parseRemoteURL`, `bitbucketHost`); `cmdutil.Browser`.
- Produces:
  - `(*gitctx.Resolver).RemoteFor(repo gitctx.Repo) (string, error)` — the name of a remote pointing at `repo` (case-insensitive, SSH aliases resolved), `origin` first, `""` when none.
  - `prtest.FakeGit{Outputs, Errors map[string]string; Calls []string}` with `Run(args ...string) (string, error)` — answers by the arguments joined with spaces, fails unknown commands; `prtest.SetGit(f, g)` installs it as `f.Git`.
  - `CreateOptions` gains `Browser cmdutil.Browser`, `Git *gitctx.Resolver`, `Fill`, `Web bool`; flags `--fill` and `-w/--web`.
  - `--fill`: `git log --reverse --format=%s%x1f%b%x1e <remote>/<base>..<head>`; one commit → its subject and body; several → the head branch name with `-`/`_` as spaces, and a `- subject` list; `-t`/`-b` still win.
  - `--web`: opens `https://bitbucket.org/<ws>/<repo>/pull-requests/new?` + `url.Values{source, t=1[, dest]}` encoded; no API request.
  - After a successful create, a stderr warning when `refs/heads/<head>` is not the commit Bitbucket used.

- [ ] **Step 1: Write the failing tests**

Append to `internal/gitctx/resolver_test.go`:

```go
func TestRemoteFor(t *testing.T) {
	r := fakeResolver(strings.Join([]string{
		"fork\tgit@bitbucket.org:dev/widgets.git (fetch)",
		"upstream\tgit@bitbucket.org:acme/widgets.git (fetch)",
		"origin\thttps://bitbucket.org/acme/widgets.git (fetch)",
		"mirror\tgit@github.com:acme/widgets.git (fetch)",
	}, "\n"), "", nil)
	cases := map[string]string{"acme/widgets": "origin", "ACME/Widgets": "origin", "dev/widgets": "fork", "acme/other": ""}
	for full, want := range cases {
		repo, _ := gitctx.ParseRepo(full)
		if got, err := r.RemoteFor(repo); err != nil || got != want {
			t.Errorf("RemoteFor(%s) = %q, %v; want %q", full, got, err, want)
		}
	}
}

func TestRemoteFor_SSHAlias(t *testing.T) {
	r := fakeResolver("upstream\tgit@bb-work:acme/widgets.git (fetch)", "", nil)
	r.SSHHostname = func(alias string) (string, error) { return "bitbucket.org", nil }
	if got, err := r.RemoteFor(gitctx.Repo{Workspace: "acme", Slug: "widgets"}); err != nil || got != "upstream" {
		t.Errorf("RemoteFor = %q, %v", got, err)
	}
}
```

Append to `pkg/cmd/pr/create/create_test.go` (add `"net/url"` and `"slices"` to its imports):

```go
const originRemotes = "origin\tgit@bitbucket.org:acme/widgets.git (fetch)\norigin\tgit@bitbucket.org:acme/widgets.git (push)"

func gitWith(outputs map[string]string) *prtest.FakeGit {
	all := map[string]string{
		"remote -v":                        originRemotes,
		"symbolic-ref --quiet --short HEAD": "feature/widgets",
	}
	for k, v := range outputs {
		all[k] = v
	}
	return &prtest.FakeGit{Outputs: all}
}

type fakeBrowser struct{ urls []string }

func (b *fakeBrowser) Browse(u string) error {
	b.urls = append(b.urls, u)
	return nil
}

func TestCreate_FillOneCommit(t *testing.T) {
	reg := httpmock.New(t)
	noOpenPR(reg)
	reg.Register("POST", prtest.PRs, httpmock.JSONResponse(201, prtest.PR42))
	f, _, _, _ := prtest.NewFactory(reg)
	prtest.SetGit(f, gitWith(map[string]string{
		"log --reverse --format=%s%x1f%b%x1e origin/main..feature/widgets": "Add widgets\x1fAdds the widget factory.\n\x1e",
	}))

	if err := prtest.Run(NewCmdCreate(f, nil), "--fill", "-B", "main", "--no-default-reviewers"); err != nil {
		t.Fatal(err)
	}
	body := prtest.JSONBodyOf(t, lastCall(reg))
	if body["title"] != "Add widgets" || body["description"] != "Adds the widget factory." {
		t.Errorf("body = %v", body)
	}
}

func TestCreate_FillSeveralCommits(t *testing.T) {
	reg := httpmock.New(t)
	noOpenPR(reg)
	reg.Register("POST", prtest.PRs, httpmock.JSONResponse(201, prtest.PR42))
	f, _, _, _ := prtest.NewFactory(reg)
	prtest.SetGit(f, gitWith(map[string]string{
		"log --reverse --format=%s%x1f%b%x1e origin/main..feature/more_widgets-v2": "Add gears\x1f\x1e\nAdd widgets\x1fBody\n\x1e",
	}))

	if err := prtest.Run(NewCmdCreate(f, nil), "--fill", "-H", "feature/more_widgets-v2", "-B", "main", "--no-default-reviewers"); err != nil {
		t.Fatal(err)
	}
	body := prtest.JSONBodyOf(t, lastCall(reg))
	if body["title"] != "feature/more widgets v2" || body["description"] != "- Add gears\n- Add widgets" {
		t.Errorf("body = %v", body)
	}
}

func TestCreate_FillFlagsWin(t *testing.T) {
	reg := httpmock.New(t)
	noOpenPR(reg)
	reg.Register("POST", prtest.PRs, httpmock.JSONResponse(201, prtest.PR42))
	f, _, _, _ := prtest.NewFactory(reg)
	prtest.SetGit(f, gitWith(map[string]string{
		"log --reverse --format=%s%x1f%b%x1e origin/main..feature/widgets": "Add widgets\x1fFrom the commit\x1e",
	}))

	if err := prtest.Run(NewCmdCreate(f, nil), "--fill", "-t", "Custom", "-B", "main", "--no-default-reviewers"); err != nil {
		t.Fatal(err)
	}
	body := prtest.JSONBodyOf(t, lastCall(reg))
	if body["title"] != "Custom" || body["description"] != "From the commit" {
		t.Errorf("body = %v", body)
	}
}

func TestCreate_FillErrors(t *testing.T) {
	cases := map[string]map[string]string{
		"no commits": {"log --reverse --format=%s%x1f%b%x1e origin/main..feature/widgets": ""},
		"no remote":  {"remote -v": "origin\tgit@github.com:acme/widgets.git (fetch)"},
	}
	for name, outputs := range cases {
		reg := httpmock.New(t)
		f, _, _, _ := prtest.NewFactory(reg)
		prtest.SetGit(f, gitWith(outputs))
		err := prtest.Run(NewCmdCreate(f, nil), "--fill", "-B", "main", "--no-default-reviewers")
		if err == nil || !strings.Contains(err.Error(), "--fill") || len(prtest.Writes(reg)) != 0 {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestCreate_Web(t *testing.T) {
	reg := httpmock.New(t)
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")
	b := &fakeBrowser{}
	f.Browser = b

	if err := prtest.Run(NewCmdCreate(f, nil), "--web", "-B", "develop"); err != nil {
		t.Fatal(err)
	}
	want := "https://bitbucket.org/acme/widgets/pull-requests/new?" + url.Values{"source": {"feature/widgets"}, "dest": {"develop"}, "t": {"1"}}.Encode()
	if !slices.Equal(b.urls, []string{want}) || out.Len() != 0 || len(reg.Calls) != 0 {
		t.Errorf("urls %v out %q calls %d", b.urls, out.String(), len(reg.Calls))
	}
}

func TestCreate_WebConflicts(t *testing.T) {
	for _, args := range [][]string{{"--web", "--json", "id"}, {"--web", "--dry-run"}} {
		f, _, _, _ := prtest.NewFactory(httpmock.New(t))
		f.Browser = &fakeBrowser{}
		err := prtest.Run(NewCmdCreate(f, nil), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

func TestCreate_WarnsWhenLocalBranchIsAhead(t *testing.T) {
	for _, tc := range []struct{ local, warning string }{
		{"9999999aaaabbbbccccdddd", "warning: the pull request uses commit abc1234, but your local branch feature/widgets is at 9999999; push your latest commits\n"},
		{"abc1234aaaabbbbccccdddd", ""},
	} {
		reg := httpmock.New(t)
		noOpenPR(reg)
		reg.Register("POST", prtest.PRs, httpmock.JSONResponse(201, prtest.PR42))
		f, _, _, errOut := prtest.NewFactory(reg)
		prtest.SetGit(f, gitWith(map[string]string{"rev-parse --verify --quiet refs/heads/feature/widgets": tc.local}))

		if err := prtest.Run(NewCmdCreate(f, nil), "-B", "main", "-t", "x", "-b", "", "--no-default-reviewers"); err != nil {
			t.Fatal(err)
		}
		if errOut.String() != tc.warning {
			t.Errorf("local %s: stderr = %q", tc.local, errOut.String())
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/gitctx/ ./pkg/cmd/pr/create/`
Expected: FAIL — `r.RemoteFor undefined`, `undefined: prtest.FakeGit`.

- [ ] **Step 3: Implement**

In `internal/gitctx/resolver.go`, replace `BaseRepo` with the following and add `RemoteFor` and `bitbucketRepo`:

```go
// BaseRepo returns the Bitbucket repository of the working copy, preferring the "origin" remote.
func (r *Resolver) BaseRepo() (Repo, error) {
	remotes, err := r.Remotes()
	if err != nil {
		return Repo{}, fmt.Errorf("%w (%v)", ErrNoRepo, err)
	}
	var found []Repo
	for _, rem := range remotes {
		repo, ok := r.bitbucketRepo(rem.URL)
		if !ok {
			continue
		}
		if rem.Name == "origin" {
			return repo, nil
		}
		found = append(found, repo)
	}
	if len(found) > 0 {
		return found[0], nil
	}
	return Repo{}, ErrNoRepo
}

// RemoteFor returns the name of a remote that points at repo, preferring "origin", or "" when no
// remote does.
func (r *Resolver) RemoteFor(repo Repo) (string, error) {
	remotes, err := r.Remotes()
	if err != nil {
		return "", err
	}
	name := ""
	for _, rem := range remotes {
		got, ok := r.bitbucketRepo(rem.URL)
		if !ok || !strings.EqualFold(got.FullName(), repo.FullName()) {
			continue
		}
		if rem.Name == "origin" {
			return rem.Name, nil
		}
		if name == "" {
			name = rem.Name
		}
	}
	return name, nil
}

// bitbucketRepo returns the repository a remote URL points at; ok is false for hosts other than
// bitbucket.org, after resolving SSH host aliases.
func (r *Resolver) bitbucketRepo(rawURL string) (Repo, bool) {
	host, repo, isSSH, err := parseRemoteURL(rawURL)
	if err != nil {
		return Repo{}, false
	}
	if host != bitbucketHost && isSSH && r.SSHHostname != nil {
		if real, err := r.SSHHostname(host); err == nil {
			host = real
		}
	}
	return repo, host == bitbucketHost
}
```

In `pkg/cmd/pr/shared/prtest/prtest.go`, add `"fmt"` to the imports and append:

```go
// FakeGit stands in for git. It answers a command from Outputs, or fails it with the message in
// Errors, keyed by the arguments joined with spaces; it records every call and fails any command
// it does not know.
type FakeGit struct {
	Outputs map[string]string
	Errors  map[string]string
	Calls   []string
}

// Run implements gitctx.Resolver.Git.
func (g *FakeGit) Run(args ...string) (string, error) {
	key := strings.Join(args, " ")
	g.Calls = append(g.Calls, key)
	if msg, ok := g.Errors[key]; ok {
		return "", errors.New(msg)
	}
	if out, ok := g.Outputs[key]; ok {
		return out, nil
	}
	return "", fmt.Errorf("fake git: unexpected command: git %s", key)
}

// SetGit makes f run every git command through g.
func SetGit(f *cmdutil.Factory, g *FakeGit) {
	f.Git = &gitctx.Resolver{Git: g.Run}
}
```

In `pkg/cmd/pr/create/create.go`:

1. Add `"net/url"` to the imports.
2. Add to `CreateOptions`, after `Exporter`:

```go
	Browser    cmdutil.Browser
	Git        *gitctx.Resolver
```

and after `DeleteBranch`:

```go
	Fill bool
	Web  bool
```

3. In `NewCmdCreate`, set `Browser: f.Browser, Git: f.Git` in the `opts` literal; register

```go
	fl.BoolVar(&opts.Fill, "fill", false, "Use commit messages for the title and description")
	fl.BoolVarP(&opts.Web, "web", "w", false, "Open the web page to create a pull request instead")
```

before `cmdutil.AddDryRunFlag(cmd, f)`; in `RunE`, add these cases at the top of the `switch`:

```go
			case opts.Web && opts.Exporter != nil:
				return cmdutil.FlagErrorf("--web cannot be combined with --json")
			case opts.Web && f.DryRun:
				return cmdutil.FlagErrorf("--web cannot be combined with --dry-run")
```

and change `if !opts.IO.CanPrompt() {` to `if !opts.IO.CanPrompt() && !opts.Fill && !opts.Web {`. Add to the `Long` help: ``--fill takes the title and description from the commits that are on the head branch but not on the base branch (one commit: its message; several: the branch name and a list of subjects); --title and --body still win. --web opens Bitbucket's page for creating a pull request instead.``

4. In `createRun`, right after the `head` block and before `client, err := opts.HTTPClient()`, add:

```go
	if opts.Web {
		return openWeb(opts, repo, head)
	}
```

5. Change the call `title, body, err := titleAndBody(opts)` to `title, body, err := titleAndBody(opts, repo, head, base)`, and after the `explainCreateError` block (before the exporter) add `warnUnpushed(opts, head, pr)`.

6. Replace `titleAndBody` and add the new helpers:

```go
// titleAndBody returns the title and body from the flags, then from --fill, and asks on a terminal
// for what is still missing.
func titleAndBody(opts *CreateOptions, repo gitctx.Repo, head, base string) (string, string, error) {
	body, bodyGiven, err := shared.ReadBody(opts.IO, opts.Body, opts.BodySet, opts.BodyFile)
	if err != nil {
		return "", "", err
	}
	title := opts.Title
	if opts.Fill && (title == "" || !bodyGiven) {
		fillTitle, fillBody, err := commitSummary(opts.Git, repo, head, base)
		if err != nil {
			return "", "", err
		}
		if title == "" {
			title = fillTitle
		}
		if !bodyGiven {
			body, bodyGiven = fillBody, true
		}
	}
	if title == "" || !bodyGiven {
		if !opts.IO.CanPrompt() || opts.Prompter == nil {
			return "", "", cmdutil.FlagErrorf("--title and --body (or --fill) required when not running interactively")
		}
		if title == "" {
			if title, err = opts.Prompter.Input("Title", ""); err != nil {
				return "", "", err
			}
		}
		if !bodyGiven {
			if body, err = opts.Prompter.Input("Body", ""); err != nil {
				return "", "", err
			}
		}
	}
	if strings.TrimSpace(title) == "" {
		return "", "", cmdutil.FlagErrorf("the title cannot be empty")
	}
	return title, body, nil
}

// commitSummary derives a title and body from the commits on head that are not on base, as gh's
// --fill does: one commit gives its subject and body; several give the branch name and a list.
func commitSummary(git *gitctx.Resolver, repo gitctx.Repo, head, base string) (string, string, error) {
	if git == nil {
		return "", "", errors.New("--fill needs git")
	}
	remote, err := git.RemoteFor(repo)
	if err != nil {
		return "", "", fmt.Errorf("--fill: %w", err)
	}
	if remote == "" {
		return "", "", fmt.Errorf("--fill needs a git remote for %s", repo.FullName())
	}
	out, err := git.Git("log", "--reverse", "--format=%s%x1f%b%x1e", remote+"/"+base+".."+head)
	if err != nil {
		return "", "", fmt.Errorf("--fill: %w (is %s/%s fetched?)", err, remote, base)
	}
	type commit struct{ subject, body string }
	var commits []commit
	for _, rec := range strings.Split(out, "\x1e") {
		if rec = strings.TrimSpace(rec); rec == "" {
			continue
		}
		subject, body, _ := strings.Cut(rec, "\x1f")
		commits = append(commits, commit{strings.TrimSpace(subject), strings.TrimSpace(body)})
	}
	switch len(commits) {
	case 0:
		return "", "", fmt.Errorf("--fill: no commits on %s that are not on %s/%s", head, remote, base)
	case 1:
		return commits[0].subject, commits[0].body, nil
	}
	lines := make([]string, len(commits))
	for i, c := range commits {
		lines[i] = "- " + c.subject
	}
	return humanize(head), strings.Join(lines, "\n"), nil
}

// humanize turns a branch name into a title: dashes and underscores become spaces.
func humanize(branch string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' {
			return ' '
		}
		return r
	}, branch)
}

// openWeb opens Bitbucket's page for creating a pull request from head. source and t=1 form the URL
// Bitbucket prints after a push; dest is only sent when --base names a branch.
func openWeb(opts *CreateOptions, repo gitctx.Repo, head string) error {
	q := url.Values{"source": {head}, "t": {"1"}}
	if opts.Base != "" {
		q.Set("dest", opts.Base)
	}
	u := fmt.Sprintf("https://bitbucket.org/%s/%s/pull-requests/new?%s", repo.Workspace, repo.Slug, q.Encode())
	if opts.IO.IsStderrTTY() {
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", u)
	}
	return opts.Browser.Browse(u)
}

// warnUnpushed warns when the pull request does not point at the local head commit, which usually
// means commits were not pushed. It stays quiet whenever git cannot tell.
func warnUnpushed(opts *CreateOptions, head string, pr *bitbucket.PullRequest) {
	if opts.Git == nil || pr.Source.Commit == nil || pr.Source.Commit.Hash == "" {
		return
	}
	local, err := opts.Git.Git("rev-parse", "--verify", "--quiet", "refs/heads/"+head)
	if err != nil || local == "" {
		return
	}
	remote := pr.Source.Commit.Hash
	if strings.HasPrefix(local, remote) || strings.HasPrefix(remote, local) {
		return
	}
	fmt.Fprintf(opts.IO.ErrOut, "warning: the pull request uses commit %s, but your local branch %s is at %s; push your latest commits\n",
		shortHash(remote), head, shortHash(local))
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}
```

Note: Task 8's tests use `prtest.SetBranch`, whose fake git fails `rev-parse`, so `warnUnpushed` stays silent there.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/gitctx/ ./pkg/cmd/pr/... && go vet ./...`
Expected: PASS, including the existing `TestBaseRepo_*` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/gitctx/resolver.go internal/gitctx/resolver_test.go pkg/cmd/pr/create/ pkg/cmd/pr/shared/prtest/prtest.go
git commit -m "feat(pr): pr create --fill and --web; warn about unpushed commits"
```

---
### Task 10: `khbb pr edit`

**Files:**
- Create: `pkg/cmd/pr/edit/edit.go`, `pkg/cmd/pr/edit/edit_test.go`
- Modify: `pkg/cmd/pr/pr.go`

**Interfaces:**
- Consumes: `shared.Finder`, `shared.CheckRepoSelector`, `shared.RequireOpen`, `shared.ReadBody` (Task 4); `shared.ReviewerResolver`, `shared.UniqueUUIDs` (Task 5); `Client.UpdatePullRequest`, `bitbucket.PRUpdate` (Task 2); `shared.NewPullRequest`, `shared.PullRequestFields`; `prtest` helpers.
- Produces: `edit.EditOptions`, `edit.NewCmdEdit(f, runF)`. Flags: `-t/--title`, `-b/--body`, `-F/--body-file`, `-B/--base`, `--add-reviewer`, `--remove-reviewer` (both repeatable, comma-separated allowed), `--draft`, `--ready`, `--dry-run`, `--json`. Sends only the changed fields; reviewer changes send the full new list (current reviewers + added − removed − author). Prints the pull request URL on stdout.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pr/edit/edit_test.go`:

```go
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
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/edit/`
Expected: FAIL — `undefined: NewCmdEdit`.

- [ ] **Step 3: Implement**

`pkg/cmd/pr/edit/edit.go`:

```go
// Package edit implements `khbb pr edit`.
package edit

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// EditOptions holds the inputs and dependencies of `khbb pr edit`.
type EditOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Exporter   cmdutil.Exporter

	Selector        string
	Title           string
	TitleSet        bool
	Body            string
	BodySet         bool
	BodyFile        string
	Base            string
	AddReviewers    []string
	RemoveReviewers []string
	Draft           bool
	Ready           bool
}

// NewCmdEdit returns `khbb pr edit`.
func NewCmdEdit(f *cmdutil.Factory, runF func(*EditOptions) error) *cobra.Command {
	opts := &EditOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch}
	cmd := &cobra.Command{
		Use:   "edit [<number> | <url>]",
		Short: "Edit a pull request",
		Long: `Change the title, description, base branch, reviewers or draft state of an open pull request.
Only the fields you name are changed. Without an argument, edit the open pull request of the
current branch. Reviewers accept the same forms as in pr create; you are never added as a
reviewer of your own pull request.`,
		Example: `  $ khbb pr edit 42 --title "Add widgets and gears"
  $ khbb pr edit 42 --add-reviewer bob --remove-reviewer cy
  $ khbb pr edit --ready`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			fl := cmd.Flags()
			opts.TitleSet, opts.BodySet = fl.Changed("title"), fl.Changed("body")
			changed := opts.Draft || opts.Ready
			for _, name := range []string{"title", "body", "body-file", "base", "add-reviewer", "remove-reviewer"} {
				changed = changed || fl.Changed(name)
			}
			switch {
			case !changed:
				return cmdutil.FlagErrorf("specify at least one change: --title, --body, --body-file, --base, --add-reviewer, --remove-reviewer, --draft or --ready")
			case opts.Draft && opts.Ready:
				return cmdutil.FlagErrorf("--draft and --ready cannot be used together")
			case opts.TitleSet && strings.TrimSpace(opts.Title) == "":
				return cmdutil.FlagErrorf("the title cannot be empty")
			case opts.BodySet && opts.BodyFile != "":
				return cmdutil.FlagErrorf("specify only one of --body and --body-file")
			case fl.Changed("base") && strings.TrimSpace(opts.Base) == "":
				return cmdutil.FlagErrorf("--base cannot be empty")
			}
			if runF != nil {
				return runF(opts)
			}
			return editRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.Title, "title", "t", "", "Set the title")
	fl.StringVarP(&opts.Body, "body", "b", "", "Set the description")
	fl.StringVarP(&opts.BodyFile, "body-file", "F", "", "Read the description from `file` (use \"-\" to read from standard input)")
	fl.StringVarP(&opts.Base, "base", "B", "", "Change the `branch` to merge into")
	fl.StringSliceVar(&opts.AddReviewers, "add-reviewer", nil, "Add these `users` as reviewers")
	fl.StringSliceVar(&opts.RemoveReviewers, "remove-reviewer", nil, "Remove these `users` from the reviewers")
	fl.BoolVar(&opts.Draft, "draft", false, "Mark the pull request as a draft")
	fl.BoolVar(&opts.Ready, "ready", false, "Mark the pull request as ready for review")
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PullRequestFields)
	return cmd
}

func editRun(ctx context.Context, opts *EditOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if err := shared.RequireOpen(pr, "edited"); err != nil {
		return err
	}
	var upd bitbucket.PRUpdate
	if opts.TitleSet {
		upd.Title = &opts.Title
	}
	body, bodyGiven, err := shared.ReadBody(opts.IO, opts.Body, opts.BodySet, opts.BodyFile)
	if err != nil {
		return err
	}
	if bodyGiven {
		upd.Description = &body
	}
	if opts.Base != "" {
		upd.Destination = &opts.Base
	}
	if opts.Draft || opts.Ready {
		draft := opts.Draft
		upd.Draft = &draft
	}
	if len(opts.AddReviewers) > 0 || len(opts.RemoveReviewers) > 0 {
		if upd.Reviewers, err = editedReviewers(ctx, client, repo, pr, opts); err != nil {
			return err
		}
	}
	updated, err := client.UpdatePullRequest(ctx, repo.Workspace, repo.Slug, pr.ID, upd)
	if err != nil {
		return err
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, shared.NewPullRequest(updated))
	}
	fmt.Fprintln(opts.IO.Out, updated.Links.HTML.Href)
	return nil
}

// editedReviewers returns the pull request's reviewers with --add-reviewer and --remove-reviewer
// applied. The author is never added: Bitbucket rejects that.
func editedReviewers(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, pr *bitbucket.PullRequest, opts *EditOptions) ([]string, error) {
	resolver := &shared.ReviewerResolver{Client: client, Workspace: repo.Workspace}
	add, err := resolver.ResolveAll(ctx, opts.AddReviewers)
	if err != nil {
		return nil, err
	}
	remove, err := resolver.ResolveAll(ctx, opts.RemoveReviewers)
	if err != nil {
		return nil, err
	}
	uuids := make([]string, 0, len(pr.Reviewers)+len(add))
	for _, r := range pr.Reviewers {
		uuids = append(uuids, r.UUID)
	}
	uuids = append(uuids, add...)
	if pr.Author != nil {
		remove = append(remove, pr.Author.UUID)
	}
	return shared.UniqueUUIDs(uuids, remove...), nil
}
```

In `pkg/cmd/pr/pr.go`, import `"github.com/khipu/khbb/pkg/cmd/pr/edit"` and add `edit.NewCmdEdit(f, nil),` to `cmd.AddCommand(...)`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/cmd/pr/... && go vet ./pkg/cmd/pr/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr/edit/ pkg/cmd/pr/pr.go
git commit -m "feat(pr): pr edit with partial updates and reviewer changes"
```

---

### Task 11: `khbb pr merge`

**Files:**
- Create: `pkg/cmd/pr/merge/merge.go`, `pkg/cmd/pr/merge/merge_test.go`
- Modify: `pkg/cmd/pr/pr.go`

**Interfaces:**
- Consumes: `Client.PRMergeStrategies`, `PRMergeabilityChecks`, `StartMerge`, `MergeTaskStatus`, `bitbucket.PRMerge`, `bitbucket.IsTransient` (Task 3); `shared.RequireOpen`, `shared.Describe`, `shared.PrintSuccess`, `cmdutil.ConflictError` (Task 4); `cmdutil.ConfirmDestructive`, `cmdutil.AddYesFlag`, `cmdutil.AddDryRunFlag`; `prtest` helpers; `prompter.NewMock`.
- Produces: `merge.MergeOptions` (with injectable `Sleep func(time.Duration)` and `Now func() time.Time`), `merge.NewCmdMerge(f, runF)`, `mergeRun(ctx, opts)`. Flags: `--merge` | `--squash` | `--fast-forward`, `-d/--delete-branch`, `-m/--message`, `--yes`, `--dry-run`, `--json`. stdout: nothing, or the merged PullRequest with `--json`.

Flow: find the pull request → require OPEN → strategy (flag or the destination's default; flag must be allowed) → merge checks (any FAILED blocking check → `conflict`) → note when the pull request asks to close its branch but `-d` is absent → confirmation → `POST …/merge?async=true` → poll the task every 2 s for at most 2 minutes, retrying through transient errors → `Merged pull request #42 (feature/widgets → main) with squash[ and deleted branch feature/widgets]` on stderr.

Request order for a pull request without a URL selector: `GET …/42` (find), `GET …/42?fields=destination.branch.*` (strategies), `GET …/42/mergeability/checks`, `POST …/42/merge`, then `GET …/merge/task-status/…`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pr/merge/merge_test.go`:

```go
package merge

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const (
	taskURL    = "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests/42/merge/task-status/t1"
	taskPath   = prtest.PRs + "/42/merge/task-status/t1"
	strategies = `{"destination":{"branch":{"name":"main","merge_strategies":["merge_commit","squash"],"default_merge_strategy":"squash"}}}`
	clean      = `{"size":1,"values":[{"type":"git_mergeability_check","status":"PASSED","required":true,"blocking":false,"reason":"clean"}]}`
	conflicts  = `{"size":1,"values":[{"type":"git_mergeability_check","status":"FAILED","required":true,"blocking":true,"reason":"conflicts"}]}`
	pending    = `{"task_status":"PENDING","links":{}}`
)

var merged = strings.Replace(prtest.WithState(prtest.PR42, "MERGED"), `"merge_commit":null`, `"merge_commit":{"hash":"fff9999"}`, 1)

// prechecks registers the requests made before the merge itself.
func prechecks(reg *httpmock.Registry, checks string) {
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, strategies))
	reg.Register("GET", prtest.PRs+"/42/mergeability/checks", httpmock.JSONResponse(200, checks))
}

func accepted() httpmock.Responder {
	return httpmock.WithHeader(httpmock.JSONResponse(202, `""`), "Location", taskURL)
}

type result struct {
	out, errOut string
	slept       []time.Duration
	err         error
}

func run(t *testing.T, reg *httpmock.Registry, setup func(*cmdutil.Factory), args ...string) result {
	t.Helper()
	f, _, out, errOut := prtest.NewFactory(reg)
	if setup != nil {
		setup(f)
	}
	clock := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	var slept []time.Duration
	cmd := NewCmdMerge(f, func(o *MergeOptions) error {
		o.Now = func() time.Time { return clock }
		o.Sleep = func(d time.Duration) { slept = append(slept, d); clock = clock.Add(d) }
		return mergeRun(context.Background(), o)
	})
	err := prtest.Run(cmd, append([]string{"42"}, args...)...)
	return result{out.String(), errOut.String(), slept, err}
}

func TestMerge_AsyncWithDefaultStrategy(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)
	reg.Register("POST", prtest.PRs+"/42/merge", accepted())
	reg.Register("GET", taskPath, httpmock.JSONResponse(200, pending))
	reg.Register("GET", taskPath, httpmock.JSONResponse(200, `{"task_status":"SUCCESS","merge_result":`+merged+`}`))

	r := run(t, reg, nil, "--yes")
	if r.err != nil {
		t.Fatal(r.err)
	}
	post := reg.Calls[3]
	if post.URL.Query().Get("async") != "true" {
		t.Errorf("merge URL = %s", post.URL)
	}
	prtest.AssertJSONBody(t, post, `{"type":"pullrequest","merge_strategy":"squash","close_source_branch":false}`)
	want := "note: keeping branch feature/widgets; pass --delete-branch to delete it\n" +
		"Merged pull request #42 (feature/widgets → main) with squash\n"
	if r.out != "" || r.errOut != want || !slices.Equal(r.slept, []time.Duration{2 * time.Second}) {
		t.Errorf("out %q stderr %q slept %v", r.out, r.errOut, r.slept)
	}
}

func TestMerge_FinishedAtOnceWithFlagsAndJSON(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)
	reg.Register("POST", prtest.PRs+"/42/merge", httpmock.JSONResponse(200, merged))

	r := run(t, reg, nil, "--merge", "-d", "-m", "Release widgets", "--yes", "--json", "state,mergeCommit")
	if r.err != nil {
		t.Fatal(r.err)
	}
	prtest.AssertJSONBody(t, reg.Calls[3], `{"type":"pullrequest","merge_strategy":"merge_commit","message":"Release widgets","close_source_branch":true}`)
	if r.out != `{"mergeCommit":"fff9999","state":"MERGED"}`+"\n" {
		t.Errorf("out = %q", r.out)
	}
	if r.errOut != "Merged pull request #42 (feature/widgets → main) with a merge commit and deleted branch feature/widgets\n" {
		t.Errorf("stderr = %q", r.errOut)
	}
}

func TestMerge_StrategyNotAllowed(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, strategies))

	r := run(t, reg, nil, "--fast-forward", "--yes")
	var flagErr *cmdutil.FlagError
	if !errors.As(r.err, &flagErr) || r.err.Error() != "merge strategy fast_forward is not allowed into main; allowed: merge_commit, squash" {
		t.Errorf("err = %v", r.err)
	}
}

func TestMerge_BlockedByConflicts(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, conflicts)

	r := run(t, reg, nil, "--yes")
	var conflict *cmdutil.ConflictError
	if !errors.As(r.err, &conflict) || r.err.Error() != "pull request #42 cannot be merged: it has merge conflicts" || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", r.err)
	}
}

func TestMerge_RefusesClosedPullRequest(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.WithState(prtest.PR42, "MERGED")))

	r := run(t, reg, nil, "--yes")
	var conflict *cmdutil.ConflictError
	if !errors.As(r.err, &conflict) || !strings.Contains(r.err.Error(), "is merged") {
		t.Errorf("err = %v", r.err)
	}
}

func TestMerge_NeedsConfirmationWithoutATerminal(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)

	if r := run(t, reg, nil); !errors.Is(r.err, cmdutil.ErrConfirmationRequired) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", r.err)
	}
}

func TestMerge_PromptsOnATerminal(t *testing.T) {
	for _, answer := range []bool{false, true} {
		reg := httpmock.New(t)
		prechecks(reg, clean)
		if answer {
			reg.Register("POST", prtest.PRs+"/42/merge", httpmock.JSONResponse(200, merged))
		}
		r := run(t, reg, func(f *cmdutil.Factory) {
			prtest.SetTTY(f.IOStreams)
			pm := prompter.NewMock(t)
			pm.RegisterConfirm("Merge #42 (feature/widgets → main) with squash and delete the source branch?", func(_ string, def bool) (bool, error) {
				if def {
					t.Error("the default answer must be No")
				}
				return answer, nil
			})
			f.Prompter = pm
		}, "-d")
		if !answer && !errors.Is(r.err, cmdutil.ErrCancel) {
			t.Errorf("declined: err = %v", r.err)
		}
		if answer && r.err != nil {
			t.Errorf("accepted: err = %v", r.err)
		}
	}
}

func TestMerge_DryRun(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)

	r := run(t, reg, nil, "--dry-run")
	if !errors.Is(r.err, bitbucket.ErrDryRun) || !strings.Contains(r.out, `"method": "POST"`) ||
		!strings.Contains(r.out, "/pullrequests/42/merge?async=true") || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err %v out %q", r.err, r.out)
	}
}

func TestMerge_Timeout(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)
	reg.Register("POST", prtest.PRs+"/42/merge", accepted())
	// Polls at 0 s, 2 s, …, 120 s: 61 polls and 60 sleeps before giving up.
	for range 61 {
		reg.Register("GET", taskPath, httpmock.JSONResponse(200, pending))
	}

	r := run(t, reg, nil, "--yes")
	if r.err == nil || r.err.Error() != "the merge of pull request #42 is still running after 2m0s; check "+taskURL || len(r.slept) != 60 {
		t.Errorf("err %v sleeps %d", r.err, len(r.slept))
	}
}

func TestMerge_TransientErrorWhilePolling(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)
	reg.Register("POST", prtest.PRs+"/42/merge", accepted())
	reg.Register("GET", taskPath, httpmock.JSONResponse(503, `{"type":"error","error":{"message":"Service unavailable"}}`))
	reg.Register("GET", taskPath, httpmock.JSONResponse(200, `{"task_status":"SUCCESS","merge_result":`+merged+`}`))

	if r := run(t, reg, nil, "--yes"); r.err != nil || len(r.slept) != 1 {
		t.Errorf("err %v slept %v", r.err, r.slept)
	}
}

func TestMerge_FailedTask(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)
	reg.Register("POST", prtest.PRs+"/42/merge", accepted())
	reg.Register("GET", taskPath, httpmock.JSONResponse(400,
		`{"type":"error","error":{"message":"You can't merge until you resolve all merge conflicts."}}`))

	r := run(t, reg, nil, "--yes")
	var httpErr *bitbucket.HTTPError
	if !errors.As(r.err, &httpErr) || httpErr.StatusCode != 400 {
		t.Errorf("err = %v", r.err)
	}
}

func TestMerge_OneStrategyFlag(t *testing.T) {
	r := run(t, httpmock.New(t), nil, "--merge", "--squash", "--yes")
	var flagErr *cmdutil.FlagError
	if !errors.As(r.err, &flagErr) {
		t.Errorf("err = %v", r.err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/merge/`
Expected: FAIL — `undefined: NewCmdMerge`.

- [ ] **Step 3: Implement**

`pkg/cmd/pr/merge/merge.go`:

```go
// Package merge implements `khbb pr merge`.
package merge

import (
	"context"
	"fmt"
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

const (
	pollInterval = 2 * time.Second
	mergeTimeout = 2 * time.Minute
)

// MergeOptions holds the inputs and dependencies of `khbb pr merge`.
type MergeOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Prompter   cmdutil.Prompter
	Exporter   cmdutil.Exporter
	Sleep      func(time.Duration)
	Now        func() time.Time

	Selector     string
	Strategy     string // merge_commit, squash or fast_forward; "" means the destination's default
	DeleteBranch bool
	Message      string
	Yes          bool
	DryRun       bool
}

// NewCmdMerge returns `khbb pr merge`.
func NewCmdMerge(f *cmdutil.Factory, runF func(*MergeOptions) error) *cobra.Command {
	opts := &MergeOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch,
		Prompter: f.Prompter, Sleep: time.Sleep, Now: time.Now}
	var mergeCommit, squash, fastForward bool
	cmd := &cobra.Command{
		Use:   "merge [<number> | <url>]",
		Short: "Merge a pull request",
		Long: `Merge an open pull request. Without a strategy flag, the default strategy of the destination
branch is used. The source branch is kept unless --delete-branch is given.

Merging cannot be undone: on a terminal you are asked to confirm; otherwise --yes is required.
Blocking merge checks, such as conflicts, stop the merge before anything is sent. khbb waits up
to 2 minutes for Bitbucket to finish the merge.`,
		Example: `  $ khbb pr merge 42 --squash --delete-branch
  $ khbb pr merge 42 --yes --json state,mergeCommit
  $ khbb pr merge --dry-run`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			chosen := 0
			for strategy, set := range map[string]bool{"merge_commit": mergeCommit, "squash": squash, "fast_forward": fastForward} {
				if set {
					opts.Strategy = strategy
					chosen++
				}
			}
			if chosen > 1 {
				return cmdutil.FlagErrorf("specify only one of --merge, --squash or --fast-forward")
			}
			opts.DryRun = f.DryRun
			if runF != nil {
				return runF(opts)
			}
			return mergeRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.BoolVar(&mergeCommit, "merge", false, "Merge with a merge commit")
	fl.BoolVar(&squash, "squash", false, "Squash the commits into one")
	fl.BoolVar(&fastForward, "fast-forward", false, "Fast-forward the destination branch")
	fl.BoolVarP(&opts.DeleteBranch, "delete-branch", "d", false, "Delete the source branch on Bitbucket after merging")
	fl.StringVarP(&opts.Message, "message", "m", "", "The `text` of the merge commit message")
	cmdutil.AddYesFlag(cmd, &opts.Yes)
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PullRequestFields)
	return cmd
}

func mergeRun(ctx context.Context, opts *MergeOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if err := shared.RequireOpen(pr, "merged"); err != nil {
		return err
	}
	strategy, err := chooseStrategy(ctx, client, repo, pr, opts.Strategy)
	if err != nil {
		return err
	}
	if err := checkMergeable(ctx, client, repo, pr); err != nil {
		return err
	}
	if pr.CloseSourceBranch && !opts.DeleteBranch {
		fmt.Fprintf(opts.IO.ErrOut, "note: keeping branch %s; pass --delete-branch to delete it\n", pr.Source.Branch.Name)
	}
	deleteNote := ""
	if opts.DeleteBranch {
		deleteNote = " and delete the source branch"
	}
	question := fmt.Sprintf("Merge %s with %s%s?", shared.Describe(pr), strategyName(strategy), deleteNote)
	if err := cmdutil.ConfirmDestructive(opts.IO, opts.Prompter, opts.Yes || opts.DryRun, question); err != nil {
		return err
	}
	merged, taskURL, err := client.StartMerge(ctx, repo.Workspace, repo.Slug, pr.ID, bitbucket.PRMerge{
		Strategy: strategy, Message: opts.Message, CloseSourceBranch: opts.DeleteBranch,
	})
	if err != nil {
		return err
	}
	if merged == nil {
		if merged, err = waitForMerge(ctx, client, opts, pr.ID, taskURL); err != nil {
			return err
		}
	}
	deleted := ""
	if opts.DeleteBranch {
		deleted = " and deleted branch " + pr.Source.Branch.Name
	}
	shared.PrintSuccess(opts.IO, "Merged pull request %s with %s%s", shared.Describe(pr), strategyName(strategy), deleted)
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, shared.NewPullRequest(merged))
	}
	return nil
}

// chooseStrategy returns the requested strategy after checking that the destination branch allows
// it, or the branch's default strategy when none was requested.
func chooseStrategy(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, pr *bitbucket.PullRequest, requested string) (string, error) {
	allowed, err := client.PRMergeStrategies(ctx, repo.Workspace, repo.Slug, pr.ID)
	if err != nil {
		return "", err
	}
	if requested == "" {
		return allowed.Default, nil
	}
	if len(allowed.Allowed) > 0 && !slices.Contains(allowed.Allowed, requested) {
		return "", cmdutil.FlagErrorf("merge strategy %s is not allowed into %s; allowed: %s",
			requested, pr.Destination.Branch.Name, strings.Join(allowed.Allowed, ", "))
	}
	return requested, nil
}

// checkMergeable fails on a blocking merge check, such as conflicts, with Bitbucket's reason.
func checkMergeable(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, pr *bitbucket.PullRequest) error {
	checks, err := client.PRMergeabilityChecks(ctx, repo.Workspace, repo.Slug, pr.ID)
	if err != nil {
		return err
	}
	var problems []string
	for _, c := range checks {
		if c.Status == "FAILED" && c.Blocking {
			problems = append(problems, describeCheck(c))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return &cmdutil.ConflictError{Msg: fmt.Sprintf("pull request #%d cannot be merged: %s", pr.ID, strings.Join(problems, "; "))}
}

func describeCheck(c bitbucket.MergeCheck) string {
	switch {
	case c.Type == "git_mergeability_check" && c.Reason == "conflicts":
		return "it has merge conflicts"
	case c.Reason != "":
		return c.Type + ": " + c.Reason
	}
	return c.Type + " failed"
}

// waitForMerge polls the merge task until it finishes, for at most mergeTimeout. Transient errors
// keep it polling; an unknown task stays PENDING forever, hence the deadline.
func waitForMerge(ctx context.Context, client *bitbucket.Client, opts *MergeOptions, id int, taskURL string) (*bitbucket.PullRequest, error) {
	deadline := opts.Now().Add(mergeTimeout)
	for {
		pr, done, err := client.MergeTaskStatus(ctx, taskURL)
		if err != nil && !bitbucket.IsTransient(err) {
			return nil, err
		}
		if done {
			return pr, nil
		}
		if !opts.Now().Before(deadline) {
			return nil, fmt.Errorf("the merge of pull request #%d is still running after %s; check %s", id, mergeTimeout, taskURL)
		}
		opts.Sleep(pollInterval)
	}
}

// strategyName describes a merge strategy in messages.
func strategyName(strategy string) string {
	switch strategy {
	case "merge_commit":
		return "a merge commit"
	case "fast_forward":
		return "fast-forward"
	case "":
		return "the default strategy"
	}
	return strategy
}
```

In `pkg/cmd/pr/pr.go`, import `"github.com/khipu/khbb/pkg/cmd/pr/merge"` and add `merge.NewCmdMerge(f, nil),` to `cmd.AddCommand(...)`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -race ./pkg/cmd/pr/... && go vet ./pkg/cmd/pr/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr/merge/ pkg/cmd/pr/pr.go
git commit -m "feat(pr): pr merge with strategies, merge checks and task polling"
```

---
### Task 12: `khbb pr decline`

**Files:**
- Create: `pkg/cmd/pr/decline/decline.go`, `pkg/cmd/pr/decline/decline_test.go`
- Modify: `pkg/cmd/pr/pr.go`

**Interfaces:**
- Consumes: `Client.DeclinePR` (Task 2); `shared.RequireOpen`, `shared.Describe`, `shared.PrintSuccess` (Task 4); `cmdutil.ConfirmDestructive`, `cmdutil.AddYesFlag`, `cmdutil.AddDryRunFlag`; `prtest` helpers; `prompter.NewMock`.
- Produces: `decline.DeclineOptions`, `decline.NewCmdDecline(f, runF)`. Flags: `-m/--message`, `--yes`, `--dry-run`, `--json`. Confirmation question: `Decline #42 (feature/widgets → main)? Declined pull requests cannot be reopened.` stderr: `Declined pull request #42 (feature/widgets → main)`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pr/decline/decline_test.go`:

```go
package decline

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

const question = "Decline #42 (feature/widgets → main)? Declined pull requests cannot be reopened."

func newReg(t *testing.T, pr string) *httpmock.Registry {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, pr))
	return reg
}

func declined() httpmock.Responder {
	return httpmock.JSONResponse(200, prtest.WithState(prtest.PR42, "DECLINED"))
}

func TestDecline_WithMessage(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	reg.Register("POST", prtest.PRs+"/42/decline", declined())
	f, _, out, errOut := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdDecline(f, nil), "42", "-m", "Superseded by #43", "--yes"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[1], `{"message":"Superseded by #43"}`)
	if out.String() != "" || errOut.String() != "Declined pull request #42 (feature/widgets → main)\n" {
		t.Errorf("out %q stderr %q", out.String(), errOut.String())
	}
}

func TestDecline_WithoutMessageAndJSON(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	reg.Register("POST", prtest.PRs+"/42/decline", declined())
	f, _, out, _ := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdDecline(f, nil), "42", "--yes", "--json", "state"); err != nil {
		t.Fatal(err)
	}
	if len(reg.Calls[1].Body) != 0 || out.String() != `{"state":"DECLINED"}`+"\n" {
		t.Errorf("body %q out %q", reg.Calls[1].Body, out.String())
	}
}

func TestDecline_NeedsConfirmationWithoutATerminal(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	f, _, _, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdDecline(f, nil), "42")
	if !errors.Is(err, cmdutil.ErrConfirmationRequired) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestDecline_PromptOnATerminal(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	f, ios, _, _ := prtest.NewFactory(reg)
	prtest.SetTTY(ios)
	pm := prompter.NewMock(t)
	pm.RegisterConfirm(question, func(_ string, def bool) (bool, error) { return false, nil })
	f.Prompter = pm

	if err := prtest.Run(NewCmdDecline(f, nil), "42"); !errors.Is(err, cmdutil.ErrCancel) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestDecline_RefusesClosedPullRequest(t *testing.T) {
	reg := newReg(t, prtest.WithState(prtest.PR42, "MERGED"))
	f, _, _, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdDecline(f, nil), "42", "--yes")
	var conflict *cmdutil.ConflictError
	if !errors.As(err, &conflict) || !strings.Contains(err.Error(), "only open pull requests can be declined") {
		t.Errorf("err = %v", err)
	}
}

func TestDecline_DryRun(t *testing.T) {
	reg := newReg(t, prtest.PR42)
	f, _, out, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdDecline(f, nil), "42", "--dry-run", "-m", "Not needed")
	if !errors.Is(err, bitbucket.ErrDryRun) || !strings.Contains(out.String(), "/pullrequests/42/decline") ||
		!strings.Contains(out.String(), `"message": "Not needed"`) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err %v out %q", err, out.String())
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/decline/`
Expected: FAIL — `undefined: NewCmdDecline`.

- [ ] **Step 3: Implement**

`pkg/cmd/pr/decline/decline.go`:

```go
// Package decline implements `khbb pr decline`.
package decline

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// DeclineOptions holds the inputs and dependencies of `khbb pr decline`.
type DeclineOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Prompter   cmdutil.Prompter
	Exporter   cmdutil.Exporter

	Selector string
	Message  string
	Yes      bool
	DryRun   bool
}

// NewCmdDecline returns `khbb pr decline`.
func NewCmdDecline(f *cmdutil.Factory, runF func(*DeclineOptions) error) *cobra.Command {
	opts := &DeclineOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Prompter: f.Prompter}
	cmd := &cobra.Command{
		Use:   "decline [<number> | <url>]",
		Short: "Decline a pull request",
		Long: `Decline an open pull request. Bitbucket Cloud cannot reopen a declined pull request, so on a
terminal you are asked to confirm; otherwise --yes is required.`,
		Example: `  $ khbb pr decline 42 --message "Superseded by #43"
  $ khbb pr decline 42 --yes`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			opts.DryRun = f.DryRun
			if runF != nil {
				return runF(opts)
			}
			return declineRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Message, "message", "m", "", "The `reason` for declining")
	cmdutil.AddYesFlag(cmd, &opts.Yes)
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PullRequestFields)
	return cmd
}

func declineRun(ctx context.Context, opts *DeclineOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if err := shared.RequireOpen(pr, "declined"); err != nil {
		return err
	}
	question := fmt.Sprintf("Decline %s? Declined pull requests cannot be reopened.", shared.Describe(pr))
	if err := cmdutil.ConfirmDestructive(opts.IO, opts.Prompter, opts.Yes || opts.DryRun, question); err != nil {
		return err
	}
	declined, err := client.DeclinePR(ctx, repo.Workspace, repo.Slug, pr.ID, opts.Message)
	if err != nil {
		return err
	}
	shared.PrintSuccess(opts.IO, "Declined pull request %s", shared.Describe(pr))
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, shared.NewPullRequest(declined))
	}
	return nil
}
```

In `pkg/cmd/pr/pr.go`, import `"github.com/khipu/khbb/pkg/cmd/pr/decline"` and add `decline.NewCmdDecline(f, nil),` to `cmd.AddCommand(...)`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/cmd/pr/... && go vet ./pkg/cmd/pr/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr/decline/ pkg/cmd/pr/pr.go
git commit -m "feat(pr): pr decline with confirmation"
```

---

### Task 13: `khbb pr checkout`

**Files:**
- Create: `pkg/cmd/pr/checkout/checkout.go`, `pkg/cmd/pr/checkout/checkout_test.go`
- Modify: `pkg/cmd/pr/pr.go`

**Interfaces:**
- Consumes: `shared.Finder`, `shared.Describe`, `shared.PrintSuccess` (Task 4); `(*gitctx.Resolver).RemoteFor`, `prtest.FakeGit`, `prtest.SetGit` (Task 9); `config.Config.GitProtocol`; `gitctx.ParseRepo`.
- Produces: `checkout.CheckoutOptions`, `checkout.NewCmdCheckout(f, runF)`. Requires exactly one argument. Flags: `-b/--branch <name>` (local branch name, default: the source branch), `-f/--force` (reset an existing local branch instead of fast-forwarding it). stderr: `Checked out pull request #42 (feature/widgets → main) on branch feature/widgets`.

Git commands, in order:
1. `remote -v` (through `RemoteFor`, for the pull request's source repository).
2. `rev-parse --verify --quiet refs/heads/<local>` — the local branch exists when this succeeds.
3. With a remote for the source repository:
   - `fetch <remote> +refs/heads/<branch>:refs/remotes/<remote>/<branch>`
   - new local branch: `checkout -b <local> --track <remote>/<branch>`
   - existing: `checkout <local>`, then `merge --ff-only refs/remotes/<remote>/<branch>` (or `reset --hard refs/remotes/<remote>/<branch>` with `--force`)
4. Without one (a fork): URL from `git_protocol` (`ssh` → `git@bitbucket.org:<ws>/<repo>.git`, otherwise `https://bitbucket.org/<ws>/<repo>.git`):
   - `fetch <url> refs/heads/<branch>`
   - new local branch: `checkout -b <local> FETCH_HEAD`, `config branch.<local>.remote <url>`, `config branch.<local>.merge refs/heads/<branch>`
   - existing: `checkout <local>`, then `merge --ff-only FETCH_HEAD` (or `reset --hard FETCH_HEAD` with `--force`)

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pr/checkout/checkout_test.go`:

```go
package checkout

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const remotes = "origin\tgit@bitbucket.org:acme/widgets.git (fetch)\norigin\tgit@bitbucket.org:acme/widgets.git (push)"

// forkPR42 is PR 42 opened from the fork dev/widgets-fork.
var forkPR42 = strings.Replace(prtest.PR42, `"repository":{"full_name":"acme/widgets"}`, `"repository":{"full_name":"dev/widgets-fork"}`, 1)

func setup(t *testing.T, pr string, git *prtest.FakeGit) (*cmdutil.Factory, func() string) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, pr))
	f, _, _, errOut := prtest.NewFactory(reg)
	prtest.SetGit(f, git)
	return f, errOut.String
}

func TestCheckout_NewBranchFromOrigin(t *testing.T) {
	git := &prtest.FakeGit{
		Outputs: map[string]string{
			"remote -v": remotes,
			"fetch origin +refs/heads/feature/widgets:refs/remotes/origin/feature/widgets": "",
			"checkout -b feature/widgets --track origin/feature/widgets":                   "",
		},
		Errors: map[string]string{"rev-parse --verify --quiet refs/heads/feature/widgets": "exit status 1"},
	}
	f, stderr := setup(t, prtest.PR42, git)

	if err := prtest.Run(NewCmdCheckout(f, nil), "42"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"remote -v",
		"rev-parse --verify --quiet refs/heads/feature/widgets",
		"fetch origin +refs/heads/feature/widgets:refs/remotes/origin/feature/widgets",
		"checkout -b feature/widgets --track origin/feature/widgets",
	}
	if !slices.Equal(git.Calls, want) {
		t.Errorf("git calls:\n%s", strings.Join(git.Calls, "\n"))
	}
	if stderr() != "Checked out pull request #42 (feature/widgets → main) on branch feature/widgets\n" {
		t.Errorf("stderr = %q", stderr())
	}
}

func TestCheckout_ExistingBranchFastForwards(t *testing.T) {
	git := &prtest.FakeGit{Outputs: map[string]string{
		"remote -v": remotes,
		"rev-parse --verify --quiet refs/heads/feature/widgets":                        "abc1234",
		"fetch origin +refs/heads/feature/widgets:refs/remotes/origin/feature/widgets": "",
		"checkout feature/widgets":                                       "",
		"merge --ff-only refs/remotes/origin/feature/widgets":            "",
	}}
	f, _ := setup(t, prtest.PR42, git)

	if err := prtest.Run(NewCmdCheckout(f, nil), "42"); err != nil {
		t.Fatal(err)
	}
	if last := git.Calls[len(git.Calls)-1]; last != "merge --ff-only refs/remotes/origin/feature/widgets" {
		t.Errorf("last git call = %q", last)
	}
}

func TestCheckout_ForceResetsANamedBranch(t *testing.T) {
	git := &prtest.FakeGit{Outputs: map[string]string{
		"remote -v": remotes,
		"rev-parse --verify --quiet refs/heads/review-42":                              "abc1234",
		"fetch origin +refs/heads/feature/widgets:refs/remotes/origin/feature/widgets": "",
		"checkout review-42":                                      "",
		"reset --hard refs/remotes/origin/feature/widgets":        "",
	}}
	f, _ := setup(t, prtest.PR42, git)

	if err := prtest.Run(NewCmdCheckout(f, nil), "42", "-b", "review-42", "--force"); err != nil {
		t.Fatal(err)
	}
	if last := git.Calls[len(git.Calls)-1]; last != "reset --hard refs/remotes/origin/feature/widgets" {
		t.Errorf("last git call = %q", last)
	}
}

func TestCheckout_DivergedBranchSuggestsForce(t *testing.T) {
	git := &prtest.FakeGit{
		Outputs: map[string]string{
			"remote -v": remotes,
			"rev-parse --verify --quiet refs/heads/feature/widgets":                        "abc1234",
			"fetch origin +refs/heads/feature/widgets:refs/remotes/origin/feature/widgets": "",
			"checkout feature/widgets":                                                     "",
		},
		Errors: map[string]string{"merge --ff-only refs/remotes/origin/feature/widgets": "git merge: fatal: Not possible to fast-forward, aborting."},
	}
	f, _ := setup(t, prtest.PR42, git)

	err := prtest.Run(NewCmdCheckout(f, nil), "42")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("err = %v", err)
	}
}

func TestCheckout_ForkFetchesByURL(t *testing.T) {
	url := "git@bitbucket.org:dev/widgets-fork.git"
	git := &prtest.FakeGit{
		Outputs: map[string]string{
			"remote -v":                                   remotes,
			"fetch " + url + " refs/heads/feature/widgets": "",
			"checkout -b feature/widgets FETCH_HEAD":       "",
			"config branch.feature/widgets.remote " + url:  "",
			"config branch.feature/widgets.merge refs/heads/feature/widgets": "",
		},
		Errors: map[string]string{"rev-parse --verify --quiet refs/heads/feature/widgets": "exit status 1"},
	}
	f, _ := setup(t, forkPR42, git)
	f.Config = func() (*config.Config, error) { return &config.Config{GitProtocol: "ssh"}, nil }

	if err := prtest.Run(NewCmdCheckout(f, nil), "42"); err != nil {
		t.Fatal(err)
	}
	if n := len(git.Calls); n != 6 || git.Calls[n-1] != "config branch.feature/widgets.merge refs/heads/feature/widgets" {
		t.Errorf("git calls:\n%s", strings.Join(git.Calls, "\n"))
	}
}

func TestCheckout_ForkUsesHTTPSByDefault(t *testing.T) {
	url := "https://bitbucket.org/dev/widgets-fork.git"
	git := &prtest.FakeGit{Outputs: map[string]string{
		"remote -v": remotes,
		"rev-parse --verify --quiet refs/heads/feature/widgets": "abc1234",
		"fetch " + url + " refs/heads/feature/widgets":          "",
		"checkout feature/widgets":                              "",
		"merge --ff-only FETCH_HEAD":                            "",
	}}
	f, _ := setup(t, forkPR42, git)

	if err := prtest.Run(NewCmdCheckout(f, nil), "42"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckout_NeedsOneArgument(t *testing.T) {
	f, _, _, _ := prtest.NewFactory(httpmock.New(t))
	for _, args := range [][]string{{}, {"1", "2"}} {
		err := prtest.Run(NewCmdCheckout(f, nil), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pr/checkout/`
Expected: FAIL — `undefined: NewCmdCheckout`.

- [ ] **Step 3: Implement**

`pkg/cmd/pr/checkout/checkout.go`:

```go
// Package checkout implements `khbb pr checkout`.
package checkout

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// CheckoutOptions holds the inputs and dependencies of `khbb pr checkout`.
type CheckoutOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Config     func() (*config.Config, error)
	Git        *gitctx.Resolver

	Selector   string
	BranchName string
	Force      bool
}

// NewCmdCheckout returns `khbb pr checkout`.
func NewCmdCheckout(f *cmdutil.Factory, runF func(*CheckoutOptions) error) *cobra.Command {
	opts := &CheckoutOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Git: f.Git}
	cmd := &cobra.Command{
		Use:   "checkout {<number> | <url>}",
		Short: "Check out a pull request in git",
		Long: `Fetch the source branch of a pull request and switch to it, creating a local branch that
tracks it. An existing local branch is fast-forwarded; --force resets it instead. Pull requests
from forks are fetched from the fork's URL (git_protocol: ssh or https). Your git credentials
are used for fetching.`,
		Example: `  $ khbb pr checkout 42
  $ khbb pr checkout 42 --branch review-42 --force`,
		Args: cmdutil.ExactArgs(1, "<number> | <url>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Selector = args[0]
			opts.Config = f.Config
			if runF != nil {
				return runF(opts)
			}
			return checkoutRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.BranchName, "branch", "b", "", "Local branch `name` to use (default: the source branch name)")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Reset an existing local branch to the pull request")
	return cmd
}

func checkoutRun(ctx context.Context, opts *CheckoutOptions) error {
	if opts.Git == nil {
		return errors.New("pr checkout needs git")
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	source := repo
	if pr.Source.Repository != nil && pr.Source.Repository.FullName != "" {
		if source, err = gitctx.ParseRepo(pr.Source.Repository.FullName); err != nil {
			return err
		}
	}
	branch := pr.Source.Branch.Name
	local := opts.BranchName
	if local == "" {
		local = branch
	}
	remote, err := opts.Git.RemoteFor(source)
	if err != nil {
		return err
	}
	git := opts.Git.Git
	_, revErr := git("rev-parse", "--verify", "--quiet", "refs/heads/"+local)
	exists := revErr == nil

	if remote != "" {
		tracking := "refs/remotes/" + remote + "/" + branch
		if _, err := git("fetch", remote, "+refs/heads/"+branch+":"+tracking); err != nil {
			return err
		}
		if exists {
			err = update(git, local, tracking, opts.Force)
		} else {
			_, err = git("checkout", "-b", local, "--track", remote+"/"+branch)
		}
	} else {
		url, uerr := cloneURL(opts.Config, source)
		if uerr != nil {
			return uerr
		}
		if _, err := git("fetch", url, "refs/heads/"+branch); err != nil {
			return err
		}
		if exists {
			err = update(git, local, "FETCH_HEAD", opts.Force)
		} else {
			err = newForkBranch(git, local, url, branch)
		}
	}
	if err != nil {
		return err
	}
	shared.PrintSuccess(opts.IO, "Checked out pull request %s on branch %s", shared.Describe(pr), local)
	return nil
}

// update switches to an existing local branch and brings it to ref: fast-forward only, or a hard
// reset with --force.
func update(git func(...string) (string, error), local, ref string, force bool) error {
	if _, err := git("checkout", local); err != nil {
		return err
	}
	if force {
		_, err := git("reset", "--hard", ref)
		return err
	}
	if _, err := git("merge", "--ff-only", ref); err != nil {
		return fmt.Errorf("branch %s cannot be fast-forwarded to the pull request; use --force to reset it (%w)", local, err)
	}
	return nil
}

// newForkBranch creates local at FETCH_HEAD and makes it track branch in the fork at url, so that
// `git pull` keeps working.
func newForkBranch(git func(...string) (string, error), local, url, branch string) error {
	if _, err := git("checkout", "-b", local, "FETCH_HEAD"); err != nil {
		return err
	}
	if _, err := git("config", "branch."+local+".remote", url); err != nil {
		return err
	}
	_, err := git("config", "branch."+local+".merge", "refs/heads/"+branch)
	return err
}

// cloneURL returns the URL to fetch a repository that has no local remote, following the
// git_protocol setting: ssh, or https by default.
func cloneURL(cfg func() (*config.Config, error), repo gitctx.Repo) (string, error) {
	protocol := ""
	if cfg != nil {
		c, err := cfg()
		if err != nil {
			return "", err
		}
		protocol = c.GitProtocol
	}
	if protocol == "ssh" {
		return "git@bitbucket.org:" + repo.FullName() + ".git", nil
	}
	return "https://bitbucket.org/" + repo.FullName() + ".git", nil
}
```

In `pkg/cmd/pr/pr.go`, import `"github.com/khipu/khbb/pkg/cmd/pr/checkout"` and add `checkout.NewCmdCheckout(f, nil),` to `cmd.AddCommand(...)`.

- [ ] **Step 4: Run the full suite**

Run: `go test -race ./... && go vet ./... && go build -o bin/khbb ./cmd/khbb && ./bin/khbb pr --help`
Expected: PASS; `khbb pr --help` lists `approve, checkout, checks, comment, create, decline, diff, edit, list, merge, request-changes, status, unapprove, view`. If `golangci-lint` is available (`$(go env GOPATH)/bin/golangci-lint` or the controller's scratchpad copy), run `golangci-lint run ./...` and expect `0 issues`.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pr/checkout/ pkg/cmd/pr/pr.go
git commit -m "feat(pr): pr checkout for branches and forks"
```

---

### Task 14: Live smoke test in the sandbox (controller only)

This task is run by the controller, not by a subagent: it uses the user's Bitbucket login and the sandbox repository `khipu/khipubb-sandbox`, where the user approved write tests. Never run it against another repository. Print only status codes, states, counts and khbb's own messages — never full JSON that contains people's names or account IDs.

**Files:** none (results go into the SDD ledger; anything surprising also goes into `docs/superpowers/specs/2026-10-05-bitbucket-api-findings.md` and becomes a fix task).

- [ ] **Step 1: Prepare a probe branch**

```bash
go build -o bin/khbb ./cmd/khbb
SB=<scratchpad>/sandbox   # clone of git@bitbucket.org:khipu/khipubb-sandbox.git
cd "$SB" && git fetch -q && git checkout -q -B khbb-probe/smoke origin/main
printf 'smoke\n' > probe/smoke.txt && git add probe/smoke.txt
git -c user.name=khbb-probe -c user.email=probe@example.invalid commit -qm "probe: smoke test"
git push -q origin khbb-probe/smoke
```

- [ ] **Step 2: Create, inspect and edit**

Run from `$SB` with `KHBB=<repo>/bin/khbb`:

```bash
git commit -q --allow-empty -m "probe: not pushed yet"        # the local branch is now one commit ahead
$KHBB pr create -t "khbb smoke test" -b "Smoke test of khbb Plan 2b." --draft --no-default-reviewers   # URL + unpushed warning
git push -q origin khbb-probe/smoke
$KHBB pr create -t "dup" -b "" --no-default-reviewers; echo "exit=$?"     # conflict: already exists, exit 1
$KHBB pr create -t x -b "" -B khbb-missing --no-default-reviewers; echo "exit=$?"   # 400 destination: branch not found, exit 1
$KHBB pr view --json state,draft --jq '.'                                  # OPEN, draft true
$KHBB pr edit --ready -t "khbb smoke test (edited)" --json draft,title --jq '.'
```

Expected: the first create prints a URL on stdout and `warning: the pull request uses commit …, but your local branch khbb-probe/smoke is at …` on stderr; the duplicate exits 1 with `a pull request for khbb-probe/smoke into main already exists`; the missing base exits 1 without creating anything; edit shows `draft: false` and the new title.

- [ ] **Step 3: Comment and review**

```bash
$KHBB pr comment --body "General comment" --json id,url --jq '.url | length > 0'   # true: comment URLs come back on create
$KHBB pr comment --file probe/smoke.txt --line 1 --body "Inline comment"
$KHBB pr comment --file not/in/diff.txt --body x; echo "exit=$?"                  # not_found, exit 1
$KHBB pr approve && $KHBB pr unapprove && $KHBB pr request-changes
$KHBB api -X DELETE 'repositories/khipu/khipubb-sandbox/pullrequests/<id>/request-changes' --silent
```

- [ ] **Step 4: Merge flow**

```bash
$KHBB pr merge --dry-run                         # JSON with merge?async=true, nothing sent, exit 0
$KHBB pr merge; echo "exit=$?"                   # non-TTY without --yes: confirmation_required, exit 1
$KHBB pr merge --fast-forward --yes; echo "exit=$?"   # allowed or not, per the sandbox's settings; record which
$KHBB pr merge --squash -d --yes --json state --jq .state   # MERGED
git fetch -q --prune && git branch -r | grep -c khbb-probe/smoke  # 0: branch deleted
$KHBB pr approve <id>; echo "exit=$?"            # conflict: is merged, exit 1
```

If `--fast-forward` succeeded, the PR is already merged: skip the squash line, and note it.

- [ ] **Step 5: Decline and checkout**

```bash
git checkout -q -B khbb-probe/smoke2 origin/main && printf 'two\n' > probe/smoke2.txt && git add probe && \
  git -c user.name=khbb-probe -c user.email=probe@example.invalid commit -qm "probe: smoke 2" && git push -q origin khbb-probe/smoke2
$KHBB pr create -t "khbb smoke 2" -b "" --no-default-reviewers
git checkout -q main && $KHBB pr checkout <id2> && git branch --show-current     # khbb-probe/smoke2
$KHBB pr decline <id2> -m "Smoke test done" --yes --json state --jq .state      # DECLINED
git push -q origin --delete khbb-probe/smoke2; git checkout -q main; git branch -D khbb-probe/smoke khbb-probe/smoke2
```

- [ ] **Step 6: Record and clean up**

Confirm with `$KHBB pr list -s open --json id` that no smoke-test pull request is left open and with `git branch -r` that no `khbb-probe/*` branch remains. Write the results (each step: expected vs. observed) into the ledger. Any mismatch becomes a fix task before the final review.

---

## Spec Coverage (Plan 2b)

| Spec item | Covered by |
|---|---|
| §7.1 `pr create` (title, body, body-file, base, head, reviewer, draft, no-default-reviewers, delete-branch, fill, web, dry-run; default reviewers; no implicit push; prompts on a TTY, required flags otherwise) | Tasks 8, 9 |
| §7.1 `pr approve` / `pr unapprove` / `pr request-changes` (`--dry-run`) | Task 6 |
| §7.1 `pr comment` (body, body-file, `--file --line`, `--reply-to`, dry-run) | Task 7 |
| §7.1 `pr edit` (title, body, body-file, base, add/remove reviewer, draft/ready, dry-run; partial PUT) | Task 10 |
| §7.1 `pr merge` (merge/squash/fast-forward, default strategy, delete-branch, message, yes, dry-run; 202 polling with a 2-minute timeout; blocking merge checks) | Tasks 3, 11 |
| §7.1 `pr decline` (message, yes, dry-run; no reopen) | Tasks 2, 12 |
| §7.1 `pr checkout` (branch, force; forks via `git_protocol`) | Tasks 9, 13 |
| §7.1 reviewer identifiers (uuid, account ID, nickname, display name; exactly one match; author dropped) | Task 5 |
| §8.1 JSON output of write commands | Tasks 7, 8, 10, 11, 12 |
| §8.3 `conflict` error code | Task 4 |
| §9 destructive confirmation, `--yes`, `--dry-run`, no implicit branch deletion | Tasks 4, 11, 12 |
| §10 merge-check reasons, merge task timeout, transient errors | Tasks 3, 11 |
| Findings: non-JSON error pages never printed | Task 1 |
| Findings: duplicate PR silently retitled; approve on merged PR accepted; display_name not filterable | Tasks 5, 6, 8 |
| Deferred from Plan 2a: dry run in `prtest`, write commands check state, `CheckRepoSelector`, member resolution, `pr checks --watch` decision | Tasks 4, 5, 6–12; rulings |
| Live verification | Task 14 |
| §7.2 pipelines, §7.4 `skill install`, §12 distribution | Plans 3 and 4 |
