# khbb — Bitbucket Cloud CLI (v1) — Design

- **Status:** Draft for review
- **Date:** 2026-10-05
- **Repository:** `github.com/khipu/khbb` (public)
- **Language of code, help text, messages and docs:** English

## 1. Context

Khipu hosts most of its repositories on Bitbucket Cloud (`bitbucket.org/khipu`) and
relies heavily on Bitbucket Pipelines. There is no first-party CLI comparable to
GitHub's `gh`, and AI coding agents (Claude Code and similar) need a reliable,
scriptable way to work with pull requests and pipelines without a browser.

`khbb` is a Bitbucket Cloud CLI modeled on `gh`: same command shape, same flag
conventions, same output contract (`--json`, `--jq`, `--template`), same
TTY/non-TTY behavior. It is built for two audiences at once: Khipu engineers and the
agents they run.

## 2. Goals and success criteria

**Goals (v1)**

- Full pull request lifecycle: create, list, view, status, diff, checks, review
  (approve / unapprove / request changes), comment (including inline), edit, merge,
  decline, checkout.
- Pipelines: list, view, logs, run (branch / commit / tag / custom pipeline with
  variables), stop, rerun, watch.
- A generic `khbb api` escape hatch for anything not covered by commands.
- Safe to drive from an agent: stable JSON output, no prompts without a TTY,
  predictable exit codes, explicit confirmation for irreversible actions.
- Single static binary for macOS, Linux and Windows (amd64 + arm64).
- Each user authenticates with their own Atlassian API token.

**Success looks like:** an agent can take a branch from "pushed" to "merged" —
open the PR, wait for the pipeline, read failing step logs, address review
comments, and merge after human approval — using only `khbb`, with no browser.

## 3. Non-goals (v1)

- Repository security settings (branch restrictions, permissions, default
  reviewers, deploy keys, pipeline variables). Planned for v2; reachable meanwhile
  via `khbb api`.
- Bitbucket Data Center / Server.
- Multiple accounts or hosts; OAuth login.
- MCP server, auto-update checks, extensions.
- Issues, `repo create`, `repo clone`, `config` command.
- Code signing / notarization of binaries.
- Configuring git credentials (`gh auth setup-git` equivalent). `pr checkout` uses
  whatever git credentials the user already has.

## 4. Architecture

### 4.1 Approach

