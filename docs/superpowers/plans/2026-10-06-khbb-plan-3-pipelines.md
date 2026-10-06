# khbb Plan 3 — Pipeline commands

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `khbb pipeline list`, `view`, `logs`, `watch`, `run`, `stop` and `rerun`, and harden the HTTP client for them (transport-level timeouts so large logs stream, no https→http redirects, Bitbucket's `detail` shown when its `message` is generic).

**Architecture:** Same patterns as Plans 1–2b: one package per command under `pkg/cmd/pipeline/`, `XxxOptions` + injectable `runF`; the Bitbucket client in `internal/bitbucket/pipelines.go` sends requests and returns raw API structs; `pkg/cmd/pipeline/shared` maps them to the stable JSON shapes of spec §8.1, normalizes statuses, finds pipelines, renders them and runs the watch loop. Tests use `internal/httpmock`, the existing `prtest` helpers (factory, fake git, request assertions) and pipeline fixtures in `pkg/cmd/pipeline/shared/ptest`.

**Tech Stack:** Go 1.26, cobra v1.10.2, go-gh v2.16.1 (`tableprinter`, `prompter`, `jq`, `template`), go-keyring v0.2.8, yaml.v3, safeexec. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-10-05-khbb-cli-design.md` (§7.2, §8, §9, §10). API facts: `docs/superpowers/specs/2026-10-05-bitbucket-api-findings.md`, section "Pipelines API (Sources B and C, for Plan 3)" — every Bitbucket behavior this plan relies on was verified there, read-only against a Khipu repository and with real runs in the sandbox `khipu/khipubb-sandbox`.

**Roadmap:** Plans 1, 2a, 2b (done) → **Plan 3 (this)** → Plan 4: release (GoReleaser, brew/scoop), `khbb skill install`, README.

## Global Constraints

Everything in the earlier plans' Global Constraints still applies. The ones that matter most here:

- Module `github.com/khipu/khbb`; all code, help text, messages and docs in English.
- Must build and pass `go test -race ./...` on macOS and Linux and `go test ./...` on Windows; CI runs `golangci-lint` (`default: standard`, staticcheck ST1005: error strings start lowercase, no trailing punctuation). Run `gofmt -l` on changed files; it must print nothing.
- stdout carries data only; prompts, progress notices, warnings, confirmations and errors go to stderr. Exception by spec §7.2: `pipeline watch` without a terminal prints one line per state change on stdout.
- Exit codes: `0` success (also after `--dry-run`), `1` error, `2` cancelled, `4` auth required. `pipeline watch --exit-status` (and `run`/`rerun --watch`) exit `1` when the pipeline ends `failed`, `error`, `stopped` or `expired`.
- Pipelines are addressed by build number (`42`, `#42`) or URL `https://bitbucket.org/<ws>/<repo>/pipelines/results/<n>`; without one, commands use the newest pipeline of the current branch (`pipeline watch`: see rulings).
- Status values (spec §7.2 plus findings #7): pipelines `pending | running | paused | successful | failed | error | stopped | expired`; steps add `skipped`.
- Pipeline JSON fields (spec §8.1): `number, uuid, status, trigger, creator, refType, refName, commit, selector, durationSeconds, url, createdOn, completedOn`; `pipeline view` adds `steps`. Step JSON fields: `uuid, name, status, startedOn, completedOn, durationSeconds`. `selector` is `{type, pattern}`. Timestamps RFC 3339 UTC; `completedOn`/`startedOn` are `null` until set. Slices never `null`.
- Every mutating command has `--dry-run` (the client prints `{"dryRun": true, "method", "url", "body"}` to stdout and sends nothing; GETs still run). Secured variable values print as `****`.
- `pipeline stop` is destructive (spec §9): a confirmation prompt on a terminal (default No), `--yes` otherwise; `--dry-run` skips the prompt.
- Flag shorthands: `-R` = `--repo`, `-q` = `--jq`, `-t` = `--template`, `-b` = `--branch`, `-s` = `--status` (list) / `--step` (logs), `-L` = `--limit`, `-v` = `--verbose`, `-w` = `--web`, `-i` = `--interval`.
- Never print a secret: `--secret-var` values appear nowhere — not in errors, not in dry runs (masked), not in notices.
- Test fixtures use fake identities only (`@example.com`, "Ada Example", `{00000000-…}` UUIDs, workspace `acme`, repository `acme/widgets`).
- Tests that build commands through `pipeline.NewCmdPipeline(f)` must pass `-R` (registering `--repo` resets `f.RepoOverride`); tests in this plan build commands directly and keep `prtest.NewFactory`'s `acme/widgets`.

## Decisions taken for this plan (rulings)

- **Status mapping** (findings): PARSING/PENDING/READY → `pending`, IN_PROGRESS → `running`, any state with `stage PAUSED` → `paused`, COMPLETED + result SUCCESSFUL/FAILED/ERROR/STOPPED/EXPIRED/NOT_RUN → `successful`/`failed`/`error`/`stopped`/`expired`/`skipped`. Unknown values pass through lowercased. `expired` counts as unsuccessful for exit codes. — Cost if wrong: a rare state shows under an unexpected name.
- **`pipeline list --status`** accepts the spec's seven values and maps them to Bitbucket's own filter values (`pending` → PENDING+PARSING, `running` → BUILDING, `paused` → PAUSED+HALTED, `successful` → PASSED, `failed` → FAILED, `error` → ERROR, `stopped` → STOPPED); repeated `status=` params are ORed (verified). Lists always send `sort=-created_on` (the default order is oldest first).
- **`pipeline watch` without a number** watches the newest pipeline of the current branch **for the local HEAD commit** (`target.commit.hash`, verified), waiting up to 60 s for it to appear after a push (stderr notice), then failing with `not_found`. Without git (no HEAD), it falls back to the newest pipeline of the branch. `view`/`logs`/`stop`/`rerun` without a number use the newest pipeline of the branch (spec). — Why: right after `git push`, the newest pipeline is often the previous one; agents would watch the wrong run. Cost if wrong: a 60 s wait on branches without pipelines.
- **`pipeline watch` output:** on a terminal, a redrawn summary + step table every interval; without one, one stdout line per pipeline or step status change (`#42 step "Build": successful (12s)`, `#42 failed (1m02s)`). It stops at a final status or when the pipeline is paused on a manual step (`#42 paused (waiting for manual step "Deploy")`), which exits 0 even with `--exit-status`.
- **`pipeline run --watch` and `rerun --watch`** exit like `watch --exit-status`. `--watch` cannot be combined with `--json` (both would write stdout).
- **`pipeline run`:** `--branch` (default: current branch), `--commit` and `--tag` are mutually exclusive; `--commit` sends a `pipeline_commit_target` with `selector {type: default}` unless `--custom` (a commit target without selector → 400, verified). With `--repo`, one of `--branch/--commit/--tag` is required. Output: the pipeline URL on stdout (or Pipeline JSON with `--json`), a confirmation on stderr.
- **`pipeline rerun`** posts the original pipeline's target back (verified for `default`, `branches`, `custom` and `pull-requests` selectors): same commit, same definition. Bitbucket never returns a pipeline's variables (verified), so `rerun` takes `--var` and `--secret-var` (adds `--var` to spec §7.2) and, for custom pipelines run without them, prints a note that variables cannot be copied.
- **`pipeline stop`** refuses, before sending anything, pipelines that already finished and pipelines paused on a manual step (Bitbucket answers 204 to stopping a paused pipeline but leaves it paused — verified twice): the message points to the web page.
- **`pipeline logs`:** no filter → every step that has a log, each preceded by `==> <step name> <==` when more than one is printed; steps that are pending, paused (a manual step not started) or skipped, or whose log answers 404, get a stderr notice and are skipped. `--failed` selects `failed`/`error` steps (none → stderr notice, exit 0). `--step` matches a step name (case-insensitive) or UUID; no match → `not_found` listing the step names. `--tail N` keeps the last N lines of each log. Logs are fetched whole (the Range header works, but whole lines are needed).
- **`pipeline view`:** a summary and step table on a terminal; tab-separated key/values and step rows otherwise; `-v/--verbose` adds step UUIDs and timestamps; `--web` opens the pipeline page; `--json` with `steps`.
- **`refType`/`refName`** for non-ref targets: pull-request pipelines → `pullrequest` + the source branch; commit pipelines → `commit` + empty name. `trigger` is Bitbucket's trigger name lowercased (`push`, `manual`, `scheduled`, `parent_step`).
- **HTTP client:** the 30 s whole-request `http.Client.Timeout` is replaced by transport timeouts (dial 30 s, TLS handshake 10 s, response headers 30 s) so large logs can stream (spec §10's "per-request timeout 30s" now bounds the wait for a response); at most 10 redirects and never https→http. Context-aware sleeps are not added: Ctrl-C ends the process, which is acceptable for watch loops. — Cost if wrong: a stalled body download can hang until Ctrl-C.
- **Errors:** when Bitbucket's `message` is generic ("Bad request", "Not found" or the status text) and `detail` is a string, the detail becomes the message (pipelines put the real reason there). Pipeline 404s get no repository hint (their messages already name the missing build number, branch or tag).

## Review Focus

1. **`pipeline watch` right after a push** must follow the pipeline of the pushed commit, not the previous one, and fail clearly when none appears — tests `TestWatch_FollowsTheHeadCommit`, `TestWatch_NoPipelineForTheHeadCommit` (Task 8).
2. **A pipeline paused on a manual step** stops `watch` with the step's name and exit 0, and `stop` refuses it — tests `TestWatch_StopsWhenPaused` (Task 4), `TestStop_RefusesPausedPipeline` (Task 10).
3. **Steps without a log** (pending, skipped, 404) are skipped with a notice while the others still print — tests `TestLogs_AllStepsWithHeaders`, `TestLogs_SkipsStepsWithoutALog` (Task 7).
4. **Secret variables** never appear: dry runs mask them, bad `--secret-var` values are not echoed — tests `TestParseVariables` (Task 3), `TestRun_DryRunMasksSecrets` (Task 9).
5. **Transient errors while watching** (429, 5xx, network) keep polling up to 5 times in a row, while 404 stops at once — tests `TestWatch_RetriesTransientErrors`, `TestWatch_StopsOnNotFound` (Task 4).

---

## File Map

| File | Responsibility | Task |
|---|---|---|
| `internal/bitbucket/client.go` | `NewHTTPClient`: transport timeouts, redirect policy | 1 |
| `internal/bitbucket/errors.go` | Generic message → `detail` | 1 |
| `internal/cmdutil/errors.go` | No repository hint for pipeline 404s | 1 |
| `internal/bitbucket/pipelines.go` | Pipeline/step types, list/get/steps/log/run/stop | 2 |
| `pkg/cmd/pipeline/shared/status.go` | Status normalization, list filter values | 3 |
| `pkg/cmd/pipeline/shared/export.go` | Pipeline and Step JSON shapes, web URL | 3 |
| `pkg/cmd/pipeline/shared/display.go` | Status labels, durations, ref labels, summary table | 3 |
| `pkg/cmd/pipeline/shared/variables.go` | `--var` / `--secret-var` parsing | 3 |
| `pkg/cmd/pipeline/shared/ptest/ptest.go` | Pipeline and step fixtures | 3 |
| `pkg/cmd/pipeline/shared/finder.go` | Selector parsing and pipeline lookup | 4 |
| `pkg/cmd/pipeline/shared/watch.go` | Watch loop and exit status | 4 |
| `pkg/cmd/pipeline/pipeline.go` | `khbb pipeline` group | 5–11 |
| `pkg/cmd/root/root.go` | Register the group | 5 |
| `pkg/cmd/pipeline/list/list.go` | `khbb pipeline list` | 5 |
| `pkg/cmd/pipeline/view/view.go` | `khbb pipeline view` | 6 |
| `pkg/cmd/pipeline/logs/logs.go` | `khbb pipeline logs` | 7 |
| `pkg/cmd/pipeline/watch/watch.go` | `khbb pipeline watch` | 8 |
| `pkg/cmd/pipeline/shared/started.go` | Report a started pipeline (run, rerun) | 9 |
| `pkg/cmd/pipeline/run/run.go` | `khbb pipeline run` | 9 |
| `pkg/cmd/pipeline/stop/stop.go` | `khbb pipeline stop` | 10 |
| `pkg/cmd/pipeline/rerun/rerun.go` | `khbb pipeline rerun` | 11 |
| — | Live smoke test in the sandbox (controller only) | 12 |

---

### Task 1: HTTP client for long downloads; Bitbucket's `detail` as the message

**Files:**
- Modify: `internal/bitbucket/client.go` (default client), `internal/bitbucket/errors.go` (`ParseHTTPError`), `internal/cmdutil/errors.go` (`classifyHTTP` 404 hints)
- Test: `internal/bitbucket/client_test.go`, `internal/bitbucket/errors_test.go`, `internal/cmdutil/errors_test.go`

**Interfaces:**
- Produces: `bitbucket.NewHTTPClient() *http.Client` — used by `bitbucket.New` when `Options.HTTPClient` is nil. `ParseHTTPError` turns a generic `message` plus a string `detail` into `Message = detail, Detail = ""`. `cmdutil.Classify` gives pipeline 404s (`/pipelines` in the URL) no hint.

- [ ] **Step 1: Write the failing tests**

Append to `internal/bitbucket/client_test.go` (package `bitbucket_test`; it already imports `net/http`, `testing`, `time`):

```go
func TestNewHTTPClient(t *testing.T) {
	c := bitbucket.NewHTTPClient()
	if c.Timeout != 0 {
		t.Errorf("Timeout = %v: a whole-request timeout would cut long log downloads", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok || tr.ResponseHeaderTimeout != 30*time.Second || tr.TLSHandshakeTimeout != 10*time.Second || tr.Proxy == nil {
		t.Fatalf("transport = %#v", c.Transport)
	}
	from, _ := http.NewRequest("GET", "https://api.bitbucket.org/2.0/repositories/acme/widgets/pipelines/1/steps/x/log", nil)
	toHTTP, _ := http.NewRequest("GET", "http://storage.example.com/log", nil)
	toHTTPS, _ := http.NewRequest("GET", "https://storage.example.com/log", nil)
	if err := c.CheckRedirect(toHTTP, []*http.Request{from}); err == nil {
		t.Error("a redirect from https to http must be refused")
	}
	if err := c.CheckRedirect(toHTTPS, []*http.Request{from}); err != nil {
		t.Errorf("https to https: %v", err)
	}
	via := make([]*http.Request, 10)
	for i := range via {
		via[i] = from
	}
	if err := c.CheckRedirect(toHTTPS, via); err == nil {
		t.Error("an 11th redirect must be refused")
	}
}
```

If `client_test.go` lacks one of those imports, add it.

Append to `internal/bitbucket/errors_test.go` (package `bitbucket_test`; it imports `net/http`, `net/http/httptest`):

```go
func TestParseHTTPError_DetailReplacesAGenericMessage(t *testing.T) {
	resp := &http.Response{StatusCode: 404, Request: httptest.NewRequest("GET", "https://api.bitbucket.org/2.0/repositories/acme/widgets/pipelines/99", nil)}
	e := bitbucket.ParseHTTPError(resp, []byte(`{"error":{"message":"Not found","detail":"Pipeline with build number '99' not found in repository {x}","data":{"key":"result-service.pipeline-id.not-found"}}}`))
	if e.Message != "Pipeline with build number '99' not found in repository {x}" || e.Detail != "" {
		t.Errorf("message %q detail %q", e.Message, e.Detail)
	}
	resp.StatusCode = 400
	e = bitbucket.ParseHTTPError(resp, []byte(`{"error":{"message":"Bad request","detail":"Requested selector is not found in bitbucket-pipelines.yml."}}`))
	if e.Message != "Requested selector is not found in bitbucket-pipelines.yml." {
		t.Errorf("message %q", e.Message)
	}
	keep := bitbucket.ParseHTTPError(resp, []byte(`{"error":{"message":"Repository acme/nope not found","detail":"more"}}`))
	if keep.Message != "Repository acme/nope not found" || keep.Detail != "more" {
		t.Errorf("a specific message must stay: %+v", keep)
	}
	scopes := bitbucket.ParseHTTPError(&http.Response{StatusCode: 403}, []byte(`{"error":{"message":"Forbidden","detail":{"required":["read:pipeline:bitbucket"]}}}`))
	if scopes.Message != "Forbidden" || len(scopes.RequiredScopes) != 1 {
		t.Errorf("an object detail must not become the message: %+v", scopes)
	}
}
```

Append to `internal/cmdutil/errors_test.go`:

```go
func TestClassify_NoRepositoryHintForPipelines(t *testing.T) {
	err := &bitbucket.HTTPError{StatusCode: 404, Message: "Could not find last reference for branch nope",
		URL: "https://api.bitbucket.org/2.0/repositories/acme/widgets/pipelines"}
	if info := cmdutil.Classify(err); info.Code != "not_found" || info.Hint != "" {
		t.Errorf("Classify = %+v, want not_found without a hint", info)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/bitbucket/ ./internal/cmdutil/`
Expected: FAIL — `undefined: bitbucket.NewHTTPClient`; the detail test and the pipeline hint test fail.

- [ ] **Step 3: Implement**

In `internal/bitbucket/client.go`, add `"net"` to the imports; in `New`, replace

```go
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
```

with

```go
	if opts.HTTPClient == nil {
		opts.HTTPClient = NewHTTPClient()
	}
```

and add after `New`:

```go
// NewHTTPClient returns khbb's default HTTP client. It bounds connecting, the TLS handshake and the
// wait for response headers — not the whole request, so large step logs can stream — and it
// follows at most 10 redirects, never from https to http.
func NewHTTPClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Transport: tr, CheckRedirect: checkRedirect}
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refusing to follow a redirect from https to %s", req.URL.Scheme)
	}
	return nil
}
```

In `internal/bitbucket/errors.go`, add `"strings"` to the imports; in `ParseHTTPError`, replace

```go
	if json.Unmarshal(body, &payload) == nil {
		e.Message = payload.Error.Message
		e.Detail, e.RequiredScopes = parseDetail(payload.Error.Detail)
		e.Fields = parseFields(payload.Error.Fields)
	}
```

with

```go
	if json.Unmarshal(body, &payload) == nil {
		e.Message = payload.Error.Message
		e.Detail, e.RequiredScopes = parseDetail(payload.Error.Detail)
		e.Fields = parseFields(payload.Error.Fields)
		// Pipelines errors say "Bad request" or "Not found" and put the reason in a string detail.
		if isText(payload.Error.Detail) && e.Detail != "" && isGenericMessage(e.Message, e.StatusCode) {
			e.Message, e.Detail = e.Detail, ""
		}
	}
```

and add:

```go
func isText(raw json.RawMessage) bool {
	return len(raw) > 0 && raw[0] == '"'
}

func isGenericMessage(msg string, status int) bool {
	switch strings.ToLower(strings.TrimSpace(msg)) {
	case "", "bad request", "not found", strings.ToLower(http.StatusText(status)):
		return true
	}
	return false
}
```

In `internal/cmdutil/errors.go`, in `classifyHTTP`'s 404 case, add a first case to the inner `switch`:

```go
		case strings.Contains(e.URL, "/pipelines"):
			// Pipeline 404s name what is missing (a build number, a branch, a tag); a repository hint would mislead.
```

so that the switch reads `case strings.Contains(e.URL, "/pipelines"):` (no hint), then the existing `/pullrequests/` and `/repositories/` cases.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/... ./pkg/... && go vet ./... && gofmt -l internal`
Expected: PASS; gofmt prints nothing.

- [ ] **Step 5: Commit**

```bash
git add internal/bitbucket/client.go internal/bitbucket/client_test.go internal/bitbucket/errors.go internal/bitbucket/errors_test.go internal/cmdutil/errors.go internal/cmdutil/errors_test.go
git commit -m "fix(bitbucket): stream long downloads, refuse https→http redirects, show error details"
```

---

### Task 2: Pipelines API in the Bitbucket client

**Files:**
- Create: `internal/bitbucket/pipelines.go`, `internal/bitbucket/pipelines_test.go`

**Interfaces:**
- Consumes: `Client.Do`, `Client.GetText`, `List[T]`, `RepoPath`, `User`, `Commit` (existing); test helpers in package `bitbucket_test`: `newTestClient`, `assertBody` (prwrite_test.go), `userJSON`.
- Produces:
  - Types `Pipeline{UUID string; BuildNumber int; State PipelineState; Target PipelineTarget; Trigger Named; Creator *User; CreatedOn time.Time; CompletedOn *time.Time; DurationInSeconds int}`, `PipelineState{Name string; Stage, Result *Named}`, `Named{Name string}`, `PipelineTarget{Type, RefType, RefName string; Commit *Commit; Selector *PipelineSelector; Source, Destination string; DestinationCommit *Commit; PullRequest *PipelinePullRequest}`, `PipelinePullRequest{ID int}`, `PipelineSelector{Type, Pattern string}`, `PipelineStep{UUID, Name string; State PipelineState; StartedOn, CompletedOn *time.Time; DurationInSeconds int; Trigger PipelineStepTrigger}`, `PipelineStepTrigger{Type string}`, `PipelineListOptions{Branch string; Statuses []string; CreatorUUID, CommitHash string}`, `PipelineVariable{Key, Value string; Secured bool}`.
  - `(*Client).ListPipelines(ctx, workspace, slug string, opts PipelineListOptions, limit int) ([]Pipeline, error)` — always `sort=-created_on`; `target.branch`, repeated `status`, `creator.uuid`, `target.commit.hash` when set.
  - `(*Client).GetPipeline(ctx, workspace, slug string, number int) (*Pipeline, error)`
  - `(*Client).ListPipelineSteps(ctx, workspace, slug string, number int) ([]PipelineStep, error)`
  - `(*Client).StepLog(ctx, workspace, slug string, number int, stepUUID string) (string, error)` — 404 for steps without a log comes back as `*HTTPError`.
  - `(*Client).RunPipeline(ctx, workspace, slug string, target PipelineTarget, vars []PipelineVariable) (*Pipeline, error)` — body `{"target": <request form>, "variables": [...]}` (variables only when non-empty).
  - `(*Client).StopPipeline(ctx, workspace, slug string, number int) error`
  - The request form of a target keeps only: `type`; `ref_type`, `ref_name` (ref targets); `source`, `destination`, `destination_commit{hash}`, `pullrequest{id}` (pull-request targets); `commit{type: commit, hash}` and `selector{type[, pattern]}` when set. A target read from a pipeline can therefore be sent back to rerun it.

- [ ] **Step 1: Write the failing tests**

`internal/bitbucket/pipelines_test.go`:

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

const pipelinesPath = "/2.0/repositories/acme/widgets/pipelines"

const pipelineJSON = `{"type":"pipeline","uuid":"{00000000-0000-0000-0000-000000000042}","build_number":42,` +
	`"state":{"name":"COMPLETED","type":"pipeline_state_completed","result":{"name":"FAILED","type":"pipeline_state_completed_failed"}},` +
	`"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main",` +
	`"commit":{"type":"commit","hash":"abc1234def5678abc1234def5678abc1234def56","links":{}},"selector":{"type":"branches","pattern":"main"}},` +
	`"trigger":{"name":"PUSH","type":"pipeline_trigger_push"},"creator":` + userJSON + `,` +
	`"created_on":"2026-10-06T12:00:00.000000+00:00","completed_on":"2026-10-06T12:01:02.000000+00:00","duration_in_seconds":62}`

func TestListPipelines(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", pipelinesPath, httpmock.JSONResponse(200, `{"values":[`+pipelineJSON+`]}`))

	ps, err := c.ListPipelines(context.Background(), "acme", "widgets", bitbucket.PipelineListOptions{
		Branch: "feature/widgets", Statuses: []string{"FAILED", "STOPPED"}, CreatorUUID: "{a}", CommitHash: "abc",
	}, 20)
	if err != nil || len(ps) != 1 {
		t.Fatalf("pipelines %v err %v", ps, err)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("sort") != "-created_on" || q.Get("target.branch") != "feature/widgets" || !slices.Equal(q["status"], []string{"FAILED", "STOPPED"}) ||
		q.Get("creator.uuid") != "{a}" || q.Get("target.commit.hash") != "abc" || q.Get("pagelen") != "20" {
		t.Errorf("query = %v", q)
	}
	p := ps[0]
	if p.BuildNumber != 42 || p.State.Name != "COMPLETED" || p.State.Result.Name != "FAILED" || p.State.Stage != nil ||
		p.Target.RefName != "main" || p.Target.Commit.Hash[:7] != "abc1234" || p.Target.Selector.Type != "branches" ||
		p.Trigger.Name != "PUSH" || p.Creator.Nickname != "ada" || p.CompletedOn == nil || p.DurationInSeconds != 62 {
		t.Errorf("pipeline = %+v", p)
	}
}

func TestListPipelines_NoFilters(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", pipelinesPath, httpmock.JSONResponse(200, `{"values":[]}`))

	if _, err := c.ListPipelines(context.Background(), "acme", "widgets", bitbucket.PipelineListOptions{}, 5); err != nil {
		t.Fatal(err)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("sort") != "-created_on" || q.Has("status") || q.Has("target.branch") || q.Has("creator.uuid") || q.Has("target.commit.hash") {
		t.Errorf("query = %v", q)
	}
}

func TestGetPipelineAndSteps(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", pipelinesPath+"/42", httpmock.JSONResponse(200, pipelineJSON))
	reg.Register("GET", pipelinesPath+"/42/steps", httpmock.JSONResponse(200, `{"values":[`+
		`{"uuid":"{s1}","name":"Build","state":{"name":"COMPLETED","result":{"name":"SUCCESSFUL"}},"started_on":"2026-10-06T12:00:05.000000+00:00","completed_on":"2026-10-06T12:00:17.000000+00:00","duration_in_seconds":12,"trigger":{"type":"pipeline_step_trigger_automatic"}},`+
		`{"uuid":"{s2}","name":"Deploy","state":{"name":"PENDING","stage":{"name":"PAUSED"}},"started_on":null,"completed_on":null,"trigger":{"type":"pipeline_step_trigger_manual"}}]}`))

	p, err := c.GetPipeline(context.Background(), "acme", "widgets", 42)
	if err != nil || p.UUID != "{00000000-0000-0000-0000-000000000042}" {
		t.Fatalf("pipeline %+v err %v", p, err)
	}
	steps, err := c.ListPipelineSteps(context.Background(), "acme", "widgets", 42)
	if err != nil || len(steps) != 2 {
		t.Fatalf("steps %+v err %v", steps, err)
	}
	if steps[0].Name != "Build" || steps[0].DurationInSeconds != 12 || steps[0].StartedOn == nil ||
		steps[1].State.Stage.Name != "PAUSED" || steps[1].StartedOn != nil || steps[1].Trigger.Type != "pipeline_step_trigger_manual" {
		t.Errorf("steps = %+v", steps)
	}
}

func TestStepLog(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", pipelinesPath+"/42/steps/{s1}/log", httpmock.StringResponse(200, "line 1\nline 2\n"))
	reg.Register("GET", pipelinesPath+"/42/steps/{s2}/log", httpmock.JSONResponse(404,
		`{"error":{"message":"Not Found","detail":"Log in step {s2} does not exist."}}`))

	log, err := c.StepLog(context.Background(), "acme", "widgets", 42, "{s1}")
	if err != nil || log != "line 1\nline 2\n" {
		t.Errorf("log %q err %v", log, err)
	}
	if reg.Calls[0].Header.Get("Accept") != "*/*" {
		t.Errorf("Accept = %q", reg.Calls[0].Header.Get("Accept"))
	}
	_, err = c.StepLog(context.Background(), "acme", "widgets", 42, "{s2}")
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 {
		t.Errorf("err = %v", err)
	}
}

func TestRunPipeline(t *testing.T) {
	cases := []struct {
		name   string
		target bitbucket.PipelineTarget
		vars   []bitbucket.PipelineVariable
		want   string
	}{
		{"branch", bitbucket.PipelineTarget{Type: "pipeline_ref_target", RefType: "branch", RefName: "main"}, nil,
			`{"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main"}}`},
		{"custom with variables", bitbucket.PipelineTarget{Type: "pipeline_ref_target", RefType: "branch", RefName: "main",
			Selector: &bitbucket.PipelineSelector{Type: "custom", Pattern: "deploy"}},
			[]bitbucket.PipelineVariable{{Key: "ENV", Value: "staging"}, {Key: "TOKEN", Value: "s3cret", Secured: true}},
			`{"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main","selector":{"type":"custom","pattern":"deploy"}},` +
				`"variables":[{"key":"ENV","value":"staging"},{"key":"TOKEN","value":"s3cret","secured":true}]}`},
		{"commit", bitbucket.PipelineTarget{Type: "pipeline_commit_target", Commit: &bitbucket.Commit{Hash: "abc"},
			Selector: &bitbucket.PipelineSelector{Type: "default"}}, nil,
			`{"target":{"type":"pipeline_commit_target","commit":{"type":"commit","hash":"abc"},"selector":{"type":"default"}}}`},
		{"pull request (rerun)", bitbucket.PipelineTarget{Type: "pipeline_pullrequest_target", Source: "feature/widgets", Destination: "main",
			DestinationCommit: &bitbucket.Commit{Hash: "def"}, Commit: &bitbucket.Commit{Hash: "abc"},
			PullRequest: &bitbucket.PipelinePullRequest{ID: 7}, Selector: &bitbucket.PipelineSelector{Type: "pull-requests", Pattern: "**"}}, nil,
			`{"target":{"type":"pipeline_pullrequest_target","source":"feature/widgets","destination":"main","destination_commit":{"hash":"def"},` +
				`"commit":{"type":"commit","hash":"abc"},"pullrequest":{"id":7},"selector":{"type":"pull-requests","pattern":"**"}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, reg := newTestClient(t, bitbucket.Options{})
			reg.Register("POST", pipelinesPath, httpmock.JSONResponse(201, pipelineJSON))
			p, err := c.RunPipeline(context.Background(), "acme", "widgets", tc.target, tc.vars)
			if err != nil || p.BuildNumber != 42 {
				t.Fatalf("pipeline %+v err %v", p, err)
			}
			assertBody(t, reg.Calls[0], tc.want)
		})
	}
}