Go + Cobra, mirroring `gh`'s architecture. Generic, GitHub-agnostic packages are
imported from [`github.com/cli/go-gh/v2`](https://github.com/cli/go-gh) (MIT). The
Bitbucket REST 2.0 client is hand-written and covers only the endpoints `khbb` uses.

Rejected alternatives:

- *Client generated from Bitbucket's OpenAPI spec* — the spec is large and has known
  inconsistencies; generated code is noisy. Can be reconsidered for v2 security
  endpoints only.
- *Adopt/fork an existing open-source Bitbucket CLI* — third-party ownership of a
  tool that handles every engineer's token, uncertain fit with the agent contract,
  and possible language mismatch.

### 4.2 Repository layout

```
khbb/
├── cmd/khbb/main.go            # builds the Factory, runs root, maps errors → exit codes
├── internal/
│   ├── bitbucket/              # REST 2.0 client
│   │   ├── client.go           # base URL, auth transport, pagination, retries, dry-run, errors
│   │   ├── users.go            # current user, workspace members (reviewer resolution)
│   │   ├── repositories.go     # repository (main branch), effective default reviewers
│   │   ├── pullrequests.go     # PR models + calls (incl. comments, statuses, diff, merge task)
│   │   └── pipelines.go        # pipeline/step models + calls (incl. logs, stop)
│   ├── config/                 # config.yml + keyring-backed token store + env overrides
│   ├── gitctx/                 # remote parsing, SSH alias resolution, current branch, git exec
│   └── cmdutil/                # Factory, JSON/jq/template flags, confirmation, error types
├── pkg/cmd/
│   ├── root/
│   ├── auth/      {login,logout,status}
│   ├── pr/        {create,list,view,status,diff,checks,approve,unapprove,
│   │               request-changes,comment,edit,merge,decline,checkout}
│   ├── pipeline/  {list,view,logs,run,stop,rerun,watch}
│   ├── api/
│   ├── skill/     {install}
│   └── version/
├── skills/khbb/SKILL.md        # embedded via go:embed
├── .goreleaser.yaml
└── .github/workflows/          # ci.yml, release.yml
```

### 4.3 Command pattern

Every command follows `gh`'s pattern:

```go
type MergeOptions struct {
    IO         *iostreams.IOStreams
    HTTPClient func() (*bitbucket.Client, error)
    BaseRepo   func() (gitctx.Repo, error)
    Prompter   prompter.Prompter

    Selector     string // PR number; empty = PR for current branch
    Strategy     string
    DeleteBranch bool
    Message      string
    Yes          bool
}

func NewCmdMerge(f *cmdutil.Factory, runF func(*MergeOptions) error) *cobra.Command
```

`runF` lets tests assert flag parsing without executing; the `mergeRun(opts)` logic
is tested separately against a mock HTTP registry.

### 4.4 Data flow

1. Cobra parses flags into `XxxOptions`.
2. The `Factory` lazily provides: IO streams, config, the authenticated
   `bitbucket.Client`, the resolved repo (`workspace/repo`), the current branch.
3. The client returns typed Go structs (raw Bitbucket shapes stay inside
   `internal/bitbucket`).
4. Commands map those structs to `khbb`'s exported shapes (§8.1).
5. Output: `--json` exporter (+ `--jq` / `--template`), or `tableprinter` (aligned
   table on a TTY, tab-separated otherwise).

### 4.5 Dependencies

- `github.com/spf13/cobra`
- `github.com/cli/go-gh/v2` — `pkg/jq`, `pkg/template`, `pkg/tableprinter`,
  `pkg/term`, `pkg/browser`, `pkg/prompter`, `pkg/jsonpretty`
- `github.com/zalando/go-keyring`
- `gopkg.in/yaml.v3`

Exact reuse of each `go-gh` package is confirmed during implementation; any package
that turns out to be GitHub-coupled is replaced with a local equivalent.

## 5. Authentication and configuration

### 5.1 Tokens

Bitbucket Cloud app passwords are deprecated (creation disabled 2025-09-09, fully
disabled 2026-06-09). `khbb` uses **Atlassian API tokens with scopes**, sent as HTTP
Basic auth with `email:api_token`.

Required scopes (v1):

| Scope | Used by |
|---|---|
| `read:user:bitbucket` | `auth login/status`, `pr status`, `--mine` filters |
| `read:workspace:bitbucket` | reviewer name resolution (to verify: exact scope for workspace members) |
| `read:repository:bitbucket` | repo metadata (main branch), default reviewers |
| `read:pullrequest:bitbucket` | all PR reads |
| `write:pullrequest:bitbucket` | PR create/edit/comment/approve/merge/decline |
| `read:pipeline:bitbucket` | pipeline list/view/logs/watch, PR checks |
| `write:pipeline:bitbucket` | pipeline run/stop/rerun |

### 5.2 `khbb auth`

- `auth login`
  - TTY: prints the token creation URL
    (`https://id.atlassian.com/manage-profile/security/api-tokens`) and the scope
    list, prompts for email and token (masked input), validates with
    `GET /2.0/user`, stores the token in the OS keyring
    (service `khbb:bitbucket.org`, account = email), and writes email + username to
    config.
  - Non-TTY: `echo "$TOKEN" | khbb auth login --email me@khipu.com --with-token`.
  - No keyring available (e.g. headless Linux without Secret Service): fail with a
    message suggesting env vars or `--insecure-storage`, which stores the token in
    `config.yml` with mode `0600` and prints a warning.
- `auth status` — user, token source (keyring / env / file), validity check.
  Whether the API exposes a token's scopes is to be verified; if not, scope problems
  surface through 403 hints (§10).
- `auth logout` — deletes the keyring entry (and any insecure-storage token).
- There is deliberately **no `auth token`** command, so agents cannot echo the token
  into transcripts or logs.

### 5.3 Environment variables

| Variable | Purpose |
|---|---|
| `KHBB_TOKEN`, `KHBB_EMAIL` | Credentials; override keyring/config |
| `KHBB_REPO` | Default `workspace/repo` |
| `KHBB_CONFIG_DIR` | Override config directory |
| `KHBB_PROMPT_DISABLED` | Never prompt, even on a TTY |
| `KHBB_PAGER` | Pager command (empty disables) |
| `KHBB_DEBUG` | `api` logs HTTP requests/responses to stderr, with `Authorization` redacted |
| `NO_COLOR` | Disable color |

### 5.4 Config file

Location: `$KHBB_CONFIG_DIR`, else `$XDG_CONFIG_HOME/khbb`, else `~/.config/khbb`
on macOS/Linux and `%AppData%\khbb` on Windows (same rules as `gh`).

```yaml
email: me@khipu.com
username: mynickname
git_protocol: ssh      # ssh | https — used by pr checkout for fork remotes
editor: ""             # falls back to $VISUAL / $EDITOR
pager: ""
browser: ""
```

The token is never written here unless `--insecure-storage` was used.

## 6. Repository and branch resolution

Repo resolution order:

1. `-R/--repo workspace/repo` flag (available on `pr`, `pipeline` and `api`).
2. `KHBB_REPO`.
3. Git remotes of the current directory pointing at `bitbucket.org`:
   - SSH `git@bitbucket.org:ws/repo.git`, `ssh://git@bitbucket.org/ws/repo.git`
   - HTTPS `https://[user@]bitbucket.org/ws/repo.git`
   - SSH host aliases are resolved with `ssh -G <host>` (as `gh` does).
   - If several Bitbucket remotes exist, prefer `origin`; otherwise the first in
     `git remote` order. Non-TTY never prompts.
4. Otherwise exit 1: `could not determine repository; use -R workspace/repo`.

Current branch: `git rev-parse --abbrev-ref HEAD`. Used as the default PR source in
`pr create`, and to find "the open PR for this branch" when `pr view|merge|checks|
diff|…` are called without a number (query: `source.branch.name = "<branch>" AND
state = "OPEN"`). If none is found: exit 1 with a clear message.

## 7. Command reference

Global flags on every command: `-h/--help`. Output flags (`--json`, `--jq`,
`--template`) on every command that prints entities. `--web` opens the
corresponding Bitbucket page instead of printing.

### 7.1 Pull requests

| Command | Key flags | Bitbucket notes |
|---|---|---|
| `pr create` | `-t/--title`, `-b/--body`, `--body-file`, `-B/--base` (default: repo main branch), `-H/--head` (default: current branch), `-r/--reviewer` (repeatable), `--draft`, `--no-default-reviewers`, `-d/--delete-branch`, `--fill`, `--web`, `--dry-run` | Effective default reviewers are fetched and included unless `--no-default-reviewers` (to verify: API does not add them automatically). Fails if the head branch is not on the remote — no implicit push. In TTY, missing title/body are prompted (or filled from commits with `--fill`); in non-TTY they are required unless `--fill`. |
| `pr list` | `-s/--state open\|merged\|declined\|superseded\|all` (default open), `-A/--author`, `--reviewer`, `-B/--base`, `-H/--head`, `-q/--query` (raw BBQL, ANDed), `-L/--limit` (default 30) | Follows `next` until limit. |
| `pr view [n]` | `--comments`, `--web` | Without `n`: open PR for current branch. |
| `pr status` | — | Current branch PR, PRs I authored, PRs requesting my review (open, current repo). |
| `pr diff [n]` | `--name-only`, `--patch`, `--color auto\|always\|never` | `/diff`, `/diffstat`, `/patch`. |
| `pr checks [n]` | `--watch`, `-i/--interval` (default 5s), `--fail-fast` | Commit statuses of the PR (pipelines + external builds). Exit 1 if any failed or stopped (takes precedence), else 8 if any pending, else 0. A PR with no statuses exits 0 with a "no checks reported" notice on stderr. |
| `pr approve [n]` / `pr unapprove [n]` / `pr request-changes [n]` | `--dry-run` | Native endpoints. |
| `pr comment [n]` | `-b/--body`, `--body-file`, `--file <path> --line <N>` (inline, new side), `--reply-to <commentId>`, `--dry-run` | Inline uses `inline.path` + `inline.to`. |
| `pr edit [n]` | `-t`, `-b`, `--body-file`, `-B/--base`, `--add-reviewer`, `--remove-reviewer`, `--draft` / `--ready`, `--dry-run` | Partial PUT (fetch, modify, PUT). |
| `pr merge [n]` | `--merge` \| `--squash` \| `--fast-forward` (default: repo's default strategy), `-d/--delete-branch`, `-m/--message`, `--yes`, `--dry-run` | May return **202** with a task-status location: poll until done (timeout 2 min, then exit 1 with the task URL). Blocking merge checks are surfaced with Bitbucket's reason. **Destructive** (§9). |
| `pr decline [n]` | `-m/--message`, `--yes`, `--dry-run` | Declined PRs cannot be reopened in Bitbucket Cloud (to verify), hence no `pr reopen`. **Destructive** (§9). |
| `pr checkout [n]` | `-b/--branch <name>`, `--force` | Fetches the source branch; for forks, fetches from the fork's URL using `git_protocol`. Uses existing git credentials. |

**Reviewer identifiers.** Bitbucket's API no longer accepts usernames. `--reviewer`
accepts `{uuid}`, an `account_id`, or a nickname / display name. Names are resolved
against workspace members: exactly one match is required; zero or several matches
fail with the candidate list (never guess). The author is silently dropped from
reviewers (Bitbucket rejects self-review).

**Drafts.** `--draft` / `--ready` depend on the PR `draft` field; if verification
shows it is unavailable in the API, these flags are dropped from v1.

### 7.2 Pipelines

Pipelines are addressed by **build number** (`#123` in the UI). Whether endpoints
accept the build number directly or it must be resolved to the pipeline UUID is
verified during implementation; the user-facing contract is the build number either
way. Without `[n]`, commands use the most recent pipeline for the current branch.

| Command | Key flags | Bitbucket notes |
|---|---|---|
| `pipeline list` | `-b/--branch`, `-s/--status pending\|running\|paused\|successful\|failed\|error\|stopped`, `--mine`, `-L/--limit` (default 20) | `sort=-created_on`. |
| `pipeline view [n]` | `-v/--verbose`, `--web` | Summary (trigger, ref, commit, creator, duration) and step table (name, status, duration). |
| `pipeline logs [n]` | `-s/--step <name\|uuid>`, `--failed`, `--tail <N>` | Plain-text step logs; with no step filter, all steps with a header per step. |
| `pipeline run` | `-b/--branch` (default current), `--commit <sha>`, `--tag <tag>`, `--custom <name>`, `--var KEY=VALUE` (repeatable), `--secret-var KEY=VALUE` (repeatable, secured), `--watch`, `--dry-run` | `POST /pipelines` with `target` + `selector`. `--branch`/`--commit`/`--tag` are mutually exclusive. |
| `pipeline stop [n]` | `--yes`, `--dry-run` | `stopPipeline`. **Destructive** (§9). |
| `pipeline rerun [n]` | `--secret-var KEY=VALUE`, `--watch`, `--dry-run` | Uses a native rerun endpoint if one exists (to verify); otherwise triggers a new pipeline with the same target, selector and non-secured variables. Secured variables cannot be read back: if the original had any, warn and require them via `--secret-var`. |
| `pipeline watch [n]` | `-i/--interval` (default 5s), `--exit-status` | TTY: in-place refresh of step progress. Non-TTY: one line per state change. Stops when the pipeline is `successful`, `failed`, `error`, `stopped` or `paused` (waiting on a manual step — prints which step). With `--exit-status`: exit 1 on `failed`/`error`/`stopped`, else 0. |

**Status normalization.** Bitbucket splits pipeline/step status into `state`
(PENDING, IN_PROGRESS, COMPLETED, …), `stage` and `result` (SUCCESSFUL, FAILED,
ERROR, STOPPED, …). `khbb` exposes one `status` field with values
`pending | running | paused | successful | failed | error | stopped`. The exact
mapping table is built from real responses during implementation and pinned in
tests.

### 7.3 `khbb api`

Modeled on `gh api`.

```
khbb api <path> [flags]
```

- Path is relative to `https://api.bitbucket.org/2.0/`; absolute URLs on the same
  host are accepted.
- Placeholders `{workspace}`, `{repo}`, `{branch}` are filled from the resolved
  context (`-R`, `KHBB_REPO`, git).
- Flags: `-X/--method` (default GET, or POST if fields are given), `-f key=value`
  (string), `-F key=value` (typed: numbers, booleans, `null`, `@file`),
  `--input <file|->` (raw body), `-H/--header`, `--paginate` (follow `next`, emit
  the merged `values` array), `-i/--include` (response status + headers), `--jq`,
  `--template`, `--silent`.
- No `--yes` requirement for mutating methods (parity with `gh`); guard rails are
  token scopes and the agent skill (§13).

### 7.4 Other commands

- `khbb skill install [--dir <path>]` — writes the embedded `SKILL.md` to
  `~/.claude/skills/khbb/SKILL.md` by default; overwrites only with `--force`.
- `khbb version` — version, commit, build date.

## 8. Output contract

### 8.1 JSON

- `--json` with no value lists the available fields for that command and exits 1
  (same as `gh`).
- `--json f1,f2` prints an object for single-entity commands and an array for list
  commands, containing only the requested fields.
- On entity commands, `--jq <expr>` and `--template <tmpl>` require `--json` and
  apply to its output. On `khbb api` they apply directly to the response body.
- Field names are `khbb`'s own, camelCase, and **stable**. Renaming or removing a
  field is a breaking change (major version). Raw Bitbucket JSON is available via
  `khbb api`.

Field sets (v1):

- **User:** `displayName`, `nickname`, `uuid`, `accountId`
- **PullRequest:** `id`, `title`, `body`, `state`, `draft`, `author` (User),
  `sourceBranch`, `sourceCommit`, `sourceRepo`, `destinationBranch`,
  `destinationCommit`, `mergeCommit`, `reviewers` (`[{user, state}]`, state =
  `approved | changes_requested | pending`), `participants`, `commentCount`,
  `taskCount`, `closeSourceBranch`, `url`, `createdOn`, `updatedOn`, `closedBy`
- **Comment:** `id`, `author`, `body`, `path`, `line`, `parentId`, `deleted`,
  `url`, `createdOn`, `updatedOn`
- **Check** (commit status): `key`, `name`, `state` (`successful | failed |
  inprogress | stopped`), `description`, `url`, `updatedOn`
- **Pipeline:** `number`, `uuid`, `status`, `trigger`, `creator` (User), `refType`,
  `refName`, `commit`, `selector` (`{type, pattern}`), `durationSeconds`, `url`,
  `createdOn`, `completedOn`, `steps` (`view` only)
- **Step:** `uuid`, `name`, `status`, `startedOn`, `completedOn`,
  `durationSeconds`
- **pr status:** `currentBranch` (PullRequest|null), `createdByMe` ([]),
  `needsMyReview` ([])

Timestamps are RFC 3339 UTC strings.

### 8.2 Streams and TTY behavior

- stdout carries data only. Progress, prompts, warnings and errors go to stderr.
- Without a TTY on stdout: no color, no spinners, no pager, no truncation, tables
  are tab-separated.
- Without a TTY on stdin, or with `KHBB_PROMPT_DISABLED`: never prompt. Missing
  required input fails with the flag name (`required flag --title not set`).

### 8.3 Errors

- Human mode: `error: <message>` on stderr, plus a hint line where useful.
- When `--json` is set, errors are a single JSON line on stderr:
  `{"error":{"code":"not_found","status":404,"message":"...","hint":"..."}}`.
  Codes: `auth_required`, `forbidden`, `not_found`, `conflict`, `validation`,
  `rate_limited`, `server_error`, `network`, `confirmation_required`, `cancelled`,
  `usage`.

### 8.4 Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Error (including usage errors and `confirmation_required`) |
| 2 | Cancelled by the user at a prompt |
| 4 | Authentication required or token invalid (401 / no credentials) |
| 8 | Checks pending (`pr checks` only) |

## 9. Safety

- **Destructive commands:** `pr merge`, `pr decline`, `pipeline stop`.
  - TTY: explicit confirmation prompt describing the action, e.g.
    `Merge #42 (feature-x → main) with squash and delete source branch? [y/N]`.
    Default is No.
  - Non-TTY or prompts disabled: `--yes` is required; without it, exit 1 with code
    `confirmation_required` and no request is sent.
  - `--yes` also skips the prompt on a TTY.
- **`--dry-run`** on every mutating command: prints method, URL and JSON body to
  stdout (or a JSON object with `--json`) and sends nothing. Implemented once in the
  client: mutating requests short-circuit before the transport.
- **Secrets:** tokens are never printed; `KHBB_DEBUG=api` redacts `Authorization`.
  `--secret-var` values are shown as `****` in all output, including `--dry-run`.
- **No implicit side effects:** no implicit `git push`, no implicit branch deletion
  (only with `-d`), no auto-retry of mutating requests.

## 10. Error handling

| Condition | Behavior |
|---|---|
| No credentials | exit 4, hint `run khbb auth login` |
| 401 | exit 4, hint: token invalid or expired |
| 403 | exit 1, `forbidden`; hint names the scope the endpoint needs (static endpoint→scope table) |
| 404 | exit 1, `not_found`; for repos, hint that private repos return 404 without access |
| 409 / 400 on merge | exit 1; surface Bitbucket's merge-check reason verbatim |
| 429 | GET: retry with exponential backoff honoring `Retry-After`, max 3 attempts; then `rate_limited` |
| 502 / 503 / 504 | GET: same retry policy; mutating: no retry, `server_error` |
| Network error / timeout | `network`; default per-request timeout 30s |
| Merge task timeout | exit 1 with the task status URL |

`pipeline watch` and `pr checks --watch` treat transient errors (429, 5xx, network)
as "keep polling" with backoff, up to 5 consecutive failures.

## 11. Testing

- **Command tests:** flag parsing via injected `runF`; run logic via a mock HTTP
  registry (stubs by method + path; test fails if any stub is unused or an
  unstubbed request is made).
- **Fixtures** in `testdata/` captured from real responses and **sanitized**: fake
  names, emails, UUIDs and account IDs only — no personal data of real people
  (data minimization, Ley 21.719). The repo is public.
- **Golden tests** for each `--json` field set, so contract changes fail loudly.
- **TTY simulation** via test IO streams: prompts, `--yes`, `confirmation_required`,
  output formatting.
- **`gitctx`:** temp git repos with SSH, HTTPS, alias and multi-remote setups.
- **Status mapping:** table-driven tests from captured pipeline/step payloads.
- **CI (GitHub Actions):** `go vet`, `golangci-lint`, `go test -race ./...` on an
  ubuntu / macos / windows matrix.
- **E2E (opt-in, `KHBB_E2E=1`):** against a sandbox repo (`khipu/khbb-sandbox`):
  create PR → comment → approve → decline; run pipeline → watch → logs. Run
  manually before each release; not part of per-PR CI.

## 12. Distribution

- GoReleaser on tags `v*`, via `.github/workflows/release.yml`:
  - Targets: `darwin`, `linux`, `windows` × `amd64`, `arm64`.
  - Archives (`tar.gz`, `zip` for Windows) + `checksums.txt` on GitHub Releases of
    `khipu/khbb`.
  - Homebrew: `brew install khipu/tap/khbb` (tap repo `khipu/homebrew-tap`).
  - Scoop: `scoop bucket add khipu https://github.com/khipu/scoop-bucket` then
    `scoop install khbb`.
- Version info injected via `-ldflags` (`version`, `commit`, `date`).
- Semantic versioning; JSON contract changes are major.

## 13. Agent skill

`skills/khbb/SKILL.md` is embedded in the binary and installed with
`khbb skill install`, so the skill always matches the installed version. It covers:

- When to use `khbb` (any Bitbucket PR or pipeline task) and first checking
  `khbb auth status`.
- Always use `--json <fields>` with `--jq` for reading; never scrape tables.
- **Never run `pr merge`, `pr decline` or `pipeline stop` without explicit human
  approval in the conversation**, even though `--yes` exists; show `--dry-run`
  output first.
- `khbb api`: GET only unless the human approved the specific mutating call.
- Never print or log tokens; never pass secrets as plain `--var`.
- Exit code table and how to react (4 → ask the human to log in; 8 → keep waiting).
- Recipes: PR from current branch; wait for pipeline with
  `khbb pipeline watch --exit-status`; read the failed step with
  `khbb pipeline logs --failed`; list review comments; address and reply.

## 14. Items to verify during implementation

Each of these has a defined fallback in this spec; verification decides which path
is built.

1. Whether `POST /pullrequests` adds default reviewers automatically (§7.1).
2. PR `draft` support in the Cloud REST API (§7.1).
3. Declined PRs cannot be reopened in Cloud (§7.1).
4. Merge `202` + task-status flow and response shape (§7.1).
5. Pipeline endpoints accepting build numbers vs UUID only (§7.2).
6. Existence of a pipeline rerun endpoint (§7.2).
7. Exact pipeline/step `state`/`stage`/`result` values (§7.2).
8. Exact scope required for listing workspace members (§5.1).
9. Whether the API exposes a token's scopes (§5.2).
10. Bearer vs Basic for API tokens (Basic is the default; Bearer adopted only if it
    removes the email requirement without drawbacks).
11. Which `go-gh` packages are usable as-is (§4.5).

## 15. Future (v2+)

- `khbb repo` security commands: branch restrictions, merge checks, permissions
  (users/groups), default reviewers, deploy keys, pipeline variables, repo
  visibility/fork policy. Same patterns: `internal/bitbucket/<area>.go` +
  `pkg/cmd/repo/<area>/`.
- Possibly: multiple accounts, `config` command, `auth setup-git`, MCP server.