func TestRunPipeline_TargetReadBackIsAccepted(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", pipelinesPath+"/42", httpmock.JSONResponse(200, pipelineJSON))
	reg.Register("POST", pipelinesPath, httpmock.JSONResponse(201, pipelineJSON))

	p, err := c.GetPipeline(context.Background(), "acme", "widgets", 42)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.RunPipeline(context.Background(), "acme", "widgets", p.Target, nil); err != nil {
		t.Fatal(err)
	}
	assertBody(t, reg.Calls[1], `{"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main",`+
		`"commit":{"type":"commit","hash":"abc1234def5678abc1234def5678abc1234def56"},"selector":{"type":"branches","pattern":"main"}}}`)
}

func TestStopPipeline(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", pipelinesPath+"/42/stopPipeline", httpmock.StringResponse(204, ""))

	if err := c.StopPipeline(context.Background(), "acme", "widgets", 42); err != nil {
		t.Fatal(err)
	}
	if len(reg.Calls[0].Body) != 0 {
		t.Errorf("body = %q", reg.Calls[0].Body)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/bitbucket/`
Expected: FAIL — `undefined: bitbucket.PipelineListOptions` and the other new names.

- [ ] **Step 3: Implement**

`internal/bitbucket/pipelines.go`:

```go
package bitbucket

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Pipeline is a Bitbucket Pipelines run.
type Pipeline struct {
	UUID              string         `json:"uuid"`
	BuildNumber       int            `json:"build_number"`
	State             PipelineState  `json:"state"`
	Target            PipelineTarget `json:"target"`
	Trigger           Named          `json:"trigger"`
	Creator           *User          `json:"creator"`
	CreatedOn         time.Time      `json:"created_on"`
	CompletedOn       *time.Time     `json:"completed_on"`
	DurationInSeconds int            `json:"duration_in_seconds"`
}

// PipelineState is the state of a pipeline or step. Name is PARSING, PENDING, READY, IN_PROGRESS or
// COMPLETED; Stage (PENDING, RUNNING, PAUSED) qualifies an unfinished state and Result (SUCCESSFUL,
// FAILED, ERROR, STOPPED, EXPIRED, NOT_RUN) a completed one.
type PipelineState struct {
	Name   string `json:"name"`
	Stage  *Named `json:"stage"`
	Result *Named `json:"result"`
}

// Named is an API object identified by its name.
type Named struct {
	Name string `json:"name"`
}

// PipelineTarget is what a pipeline runs on: a branch or tag (pipeline_ref_target), a commit
// (pipeline_commit_target) or a pull request (pipeline_pullrequest_target).
type PipelineTarget struct {
	Type              string               `json:"type"`
	RefType           string               `json:"ref_type"`
	RefName           string               `json:"ref_name"`
	Commit            *Commit              `json:"commit"`
	Selector          *PipelineSelector    `json:"selector"`
	Source            string               `json:"source"`
	Destination       string               `json:"destination"`
	DestinationCommit *Commit              `json:"destination_commit"`
	PullRequest       *PipelinePullRequest `json:"pullrequest"`
}

// PipelinePullRequest identifies the pull request of a pull-request pipeline.
type PipelinePullRequest struct {
	ID int `json:"id"`
}

// PipelineSelector names a definition in bitbucket-pipelines.yml. Type is default, branches, tags,
// custom or pull-requests.
type PipelineSelector struct {
	Type    string `json:"type"`
	Pattern string `json:"pattern"`
}

// PipelineStep is one step of a pipeline.
type PipelineStep struct {
	UUID              string              `json:"uuid"`
	Name              string              `json:"name"`
	State             PipelineState       `json:"state"`
	StartedOn         *time.Time          `json:"started_on"`
	CompletedOn       *time.Time          `json:"completed_on"`
	DurationInSeconds int                 `json:"duration_in_seconds"`
	Trigger           PipelineStepTrigger `json:"trigger"`
}

// PipelineStepTrigger says how a step starts: pipeline_step_trigger_automatic or pipeline_step_trigger_manual.
type PipelineStepTrigger struct {
	Type string `json:"type"`
}

// PipelineListOptions filters ListPipelines. Statuses take the list filter's own values (PENDING,
// PARSING, BUILDING, PAUSED, HALTED, PASSED, FAILED, ERROR, STOPPED); several are ORed. CommitHash
// must be a full 40-character hash.
type PipelineListOptions struct {
	Branch      string
	Statuses    []string
	CreatorUUID string
	CommitHash  string
}

// PipelineVariable is a variable for a new pipeline. Bitbucket hides secured values in logs and never
// returns any variable.
type PipelineVariable struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Secured bool   `json:"secured,omitempty"`
}

func pipelinePath(workspace, slug string, number int, segments ...string) string {
	return RepoPath(workspace, slug, append([]string{"pipelines", strconv.Itoa(number)}, segments...)...)
}

// ListPipelines lists a repository's pipelines, newest first (limit <= 0 means all). Bitbucket
// returns the oldest first unless asked to sort.
func (c *Client) ListPipelines(ctx context.Context, workspace, slug string, opts PipelineListOptions, limit int) ([]Pipeline, error) {
	q := url.Values{"sort": {"-created_on"}}
	if opts.Branch != "" {
		q.Set("target.branch", opts.Branch)
	}
	for _, s := range opts.Statuses {
		q.Add("status", s)
	}
	if opts.CreatorUUID != "" {
		q.Set("creator.uuid", opts.CreatorUUID)
	}
	if opts.CommitHash != "" {
		q.Set("target.commit.hash", opts.CommitHash)
	}
	return List[Pipeline](ctx, c, RepoPath(workspace, slug, "pipelines")+"?"+q.Encode(), limit)
}

// GetPipeline returns pipeline number (its build number).
func (c *Client) GetPipeline(ctx context.Context, workspace, slug string, number int) (*Pipeline, error) {
	var p Pipeline
	if err := c.Do(ctx, http.MethodGet, pipelinePath(workspace, slug, number), nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPipelineSteps returns every step of a pipeline.
func (c *Client) ListPipelineSteps(ctx context.Context, workspace, slug string, number int) ([]PipelineStep, error) {
	return List[PipelineStep](ctx, c, pipelinePath(workspace, slug, number, "steps"), 0)
}

// StepLog returns a step's log so far. Bitbucket answers 404 for a step that has not started or did not run.
func (c *Client) StepLog(ctx context.Context, workspace, slug string, number int, stepUUID string) (string, error) {
	return c.GetText(ctx, pipelinePath(workspace, slug, number, "steps", stepUUID, "log"))
}

// RunPipeline starts a pipeline on target, with optional variables.
func (c *Client) RunPipeline(ctx context.Context, workspace, slug string, target PipelineTarget, vars []PipelineVariable) (*Pipeline, error) {
	body := map[string]any{"target": target.requestBody()}
	if len(vars) > 0 {
		body["variables"] = vars
	}
	var p Pipeline
	if err := c.Do(ctx, http.MethodPost, RepoPath(workspace, slug, "pipelines"), body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// StopPipeline stops a running pipeline. Bitbucket accepts but ignores stopping a pipeline paused on a manual step.
func (c *Client) StopPipeline(ctx context.Context, workspace, slug string, number int) error {
	return c.Do(ctx, http.MethodPost, pipelinePath(workspace, slug, number, "stopPipeline"), nil, nil)
}

// requestBody returns the target in the form POST …/pipelines expects, keeping only the fields
// that say what to run — so a target read from an existing pipeline can be sent back to rerun it.
func (t PipelineTarget) requestBody() map[string]any {
	body := map[string]any{"type": t.Type}
	switch t.Type {
	case "pipeline_ref_target":
		body["ref_type"] = t.RefType
		body["ref_name"] = t.RefName
	case "pipeline_pullrequest_target":
		body["source"] = t.Source
		body["destination"] = t.Destination
		if t.DestinationCommit != nil {
			body["destination_commit"] = map[string]any{"hash": t.DestinationCommit.Hash}
		}
		if t.PullRequest != nil {
			body["pullrequest"] = map[string]any{"id": t.PullRequest.ID}
		}
	}
	if t.Commit != nil && t.Commit.Hash != "" {
		body["commit"] = map[string]any{"type": "commit", "hash": t.Commit.Hash}
	}
	if t.Selector != nil {
		selector := map[string]any{"type": t.Selector.Type}
		if t.Selector.Pattern != "" {
			selector["pattern"] = t.Selector.Pattern
		}
		body["selector"] = selector
	}
	return body
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/bitbucket/ && go vet ./internal/bitbucket/ && gofmt -l internal/bitbucket`
Expected: PASS; gofmt prints nothing.

- [ ] **Step 5: Commit**

```bash
git add internal/bitbucket/pipelines.go internal/bitbucket/pipelines_test.go
git commit -m "feat(bitbucket): pipelines, steps, logs, run and stop"
```

---
### Task 3: Pipeline statuses, JSON shapes, display helpers, variables and fixtures

**Files:**
- Create: `pkg/cmd/pipeline/shared/status.go`, `export.go`, `display.go`, `variables.go`, `pkg/cmd/pipeline/shared/ptest/ptest.go`
- Test: `pkg/cmd/pipeline/shared/status_test.go`, `export_test.go`, `display_test.go`, `variables_test.go`

**Interfaces:**
- Consumes: `bitbucket.Pipeline`, `PipelineState`, `PipelineStep`, `PipelineVariable` (Task 2); `prshared.User`, `prshared.NewUser` (`pkg/cmd/pr/shared`); `prtest.Ada`, `prtest.Page`; `gitctx.Repo`; `cmdutil.FlagErrorf`; go-gh `tableprinter`.
- Produces (package `shared` under `pkg/cmd/pipeline/shared`, imported by commands as `"github.com/khipu/khbb/pkg/cmd/pipeline/shared"`):
  - Constants `StatusPending, StatusRunning, StatusPaused, StatusSuccessful, StatusFailed, StatusError, StatusStopped, StatusExpired, StatusSkipped` (`"pending"` … `"skipped"`).
  - `Status(s bitbucket.PipelineState) string`, `Finished(status string) bool`, `Unsuccessful(status string) bool`, `var ListStatuses map[string][]string`.
  - `type Pipeline struct{Number int; UUID, Status, Trigger string; Creator *prshared.User; RefType, RefName, Commit string; Selector Selector; DurationSeconds int; URL string; CreatedOn time.Time; CompletedOn *time.Time}`, `type Selector struct{Type, Pattern string}`, `type Step struct{UUID, Name, Status string; StartedOn, CompletedOn *time.Time; DurationSeconds int}`, `var PipelineFields []string`, `URL(repo gitctx.Repo, n int) string`, `NewPipeline(p *bitbucket.Pipeline, repo gitctx.Repo) Pipeline`, `NewStep(s *bitbucket.PipelineStep) Step`.
  - `StatusLabel(ios, status string) string`, `FormatDuration(seconds int) string`, `ShortHash(hash string) string`, `RefLabel(p Pipeline) string`, `CreatorName(p Pipeline) string`, `RenderSummary(ios, p Pipeline, steps []Step, verbose bool) error`.
  - `ParseVariables(vars, secrets []string) ([]bitbucket.PipelineVariable, error)`.
  - Package `ptest` (`pkg/cmd/pipeline/shared/ptest`): `Pipelines` (`/2.0/repositories/acme/widgets/pipelines`), `Commit`, state constants `StatePending, StateRunning, StatePaused, StateSuccessful, StateFailed, StateStopped, StepInProgress, StepWaiting, StepNotRun`, `Pipeline(n int, state string) string`, `CustomPipeline(n int, state, name string) string`, `PullRequestPipeline(n int, state string) string`, `Step(uuid, name, state string) string`, `Steps(steps ...string) string`.

- [ ] **Step 1: Write the fixtures and the failing tests**

`pkg/cmd/pipeline/shared/ptest/ptest.go`:

```go
// Package ptest holds Bitbucket Pipelines fixtures for `khbb pipeline` tests. Every identity is fake.
package ptest

import (
	"fmt"
	"strings"

	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

// Pipelines is the request path of acme/widgets pipelines.
const Pipelines = "/2.0/repositories/acme/widgets/pipelines"

// Commit is the full hash every fixture pipeline runs on.
const Commit = "abc1234def5678abc1234def5678abc1234def56"

// Pipeline and step states, as Bitbucket sends them.
const (
	StatePending    = `{"name":"PENDING","stage":{"name":"PENDING"}}`
	StateRunning    = `{"name":"IN_PROGRESS","stage":{"name":"RUNNING"}}`
	StatePaused     = `{"name":"IN_PROGRESS","stage":{"name":"PAUSED"}}`
	StateSuccessful = `{"name":"COMPLETED","result":{"name":"SUCCESSFUL"}}`
	StateFailed     = `{"name":"COMPLETED","result":{"name":"FAILED"}}`
	StateStopped    = `{"name":"COMPLETED","result":{"name":"STOPPED"}}`
	StepInProgress  = `{"name":"IN_PROGRESS"}`
	StepWaiting     = `{"name":"PENDING","stage":{"name":"PAUSED"}}` // a manual step not started yet
	StepNotRun      = `{"name":"COMPLETED","result":{"name":"NOT_RUN"}}`
)

// Pipeline returns build n on branch main at Commit, pushed by ada, in state.
func Pipeline(n int, state string) string {
	return pipeline(n, state, `{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main",`+
		`"commit":{"type":"commit","hash":"`+Commit+`"},"selector":{"type":"branches","pattern":"main"}}`, "PUSH")
}

// CustomPipeline returns build n of the custom pipeline name on main, started by hand.
func CustomPipeline(n int, state, name string) string {
	return pipeline(n, state, `{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main",`+
		`"commit":{"type":"commit","hash":"`+Commit+`"},"selector":{"type":"custom","pattern":"`+name+`"}}`, "MANUAL")
}

// PullRequestPipeline returns build n of pull request 7 (feature/widgets → main).
func PullRequestPipeline(n int, state string) string {
	return pipeline(n, state, `{"type":"pipeline_pullrequest_target","source":"feature/widgets","destination":"main",`+
		`"destination_commit":{"type":"commit","hash":"def5678"},"commit":{"type":"commit","hash":"`+Commit+`"},`+
		`"pullrequest":{"id":7,"title":"Add widgets","draft":false},"selector":{"type":"pull-requests","pattern":"**"}}`, "PUSH")
}

func pipeline(n int, state, target, trigger string) string {
	completed, duration := "null", 0
	if strings.Contains(state, "COMPLETED") {
		completed, duration = `"2026-10-06T12:01:02.000000+00:00"`, 62
	}
	return fmt.Sprintf(`{"type":"pipeline","uuid":"{00000000-0000-0000-0000-%012d}","build_number":%d,"state":%s,"target":%s,`+
		`"trigger":{"name":"%s"},"creator":%s,"created_on":"2026-10-06T12:00:00.000000+00:00","completed_on":%s,"duration_in_seconds":%d}`,
		n, n, state, target, trigger, prtest.Ada, completed, duration)
}

// Step returns a step with the given UUID, name and state. Steps that ran took 12 s.
func Step(uuid, name, state string) string {
	ran := !strings.Contains(state, `"PENDING"`) && !strings.Contains(state, "NOT_RUN")
	started, completed, duration := "null", "null", 0
	if ran {
		started = `"2026-10-06T12:00:05.000000+00:00"`
	}
	if ran && strings.Contains(state, "COMPLETED") {
		completed, duration = `"2026-10-06T12:00:17.000000+00:00"`, 12
	}
	return fmt.Sprintf(`{"uuid":"%s","name":"%s","state":%s,"started_on":%s,"completed_on":%s,"duration_in_seconds":%d,`+
		`"trigger":{"type":"pipeline_step_trigger_automatic"}}`, uuid, name, state, started, completed, duration)
}

// Steps wraps step fixtures in a one-page collection.
func Steps(steps ...string) string {
	return prtest.Page(steps...)
}
```

`pkg/cmd/pipeline/shared/status_test.go`:

```go
package shared_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
)

func state(name, stage, result string) bitbucket.PipelineState {
	s := bitbucket.PipelineState{Name: name}
	if stage != "" {
		s.Stage = &bitbucket.Named{Name: stage}
	}
	if result != "" {
		s.Result = &bitbucket.Named{Name: result}
	}
	return s
}

func TestStatus(t *testing.T) {
	cases := []struct{ name, stage, result, want string }{
		{"PARSING", "", "", "pending"},
		{"PENDING", "PENDING", "", "pending"},
		{"READY", "", "", "pending"},
		{"PENDING", "PAUSED", "", "paused"},
		{"IN_PROGRESS", "RUNNING", "", "running"},
		{"IN_PROGRESS", "", "", "running"},
		{"IN_PROGRESS", "PAUSED", "", "paused"},
		{"COMPLETED", "", "SUCCESSFUL", "successful"},
		{"COMPLETED", "", "FAILED", "failed"},
		{"COMPLETED", "", "ERROR", "error"},
		{"COMPLETED", "", "STOPPED", "stopped"},
		{"COMPLETED", "", "EXPIRED", "expired"},
		{"COMPLETED", "", "NOT_RUN", "skipped"},
		{"COMPLETED", "", "SOMETHING_NEW", "something_new"},
		{"HALTED", "", "", "halted"},
	}
	for _, tc := range cases {
		if got := shared.Status(state(tc.name, tc.stage, tc.result)); got != tc.want {
			t.Errorf("Status(%s/%s/%s) = %q, want %q", tc.name, tc.stage, tc.result, got, tc.want)
		}
	}
}

func TestFinishedAndUnsuccessful(t *testing.T) {
	cases := []struct {
		status                 string
		finished, unsuccessful bool
	}{
		{"pending", false, false}, {"running", false, false}, {"paused", false, false},
		{"successful", true, false}, {"skipped", true, false},
		{"failed", true, true}, {"error", true, true}, {"stopped", true, true}, {"expired", true, true},
	}
	for _, tc := range cases {
		if shared.Finished(tc.status) != tc.finished || shared.Unsuccessful(tc.status) != tc.unsuccessful {
			t.Errorf("%s: Finished %v Unsuccessful %v", tc.status, shared.Finished(tc.status), shared.Unsuccessful(tc.status))
		}
	}
}

func TestListStatuses(t *testing.T) {
	want := []string{"error", "failed", "paused", "pending", "running", "stopped", "successful"}
	if got := slices.Sorted(maps.Keys(shared.ListStatuses)); !slices.Equal(got, want) {
		t.Errorf("keys = %v", got)
	}
	if !slices.Equal(shared.ListStatuses["successful"], []string{"PASSED"}) || !slices.Equal(shared.ListStatuses["running"], []string{"BUILDING"}) ||
		!slices.Equal(shared.ListStatuses["pending"], []string{"PENDING", "PARSING"}) || !slices.Equal(shared.ListStatuses["paused"], []string{"PAUSED", "HALTED"}) {
		t.Errorf("ListStatuses = %v", shared.ListStatuses)
	}
}
```

`pkg/cmd/pipeline/shared/export_test.go`:

```go
package shared_test

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
)

var repo = gitctx.Repo{Workspace: "acme", Slug: "widgets"}

func decodePipeline(t *testing.T, s string) *bitbucket.Pipeline {
	t.Helper()
	var p bitbucket.Pipeline
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		t.Fatal(err)
	}
	return &p
}

func TestNewPipeline(t *testing.T) {
	p := shared.NewPipeline(decodePipeline(t, ptest.Pipeline(42, ptest.StateFailed)), repo)
	completed := time.Date(2026, 10, 6, 12, 1, 2, 0, time.UTC)
	if p.Number != 42 || p.UUID != "{00000000-0000-0000-0000-000000000042}" || p.Status != "failed" || p.Trigger != "push" ||
		p.Creator == nil || p.Creator.Nickname != "ada" || p.RefType != "branch" || p.RefName != "main" || p.Commit != ptest.Commit ||
		p.Selector != (shared.Selector{Type: "branches", Pattern: "main"}) || p.DurationSeconds != 62 ||
		p.URL != "https://bitbucket.org/acme/widgets/pipelines/results/42" ||
		!p.CreatedOn.Equal(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)) || p.CompletedOn == nil || !p.CompletedOn.Equal(completed) {
		t.Errorf("NewPipeline = %+v", p)
	}
	running := shared.NewPipeline(decodePipeline(t, ptest.Pipeline(43, ptest.StateRunning)), repo)
	if running.Status != "running" || running.CompletedOn != nil {
		t.Errorf("running = %+v", running)
	}
}

func TestNewPipeline_OtherTargets(t *testing.T) {
	pr := shared.NewPipeline(decodePipeline(t, ptest.PullRequestPipeline(44, ptest.StateSuccessful)), repo)
	if pr.RefType != "pullrequest" || pr.RefName != "feature/widgets" || pr.Selector.Type != "pull-requests" {
		t.Errorf("pull request pipeline = %+v", pr)
	}
	raw := decodePipeline(t, ptest.Pipeline(45, ptest.StatePending))
	raw.Target = bitbucket.PipelineTarget{Type: "pipeline_commit_target", Commit: &bitbucket.Commit{Hash: "abc"}, Selector: &bitbucket.PipelineSelector{Type: "default"}}
	if c := shared.NewPipeline(raw, repo); c.RefType != "commit" || c.RefName != "" || c.Commit != "abc" {
		t.Errorf("commit pipeline = %+v", c)
	}
	raw.Target = bitbucket.PipelineTarget{Type: "pipeline_ref_target", RefType: "tag", RefName: "v1.2.0"}
	raw.Creator = nil
	if tag := shared.NewPipeline(raw, repo); tag.RefType != "tag" || tag.Commit != "" || tag.Selector != (shared.Selector{}) || tag.Creator != nil {
		t.Errorf("tag pipeline = %+v", tag)
	}
}

func TestNewStep(t *testing.T) {
	var raw bitbucket.PipelineStep
	if err := json.Unmarshal([]byte(ptest.Step("{s1}", "Build", ptest.StateSuccessful)), &raw); err != nil {
		t.Fatal(err)
	}
	s := shared.NewStep(&raw)
	if s.UUID != "{s1}" || s.Name != "Build" || s.Status != "successful" || s.DurationSeconds != 12 || s.StartedOn == nil || s.CompletedOn == nil {
		t.Errorf("NewStep = %+v", s)
	}
}

func TestPipelineFieldsMatchJSONKeys(t *testing.T) {
	b, _ := json.Marshal(shared.Pipeline{})
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if got, want := slices.Sorted(maps.Keys(m)), slices.Sorted(slices.Values(shared.PipelineFields)); !slices.Equal(got, want) {
		t.Errorf("JSON keys %v, PipelineFields %v", got, want)
	}
	if len(shared.PipelineFields) != 13 || shared.PipelineFields[0] != "number" {
		t.Errorf("PipelineFields = %v", shared.PipelineFields)
	}
}
```

`pkg/cmd/pipeline/shared/display_test.go`:

```go
package shared_test

import (
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
)

func TestFormatDuration(t *testing.T) {
	for seconds, want := range map[int]string{0: "0s", 45: "45s", 60: "1m00s", 185: "3m05s", 3725: "1h02m"} {
		if got := shared.FormatDuration(seconds); got != want {
			t.Errorf("FormatDuration(%d) = %q, want %q", seconds, got, want)
		}
	}
}

func TestRefLabel(t *testing.T) {
	cases := []struct {
		p    shared.Pipeline
		want string
	}{
		{shared.Pipeline{RefType: "branch", RefName: "main"}, "branch main"},
		{shared.Pipeline{RefType: "tag", RefName: "v1.2.0"}, "tag v1.2.0"},
		{shared.Pipeline{RefType: "pullrequest", RefName: "feature/widgets"}, "pull request from feature/widgets"},
		{shared.Pipeline{RefType: "commit", Commit: "abc1234def"}, "commit abc1234"},
		{shared.Pipeline{RefType: "branch", RefName: "main", Selector: shared.Selector{Type: "custom", Pattern: "deploy"}}, "custom pipeline deploy on branch main"},
	}
	for _, tc := range cases {
		if got := shared.RefLabel(tc.p); got != tc.want {
			t.Errorf("RefLabel(%+v) = %q, want %q", tc.p, got, tc.want)
		}
	}
}

func TestStatusLabel(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	for status, want := range map[string]string{"successful": "✓ successful", "failed": "X failed", "error": "X error",
		"stopped": "- stopped", "skipped": "- skipped", "paused": "‖ paused", "running": "* running", "pending": "* pending"} {
		if got := shared.StatusLabel(ios, status); got != want {
			t.Errorf("StatusLabel(%s) = %q, want %q", status, got, want)
		}
	}
}

func TestRenderSummary(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	p := shared.NewPipeline(decodePipeline(t, ptest.Pipeline(42, ptest.StateFailed)), repo)
	steps := []shared.Step{
		shared.NewStep(decodeStep(t, ptest.Step("{s1}", "Build", ptest.StateSuccessful))),
		shared.NewStep(decodeStep(t, ptest.Step("{s2}", "Deploy", ptest.StepNotRun))),
	}
	if err := shared.RenderSummary(ios, p, steps, true); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Pipeline #42 X failed\n", "branch main · push by ada · commit abc1234\n",
		"Started 2026-10-06 12:00 UTC · took 1m02s\n", "STEP", "Build", "✓ successful", "12s", "{s1}", "Deploy", "- skipped"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
}
```

Add to `export_test.go` (same package) the helper used above:

```go
func decodeStep(t *testing.T, s string) *bitbucket.PipelineStep {
	t.Helper()
	var step bitbucket.PipelineStep
	if err := json.Unmarshal([]byte(s), &step); err != nil {
		t.Fatal(err)
	}
	return &step
}
```

and add the import `"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"` to `display_test.go`.

`pkg/cmd/pipeline/shared/variables_test.go`:

```go
package shared_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
)

func TestParseVariables(t *testing.T) {
	vars, err := shared.ParseVariables([]string{"ENV=staging", "EMPTY=", "URL=https://example.com/?a=b"}, []string{"TOKEN=s3cret"})
	want := []bitbucket.PipelineVariable{
		{Key: "ENV", Value: "staging"}, {Key: "EMPTY", Value: ""}, {Key: "URL", Value: "https://example.com/?a=b"},
		{Key: "TOKEN", Value: "s3cret", Secured: true},
	}
	if err != nil || len(vars) != len(want) {
		t.Fatalf("vars %v err %v", vars, err)
	}
	for i := range want {
		if vars[i] != want[i] {
			t.Errorf("vars[%d] = %+v, want %+v", i, vars[i], want[i])
		}
	}
	var flagErr *cmdutil.FlagError
	if _, err := shared.ParseVariables([]string{"novalue"}, nil); !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "novalue") {
		t.Errorf("--var without '=': err = %v", err)
	}
	_, err = shared.ParseVariables(nil, []string{"ok=1", "pasted-s3cret-without-key"})
	if !errors.As(err, &flagErr) || strings.Contains(err.Error(), "s3cret") || !strings.Contains(err.Error(), "--secret-var number 2") {
		t.Errorf("a bad --secret-var must not be echoed: err = %v", err)
	}
	if vars, err := shared.ParseVariables(nil, nil); err != nil || vars == nil || len(vars) != 0 {
		t.Errorf("no variables: %#v %v", vars, err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pipeline/...`
Expected: FAIL — `undefined: shared.Status` and the other new names.

- [ ] **Step 3: Implement**

`pkg/cmd/pipeline/shared/status.go`:

```go
// Package shared holds what the `khbb pipeline` commands have in common: status normalization, JSON
// shapes, lookup, display and the watch loop.
package shared

import (
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
)

// Statuses reported for pipelines and steps (spec §7.2, plus expired and, for steps, skipped).
const (
	StatusPending    = "pending"
	StatusRunning    = "running"
	StatusPaused     = "paused"
	StatusSuccessful = "successful"
	StatusFailed     = "failed"
	StatusError      = "error"
	StatusStopped    = "stopped"
	StatusExpired    = "expired"
	StatusSkipped    = "skipped"
)

// ListStatuses maps each --status value of `pipeline list` to Bitbucket's own status filter values;
// several values are ORed.
var ListStatuses = map[string][]string{
	StatusPending:    {"PENDING", "PARSING"},
	StatusRunning:    {"BUILDING"},
	StatusPaused:     {"PAUSED", "HALTED"},
	StatusSuccessful: {"PASSED"},
	StatusFailed:     {"FAILED"},
	StatusError:      {"ERROR"},
	StatusStopped:    {"STOPPED"},
}

// Status normalizes Bitbucket's state, stage and result into one status. Unknown values pass
// through lowercased.
func Status(s bitbucket.PipelineState) string {
	paused := s.Stage != nil && s.Stage.Name == "PAUSED"
	switch s.Name {
	case "PARSING", "PENDING", "READY":
		if paused {
			return StatusPaused
		}
		return StatusPending
	case "IN_PROGRESS":
		if paused {
			return StatusPaused
		}
		return StatusRunning
	case "COMPLETED":
		if s.Result == nil {
			return "unknown"
		}
		switch s.Result.Name {
		case "SUCCESSFUL":
			return StatusSuccessful
		case "FAILED":
			return StatusFailed
		case "ERROR":
			return StatusError
		case "STOPPED":
			return StatusStopped
		case "EXPIRED":
			return StatusExpired
		case "NOT_RUN":
			return StatusSkipped
		}
		return strings.ToLower(s.Result.Name)
	}
	return strings.ToLower(s.Name)
}

// Finished reports whether status is final.
func Finished(status string) bool {
	switch status {
	case StatusSuccessful, StatusFailed, StatusError, StatusStopped, StatusExpired, StatusSkipped:
		return true
	}
	return false
}

// Unsuccessful reports whether a final status counts as a failure for exit codes.
func Unsuccessful(status string) bool {
	switch status {
	case StatusFailed, StatusError, StatusStopped, StatusExpired:
		return true
	}
	return false
}
```

`pkg/cmd/pipeline/shared/export.go`:

```go
package shared

import (
	"fmt"
	"strings"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/gitctx"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// Pipeline is the stable JSON shape of a pipeline (spec §8.1).
type Pipeline struct {
	Number          int            `json:"number"`
	UUID            string         `json:"uuid"`
	Status          string         `json:"status"`
	Trigger         string         `json:"trigger"`
	Creator         *prshared.User `json:"creator"`
	RefType         string         `json:"refType"`
	RefName         string         `json:"refName"`
	Commit          string         `json:"commit"`
	Selector        Selector       `json:"selector"`
	DurationSeconds int            `json:"durationSeconds"`
	URL             string         `json:"url"`
	CreatedOn       time.Time      `json:"createdOn"`
	CompletedOn     *time.Time     `json:"completedOn"`
}

// Selector names the definition in bitbucket-pipelines.yml that ran: Type is default, branches,
// tags, custom or pull-requests.
type Selector struct {
	Type    string `json:"type"`
	Pattern string `json:"pattern"`
}

// Step is the stable JSON shape of a pipeline step.
type Step struct {
	UUID            string     `json:"uuid"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	StartedOn       *time.Time `json:"startedOn"`
	CompletedOn     *time.Time `json:"completedOn"`
	DurationSeconds int        `json:"durationSeconds"`
}

// PipelineFields lists Pipeline's JSON fields in spec order.
var PipelineFields = []string{
	"number", "uuid", "status", "trigger", "creator", "refType", "refName", "commit", "selector",
	"durationSeconds", "url", "createdOn", "completedOn",
}

// URL returns the web page of pipeline n (Bitbucket's API has no HTML link for pipelines).
func URL(repo gitctx.Repo, n int) string {
	return fmt.Sprintf("https://bitbucket.org/%s/%s/pipelines/results/%d", repo.Workspace, repo.Slug, n)
}

// NewPipeline maps an API pipeline of repo. Pull-request pipelines report refType "pullrequest"
// with the source branch; commit pipelines report refType "commit" with no name.
func NewPipeline(p *bitbucket.Pipeline, repo gitctx.Repo) Pipeline {
	out := Pipeline{
		Number:          p.BuildNumber,
		UUID:            p.UUID,
		Status:          Status(p.State),
		Trigger:         strings.ToLower(p.Trigger.Name),
		Creator:         prshared.NewUser(p.Creator),
		DurationSeconds: p.DurationInSeconds,
		URL:             URL(repo, p.BuildNumber),
		CreatedOn:       p.CreatedOn.UTC(),
		CompletedOn:     utc(p.CompletedOn),
	}
	t := p.Target
	switch t.Type {
	case "pipeline_pullrequest_target":
		out.RefType, out.RefName = "pullrequest", t.Source
	case "pipeline_commit_target":
		out.RefType = "commit"
	default:
		out.RefType, out.RefName = t.RefType, t.RefName
	}
	if t.Commit != nil {
		out.Commit = t.Commit.Hash
	}
	if t.Selector != nil {
		out.Selector = Selector{Type: t.Selector.Type, Pattern: t.Selector.Pattern}
	}
	return out
}

// NewStep maps an API step.
func NewStep(s *bitbucket.PipelineStep) Step {
	return Step{
		UUID:            s.UUID,
		Name:            s.Name,
		Status:          Status(s.State),
		StartedOn:       utc(s.StartedOn),
		CompletedOn:     utc(s.CompletedOn),
		DurationSeconds: s.DurationInSeconds,
	}
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
```

`pkg/cmd/pipeline/shared/display.go`:

```go
package shared

import (
	"fmt"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"

	"github.com/khipu/khbb/internal/iostreams"
)

// StatusLabel renders a status for humans: a symbol and the status, colored.
func StatusLabel(ios *iostreams.IOStreams, status string) string {
	switch status {
	case StatusSuccessful:
		return ios.Green("✓ " + status)
	case StatusFailed, StatusError:
		return ios.Red("X " + status)
	case StatusStopped, StatusExpired, StatusSkipped:
		return ios.Gray("- " + status)
	case StatusPaused:
		return ios.Yellow("‖ " + status)
	}
	return ios.Yellow("* " + status)
}

// FormatDuration renders seconds as 45s, 3m05s or 1h02m.
func FormatDuration(seconds int) string {
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh%02dm", seconds/3600, seconds%3600/60)
}

// ShortHash abbreviates a commit hash to 7 characters.
func ShortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// RefLabel says what a pipeline ran on, such as "branch main", "pull request from feature/x" or
// "custom pipeline deploy on branch main".
func RefLabel(p Pipeline) string {
	var label string
	switch p.RefType {
	case "pullrequest":
		label = "pull request from " + p.RefName
	case "commit":
		label = "commit " + ShortHash(p.Commit)
	default:
		label = p.RefType + " " + p.RefName
	}
	if p.Selector.Type == "custom" {
		label = "custom pipeline " + p.Selector.Pattern + " on " + label
	}
	return label
}

// CreatorName returns the creator's nickname, or "unknown" when Bitbucket sends none.
func CreatorName(p Pipeline) string {
	if p.Creator == nil || p.Creator.Nickname == "" {
		return "unknown"
	}
	return p.Creator.Nickname
}

// RenderSummary prints a pipeline's summary and step table for humans; verbose adds each step's
// UUID and timestamps.
func RenderSummary(ios *iostreams.IOStreams, p Pipeline, steps []Step, verbose bool) error {
	w := ios.Out
	fmt.Fprintf(w, "%s %s\n", ios.Bold(fmt.Sprintf("Pipeline #%d", p.Number)), StatusLabel(ios, p.Status))
	fmt.Fprintf(w, "%s · %s by %s · commit %s\n", RefLabel(p), p.Trigger, CreatorName(p), ShortHash(p.Commit))
	started := "Started " + p.CreatedOn.Format("2006-01-02 15:04 MST")
	if p.CompletedOn != nil {
		started += " · took " + FormatDuration(p.DurationSeconds)
	}
	fmt.Fprintf(w, "%s\n\n", started)
	tp := tableprinter.New(w, true, ios.TerminalWidth())
	header := []string{"STEP", "STATUS", "DURATION"}
	if verbose {
		header = append(header, "UUID", "STARTED", "COMPLETED")
	}
	tp.AddHeader(header)
	for _, s := range steps {
		tp.AddField(s.Name)
		tp.AddField(StatusLabel(ios, s.Status))
		tp.AddField(stepDuration(s))
		if verbose {
			tp.AddField(s.UUID)
			tp.AddField(timeOrDash(s.StartedOn))
			tp.AddField(timeOrDash(s.CompletedOn))
		}
		tp.EndRow()
	}
	return tp.Render()
}

func stepDuration(s Step) string {
	if s.StartedOn == nil {
		return "-"
	}
	return FormatDuration(s.DurationSeconds)
}

func timeOrDash(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
}
```

`pkg/cmd/pipeline/shared/variables.go`:

```go
package shared

import (
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
)

// ParseVariables turns --var and --secret-var values (KEY=VALUE) into pipeline variables, secured
// for --secret-var. An error about a --secret-var never repeats its text, which may be a secret.
func ParseVariables(vars, secrets []string) ([]bitbucket.PipelineVariable, error) {
	out := make([]bitbucket.PipelineVariable, 0, len(vars)+len(secrets))
	for _, v := range vars {
		key, value, ok := strings.Cut(v, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, cmdutil.FlagErrorf("invalid --var %q: expected KEY=VALUE", v)
		}
		out = append(out, bitbucket.PipelineVariable{Key: key, Value: value})
	}
	for i, v := range secrets {
		key, value, ok := strings.Cut(v, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, cmdutil.FlagErrorf("invalid --secret-var number %d: expected KEY=VALUE", i+1)
		}
		out = append(out, bitbucket.PipelineVariable{Key: key, Value: value, Secured: true})
	}
	return out, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/cmd/pipeline/... && go vet ./pkg/cmd/pipeline/... && gofmt -l pkg/cmd/pipeline`
Expected: PASS; gofmt prints nothing.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pipeline/
git commit -m "feat(pipeline): statuses, JSON shapes, display helpers and fixtures"
```

---

### Task 4: Pipeline lookup and the watch loop

**Files:**
- Create: `pkg/cmd/pipeline/shared/finder.go`, `pkg/cmd/pipeline/shared/watch.go`
- Test: `pkg/cmd/pipeline/shared/finder_test.go`, `pkg/cmd/pipeline/shared/watch_test.go`

**Interfaces:**
- Consumes: `Client.ListPipelines`, `GetPipeline`, `ListPipelineSteps`, `bitbucket.IsTransient`; Task 3's `NewPipeline`, `NewStep`, `Status`, `Unsuccessful`, `FormatDuration`, `RenderSummary`, `StatusPaused`; `cmdutil.NotFoundError`, `cmdutil.ExitError`, `cmdutil.FlagErrorf`; `gitctx.ParseRepo`; test helpers `prtest.NewFactory`, `prtest.SetBranch`, `ptest`.
- Produces:
  - `ParseSelector(s string) (int, *gitctx.Repo, error)` — `""` → 0; `"42"`, `"#42"`; `https://bitbucket.org/<ws>/<repo>/pipelines/results/<n>[/…]` → n and the repo; anything else → usage error.
  - `type Finder struct{Client *bitbucket.Client; BaseRepo func() (gitctx.Repo, error); Branch func() (string, error)}` with `Find(ctx, selector string) (*bitbucket.Pipeline, gitctx.Repo, error)` — 0 means the newest pipeline of the current branch (`not_found` "no pipelines found for branch "x" in acme/widgets").
  - `type WatchOptions struct{IO *iostreams.IOStreams; Client *bitbucket.Client; Repo gitctx.Repo; Number int; Interval time.Duration; Sleep func(time.Duration)}`, `Watch(ctx, o WatchOptions) (Pipeline, []Step, error)`, `ExitStatus(p Pipeline) error` (`&cmdutil.ExitError{Code: 1}` when `Unsuccessful`), `PausedStep(steps []Step) string`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pipeline/shared/finder_test.go`:

```go
package shared_test

import (
	"context"
	"errors"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
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
		n    int
		repo string
	}{
		{"", 0, ""},
		{"42", 42, ""},
		{" #42 ", 42, ""},
		{"https://bitbucket.org/acme/gears/pipelines/results/7", 7, "acme/gears"},
		{"https://bitbucket.org/acme/gears/pipelines/results/7/steps/{x}", 7, "acme/gears"},
	}
	for _, tc := range cases {
		n, repo, err := shared.ParseSelector(tc.in)
		got := ""
		if repo != nil {
			got = repo.FullName()
		}
		if err != nil || n != tc.n || got != tc.repo {
			t.Errorf("ParseSelector(%q) = %d, %q, %v", tc.in, n, got, err)
		}
	}
	for _, bad := range []string{"abc", "0", "#-1", "https://bitbucket.org/acme/gears/pull-requests/7"} {
		var flagErr *cmdutil.FlagError
		if _, _, err := shared.ParseSelector(bad); !errors.As(err, &flagErr) {
			t.Errorf("ParseSelector(%q): err = %v", bad, err)
		}
	}
}

func TestFind_ByNumber(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, ptest.StateFailed)))

	p, repo, err := newFinder(reg, "feature/widgets").Find(context.Background(), "#42")
	if err != nil || p.BuildNumber != 42 || repo.FullName() != "acme/widgets" {
		t.Errorf("p %v repo %v err %v", p, repo, err)
	}
}

func TestFind_ByURLUsesThatRepository(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/repositories/acme/gears/pipelines/7", httpmock.JSONResponse(200, ptest.Pipeline(7, ptest.StateSuccessful)))

	_, repo, err := newFinder(reg, "").Find(context.Background(), "https://bitbucket.org/acme/gears/pipelines/results/7")
	if err != nil || repo.FullName() != "acme/gears" {
		t.Errorf("repo %v err %v", repo, err)
	}
}

func TestFind_NewestOfTheCurrentBranch(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page(ptest.Pipeline(43, ptest.StateRunning))))

	p, _, err := newFinder(reg, "feature/widgets").Find(context.Background(), "")
	if err != nil || p.BuildNumber != 43 {
		t.Fatalf("p %v err %v", p, err)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("target.branch") != "feature/widgets" || q.Get("sort") != "-created_on" || q.Get("pagelen") != "1" {
		t.Errorf("query = %v", q)
	}
}

func TestFind_NoPipelineForTheBranch(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))

	_, _, err := newFinder(reg, "feature/widgets").Find(context.Background(), "")
	var notFound *cmdutil.NotFoundError
	if !errors.As(err, &notFound) || err.Error() != `no pipelines found for branch "feature/widgets" in acme/widgets` {
		t.Errorf("err = %v", err)
	}
}
```

`pkg/cmd/pipeline/shared/watch_test.go`:

```go
package shared_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func watchOptions(reg *httpmock.Registry, tty bool) (shared.WatchOptions, *bytes.Buffer, *[]time.Duration) {
	f, ios, out, _ := prtest.NewFactory(reg)
	ios.SetStdoutTTY(tty)
	client, _ := f.HTTPClient()
	slept := &[]time.Duration{}
	return shared.WatchOptions{
		IO: ios, Client: client, Repo: gitctx.Repo{Workspace: "acme", Slug: "widgets"}, Number: 42,
		Interval: 5 * time.Second, Sleep: func(d time.Duration) { *slept = append(*slept, d) },
	}, out, slept
}

func poll(reg *httpmock.Registry, state string, steps ...string) {
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, state)))
	reg.Register("GET", ptest.Pipelines+"/42/steps", httpmock.JSONResponse(200, ptest.Steps(steps...)))
}

func TestWatch_PrintsStateChanges(t *testing.T) {
	reg := httpmock.New(t)
	poll(reg, ptest.StatePending, ptest.Step("{s1}", "Build", ptest.StatePending), ptest.Step("{s2}", "Test", ptest.StatePending))
	poll(reg, ptest.StateRunning, ptest.Step("{s1}", "Build", ptest.StepInProgress), ptest.Step("{s2}", "Test", ptest.StatePending))
	poll(reg, ptest.StateFailed, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed))
	opts, out, slept := watchOptions(reg, false)

	p, steps, err := shared.Watch(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	want := `#42 step "Build": pending
#42 step "Test": pending
#42 pending
#42 step "Build": running
#42 running
#42 step "Build": successful (12s)
#42 step "Test": failed (12s)
#42 failed (1m02s)
`
	if out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
	if p.Status != "failed" || len(steps) != 2 || !slices.Equal(*slept, []time.Duration{5 * time.Second, 5 * time.Second}) {
		t.Errorf("p %+v steps %d slept %v", p, len(steps), *slept)
	}
	var exitErr *cmdutil.ExitError
	if err := shared.ExitStatus(p); !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Errorf("ExitStatus = %v", err)
	}
}

func TestWatch_StopsWhenPaused(t *testing.T) {
	reg := httpmock.New(t)
	poll(reg, ptest.StatePaused, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Deploy", ptest.StepWaiting))
	opts, out, slept := watchOptions(reg, false)

	p, steps, err := shared.Watch(context.Background(), opts)
	if err != nil || p.Status != "paused" || len(*slept) != 0 {
		t.Fatalf("p %+v err %v slept %v", p, err, *slept)
	}
	if !strings.HasSuffix(out.String(), "#42 paused (waiting for manual step \"Deploy\")\n") || shared.PausedStep(steps) != "Deploy" {
		t.Errorf("out = %q", out.String())
	}
	if err := shared.ExitStatus(p); err != nil {
		t.Errorf("a paused pipeline is not a failure: %v", err)
	}
}

func TestWatch_TTYRedraws(t *testing.T) {
	reg := httpmock.New(t)
	poll(reg, ptest.StateRunning, ptest.Step("{s1}", "Build", ptest.StepInProgress))
	poll(reg, ptest.StateSuccessful, ptest.Step("{s1}", "Build", ptest.StateSuccessful))
	opts, out, _ := watchOptions(reg, true)

	p, _, err := shared.Watch(context.Background(), opts)
	if err != nil || p.Status != "successful" {
		t.Fatalf("p %+v err %v", p, err)
	}
	s := out.String()
	if strings.Count(s, "\x1b[H\x1b[2J") != 2 || strings.Count(s, "Refreshing every 5s; press Ctrl-C to stop.") != 1 ||
		!strings.Contains(s, "Pipeline #42 ✓ successful") {
		t.Errorf("out = %q", s)
	}
	if err := shared.ExitStatus(p); err != nil {
		t.Errorf("ExitStatus = %v", err)
	}
}

func TestWatch_RetriesTransientErrors(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(503, `{"type":"error","error":{"message":"Service unavailable"}}`))
	poll(reg, ptest.StateSuccessful, ptest.Step("{s1}", "Build", ptest.StateSuccessful))
	opts, _, slept := watchOptions(reg, false)

	if p, _, err := shared.Watch(context.Background(), opts); err != nil || p.Status != "successful" || len(*slept) != 1 {
		t.Errorf("p %+v err %v slept %v", p, err, *slept)
	}
}

func TestWatch_GivesUpAfterFiveTransientErrors(t *testing.T) {
	reg := httpmock.New(t)
	for range 5 {
		reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(502, `{"type":"error","error":{"message":"Bad gateway"}}`))
	}
	opts, _, slept := watchOptions(reg, false)

	_, _, err := shared.Watch(context.Background(), opts)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 502 || len(*slept) != 4 {
		t.Errorf("err %v slept %v", err, *slept)
	}
}

func TestWatch_StopsOnNotFound(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(404, `{"type":"error","error":{"message":"Not found","detail":"Pipeline with build number '42' not found"}}`))
	opts, _, slept := watchOptions(reg, false)

	_, _, err := shared.Watch(context.Background(), opts)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 || len(*slept) != 0 {
		t.Errorf("err %v slept %v", err, *slept)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pipeline/shared/`
Expected: FAIL — `undefined: shared.ParseSelector`, `undefined: shared.Watch`.

- [ ] **Step 3: Implement**

`pkg/cmd/pipeline/shared/finder.go`:

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

var pipelineURLRE = regexp.MustCompile(`^https?://(?:www\.)?bitbucket\.org/([^/]+)/([^/]+)/pipelines/results/(\d+)(?:[/?#].*)?$`)

// ParseSelector reads a pipeline argument: "" (the newest pipeline of the current branch), "42",
// "#42" or a pipeline URL, which also names the repository.
func ParseSelector(s string) (int, *gitctx.Repo, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil, nil
	}
	if m := pipelineURLRE.FindStringSubmatch(s); m != nil {
		repo, err := gitctx.ParseRepo(m[1] + "/" + m[2])
		n, _ := strconv.Atoi(m[3])
		if err != nil || n <= 0 {
			return 0, nil, cmdutil.FlagErrorf("invalid pipeline URL %q", s)
		}
		return n, &repo, nil
	}
	n, err := strconv.Atoi(strings.TrimPrefix(s, "#"))
	if err != nil || n <= 0 {
		return 0, nil, cmdutil.FlagErrorf("invalid pipeline %q: expected a build number, #number or a pipeline URL", s)
	}
	return n, nil, nil
}

// Finder locates the pipeline a command acts on.
type Finder struct {
	Client   *bitbucket.Client
	BaseRepo func() (gitctx.Repo, error)
	Branch   func() (string, error)
}

// Find returns the pipeline named by selector and its repository. An empty selector means the
// newest pipeline of the current branch.
func (f *Finder) Find(ctx context.Context, selector string) (*bitbucket.Pipeline, gitctx.Repo, error) {
	n, urlRepo, err := ParseSelector(selector)
	if err != nil {
		return nil, gitctx.Repo{}, err
	}
	var repo gitctx.Repo
	if urlRepo != nil {
		repo = *urlRepo
	} else if repo, err = f.BaseRepo(); err != nil {
		return nil, gitctx.Repo{}, err
	}
	if n > 0 {
		p, err := f.Client.GetPipeline(ctx, repo.Workspace, repo.Slug, n)
		return p, repo, err
	}
	branch, err := f.Branch()
	if err != nil {
		return nil, repo, err
	}
	ps, err := f.Client.ListPipelines(ctx, repo.Workspace, repo.Slug, bitbucket.PipelineListOptions{Branch: branch}, 1)
	if err != nil {
		return nil, repo, err
	}
	if len(ps) == 0 {
		return nil, repo, &cmdutil.NotFoundError{Msg: fmt.Sprintf("no pipelines found for branch %q in %s", branch, repo.FullName())}
	}
	return &ps[0], repo, nil
}
```

`pkg/cmd/pipeline/shared/watch.go`:

```go
package shared

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
)

// maxConsecutiveErrors is how many failed polls in a row Watch accepts: it gives up on the 5th
// consecutive transient failure (spec §10).
const maxConsecutiveErrors = 5

const clearScreen = "\x1b[H\x1b[2J"

// WatchOptions configures Watch.
type WatchOptions struct {
	IO       *iostreams.IOStreams
	Client   *bitbucket.Client
	Repo     gitctx.Repo
	Number   int
	Interval time.Duration
	Sleep    func(time.Duration)
}

// Watch polls pipeline o.Number until it completes or pauses on a manual step, and returns its last
// state. On a terminal it redraws the summary on every poll; otherwise it prints one line per
// pipeline or step status change. Transient errors (429, 5xx, network) are retried; others end it.
func Watch(ctx context.Context, o WatchOptions) (Pipeline, []Step, error) {
	live := o.IO.IsStdoutTTY()
	seen := map[string]string{}
	errorsInARow := 0
	for {
		raw, steps, err := fetch(ctx, o)
		if err != nil {
			errorsInARow++
			if !bitbucket.IsTransient(err) || errorsInARow >= maxConsecutiveErrors {
				return Pipeline{}, nil, err
			}
			o.Sleep(o.Interval)
			continue
		}
		errorsInARow = 0
		p := NewPipeline(raw, o.Repo)
		done := raw.State.Name == "COMPLETED" || p.Status == StatusPaused
		if live {
			fmt.Fprint(o.IO.Out, clearScreen)
			if err := RenderSummary(o.IO, p, steps, false); err != nil {
				return Pipeline{}, nil, err
			}
			if name := PausedStep(steps); p.Status == StatusPaused && name != "" {
				fmt.Fprintf(o.IO.Out, "\nWaiting for manual step %q: start it in Bitbucket.\n", name)
			}
			if !done {
				fmt.Fprintf(o.IO.Out, "\nRefreshing every %s; press Ctrl-C to stop.\n", o.Interval)
			}
		} else {
			printChanges(o.IO.Out, p, steps, seen)
		}
		if done {
			return p, steps, nil
		}
		o.Sleep(o.Interval)
	}
}

// ExitStatus turns an unsuccessful final status into exit code 1 (`watch --exit-status`, `run --watch`).
func ExitStatus(p Pipeline) error {
	if Unsuccessful(p.Status) {
		return &cmdutil.ExitError{Code: 1}
	}
	return nil
}

// PausedStep returns the name of the step waiting to be started by hand, or "".
func PausedStep(steps []Step) string {
	for _, s := range steps {
		if s.Status == StatusPaused {
			return s.Name
		}
	}
	return ""
}

func fetch(ctx context.Context, o WatchOptions) (*bitbucket.Pipeline, []Step, error) {
	raw, err := o.Client.GetPipeline(ctx, o.Repo.Workspace, o.Repo.Slug, o.Number)
	if err != nil {
		return nil, nil, err
	}
	rawSteps, err := o.Client.ListPipelineSteps(ctx, o.Repo.Workspace, o.Repo.Slug, o.Number)
	if err != nil {
		return nil, nil, err
	}
	steps := make([]Step, len(rawSteps))
	for i := range rawSteps {
		steps[i] = NewStep(&rawSteps[i])
	}
	return raw, steps, nil
}

// printChanges writes a line for each step, then the pipeline, whose status changed since the
// last poll; seen keeps the last status printed per step UUID ("" for the pipeline).
func printChanges(w io.Writer, p Pipeline, steps []Step, seen map[string]string) {
	for _, s := range steps {
		if seen[s.UUID] == s.Status {
			continue
		}
		seen[s.UUID] = s.Status
		line := fmt.Sprintf("#%d step %q: %s", p.Number, s.Name, s.Status)
		if s.CompletedOn != nil {
			line += " (" + FormatDuration(s.DurationSeconds) + ")"
		}
		fmt.Fprintln(w, line)
	}
	if seen[""] == p.Status {
		return
	}
	seen[""] = p.Status
	line := fmt.Sprintf("#%d %s", p.Number, p.Status)
	switch {
	case p.Status == StatusPaused && PausedStep(steps) != "":
		line += fmt.Sprintf(" (waiting for manual step %q)", PausedStep(steps))
	case p.CompletedOn != nil:
		line += " (" + FormatDuration(p.DurationSeconds) + ")"
	}
	fmt.Fprintln(w, line)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -race ./pkg/cmd/pipeline/... && go vet ./pkg/cmd/pipeline/... && gofmt -l pkg/cmd/pipeline`
Expected: PASS; gofmt prints nothing.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pipeline/shared/
git commit -m "feat(pipeline): pipeline lookup and watch loop"
```

---
### Task 5: `khbb pipeline` group and `pipeline list`

**Files:**
- Create: `pkg/cmd/pipeline/pipeline.go`, `pkg/cmd/pipeline/list/list.go`, `pkg/cmd/pipeline/list/list_test.go`
- Modify: `pkg/cmd/root/root.go` (register the group), `pkg/cmd/root/root_test.go`

**Interfaces:**
- Consumes: `Client.ListPipelines`, `Client.CurrentUser`, `bitbucket.PipelineListOptions`; Task 3's `shared.ListStatuses`, `NewPipeline`, `PipelineFields`, `StatusLabel`, `RefLabel`, `FormatDuration`; `cmdutil.AddJSONFlags`, `EnableRepoOverride`, `GroupRunE`, `NoArgs`; go-gh `tableprinter`; `prtest.NewFactory`, `prtest.Run`, `prtest.Page`, `prtest.Ada`, `prtest.AdaUUID`; `ptest`.
- Produces: `pipeline.NewCmdPipeline(f *cmdutil.Factory) *cobra.Command` (later tasks add their commands to its `AddCommand` list); `list.ListOptions`, `list.NewCmdList(f, runF)`. Terminal table columns `STATUS, NUMBER, RUN ON, TRIGGER, DURATION, STARTED`; without a terminal, tab-separated `number, status, refType, refName, trigger, durationSeconds, createdOn` (no header).

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pipeline/list/list_test.go`:

```go
package list

import (
	"errors"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func run(reg *httpmock.Registry, tty bool, args ...string) (string, string, error) {
	f, ios, out, errOut := prtest.NewFactory(reg)
	ios.SetStdoutTTY(tty)
	err := prtest.Run(NewCmdList(f, nil), args...)
	return out.String(), errOut.String(), err
}

func twoPipelines(reg *httpmock.Registry) {
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200,
		prtest.Page(ptest.Pipeline(43, ptest.StateRunning), ptest.Pipeline(42, ptest.StateFailed))))
}

func TestList_TTYTable(t *testing.T) {
	reg := httpmock.New(t)
	twoPipelines(reg)
	out, _, err := run(reg, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"STATUS", "RUN ON", "* running", "#43", "branch main", "push", "X failed", "#42", "1m02s", "2026-10-06 12:00"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestList_NonTTYIsTabSeparated(t *testing.T) {
	reg := httpmock.New(t)
	twoPipelines(reg)
	out, _, err := run(reg, false)
	want := "43\trunning\tbranch\tmain\tpush\t0\t2026-10-06T12:00:00Z\n" +
		"42\tfailed\tbranch\tmain\tpush\t62\t2026-10-06T12:00:00Z\n"
	if err != nil || out != want {
		t.Errorf("out %q err %v", out, err)
	}
	if q := reg.Calls[0].URL.Query(); q.Get("sort") != "-created_on" || q.Get("pagelen") != "20" || q.Has("status") || q.Has("target.branch") {
		t.Errorf("query = %v", q)
	}
}

func TestList_Filters(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))
	if _, _, err := run(reg, false, "-b", "feature/widgets", "-s", "Successful", "--mine", "-L", "5"); err != nil {
		t.Fatal(err)
	}
	q := reg.Calls[1].URL.Query()
	if q.Get("target.branch") != "feature/widgets" || q.Get("status") != "PASSED" || q.Get("creator.uuid") != prtest.AdaUUID || q.Get("pagelen") != "5" {
		t.Errorf("query = %v", q)
	}
}

func TestList_PendingIsTwoFilterValues(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))
	if _, _, err := run(reg, false, "--status", "pending"); err != nil {
		t.Fatal(err)
	}
	if got := reg.Calls[0].URL.Query()["status"]; len(got) != 2 || got[0] != "PENDING" || got[1] != "PARSING" {
		t.Errorf("status = %v", got)
	}
}

func TestList_JSON(t *testing.T) {
	reg := httpmock.New(t)
	twoPipelines(reg)
	out, _, err := run(reg, false, "--json", "number,status,url")
	want := `[{"number":43,"status":"running","url":"https://bitbucket.org/acme/widgets/pipelines/results/43"},` +
		`{"number":42,"status":"failed","url":"https://bitbucket.org/acme/widgets/pipelines/results/42"}]` + "\n"
	if err != nil || out != want {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestList_EmptyResults(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))
	out, errOut, err := run(reg, true)
	if err != nil || out != "" || errOut != "No pipelines match your filters in acme/widgets\n" {
		t.Errorf("tty: out %q stderr %q err %v", out, errOut, err)
	}
	out, errOut, err = run(reg, false, "--json", "number")
	if err != nil || out != "[]\n" || errOut != "" {
		t.Errorf("json: out %q stderr %q err %v", out, errOut, err)
	}
}

func TestList_FlagErrors(t *testing.T) {
	for _, args := range [][]string{{"-s", "done"}, {"-L", "0"}, {"extra"}} {
		_, _, err := run(httpmock.New(t), false, args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
```

Append to `pkg/cmd/root/root_test.go`:

```go
func TestRootRegistersPipelineGroup(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{"pipeline", "lisst"})
	var flagErr *cmdutil.FlagError
	if err := cmd.Execute(); !errors.As(err, &flagErr) || !strings.Contains(err.Error(), `unknown command "lisst" for "khbb pipeline"`) {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pipeline/... ./pkg/cmd/root/`
Expected: FAIL — `undefined: NewCmdList`; the root test fails with `unknown command "pipeline"`.

- [ ] **Step 3: Implement**

`pkg/cmd/pipeline/pipeline.go`:

```go
// Package pipeline groups the `khbb pipeline` commands.
package pipeline

import (
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/pipeline/list"
)

// NewCmdPipeline returns `khbb pipeline`.
func NewCmdPipeline(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pipeline <command>",
		Short: "Work with Bitbucket Pipelines",
		Long: `List, inspect, run, stop and watch pipelines. A pipeline is named by its build number (42 or
#42) or its URL; commands that take one default to the newest pipeline of the current branch.`,
		Args: cobra.ArbitraryArgs,
		RunE: cmdutil.GroupRunE,
	}
	cmdutil.EnableRepoOverride(cmd, f)
	cmd.AddCommand(
		list.NewCmdList(f, nil),
	)
	return cmd
}
```

`pkg/cmd/pipeline/list/list.go`:

```go
// Package list implements `khbb pipeline list`.
package list

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
)

// ListOptions holds the inputs and dependencies of `khbb pipeline list`.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Exporter   cmdutil.Exporter

	Branch string
	Status string
	Mine   bool
	Limit  int
}

// NewCmdList returns `khbb pipeline list`.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo}
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List pipelines in a repository",
		Long:    "List a repository's newest pipelines, optionally only those of a branch, with a status, or that you started.",
		Example: `  $ khbb pipeline list
  $ khbb pipeline list --branch main --status failed --limit 5
  $ khbb pipeline list --mine --json number,status,refName,url`,
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Status = strings.ToLower(opts.Status)
			if _, ok := shared.ListStatuses[opts.Status]; opts.Status != "" && !ok {
				return cmdutil.FlagErrorf("invalid --status %q: use pending, running, paused, successful, failed, error or stopped", opts.Status)
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
	fl.StringVarP(&opts.Branch, "branch", "b", "", "Only pipelines of this `branch`")
	fl.StringVarP(&opts.Status, "status", "s", "", "Only pipelines with this `status`: pending, running, paused, successful, failed, error or stopped")
	fl.BoolVar(&opts.Mine, "mine", false, "Only pipelines you started")
	fl.IntVarP(&opts.Limit, "limit", "L", 20, "Maximum number of pipelines to fetch")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PipelineFields)
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
	filter := bitbucket.PipelineListOptions{Branch: opts.Branch, Statuses: shared.ListStatuses[opts.Status]}
	if opts.Mine {
		me, err := client.CurrentUser(ctx)
		if err != nil {
			return err
		}
		filter.CreatorUUID = me.UUID
	}
	raws, err := client.ListPipelines(ctx, repo.Workspace, repo.Slug, filter, opts.Limit)
	if err != nil {
		return err
	}
	pipelines := make([]shared.Pipeline, len(raws))
	for i := range raws {
		pipelines[i] = shared.NewPipeline(&raws[i], repo)
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, pipelines)
	}
	if len(pipelines) == 0 {
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "No pipelines match your filters in %s\n", repo.FullName())
		}
		return nil
	}
	return printTable(opts.IO, pipelines)
}

func printTable(ios *iostreams.IOStreams, pipelines []shared.Pipeline) error {
	tty := ios.IsStdoutTTY()
	tp := tableprinter.New(ios.Out, tty, ios.TerminalWidth())
	tp.AddHeader([]string{"STATUS", "NUMBER", "RUN ON", "TRIGGER", "DURATION", "STARTED"})
	for _, p := range pipelines {
		if tty {
			tp.AddField(shared.StatusLabel(ios, p.Status))
			tp.AddField("#" + strconv.Itoa(p.Number))
			tp.AddField(shared.RefLabel(p))
			tp.AddField(p.Trigger)
			tp.AddField(duration(p))
			tp.AddField(p.CreatedOn.Format("2006-01-02 15:04"))
		} else {
			tp.AddField(strconv.Itoa(p.Number))
			tp.AddField(p.Status)
			tp.AddField(p.RefType)
			tp.AddField(p.RefName)
			tp.AddField(p.Trigger)
			tp.AddField(strconv.Itoa(p.DurationSeconds))
			tp.AddField(p.CreatedOn.Format(time.RFC3339))
		}
		tp.EndRow()
	}
	return tp.Render()
}

func duration(p shared.Pipeline) string {
	if p.CompletedOn == nil {
		return "-"
	}
	return shared.FormatDuration(p.DurationSeconds)
}
```

In `pkg/cmd/root/root.go`, import `pipelineCmd "github.com/khipu/khbb/pkg/cmd/pipeline"` and add `pipelineCmd.NewCmdPipeline(f),` to `cmd.AddCommand(...)` after `prCmd.NewCmdPR(f),`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/... && go vet ./pkg/... && gofmt -l pkg && go build -o bin/khbb ./cmd/khbb && ./bin/khbb pipeline list --help`
Expected: PASS; gofmt prints nothing; the help lists `-b, --branch`, `-s, --status`, `--mine`, `-L, --limit`, `-R, --repo`.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pipeline/ pkg/cmd/root/
git commit -m "feat(pipeline): pipeline group and pipeline list"
```

---

### Task 6: `khbb pipeline view`

**Files:**
- Create: `pkg/cmd/pipeline/view/view.go`, `pkg/cmd/pipeline/view/view_test.go`
- Modify: `pkg/cmd/pipeline/pipeline.go`

**Interfaces:**
- Consumes: `shared.Finder`, `NewPipeline`, `NewStep`, `RenderSummary`, `RefLabel`, `CreatorName`, `PipelineFields`; `Client.ListPipelineSteps`; `prshared.CheckRepoSelector` (`pkg/cmd/pr/shared`); `cmdutil.Browser`, `AddJSONFlags`, `MaximumNArgs`; `prtest`, `ptest`.
- Produces: `view.ViewOptions`, `view.NewCmdView(f, runF)`. JSON fields: `PipelineFields` plus `steps`. Without a terminal: `number, status, run on, trigger, creator, commit, duration, url` as `key:\tvalue` lines, `--`, then one tab-separated line per step (`name, status, durationSeconds`, plus `uuid, startedOn, completedOn` with `--verbose`).

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pipeline/view/view_test.go`:

```go
package view

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const pipelineURL = "https://bitbucket.org/acme/widgets/pipelines/results/42"

type fakeBrowser struct{ urls []string }

func (b *fakeBrowser) Browse(u string) error {
	b.urls = append(b.urls, u)
	return nil
}

func failedPipeline(reg *httpmock.Registry) {
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, ptest.StateFailed)))
	reg.Register("GET", ptest.Pipelines+"/42/steps", httpmock.JSONResponse(200, ptest.Steps(
		ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed))))
}

func run(reg *httpmock.Registry, tty bool, args ...string) (string, string, error) {
	f, ios, out, errOut := prtest.NewFactory(reg)
	ios.SetStdoutTTY(tty)
	err := prtest.Run(NewCmdView(f, nil), args...)
	return out.String(), errOut.String(), err
}

func TestView_TTY(t *testing.T) {
	reg := httpmock.New(t)
	failedPipeline(reg)
	out, _, err := run(reg, true, "42")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Pipeline #42 X failed", "branch main · push by ada · commit abc1234", "Build", "✓ successful", "Test", "X failed", "12s"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestView_NonTTY(t *testing.T) {
	reg := httpmock.New(t)
	failedPipeline(reg)
	out, _, err := run(reg, false, "#42")
	want := "number:\t42\nstatus:\tfailed\nrun on:\tbranch main\ntrigger:\tpush\ncreator:\tada\ncommit:\t" + ptest.Commit +
		"\nduration:\t62\nurl:\t" + pipelineURL + "\n--\nBuild\tsuccessful\t12\nTest\tfailed\t12\n"
	if err != nil || out != want {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestView_Verbose(t *testing.T) {
	reg := httpmock.New(t)
	failedPipeline(reg)
	out, _, err := run(reg, false, "42", "-v")
	if err != nil || !strings.Contains(out, "Build\tsuccessful\t12\t{s1}\t2026-10-06T12:00:05Z\t2026-10-06T12:00:17Z\n") {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestView_JSONWithSteps(t *testing.T) {
	reg := httpmock.New(t)
	failedPipeline(reg)
	out, _, err := run(reg, false, "42", "--json", "number,status,steps")
	want := `{"number":42,"status":"failed","steps":[` +
		`{"completedOn":"2026-10-06T12:00:17Z","durationSeconds":12,"name":"Build","startedOn":"2026-10-06T12:00:05Z","status":"successful","uuid":"{s1}"},` +
		`{"completedOn":"2026-10-06T12:00:17Z","durationSeconds":12,"name":"Test","startedOn":"2026-10-06T12:00:05Z","status":"failed","uuid":"{s2}"}]}` + "\n"
	if err != nil || out != want {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestView_NewestOfTheCurrentBranch(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page(ptest.Pipeline(43, ptest.StateRunning))))
	reg.Register("GET", ptest.Pipelines+"/43/steps", httpmock.JSONResponse(200, ptest.Steps(ptest.Step("{s1}", "Build", ptest.StepInProgress))))
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")

	if err := prtest.Run(NewCmdView(f, nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "number:\t43\nstatus:\trunning\n") || reg.Calls[0].URL.Query().Get("target.branch") != "feature/widgets" {
		t.Errorf("out %q", out.String())
	}
}

func TestView_Web(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, ptest.StateFailed)))
	f, ios, out, errOut := prtest.NewFactory(reg)
	ios.SetStderrTTY(true)
	b := &fakeBrowser{}
	f.Browser = b

	if err := prtest.Run(NewCmdView(f, nil), "42", "--web"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.urls, []string{pipelineURL}) || out.Len() != 0 || errOut.String() != "Opening "+pipelineURL+" in your browser.\n" {
		t.Errorf("urls %v out %q stderr %q", b.urls, out.String(), errOut.String())
	}
}

func TestView_FlagErrors(t *testing.T) {
	for _, args := range [][]string{{"42", "--web", "--json", "number"}, {"1", "2"}, {"abc"}} {
		f, _, _, _ := prtest.NewFactory(httpmock.New(t))
		f.Browser = &fakeBrowser{}
		err := prtest.Run(NewCmdView(f, nil), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pipeline/view/`
Expected: FAIL — `undefined: NewCmdView`.

- [ ] **Step 3: Implement**

`pkg/cmd/pipeline/view/view.go`:

```go
// Package view implements `khbb pipeline view`.
package view

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// ViewOptions holds the inputs and dependencies of `khbb pipeline view`.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Browser    cmdutil.Browser
	Exporter   cmdutil.Exporter

	Selector string
	Verbose  bool
	Web      bool
}

var viewFields = append(slices.Clone(shared.PipelineFields), "steps")

type viewExport struct {
	shared.Pipeline
	Steps []shared.Step `json:"steps"`
}

// NewCmdView returns `khbb pipeline view`.
func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Browser: f.Browser}
	cmd := &cobra.Command{
		Use:   "view [<number> | <url>]",
		Short: "Show a pipeline and its steps",
		Long:  "Show a pipeline's status, what it ran on, who started it and its steps. Without an argument, show the newest pipeline of the current branch.",
		Example: `  $ khbb pipeline view 42
  $ khbb pipeline view --verbose
  $ khbb pipeline view 42 --json status,steps`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prshared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			if _, _, err := shared.ParseSelector(opts.Selector); err != nil {
				return err
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
	cmd.Flags().BoolVarP(&opts.Verbose, "verbose", "v", false, "Show step UUIDs and timestamps")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open the pipeline in the browser")
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
	p := shared.NewPipeline(raw, repo)
	if opts.Web {
		if opts.IO.IsStderrTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", p.URL)
		}
		return opts.Browser.Browse(p.URL)
	}
	rawSteps, err := client.ListPipelineSteps(ctx, repo.Workspace, repo.Slug, p.Number)
	if err != nil {
		return err
	}
	steps := make([]shared.Step, len(rawSteps))
	for i := range rawSteps {
		steps[i] = shared.NewStep(&rawSteps[i])
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, viewExport{Pipeline: p, Steps: steps})
	}
	if opts.IO.IsStdoutTTY() {
		return shared.RenderSummary(opts.IO, p, steps, opts.Verbose)
	}
	printRaw(opts.IO.Out, p, steps, opts.Verbose)
	return nil
}

func printRaw(w io.Writer, p shared.Pipeline, steps []shared.Step, verbose bool) {
	fmt.Fprintf(w, "number:\t%d\nstatus:\t%s\nrun on:\t%s\ntrigger:\t%s\ncreator:\t%s\ncommit:\t%s\nduration:\t%d\nurl:\t%s\n--\n",
		p.Number, p.Status, shared.RefLabel(p), p.Trigger, shared.CreatorName(p), p.Commit, p.DurationSeconds, p.URL)
	for _, s := range steps {
		fields := []string{s.Name, s.Status, strconv.Itoa(s.DurationSeconds)}
		if verbose {
			fields = append(fields, s.UUID, rfc3339(s.StartedOn), rfc3339(s.CompletedOn))
		}
		fmt.Fprintln(w, strings.Join(fields, "\t"))
	}
}

func rfc3339(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
```

In `pkg/cmd/pipeline/pipeline.go`, import `"github.com/khipu/khbb/pkg/cmd/pipeline/view"` and add `view.NewCmdView(f, nil),` to `cmd.AddCommand(...)`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/cmd/pipeline/... && go vet ./pkg/cmd/pipeline/... && gofmt -l pkg/cmd/pipeline`
Expected: PASS; gofmt prints nothing.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pipeline/
git commit -m "feat(pipeline): pipeline view with steps, --verbose and --web"
```

---

### Task 7: `khbb pipeline logs`

**Files:**
- Create: `pkg/cmd/pipeline/logs/logs.go`, `pkg/cmd/pipeline/logs/logs_test.go`
- Modify: `pkg/cmd/pipeline/pipeline.go`

**Interfaces:**
- Consumes: `shared.Finder`, `shared.Status`, status constants; `Client.ListPipelineSteps`, `Client.StepLog`, `bitbucket.HTTPError`; `prshared.CheckRepoSelector`; `cmdutil.NotFoundError`; `prtest`, `ptest`.
- Produces: `logs.LogsOptions`, `logs.NewCmdLogs(f, runF)`. Flags `-s/--step <name|uuid>`, `--failed`, `--tail <lines>`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pipeline/logs/logs_test.go`:

```go
package logs

import (
	"errors"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func pipelineWithSteps(reg *httpmock.Registry, steps ...string) {
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, ptest.StateFailed)))
	reg.Register("GET", ptest.Pipelines+"/42/steps", httpmock.JSONResponse(200, ptest.Steps(steps...)))
}

func stepLog(reg *httpmock.Registry, uuid, text string) {
	reg.Register("GET", ptest.Pipelines+"/42/steps/"+uuid+"/log", httpmock.StringResponse(200, text))
}

func run(reg *httpmock.Registry, args ...string) (string, string, error) {
	f, _, out, errOut := prtest.NewFactory(reg)
	err := prtest.Run(NewCmdLogs(f, nil), append([]string{"42"}, args...)...)
	return out.String(), errOut.String(), err
}

func TestLogs_AllStepsWithHeaders(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed),
		ptest.Step("{s3}", "Deploy", ptest.StepNotRun))
	stepLog(reg, "{s1}", "build ok\n")
	stepLog(reg, "{s2}", "test failed")

	out, errOut, err := run(reg)
	if err != nil {
		t.Fatal(err)
	}
	if out != "==> Build <==\nbuild ok\n==> Test <==\ntest failed\n" || errOut != "no log for step \"Deploy\" (skipped)\n" {
		t.Errorf("out %q stderr %q", out, errOut)
	}
}

func TestLogs_SkipsStepsWithoutALog(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed),
		ptest.Step("{s3}", "Gate", ptest.StepWaiting))
	reg.Register("GET", ptest.Pipelines+"/42/steps/{s1}/log", httpmock.JSONResponse(404,
		`{"error":{"message":"Not Found","detail":"Log in step {s1} does not exist."}}`))
	stepLog(reg, "{s2}", "test failed\n")

	out, errOut, err := run(reg)
	if err != nil || out != "==> Test <==\ntest failed\n" || errOut != "no log for step \"Build\"\nno log for step \"Gate\" (paused)\n" {
		t.Errorf("out %q stderr %q err %v", out, errOut, err)
	}
}

func TestLogs_FailedOnly(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed))
	stepLog(reg, "{s2}", "test failed\n")

	out, _, err := run(reg, "--failed")
	if err != nil || out != "test failed\n" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestLogs_NoFailedSteps(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful))

	out, errOut, err := run(reg, "--failed")
	if err != nil || out != "" || errOut != "no failed steps in pipeline #42\n" {
		t.Errorf("out %q stderr %q err %v", out, errOut, err)
	}
}

func TestLogs_StepByNameWithTail(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Unit tests", ptest.StateFailed))
	stepLog(reg, "{s2}", "a\nb\nc\n")

	out, _, err := run(reg, "--step", "unit TESTS", "--tail", "2")
	if err != nil || out != "b\nc\n" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestLogs_StepByUUID(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed))
	stepLog(reg, "{s1}", "build ok\n")

	if out, _, err := run(reg, "-s", "{s1}"); err != nil || out != "build ok\n" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestLogs_UnknownStep(t *testing.T) {
	reg := httpmock.New(t)
	pipelineWithSteps(reg, ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed))

	_, _, err := run(reg, "--step", "nope")
	var notFound *cmdutil.NotFoundError
	if !errors.As(err, &notFound) || err.Error() != `pipeline #42 has no step "nope"; its steps are: Build, Test` {
		t.Errorf("err = %v", err)
	}
}

func TestLogs_FlagErrors(t *testing.T) {
	for _, args := range [][]string{{"--step", "Build", "--failed"}, {"--tail", "0"}} {
		_, _, err := run(httpmock.New(t), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pipeline/logs/`
Expected: FAIL — `undefined: NewCmdLogs`.

- [ ] **Step 3: Implement**

`pkg/cmd/pipeline/logs/logs.go`:

```go
// Package logs implements `khbb pipeline logs`.
package logs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// LogsOptions holds the inputs and dependencies of `khbb pipeline logs`.
type LogsOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)

	Selector string
	Step     string
	Failed   bool
	Tail     int
}

// NewCmdLogs returns `khbb pipeline logs`.
func NewCmdLogs(f *cmdutil.Factory, runF func(*LogsOptions) error) *cobra.Command {
	opts := &LogsOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch}
	cmd := &cobra.Command{
		Use:   "logs [<number> | <url>]",
		Short: "Show the logs of a pipeline's steps",
		Long: `Print the logs of a pipeline's steps: every step that has a log, each after a "==> <step> <=="
header when there are several, only --step (a name or UUID), or only the --failed ones. Steps
that have not run have no log; they are skipped with a notice on stderr. Without an argument,
use the newest pipeline of the current branch.`,
		Example: `  $ khbb pipeline logs 42 --failed
  $ khbb pipeline logs --step Build --tail 50`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prshared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			switch {
			case opts.Step != "" && opts.Failed:
				return cmdutil.FlagErrorf("--step cannot be combined with --failed")
			case cmd.Flags().Changed("tail") && opts.Tail < 1:
				return cmdutil.FlagErrorf("invalid --tail %d: must be at least 1", opts.Tail)
			}
			if runF != nil {
				return runF(opts)
			}
			return logsRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Step, "step", "s", "", "Only the step with this `name` or UUID")
	cmd.Flags().BoolVar(&opts.Failed, "failed", false, "Only the steps that failed")
	cmd.Flags().IntVar(&opts.Tail, "tail", 0, "Only the last `lines` of each log")
	return cmd
}

func logsRun(ctx context.Context, opts *LogsOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	p, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	steps, err := client.ListPipelineSteps(ctx, repo.Workspace, repo.Slug, p.BuildNumber)
	if err != nil {
		return err
	}
	selected, err := selectSteps(p.BuildNumber, steps, opts)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Fprintf(opts.IO.ErrOut, "no failed steps in pipeline #%d\n", p.BuildNumber)
		return nil
	}
	headers := len(selected) > 1
	for _, s := range selected {
		status := shared.Status(s.State)
		if status == shared.StatusPending || status == shared.StatusPaused || status == shared.StatusSkipped {
			fmt.Fprintf(opts.IO.ErrOut, "no log for step %q (%s)\n", s.Name, status)
			continue
		}
		text, err := client.StepLog(ctx, repo.Workspace, repo.Slug, p.BuildNumber, s.UUID)
		var httpErr *bitbucket.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			fmt.Fprintf(opts.IO.ErrOut, "no log for step %q\n", s.Name)
			continue
		}
		if err != nil {
			return err
		}
		if headers {
			fmt.Fprintf(opts.IO.Out, "==> %s <==\n", s.Name)
		}
		if err := writeLog(opts.IO.Out, lastLines(text, opts.Tail)); err != nil {
			return err
		}
	}
	return nil
}

// selectSteps applies --step and --failed.
func selectSteps(number int, steps []bitbucket.PipelineStep, opts *LogsOptions) ([]bitbucket.PipelineStep, error) {
	switch {
	case opts.Step != "":
		for _, s := range steps {
			if strings.EqualFold(s.Name, opts.Step) || s.UUID == opts.Step {
				return []bitbucket.PipelineStep{s}, nil
			}
		}
		names := make([]string, len(steps))
		for i, s := range steps {
			names[i] = s.Name
		}
		return nil, &cmdutil.NotFoundError{Msg: fmt.Sprintf("pipeline #%d has no step %q; its steps are: %s", number, opts.Step, strings.Join(names, ", "))}
	case opts.Failed:
		var failed []bitbucket.PipelineStep
		for _, s := range steps {
			if status := shared.Status(s.State); status == shared.StatusFailed || status == shared.StatusError {
				failed = append(failed, s)
			}
		}
		return failed, nil
	}
	return steps, nil
}

// lastLines keeps the last n lines of text, or all of it when n <= 0.
func lastLines(text string, n int) string {
	if n <= 0 {
		return text
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= n {
		return text
	}
	return strings.Join(lines[len(lines)-n:], "")
}

// writeLog writes text and ends it with a newline if it lacks one.
func writeLog(w io.Writer, text string) error {
	if _, err := io.WriteString(w, text); err != nil {
		return err
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		_, err := io.WriteString(w, "\n")
		return err
	}
	return nil
}
```

Note: with no filter, a pipeline whose steps all lack logs prints only notices; `len(selected) == 0` can only happen with `--failed`.

In `pkg/cmd/pipeline/pipeline.go`, import `"github.com/khipu/khbb/pkg/cmd/pipeline/logs"` and add `logs.NewCmdLogs(f, nil),` to `cmd.AddCommand(...)`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/cmd/pipeline/... && go vet ./pkg/cmd/pipeline/... && gofmt -l pkg/cmd/pipeline`
Expected: PASS; gofmt prints nothing.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pipeline/
git commit -m "feat(pipeline): pipeline logs with --step, --failed and --tail"
```

---
### Task 8: `khbb pipeline watch`

**Files:**
- Create: `pkg/cmd/pipeline/watch/watch.go`, `pkg/cmd/pipeline/watch/watch_test.go`
- Modify: `pkg/cmd/pipeline/pipeline.go`

**Interfaces:**
- Consumes: `shared.Watch`, `WatchOptions`, `ExitStatus`, `Finder`, `ParseSelector`, `ShortHash`; `Client.ListPipelines` with `CommitHash`; `gitctx.Resolver.Git`; `prshared.CheckRepoSelector`; `cmdutil.NotFoundError`; `prtest.FakeGit`, `prtest.SetGit`, `prtest.SetBranch`; `ptest`.
- Produces: `watch.WatchOptions` (with injectable `Sleep`), `watch.NewCmdWatch(f, runF)`, `watchRun(ctx, opts)`. Flags `-i/--interval <seconds>` (default 5), `--exit-status`. Without an argument: the current branch's pipeline for `git rev-parse HEAD`, polled every interval for up to 60 s (stderr `Waiting for a pipeline for commit abc1234 on branch <b>…` once), then `not_found`; without a HEAD (no git), the newest pipeline of the branch.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pipeline/watch/watch_test.go`:

```go
package watch

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

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
	var slept []time.Duration
	cmd := NewCmdWatch(f, func(o *WatchOptions) error {
		o.Sleep = func(d time.Duration) { slept = append(slept, d) }
		return watchRun(context.Background(), o)
	})
	err := prtest.Run(cmd, args...)
	return result{out.String(), errOut.String(), slept, err}
}

func finished(reg *httpmock.Registry, n int, state string) {
	path := ptest.Pipelines + "/" + strconv.Itoa(n)
	reg.Register("GET", path, httpmock.JSONResponse(200, ptest.Pipeline(n, state)))
	reg.Register("GET", path+"/steps", httpmock.JSONResponse(200, ptest.Steps(ptest.Step("{s1}", "Build", state))))
}

func headGit(f *cmdutil.Factory) {
	prtest.SetGit(f, &prtest.FakeGit{Outputs: map[string]string{
		"symbolic-ref --quiet --short HEAD": "feature/widgets",
		"rev-parse HEAD":                    ptest.Commit,
	}})
}

func TestWatch_ByNumberWithExitStatus(t *testing.T) {
	reg := httpmock.New(t)
	finished(reg, 42, ptest.StateFailed)
	r := run(t, reg, nil, "42", "--exit-status")
	var exitErr *cmdutil.ExitError
	if !errors.As(r.err, &exitErr) || exitErr.Code != 1 || !strings.HasSuffix(r.out, "#42 failed (1m02s)\n") {
		t.Errorf("err %v out %q", r.err, r.out)
	}
	if r := run(t, finishedReg(t, 42, ptest.StateFailed), nil, "42"); r.err != nil {
		t.Errorf("without --exit-status a failed pipeline exits 0: %v", r.err)
	}
}

func finishedReg(t *testing.T, n int, state string) *httpmock.Registry {
	reg := httpmock.New(t)
	finished(reg, n, state)
	return reg
}

func TestWatch_FollowsTheHeadCommit(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page(ptest.Pipeline(43, ptest.StatePending))))
	finished(reg, 43, ptest.StateSuccessful)

	r := run(t, reg, headGit, "--exit-status")
	if r.err != nil {
		t.Fatal(r.err)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("target.branch") != "feature/widgets" || q.Get("target.commit.hash") != ptest.Commit || q.Get("pagelen") != "1" {
		t.Errorf("query = %v", q)
	}
	if r.errOut != "Waiting for a pipeline for commit abc1234 on branch feature/widgets…\n" ||
		!slices.Equal(r.slept, []time.Duration{5 * time.Second}) || !strings.HasSuffix(r.out, "#43 successful (1m02s)\n") {
		t.Errorf("stderr %q slept %v out %q", r.errOut, r.slept, r.out)
	}
}

func TestWatch_NoPipelineForTheHeadCommit(t *testing.T) {
	reg := httpmock.New(t)
	// Polls at 0 s, 5 s, …, 60 s: 13 lists and 12 sleeps before giving up.
	for range 13 {
		reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))
	}
	r := run(t, reg, headGit)
	var notFound *cmdutil.NotFoundError
	if !errors.As(r.err, &notFound) || len(r.slept) != 12 ||
		r.err.Error() != `no pipeline started for commit abc1234 on branch "feature/widgets" within 1m0s; push it, or name a pipeline number` {
		t.Errorf("err %v slept %d", r.err, len(r.slept))
	}
}

func TestWatch_WithoutAHeadUsesTheNewestOfTheBranch(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page(ptest.Pipeline(43, ptest.StateRunning))))
	finished(reg, 43, ptest.StateSuccessful)

	r := run(t, reg, func(f *cmdutil.Factory) { prtest.SetBranch(f, "feature/widgets") })
	if r.err != nil {
		t.Fatal(r.err)
	}
	if q := reg.Calls[0].URL.Query(); q.Has("target.commit.hash") || q.Get("target.branch") != "feature/widgets" {
		t.Errorf("query = %v", q)
	}
}

func TestWatch_FlagErrors(t *testing.T) {
	for _, args := range [][]string{{"42", "--interval", "0"}, {"abc"}, {"1", "2"}} {
		r := run(t, httpmock.New(t), nil, args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(r.err, &flagErr) {
			t.Errorf("%v: err = %v", args, r.err)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pipeline/watch/`
Expected: FAIL — `undefined: NewCmdWatch`.

- [ ] **Step 3: Implement**

`pkg/cmd/pipeline/watch/watch.go`:

```go
// Package watch implements `khbb pipeline watch`.
package watch

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// headWait is how long watch waits, after a push, for a pipeline of the local HEAD commit to start.
const headWait = 60 * time.Second

// WatchOptions holds the inputs and dependencies of `khbb pipeline watch`.
type WatchOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Git        *gitctx.Resolver
	Sleep      func(time.Duration)

	Selector   string
	Interval   int
	ExitStatus bool
}

// NewCmdWatch returns `khbb pipeline watch`.
func NewCmdWatch(f *cmdutil.Factory, runF func(*WatchOptions) error) *cobra.Command {
	opts := &WatchOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Git: f.Git, Sleep: time.Sleep}
	cmd := &cobra.Command{
		Use:   "watch [<number> | <url>]",
		Short: "Watch a pipeline until it finishes",
		Long: `Follow a pipeline until it finishes or pauses on a manual step. On a terminal the summary is
redrawn every interval; otherwise one line is printed per pipeline or step status change.

Without an argument, watch the current branch's pipeline for your local HEAD commit, waiting up
to 60 seconds for it to start after a push.

With --exit-status, exit with status 1 when the pipeline ends failed, error, stopped or expired.`,
		Example: `  $ git push && khbb pipeline watch --exit-status
  $ khbb pipeline watch 42 --interval 10`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prshared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			if _, _, err := shared.ParseSelector(opts.Selector); err != nil {
				return err
			}
			if opts.Interval < 1 {
				return cmdutil.FlagErrorf("invalid --interval %d: must be at least 1 second", opts.Interval)
			}
			if runF != nil {
				return runF(opts)
			}
			return watchRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().IntVarP(&opts.Interval, "interval", "i", 5, "Refresh interval in `seconds`")
	cmd.Flags().BoolVar(&opts.ExitStatus, "exit-status", false, "Exit with status 1 if the pipeline does not succeed")
	return cmd
}

func watchRun(ctx context.Context, opts *WatchOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	interval := time.Duration(opts.Interval) * time.Second
	number, repo, err := resolve(ctx, client, opts, interval)
	if err != nil {
		return err
	}
	p, _, err := shared.Watch(ctx, shared.WatchOptions{
		IO: opts.IO, Client: client, Repo: repo, Number: number, Interval: interval, Sleep: opts.Sleep,
	})
	if err != nil {
		return err
	}
	if opts.ExitStatus {
		return shared.ExitStatus(p)
	}
	return nil
}

// resolve returns the pipeline to watch: the selector's, or the current branch's pipeline for the
// local HEAD commit — right after a push, the newest pipeline of the branch is often the previous one.
// A pipeline named by number is not fetched here: Watch fetches it.
func resolve(ctx context.Context, client *bitbucket.Client, opts *WatchOptions, interval time.Duration) (int, gitctx.Repo, error) {
	n, urlRepo, err := shared.ParseSelector(opts.Selector)
	if err != nil {
		return 0, gitctx.Repo{}, err
	}
	if n > 0 {
		if urlRepo != nil {
			return n, *urlRepo, nil
		}
		repo, err := opts.BaseRepo()
		return n, repo, err
	}
	head := localHead(opts.Git)
	if head == "" {
		finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
		p, repo, err := finder.Find(ctx, "")
		if err != nil {
			return 0, repo, err
		}
		return p.BuildNumber, repo, nil
	}
	repo, err := opts.BaseRepo()
	if err != nil {
		return 0, repo, err
	}
	branch, err := opts.Branch()
	if err != nil {
		return 0, repo, err
	}
	for waited := time.Duration(0); ; waited += interval {
		ps, err := client.ListPipelines(ctx, repo.Workspace, repo.Slug, bitbucket.PipelineListOptions{Branch: branch, CommitHash: head}, 1)
		if err != nil {
			return 0, repo, err
		}
		if len(ps) > 0 {
			return ps[0].BuildNumber, repo, nil
		}
		if waited >= headWait {
			return 0, repo, &cmdutil.NotFoundError{Msg: fmt.Sprintf("no pipeline started for commit %s on branch %q within %s; push it, or name a pipeline number",
				shared.ShortHash(head), branch, headWait)}
		}
		if waited == 0 {
			fmt.Fprintf(opts.IO.ErrOut, "Waiting for a pipeline for commit %s on branch %s…\n", shared.ShortHash(head), branch)
		}
		opts.Sleep(interval)
	}
}

// localHead returns the full hash of the local HEAD commit, or "" without git.
func localHead(git *gitctx.Resolver) string {
	if git == nil {
		return ""
	}
	head, err := git.Git("rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return head
}
```

In `pkg/cmd/pipeline/pipeline.go`, import `"github.com/khipu/khbb/pkg/cmd/pipeline/watch"` and add `watch.NewCmdWatch(f, nil),` to `cmd.AddCommand(...)`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -race ./pkg/cmd/pipeline/... && go vet ./pkg/cmd/pipeline/... && gofmt -l pkg/cmd/pipeline`
Expected: PASS; gofmt prints nothing.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pipeline/
git commit -m "feat(pipeline): pipeline watch, following the pushed commit"
```

---

### Task 9: `khbb pipeline run`

**Files:**
- Create: `pkg/cmd/pipeline/shared/started.go`, `pkg/cmd/pipeline/run/run.go`, `pkg/cmd/pipeline/run/run_test.go`
- Modify: `pkg/cmd/pipeline/pipeline.go`

**Interfaces:**
- Consumes: `Client.RunPipeline`, `bitbucket.PipelineTarget`, `PipelineSelector`, `Commit`; `shared.ParseVariables`, `NewPipeline`, `RefLabel`, `Watch`, `ExitStatus`, `PipelineFields`; `prshared.PrintSuccess`; `cmdutil.AddDryRunFlag`, `AddJSONFlags`, `EnableRepoOverride` (tests); `prtest`, `ptest`.
- Produces:
  - `shared.ReportStarted(ios *iostreams.IOStreams, exporter cmdutil.Exporter, p Pipeline, what string) error` — stderr `Started pipeline #43 (<what>)`, then the URL on stdout (or the Pipeline JSON).
  - `run.RunOptions` (dependency `CurrentBranch func() (string, error)`, injectable `Sleep`), `run.NewCmdRun(f, runF)`. Flags `-b/--branch`, `--commit`, `--tag`, `--custom`, `--var` and `--secret-var` (repeatable `KEY=VALUE`), `--watch`, `--dry-run`, `--json`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pipeline/run/run_test.go`:

```go
package run

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const url43 = "https://bitbucket.org/acme/widgets/pipelines/results/43"

// started is a freshly created pipeline: Bitbucket fills in the selector only after parsing.
var started = strings.Replace(ptest.Pipeline(43, ptest.StatePending), `"selector":{"type":"branches","pattern":"main"}`, `"selector":null`, 1)

func run(t *testing.T, reg *httpmock.Registry, args ...string) (string, string, error) {
	t.Helper()
	f, _, out, errOut := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")
	cmd := NewCmdRun(f, func(o *RunOptions) error {
		o.Sleep = func(time.Duration) {}
		return runRun(context.Background(), o)
	})
	err := prtest.Run(cmd, args...)
	return out.String(), errOut.String(), err
}

func created(reg *httpmock.Registry) {
	reg.Register("POST", ptest.Pipelines, httpmock.JSONResponse(201, started))
}

func TestRun_CurrentBranch(t *testing.T) {
	reg := httpmock.New(t)
	created(reg)
	out, errOut, err := run(t, reg)
	if err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[0], `{"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"feature/widgets"}}`)
	if out != url43+"\n" || errOut != "Started pipeline #43 (branch main)\n" {
		t.Errorf("out %q stderr %q", out, errOut)
	}
}

func TestRun_CustomWithVariables(t *testing.T) {
	reg := httpmock.New(t)
	created(reg)
	_, errOut, err := run(t, reg, "-b", "main", "--custom", "deploy", "--var", "ENV=staging", "--secret-var", "TOKEN=s3cret")
	if err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[0], `{"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main",`+
		`"selector":{"type":"custom","pattern":"deploy"}},"variables":[{"key":"ENV","value":"staging"},{"key":"TOKEN","value":"s3cret","secured":true}]}`)
	if errOut != "Started pipeline #43 (custom pipeline deploy on branch main)\n" {
		t.Errorf("stderr %q", errOut)
	}
}

func TestRun_CommitAndTag(t *testing.T) {
	reg := httpmock.New(t)
	created(reg)
	created(reg)
	if _, _, err := run(t, reg, "--commit", "abc1234"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[0], `{"target":{"type":"pipeline_commit_target","commit":{"type":"commit","hash":"abc1234"},"selector":{"type":"default"}}}`)
	if _, _, err := run(t, reg, "--tag", "v1.2.0"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[1], `{"target":{"type":"pipeline_ref_target","ref_type":"tag","ref_name":"v1.2.0"}}`)
}

func TestRun_DryRunMasksSecrets(t *testing.T) {
	reg := httpmock.New(t)
	out, _, err := run(t, reg, "--dry-run", "-b", "main", "--secret-var", "TOKEN=s3cret")
	if !errors.Is(err, bitbucket.ErrDryRun) || !strings.Contains(out, `"value": "****"`) || strings.Contains(out, "s3cret") || len(reg.Calls) != 0 {
		t.Errorf("err %v out %q", err, out)
	}
}

func TestRun_JSON(t *testing.T) {
	reg := httpmock.New(t)
	created(reg)
	out, _, err := run(t, reg, "--json", "number,status")
	if err != nil || out != `{"number":43,"status":"pending"}`+"\n" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestRun_Watch(t *testing.T) {
	reg := httpmock.New(t)
	created(reg)
	reg.Register("GET", ptest.Pipelines+"/43", httpmock.JSONResponse(200, ptest.Pipeline(43, ptest.StateFailed)))
	reg.Register("GET", ptest.Pipelines+"/43/steps", httpmock.JSONResponse(200, ptest.Steps(ptest.Step("{s1}", "Build", ptest.StateFailed))))

	out, _, err := run(t, reg, "-b", "main", "--watch")
	var exitErr *cmdutil.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Errorf("err = %v", err)
	}
	if out != url43+"\n#43 step \"Build\": failed (12s)\n#43 failed (1m02s)\n" {
		t.Errorf("out %q", out)
	}
}

func TestRun_UnknownCustomPipeline(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("POST", ptest.Pipelines, httpmock.JSONResponse(400, `{"type":"error","error":{"message":"Bad request",`+
		`"detail":"Requested selector is not found in bitbucket-pipelines.yml.","data":{"key":"result-service.pipeline.selector-not-found"}}}`))
	_, _, err := run(t, reg, "--custom", "nope")
	if err == nil || err.Error() != "Requested selector is not found in bitbucket-pipelines.yml. (HTTP 400)" {
		t.Errorf("err = %v", err)
	}
}

func TestRun_FlagErrors(t *testing.T) {
	for _, args := range [][]string{
		{"-b", "main", "--tag", "v1"},
		{"--commit", "abc", "--tag", "v1"},
		{"--watch", "--json", "number"},
		{"--var", "novalue"},
		{"--secret-var", "novalue"},
		{"extra"},
	} {
		_, _, err := run(t, httpmock.New(t), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

func TestRun_RepoFlagNeedsATarget(t *testing.T) {
	f, _, _, _ := prtest.NewFactory(httpmock.New(t))
	cmd := NewCmdRun(f, nil)
	cmdutil.EnableRepoOverride(cmd, f) // the pipeline group normally adds --repo
	err := prtest.Run(cmd, "-R", "acme/widgets")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "--branch, --commit or --tag") {
		t.Errorf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pipeline/run/`
Expected: FAIL — `undefined: NewCmdRun`.

- [ ] **Step 3: Implement**

`pkg/cmd/pipeline/shared/started.go`:

```go
package shared

import (
	"fmt"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// ReportStarted reports a pipeline that was just started: a confirmation on stderr, then its URL
// on stdout — or its JSON with --json.
func ReportStarted(ios *iostreams.IOStreams, exporter cmdutil.Exporter, p Pipeline, what string) error {
	prshared.PrintSuccess(ios, "Started pipeline #%d (%s)", p.Number, what)
	if exporter != nil {
		return exporter.Write(ios, p)
	}
	_, err := fmt.Fprintln(ios.Out, p.URL)
	return err
}
```

`pkg/cmd/pipeline/run/run.go`:

```go
// Package run implements `khbb pipeline run`.
package run

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
)

// RunOptions holds the inputs and dependencies of `khbb pipeline run`.
type RunOptions struct {
	IO            *iostreams.IOStreams
	HTTPClient    func() (*bitbucket.Client, error)
	BaseRepo      func() (gitctx.Repo, error)
	CurrentBranch func() (string, error)
	Exporter      cmdutil.Exporter
	Sleep         func(time.Duration)

	Branch     string
	Commit     string
	Tag        string
	Custom     string
	Vars       []string
	SecretVars []string
	Watch      bool
}

// NewCmdRun returns `khbb pipeline run`.
func NewCmdRun(f *cmdutil.Factory, runF func(*RunOptions) error) *cobra.Command {
	opts := &RunOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, CurrentBranch: f.Branch, Sleep: time.Sleep}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start a pipeline",
		Long: `Start a pipeline on a branch (the current branch by default), a tag or a commit. --custom runs a
custom pipeline from bitbucket-pipelines.yml; --var and --secret-var pass variables (Bitbucket
hides secured values in logs, and khbb never prints them).

The pipeline URL is printed on stdout. With --watch, khbb then follows the pipeline and exits
with status 1 if it does not succeed.`,
		Example: `  $ khbb pipeline run
  $ khbb pipeline run --custom deploy --var ENV=staging --secret-var TOKEN="$TOKEN" --watch
  $ khbb pipeline run --tag v1.2.0 --dry-run`,
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			targets := 0
			for _, v := range []string{opts.Branch, opts.Commit, opts.Tag} {
				if v != "" {
					targets++
				}
			}
			switch {
			case targets > 1:
				return cmdutil.FlagErrorf("specify only one of --branch, --commit or --tag")
			case targets == 0 && cmd.Flags().Changed("repo"):
				return cmdutil.FlagErrorf("--branch, --commit or --tag required when using the --repo flag")
			case opts.Watch && opts.Exporter != nil:
				return cmdutil.FlagErrorf("--watch cannot be combined with --json")
			}
			if _, err := shared.ParseVariables(opts.Vars, opts.SecretVars); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return runRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.Branch, "branch", "b", "", "Run on this `branch` (default: the current branch)")
	fl.StringVar(&opts.Commit, "commit", "", "Run on this commit `sha`")
	fl.StringVar(&opts.Tag, "tag", "", "Run on this `tag`")
	fl.StringVar(&opts.Custom, "custom", "", "Run the custom pipeline with this `name`")
	fl.StringArrayVar(&opts.Vars, "var", nil, "Pass a variable as `KEY=VALUE` (repeatable)")
	fl.StringArrayVar(&opts.SecretVars, "secret-var", nil, "Pass a secured variable as `KEY=VALUE` (repeatable)")
	fl.BoolVar(&opts.Watch, "watch", false, "Follow the pipeline until it finishes")
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PipelineFields)
	return cmd
}

func runRun(ctx context.Context, opts *RunOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	target, err := runTarget(opts)
	if err != nil {
		return err
	}
	vars, err := shared.ParseVariables(opts.Vars, opts.SecretVars)
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	raw, err := client.RunPipeline(ctx, repo.Workspace, repo.Slug, target, vars)
	if err != nil {
		return err
	}
	if raw.Target.Selector == nil {
		raw.Target.Selector = target.Selector // Bitbucket fills it in only after parsing the YAML
	}
	p := shared.NewPipeline(raw, repo)
	if err := shared.ReportStarted(opts.IO, opts.Exporter, p, shared.RefLabel(p)); err != nil {
		return err
	}
	if !opts.Watch {
		return nil
	}
	final, _, err := shared.Watch(ctx, shared.WatchOptions{
		IO: opts.IO, Client: client, Repo: repo, Number: p.Number, Interval: 5 * time.Second, Sleep: opts.Sleep,
	})
	if err != nil {
		return err
	}
	return shared.ExitStatus(final)
}

// runTarget builds the pipeline target from --branch, --commit, --tag and --custom. A commit target
// needs a selector, so it uses the default pipeline unless --custom names one.
func runTarget(opts *RunOptions) (bitbucket.PipelineTarget, error) {
	var selector *bitbucket.PipelineSelector
	if opts.Custom != "" {
		selector = &bitbucket.PipelineSelector{Type: "custom", Pattern: opts.Custom}
	}
	switch {
	case opts.Commit != "":
		if selector == nil {
			selector = &bitbucket.PipelineSelector{Type: "default"}
		}
		return bitbucket.PipelineTarget{Type: "pipeline_commit_target", Commit: &bitbucket.Commit{Hash: opts.Commit}, Selector: selector}, nil
	case opts.Tag != "":
		return bitbucket.PipelineTarget{Type: "pipeline_ref_target", RefType: "tag", RefName: opts.Tag, Selector: selector}, nil
	}
	branch := opts.Branch
	if branch == "" {
		b, err := opts.CurrentBranch()
		if err != nil {
			return bitbucket.PipelineTarget{}, fmt.Errorf("%w; name the branch with --branch", err)
		}
		branch = b
	}
	return bitbucket.PipelineTarget{Type: "pipeline_ref_target", RefType: "branch", RefName: branch, Selector: selector}, nil
}
```

In `pkg/cmd/pipeline/pipeline.go`, import `"github.com/khipu/khbb/pkg/cmd/pipeline/run"` and add `run.NewCmdRun(f, nil),` to `cmd.AddCommand(...)`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/cmd/pipeline/... && go vet ./pkg/cmd/pipeline/... && gofmt -l pkg/cmd/pipeline`
Expected: PASS; gofmt prints nothing.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pipeline/
git commit -m "feat(pipeline): pipeline run on branches, tags, commits and custom pipelines"
```

---

### Task 10: `khbb pipeline stop`

**Files:**
- Create: `pkg/cmd/pipeline/stop/stop.go`, `pkg/cmd/pipeline/stop/stop_test.go`
- Modify: `pkg/cmd/pipeline/pipeline.go`

**Interfaces:**
- Consumes: `shared.Finder`, `NewPipeline`, `RefLabel`, `ParseSelector`, `StatusPaused`; `Client.StopPipeline`; `prshared.CheckRepoSelector`, `prshared.PrintSuccess`; `cmdutil.ConfirmDestructive`, `AddYesFlag`, `AddDryRunFlag`, `ConflictError`; `prtest`, `ptest`; go-gh `prompter.NewMock`.
- Produces: `stop.StopOptions`, `stop.NewCmdStop(f, runF)`. Question: `Stop pipeline #42 (branch main)?`; stderr `Stopped pipeline #42 (branch main)`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pipeline/stop/stop_test.go`:

```go
package stop

import (
	"errors"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func pipeline42(t *testing.T, state string) *httpmock.Registry {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, state)))
	return reg
}

func TestStop_Running(t *testing.T) {
	reg := pipeline42(t, ptest.StateRunning)
	reg.Register("POST", ptest.Pipelines+"/42/stopPipeline", httpmock.StringResponse(204, ""))
	f, _, out, errOut := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdStop(f, nil), "42", "--yes"); err != nil {
		t.Fatal(err)
	}
	if out.String() != "" || errOut.String() != "Stopped pipeline #42 (branch main)\n" {
		t.Errorf("out %q stderr %q", out.String(), errOut.String())
	}
}

func TestStop_RefusesFinishedPipeline(t *testing.T) {
	reg := pipeline42(t, ptest.StateFailed)
	f, _, _, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdStop(f, nil), "42", "--yes")
	var conflict *cmdutil.ConflictError
	if !errors.As(err, &conflict) || err.Error() != "pipeline #42 already finished (failed)" || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestStop_RefusesPausedPipeline(t *testing.T) {
	reg := pipeline42(t, ptest.StatePaused)
	f, _, _, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdStop(f, nil), "42", "--yes")
	var conflict *cmdutil.ConflictError
	if !errors.As(err, &conflict) || !strings.Contains(err.Error(), "paused on a manual step") ||
		!strings.Contains(err.Error(), "https://bitbucket.org/acme/widgets/pipelines/results/42") || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestStop_NeedsConfirmationWithoutATerminal(t *testing.T) {
	reg := pipeline42(t, ptest.StateRunning)
	f, _, _, _ := prtest.NewFactory(reg)

	if err := prtest.Run(NewCmdStop(f, nil), "42"); !errors.Is(err, cmdutil.ErrConfirmationRequired) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestStop_PromptsOnATerminal(t *testing.T) {
	reg := pipeline42(t, ptest.StateRunning)
	f, ios, _, _ := prtest.NewFactory(reg)
	prtest.SetTTY(ios)
	pm := prompter.NewMock(t)
	pm.RegisterConfirm("Stop pipeline #42 (branch main)?", func(_ string, def bool) (bool, error) {
		if def {
			t.Error("the default answer must be No")
		}
		return false, nil
	})
	f.Prompter = pm

	if err := prtest.Run(NewCmdStop(f, nil), "42"); !errors.Is(err, cmdutil.ErrCancel) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", err)
	}
}

func TestStop_DryRun(t *testing.T) {
	reg := pipeline42(t, ptest.StateRunning)
	f, _, out, _ := prtest.NewFactory(reg)

	err := prtest.Run(NewCmdStop(f, nil), "42", "--dry-run")
	if !errors.Is(err, bitbucket.ErrDryRun) || !strings.Contains(out.String(), "/pipelines/42/stopPipeline") || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err %v out %q", err, out.String())
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pipeline/stop/`
Expected: FAIL — `undefined: NewCmdStop`.

- [ ] **Step 3: Implement**

`pkg/cmd/pipeline/stop/stop.go`:

```go
// Package stop implements `khbb pipeline stop`.
package stop

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// StopOptions holds the inputs and dependencies of `khbb pipeline stop`.
type StopOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Prompter   cmdutil.Prompter

	Selector string
	Yes      bool
	DryRun   bool
}

// NewCmdStop returns `khbb pipeline stop`.
func NewCmdStop(f *cmdutil.Factory, runF func(*StopOptions) error) *cobra.Command {
	opts := &StopOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Prompter: f.Prompter}
	cmd := &cobra.Command{
		Use:   "stop [<number> | <url>]",
		Short: "Stop a running pipeline",
		Long: `Stop a pipeline that is pending or running. Stopping cannot be undone: on a terminal you are asked
to confirm; otherwise --yes is required. Bitbucket cannot stop a pipeline paused on a manual step
through its API; stop those in the web interface. Without an argument, stop the newest pipeline of
the current branch.`,
		Example: `  $ khbb pipeline stop 42
  $ khbb pipeline stop --yes`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prshared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			if _, _, err := shared.ParseSelector(opts.Selector); err != nil {
				return err
			}
			opts.DryRun = f.DryRun
			if runF != nil {
				return runF(opts)
			}
			return stopRun(cmd.Context(), opts)
		},
	}
	cmdutil.AddYesFlag(cmd, &opts.Yes)
	cmdutil.AddDryRunFlag(cmd, f)
	return cmd
}

func stopRun(ctx context.Context, opts *StopOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	raw, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	p := shared.NewPipeline(raw, repo)
	switch {
	case raw.State.Name == "COMPLETED":
		return &cmdutil.ConflictError{Msg: fmt.Sprintf("pipeline #%d already finished (%s)", p.Number, p.Status)}
	case p.Status == shared.StatusPaused:
		return &cmdutil.ConflictError{Msg: fmt.Sprintf("pipeline #%d is paused on a manual step; Bitbucket does not stop paused pipelines through its API: stop it at %s",
			p.Number, p.URL)}
	}
	question := fmt.Sprintf("Stop pipeline #%d (%s)?", p.Number, shared.RefLabel(p))
	if err := cmdutil.ConfirmDestructive(opts.IO, opts.Prompter, opts.Yes || opts.DryRun, question); err != nil {
		return err
	}
	if err := client.StopPipeline(ctx, repo.Workspace, repo.Slug, p.Number); err != nil {
		return err
	}
	prshared.PrintSuccess(opts.IO, "Stopped pipeline #%d (%s)", p.Number, shared.RefLabel(p))
	return nil
}
```

In `pkg/cmd/pipeline/pipeline.go`, import `"github.com/khipu/khbb/pkg/cmd/pipeline/stop"` and add `stop.NewCmdStop(f, nil),` to `cmd.AddCommand(...)`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/cmd/pipeline/... && go vet ./pkg/cmd/pipeline/... && gofmt -l pkg/cmd/pipeline`
Expected: PASS; gofmt prints nothing.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pipeline/
git commit -m "feat(pipeline): pipeline stop with confirmation"
```

---

### Task 11: `khbb pipeline rerun`

**Files:**
- Create: `pkg/cmd/pipeline/rerun/rerun.go`, `pkg/cmd/pipeline/rerun/rerun_test.go`
- Modify: `pkg/cmd/pipeline/pipeline.go`

**Interfaces:**
- Consumes: `shared.Finder`, `ParseSelector`, `ParseVariables`, `NewPipeline`, `RefLabel`, `ReportStarted`, `Watch`, `ExitStatus`, `PipelineFields`; `Client.RunPipeline` (posting the original `Target` back); `prshared.CheckRepoSelector`; `prtest`, `ptest`.
- Produces: `rerun.RerunOptions` (injectable `Sleep`), `rerun.NewCmdRerun(f, runF)`. Flags `--var`, `--secret-var`, `--watch`, `--dry-run`, `--json`. stderr `Started pipeline #43 (a rerun of #42, branch main)`; for a custom pipeline without `--var`/`--secret-var`: `note: Bitbucket does not return the variables of pipeline #45; if it needs any, pass them with --var or --secret-var`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/pipeline/rerun/rerun_test.go`:

```go
package rerun

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const note = "note: Bitbucket does not return the variables of pipeline #45; if it needs any, pass them with --var or --secret-var\n"

func run(t *testing.T, reg *httpmock.Registry, args ...string) (string, string, error) {
	t.Helper()
	f, _, out, errOut := prtest.NewFactory(reg)
	cmd := NewCmdRerun(f, func(o *RerunOptions) error {
		o.Sleep = func(time.Duration) {}
		return rerunRun(context.Background(), o)
	})
	err := prtest.Run(cmd, args...)
	return out.String(), errOut.String(), err
}

func original(reg *httpmock.Registry, n string, pipeline string) {
	reg.Register("GET", ptest.Pipelines+"/"+n, httpmock.JSONResponse(200, pipeline))
	reg.Register("POST", ptest.Pipelines, httpmock.JSONResponse(201, ptest.Pipeline(43, ptest.StatePending)))
}

func TestRerun_BranchPipeline(t *testing.T) {
	reg := httpmock.New(t)
	original(reg, "42", ptest.Pipeline(42, ptest.StateFailed))
	out, errOut, err := run(t, reg, "42")
	if err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[1], `{"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main",`+
		`"commit":{"type":"commit","hash":"`+ptest.Commit+`"},"selector":{"type":"branches","pattern":"main"}}}`)
	if out != "https://bitbucket.org/acme/widgets/pipelines/results/43\n" || errOut != "Started pipeline #43 (a rerun of #42, branch main)\n" {
		t.Errorf("out %q stderr %q", out, errOut)
	}
}

func TestRerun_PullRequestPipeline(t *testing.T) {
	reg := httpmock.New(t)
	original(reg, "44", ptest.PullRequestPipeline(44, ptest.StateFailed))
	if _, _, err := run(t, reg, "44"); err != nil {
		t.Fatal(err)
	}
	prtest.AssertJSONBody(t, reg.Calls[1], `{"target":{"type":"pipeline_pullrequest_target","source":"feature/widgets","destination":"main",`+
		`"destination_commit":{"hash":"def5678"},"commit":{"type":"commit","hash":"`+ptest.Commit+`"},"pullrequest":{"id":7},`+
		`"selector":{"type":"pull-requests","pattern":"**"}}}`)
}

func TestRerun_CustomPipelineVariables(t *testing.T) {
	reg := httpmock.New(t)
	original(reg, "45", ptest.CustomPipeline(45, ptest.StateSuccessful, "deploy"))
	if _, errOut, err := run(t, reg, "45"); err != nil || !strings.HasPrefix(errOut, note) {
		t.Errorf("stderr %q err %v", errOut, err)
	}

	reg = httpmock.New(t)
	original(reg, "45", ptest.CustomPipeline(45, ptest.StateSuccessful, "deploy"))
	_, errOut, err := run(t, reg, "45", "--var", "ENV=prod", "--secret-var", "TOKEN=s3cret")
	if err != nil || strings.Contains(errOut, "note:") {
		t.Errorf("stderr %q err %v", errOut, err)
	}
	body := prtest.JSONBodyOf(t, reg.Calls[1])
	if vars, ok := body["variables"].([]any); !ok || len(vars) != 2 {
		t.Errorf("variables = %v", body["variables"])
	}
}

func TestRerun_Watch(t *testing.T) {
	reg := httpmock.New(t)
	original(reg, "42", ptest.Pipeline(42, ptest.StateFailed))
	reg.Register("GET", ptest.Pipelines+"/43", httpmock.JSONResponse(200, ptest.Pipeline(43, ptest.StateSuccessful)))
	reg.Register("GET", ptest.Pipelines+"/43/steps", httpmock.JSONResponse(200, ptest.Steps(ptest.Step("{s1}", "Build", ptest.StateSuccessful))))

	out, _, err := run(t, reg, "42", "--watch")
	if err != nil || !strings.HasSuffix(out, "#43 successful (1m02s)\n") {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestRerun_DryRun(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, ptest.StateFailed)))
	out, _, err := run(t, reg, "42", "--dry-run")
	if !errors.Is(err, bitbucket.ErrDryRun) || !strings.Contains(out, `"method": "POST"`) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err %v out %q", err, out)
	}
}

func TestRerun_FlagErrors(t *testing.T) {
	for _, args := range [][]string{{"42", "--watch", "--json", "number"}, {"42", "--var", "bad"}, {"abc"}, {"1", "2"}} {
		_, _, err := run(t, httpmock.New(t), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/pipeline/rerun/`
Expected: FAIL — `undefined: NewCmdRerun`.

- [ ] **Step 3: Implement**

`pkg/cmd/pipeline/rerun/rerun.go`:

```go
// Package rerun implements `khbb pipeline rerun`.
package rerun

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// RerunOptions holds the inputs and dependencies of `khbb pipeline rerun`.
type RerunOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Exporter   cmdutil.Exporter
	Sleep      func(time.Duration)

	Selector   string
	Vars       []string
	SecretVars []string
	Watch      bool
}

// NewCmdRerun returns `khbb pipeline rerun`.
func NewCmdRerun(f *cmdutil.Factory, runF func(*RerunOptions) error) *cobra.Command {
	opts := &RerunOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Sleep: time.Sleep}
	cmd := &cobra.Command{
		Use:   "rerun [<number> | <url>]",
		Short: "Run a pipeline again",
		Long: `Start a new pipeline on the same commit and with the same definition as an earlier one — for a
pull-request pipeline, on the same pull request. Bitbucket never returns a pipeline's variables,
so they cannot be copied: pass them again with --var and --secret-var. Without an argument, rerun
the newest pipeline of the current branch.`,
		Example: `  $ khbb pipeline rerun 42 --watch
  $ khbb pipeline rerun 45 --var ENV=staging --secret-var TOKEN="$TOKEN"`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prshared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			if _, _, err := shared.ParseSelector(opts.Selector); err != nil {
				return err
			}
			if opts.Watch && opts.Exporter != nil {
				return cmdutil.FlagErrorf("--watch cannot be combined with --json")
			}
			if _, err := shared.ParseVariables(opts.Vars, opts.SecretVars); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return rerunRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringArrayVar(&opts.Vars, "var", nil, "Pass a variable as `KEY=VALUE` (repeatable)")
	fl.StringArrayVar(&opts.SecretVars, "secret-var", nil, "Pass a secured variable as `KEY=VALUE` (repeatable)")
	fl.BoolVar(&opts.Watch, "watch", false, "Follow the new pipeline until it finishes")
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PipelineFields)
	return cmd
}

func rerunRun(ctx context.Context, opts *RerunOptions) error {
	vars, err := shared.ParseVariables(opts.Vars, opts.SecretVars)
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	original, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if sel := original.Target.Selector; len(vars) == 0 && sel != nil && sel.Type == "custom" {
		fmt.Fprintf(opts.IO.ErrOut, "note: Bitbucket does not return the variables of pipeline #%d; if it needs any, pass them with --var or --secret-var\n",
			original.BuildNumber)
	}
	raw, err := client.RunPipeline(ctx, repo.Workspace, repo.Slug, original.Target, vars)
	if err != nil {
		return err
	}
	if raw.Target.Selector == nil {
		raw.Target.Selector = original.Target.Selector // Bitbucket fills it in only after parsing the YAML
	}
	p := shared.NewPipeline(raw, repo)
	what := fmt.Sprintf("a rerun of #%d, %s", original.BuildNumber, shared.RefLabel(p))
	if err := shared.ReportStarted(opts.IO, opts.Exporter, p, what); err != nil {
		return err
	}
	if !opts.Watch {
		return nil
	}
	final, _, err := shared.Watch(ctx, shared.WatchOptions{
		IO: opts.IO, Client: client, Repo: repo, Number: p.Number, Interval: 5 * time.Second, Sleep: opts.Sleep,
	})
	if err != nil {
		return err
	}
	return shared.ExitStatus(final)
}
```

In `pkg/cmd/pipeline/pipeline.go`, import `"github.com/khipu/khbb/pkg/cmd/pipeline/rerun"` and add `rerun.NewCmdRerun(f, nil),` to `cmd.AddCommand(...)`.

- [ ] **Step 4: Run the full suite**

Run: `go test -race ./... && go vet ./... && gofmt -l ./cmd ./internal ./pkg && go build -o bin/khbb ./cmd/khbb && ./bin/khbb pipeline --help`
Expected: PASS; gofmt prints nothing; `khbb pipeline --help` lists `list, logs, rerun, run, stop, view, watch`. If golangci-lint is available (`$(go env GOPATH)/bin/golangci-lint` or the controller's scratchpad copy), `golangci-lint run ./...` reports `0 issues`.

- [ ] **Step 5: Commit**

```bash
git add pkg/cmd/pipeline/
git commit -m "feat(pipeline): pipeline rerun on the same commit and definition"
```

---

### Task 12: Live smoke test in the sandbox (controller only)

Run by the controller, not a subagent: it uses the user's Bitbucket login and runs real pipelines in `khipu/khipubb-sandbox`, whose `main` holds the test `bitbucket-pipelines.yml` (default `Quick`; `khbb-probe/*` branches: `Build` + 60 s `Slow`; tags `khbb-probe-*`; pull requests; custom `khbb-vars`, `khbb-manual`, `khbb-fail`). The user approved pipeline runs there. Never run it elsewhere. Print only statuses, numbers and khbb's own messages; secrets used are throwaway values.

**Files:** none (results go into the SDD ledger; surprises go into the findings doc and become fix tasks).

- [ ] **Step 1: Build and list**

```bash
go build -o bin/khbb ./cmd/khbb
KHBB="$PWD/bin/khbb"; S=khipu/khipubb-sandbox
$KHBB pipeline list -R $S -L 5
$KHBB pipeline list -R $S --status successful --json number,status --jq 'length'
$KHBB pipeline list -R $S --mine -L 3 --json number --jq 'length'
```

- [ ] **Step 2: Run, watch, logs**

```bash
$KHBB pipeline run -R $S -b main --watch; echo "exit=$?"                       # exit 0, step lines, #N successful
$KHBB pipeline run -R $S -b main --custom khbb-fail --watch; echo "exit=$?"    # exit 1, #N failed
N=$($KHBB pipeline list -R $S -L 1 --json number --jq '.[0].number')
$KHBB pipeline logs $N -R $S --failed                                          # contains "about to fail"
$KHBB pipeline logs $N -R $S                                                   # stderr: no log for step "Never" (skipped)
$KHBB pipeline view $N -R $S --json status,steps --jq '[.status, (.steps|map(.status))]'
$KHBB pipeline view 99999 -R $S; echo "exit=$?"                                # Pipeline with build number '99999' not found…, no repository hint
$KHBB pipeline run -R $S -b main --custom nope; echo "exit=$?"                # Requested selector is not found in bitbucket-pipelines.yml.
```

- [ ] **Step 3: Variables, dry run, rerun**

```bash
$KHBB pipeline run -R $S -b main --custom khbb-vars --var GREETING=hi --secret-var TOKEN=probe-x --dry-run   # "value": "****", no probe-x
$KHBB pipeline run -R $S -b main --custom khbb-vars --var GREETING=hi --secret-var TOKEN=probe-x --watch; echo "exit=$?"
V=$($KHBB pipeline list -R $S -L 1 --json number --jq '.[0].number')
$KHBB pipeline logs $V -R $S | grep -c 'GREETING=hi'                           # 1
$KHBB pipeline logs $V -R $S | grep -c 'probe-x'                               # 0
$KHBB pipeline rerun $V -R $S --watch; echo "exit=$?"                          # note about variables; TOKEN missing → the step's `test -n` fails → exit 1
$KHBB pipeline rerun $V -R $S --var GREETING=again --secret-var TOKEN=probe-y --watch; echo "exit=$?"   # exit 0
```

- [ ] **Step 4: Watch after a push, stop, paused**

```bash
SB=<scratchpad>/sandbox; cd "$SB" && git fetch -q && git checkout -q -B khbb-probe/smoke3 origin/main
git -c user.name=khbb-probe -c user.email=probe@example.invalid commit -q --allow-empty -m "probe: watch" && git push -q origin khbb-probe/smoke3
$KHBB pipeline watch --exit-status; echo "exit=$?"     # stderr "Waiting for a pipeline for commit …" (maybe); follows the new pipeline; exit 0 after ~70 s
git -c user.name=khbb-probe -c user.email=probe@example.invalid commit -q --allow-empty -m "probe: stop" && git push -q origin khbb-probe/smoke3
# wait until `pipeline view` shows running, then:
$KHBB pipeline stop --yes; echo "exit=$?"               # Stopped pipeline #N (branch khbb-probe/smoke3)
$KHBB pipeline watch --exit-status; echo "exit=$?"      # this HEAD's pipeline: stopped → exit 1
$KHBB pipeline stop --yes; echo "exit=$?"               # already finished (stopped) → exit 1
cd -
$KHBB pipeline run -R $S -b main --custom khbb-manual --watch; echo "exit=$?"   # #N paused (waiting for manual step "Gate"), exit 0
M=$($KHBB pipeline list -R $S -L 1 --json number --jq '.[0].number')
$KHBB pipeline stop $M -R $S --yes; echo "exit=$?"      # refused: paused on a manual step → exit 1
```

- [ ] **Step 5: Clean up and record**

Delete `khbb-probe/*` branches (`git push origin --delete …` from the scratchpad clone). Pipelines paused on `khbb-manual` stay paused (Bitbucket's API cannot stop them); tell the user they can stop them from the web page. Write each step's expected vs. observed result into the ledger; any mismatch becomes a fix task before the final review.

---

## Spec Coverage (Plan 3)

| Spec item | Covered by |
|---|---|
| §7.2 build numbers as the contract; newest pipeline of the current branch by default | Tasks 2, 4 (finder), 8 (HEAD commit for watch) |
| §7.2 `pipeline list` (`-b`, `-s`, `--mine`, `-L` 20, `sort=-created_on`) | Tasks 2, 5 |
| §7.2 `pipeline view` (`-v`, `--web`; summary and step table) | Task 6 |
| §7.2 `pipeline logs` (`-s/--step`, `--failed`, `--tail`; header per step) | Task 7 |
| §7.2 `pipeline run` (branch/commit/tag exclusive, `--custom`, `--var`, `--secret-var`, `--watch`, `--dry-run`) | Tasks 2, 9 |
| §7.2 `pipeline stop` (`--yes`, `--dry-run`, destructive) | Task 10 |
| §7.2 `pipeline rerun` (no native endpoint: same target and selector; secured variables re-supplied) | Tasks 2, 11 |
| §7.2 `pipeline watch` (`-i` 5 s, `--exit-status`, TTY redraw vs one line per change, stops when paused) | Tasks 4, 8 |
| §7.2 status normalization, pinned in tests | Task 3 |
| §8.1 Pipeline and Step field sets | Task 3 |
| §9 destructive confirmation, `--dry-run`, secured values masked | Tasks 9, 10, 11 |
| §10 transient errors while watching (≤ 5 in a row), per-request timeout | Tasks 1, 4 |
| Findings: generic messages with a useful `detail`; pipeline 404 hints; https→http redirects; streaming logs | Task 1 |
| Live verification | Task 12 |
| §7.4 `skill install`, §12 distribution, README | Plan 4 |
