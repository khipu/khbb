# khbb Plan 1 — Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship an installable `khbb` binary with authentication (`auth login/status/logout`), the generic `khbb api` command, and the shared foundation (HTTP client, config, git context, output contract, error/exit-code mapping) that the PR and pipeline commands will build on.

**Architecture:** Go + Cobra, mirroring `gh`: one package per command under `pkg/cmd/`, a `cmdutil.Factory` that lazily provides dependencies, `XxxOptions` structs plus an injectable `runF` for testability. A hand-written Bitbucket REST 2.0 client lives in `internal/bitbucket`; generic terminal/JSON helpers come from `github.com/cli/go-gh/v2`.

**Tech Stack:** Go 1.26, `spf13/cobra` v1.10.2, `cli/go-gh/v2` v2.16.1 (`jq`, `template`, `jsonpretty`, `term`, `prompter`), `zalando/go-keyring` v0.2.8, `gopkg.in/yaml.v3` v3.0.1, `cli/safeexec` v1.0.1.

**Spec:** `docs/superpowers/specs/2026-10-05-khbb-cli-design.md` (with findings in `docs/superpowers/specs/2026-10-05-bitbucket-api-findings.md`).

**Roadmap:** Plan 1 (this) → Plan 2: pull request commands → Plan 3: pipeline commands → Plan 4: release (GoReleaser, Homebrew, Scoop), `khbb skill install`, README. Plans 2–4 are written after this plan lands, using the findings from Task 12.

## Global Constraints

- Module path `github.com/khipu/khbb`; binary name `khbb`; Go version as written by `go mod init` (1.26.x).
- All code, help text, messages and docs in English.
- Must build and pass tests on macOS, Linux and Windows; no cgo; use `filepath` for paths; run `git`/`ssh` through `github.com/cli/safeexec`.
- Bitbucket Cloud only. API base URL `https://api.bitbucket.org/2.0/`. Auth = HTTP Basic `email:api_token`.
- Credentials never leave the process: no command prints the token; debug logs redact `Authorization`; dry-run output never includes headers.
- Keyring service name `khbb:bitbucket.org`, account = email.
- Environment variables: `KHBB_TOKEN`, `KHBB_EMAIL`, `KHBB_REPO`, `KHBB_CONFIG_DIR`, `KHBB_PROMPT_DISABLED`, `KHBB_DEBUG`, `NO_COLOR` (`KHBB_PAGER` arrives in Plan 2).
- Config file `config.yml` in `$KHBB_CONFIG_DIR`, else `$XDG_CONFIG_HOME/khbb`, else `%AppData%\khbb` on Windows, else `~/.config/khbb`; written with mode `0600`, directory `0700`.
- stdout carries data only; prompts, progress, warnings and errors go to stderr.
- Exit codes: `0` success, `1` error, `2` cancelled, `4` auth required, `8` checks pending.
- Error codes (JSON errors): `auth_required`, `forbidden`, `not_found`, `conflict`, `validation`, `rate_limited`, `server_error`, `network`, `confirmation_required`, `cancelled`, `usage`, `error`.
- Flag shorthands reserved on any command that has the flag: `-R` = `--repo`, `-q` = `--jq`, `-t` = `--template`. (Plan 2 therefore gives `pr list` a long-only `--query`.)
- Dry-run output is always a JSON object `{"dryRun":true,"method":…,"url":…,"body":…}` with secured values masked as `****`.
- Test fixtures use fake identities only: `@example.com` emails, names like "Ada Example", UUIDs like `{00000000-0000-0000-0000-000000000001}`, workspace `acme`. Never real Khipu people or data (Ley 21.719; the repo is public).
- No dependencies beyond the Tech Stack list without updating the spec.

## Review Focus

1. **Token piped or pasted with Windows line endings** (`"token\r\n"`) must authenticate as `token` — test `TestLogin_WithTokenFromStdin` (Task 9).
2. **Rewriting an existing config file on Windows** must replace it, not fail — test `TestSave_OverwritesExisting` (Task 4).
3. **Remote URL variants** — SSH with port, uppercase host, trailing slash, missing `.git`, SSH host alias — must resolve to the same repo — table in `TestParseRemoteURL` and `TestBaseRepo_ResolvesSSHAlias` (Task 5).
4. **Empty results** — `--json` on an empty list and `khbb api --paginate` with no values must print `[]`, never `null` — tests `TestJSON_EmptyListIsArray` (Task 7) and `TestAPI_PaginateEmptyIsArray` (Task 11).
5. **Non-JSON error bodies** (an HTML 502 from a proxy) must yield a readable error, not a decode failure — test `TestDo_NonJSONErrorBody` (Task 2).

---

## File Map

| File | Responsibility | Task |
|---|---|---|
| `go.mod`, `go.sum` | Module and pinned dependencies | 1 |
| `.gitignore`, `.golangci.yml`, `Makefile` | Build hygiene, lint config, local build | 1 |
| `.github/workflows/ci.yml` | vet + test (3 OS) + lint | 1 |
| `cmd/khbb/main.go` | Entrypoint: build Factory, run root, map errors → exit codes | 1, 8 |
| `internal/iostreams/iostreams.go` | Std streams + TTY/color/prompt capabilities | 1 |
| `internal/cmdutil/factory.go` | `Factory`, `Prompter`, repo/branch resolution | 1, 7 |
| `internal/cmdutil/errors.go` | Error types, `Classify`, `PrintError` | 6 |
| `internal/cmdutil/json.go` | `--json/--jq/--template` flags and exporter | 7 |
| `internal/cmdutil/confirm.go` | Destructive-action confirmation | 7 |
| `internal/cmdutil/flags.go` | `-R/--repo`, `--dry-run`, `--yes` helpers | 7 |
| `internal/httpmock/httpmock.go` | Test HTTP registry (stubs by method + path) | 2 |
| `internal/bitbucket/client.go` | Client: URL resolution, auth, send, retries, `Do` | 2, 3 |
| `internal/bitbucket/errors.go` | `HTTPError`, `NetworkError`, Bitbucket error parsing | 2 |
| `internal/bitbucket/users.go` | `User`, `CurrentUser` | 2 |
| `internal/bitbucket/dryrun.go` | Dry-run printing + secured-value masking | 3 |
| `internal/bitbucket/debug.go` | `KHBB_DEBUG` request/response logging | 3 |
| `internal/bitbucket/pagination.go` | `List[T]` following `next` | 3 |
| `internal/bitbucket/scopes.go` | Required scopes, `ScopeFor` hints | 3 |
| `internal/config/config.go` | Config dir, load/save YAML | 4 |
| `internal/config/credentials.go` | Credential resolution, keyring store/delete | 4 |
| `internal/gitctx/repo.go` | `Repo`, `ParseRepo` | 5 |
| `internal/gitctx/remote.go` | Remote URL parsing | 5 |
| `internal/gitctx/resolver.go` | Remotes, base repo, current branch | 5 |
| `pkg/cmd/root/root.go` | Command tree | 1, 8, 9, 11 |
| `pkg/cmd/version/version.go` | `khbb version` | 1 |
| `pkg/cmd/factory/default.go` | Production Factory | 8 |
| `pkg/cmd/auth/auth.go` | `khbb auth` group | 9 |
| `pkg/cmd/auth/login/login.go` | `khbb auth login` | 9 |
| `pkg/cmd/auth/status/status.go` | `khbb auth status` | 10 |
| `pkg/cmd/auth/logout/logout.go` | `khbb auth logout` | 10 |
| `pkg/cmd/api/api.go` | `khbb api` | 11 |
| `docs/superpowers/specs/2026-10-05-bitbucket-api-findings.md` | Live API findings | 12 |

---

### Task 1: Scaffold, IO streams, `khbb version`, CI

**Files:**
- Create: `go.mod`, `.gitignore`, `.golangci.yml`, `Makefile`, `.github/workflows/ci.yml`
- Create: `cmd/khbb/main.go`, `internal/iostreams/iostreams.go`, `internal/cmdutil/factory.go`, `pkg/cmd/version/version.go`, `pkg/cmd/root/root.go`
- Test: `internal/iostreams/iostreams_test.go`, `pkg/cmd/version/version_test.go`, `pkg/cmd/root/root_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `iostreams.IOStreams{In io.Reader; Out, ErrOut io.Writer}` with `System() *IOStreams`, `Test() (*IOStreams, *bytes.Buffer /*in*/, *bytes.Buffer /*out*/, *bytes.Buffer /*errOut*/)`, `IsStdinTTY/IsStdoutTTY/IsStderrTTY() bool`, `SetStdinTTY/SetStdoutTTY/SetStderrTTY(bool)`, `ColorEnabled() bool`, `SetColorEnabled(bool)`, `SetNeverPrompt(bool)`, `CanPrompt() bool`, `TerminalWidth() int`.
  - `cmdutil.Factory{AppVersion, BuildCommit, BuildDate string; IOStreams *iostreams.IOStreams}` (extended in Task 7).
  - `version.Format(version, commit, date string) string`, `version.NewCmdVersion(f *cmdutil.Factory) *cobra.Command`.
  - `root.NewCmdRoot(f *cmdutil.Factory) *cobra.Command`.

- [ ] **Step 1: Initialize the module and dependencies**

```bash
cd /Users/edavis/git/khbb
go mod init github.com/khipu/khbb
go get github.com/spf13/cobra@v1.10.2 github.com/cli/go-gh/v2@v2.16.1
```

Expected: `go.mod` with module `github.com/khipu/khbb` and both requirements.

- [ ] **Step 2: Add build hygiene files**

`.gitignore`:

```gitignore
/bin/
/dist/
*.exe
coverage.out
```

`.golangci.yml`:

```yaml
version: "2"
linters:
  default: standard
  exclusions:
    presets:
      - std-error-handling
```

`Makefile`:

```make
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test lint
build:
	go build -ldflags "$(LDFLAGS)" -o bin/khbb ./cmd/khbb
test:
	go test ./...
lint:
	golangci-lint run
```

- [ ] **Step 3: Write the failing IO streams test**

`internal/iostreams/iostreams_test.go`:

```go
package iostreams_test

import (
	"testing"

	"github.com/khipu/khbb/internal/iostreams"
)

func TestCanPrompt(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if ios.CanPrompt() {
		t.Fatal("test streams must not prompt by default")
	}
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	if !ios.CanPrompt() {
		t.Fatal("expected prompting with stdin and stdout on a terminal")
	}
	ios.SetNeverPrompt(true)
	if ios.CanPrompt() {
		t.Fatal("SetNeverPrompt must disable prompting")
	}
}

func TestCanPrompt_RequiresStdinTTY(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	if ios.CanPrompt() {
		t.Fatal("piped stdin must not prompt")
	}
}

func TestTerminalWidthDefault(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if got := ios.TerminalWidth(); got != 80 {
		t.Fatalf("TerminalWidth() = %d, want 80", got)
	}
}
```

- [ ] **Step 4: Run it to verify it fails**

Run: `go test ./internal/iostreams/`
Expected: FAIL — `package github.com/khipu/khbb/internal/iostreams is not in std` / no non-test Go files.

- [ ] **Step 5: Implement IO streams**

`internal/iostreams/iostreams.go`:

```go
// Package iostreams wraps the standard streams together with terminal capabilities.
package iostreams

import (
	"bytes"
	"io"
	"os"

	"github.com/cli/go-gh/v2/pkg/term"
)

// IOStreams bundles stdin, stdout and stderr with what khbb knows about the terminal.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer

	stdinTTY     bool
	stdoutTTY    bool
	stderrTTY    bool
	colorEnabled bool
	neverPrompt  bool
	width        int
}

// System returns IOStreams bound to the process's standard streams.
func System() *IOStreams {
	t := term.FromEnv()
	width := 80
	if w, _, err := t.Size(); err == nil && w > 0 {
		width = w
	}
	return &IOStreams{
		In:           os.Stdin,
		Out:          t.Out(),
		ErrOut:       t.ErrOut(),
		stdinTTY:     term.IsTerminal(os.Stdin),
		stdoutTTY:    t.IsTerminalOutput(),
		stderrTTY:    term.IsTerminal(os.Stderr),
		colorEnabled: t.IsColorEnabled(),
		width:        width,
	}
}

// Test returns non-TTY IOStreams backed by buffers, plus the stdin, stdout and stderr buffers.
func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in, out, errOut := &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}
	return &IOStreams{In: in, Out: out, ErrOut: errOut, width: 80}, in, out, errOut
}

func (s *IOStreams) IsStdinTTY() bool  { return s.stdinTTY }
func (s *IOStreams) IsStdoutTTY() bool { return s.stdoutTTY }
func (s *IOStreams) IsStderrTTY() bool { return s.stderrTTY }

func (s *IOStreams) SetStdinTTY(v bool)  { s.stdinTTY = v }
func (s *IOStreams) SetStdoutTTY(v bool) { s.stdoutTTY = v }
func (s *IOStreams) SetStderrTTY(v bool) { s.stderrTTY = v }

func (s *IOStreams) ColorEnabled() bool     { return s.colorEnabled }
func (s *IOStreams) SetColorEnabled(v bool) { s.colorEnabled = v }

// SetNeverPrompt disables interactive prompts even on a terminal (KHBB_PROMPT_DISABLED).
func (s *IOStreams) SetNeverPrompt(v bool) { s.neverPrompt = v }

// CanPrompt reports whether khbb may ask the user questions.
func (s *IOStreams) CanPrompt() bool {
	return s.stdinTTY && s.stdoutTTY && !s.neverPrompt
}

// TerminalWidth returns the terminal width in columns (80 when unknown).
func (s *IOStreams) TerminalWidth() int { return s.width }
```

- [ ] **Step 6: Run the IO streams tests**

Run: `go test ./internal/iostreams/`
Expected: `ok  github.com/khipu/khbb/internal/iostreams`

- [ ] **Step 7: Write the failing version and root tests**

`pkg/cmd/version/version_test.go`:

```go
package version_test

import (
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/version"
)

func TestFormat(t *testing.T) {
	cases := []struct{ version, commit, date, want string }{
		{"1.2.3", "abc123", "2026-10-05T00:00:00Z", "khbb version 1.2.3 (abc123, 2026-10-05T00:00:00Z)\n"},
		{"1.2.3", "abc123", "", "khbb version 1.2.3 (abc123)\n"},
		{"dev", "", "", "khbb version dev\n"},
	}
	for _, tc := range cases {
		if got := version.Format(tc.version, tc.commit, tc.date); got != tc.want {
			t.Errorf("Format(%q, %q, %q) = %q, want %q", tc.version, tc.commit, tc.date, got, tc.want)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	cmd := version.NewCmdVersion(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "khbb version 1.2.3\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
```

`pkg/cmd/root/root_test.go`:

```go
package root_test

import (
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/root"
)

func TestRootVersionFlag(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "khbb version 1.2.3") {
		t.Fatalf("output = %q", out.String())
	}
}
```

- [ ] **Step 8: Run them to verify they fail**

Run: `go test ./pkg/...`
Expected: FAIL — packages `cmdutil`, `version`, `root` do not exist.

- [ ] **Step 9: Implement Factory, version, root and main**

`internal/cmdutil/factory.go`:

```go
// Package cmdutil holds what every khbb command shares: the Factory, flag helpers and error types.
package cmdutil

import "github.com/khipu/khbb/internal/iostreams"

// Factory provides commands with their dependencies.
type Factory struct {
	AppVersion  string
	BuildCommit string
	BuildDate   string

	IOStreams *iostreams.IOStreams
}
```

`pkg/cmd/version/version.go`:

```go
// Package version implements `khbb version`.
package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
)

// NewCmdVersion returns `khbb version`.
func NewCmdVersion(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the khbb version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprint(f.IOStreams.Out, Format(f.AppVersion, f.BuildCommit, f.BuildDate))
			return err
		},
	}
}

// Format renders the version line shared by `khbb version` and `khbb --version`.
func Format(version, commit, date string) string {
	s := "khbb version " + version
	switch {
	case commit != "" && date != "":
		s += fmt.Sprintf(" (%s, %s)", commit, date)
	case commit != "":
		s += fmt.Sprintf(" (%s)", commit)
	}
	return s + "\n"
}
```

`pkg/cmd/root/root.go`:

```go
// Package root assembles the khbb command tree.
package root

import (
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	versionCmd "github.com/khipu/khbb/pkg/cmd/version"
)

// NewCmdRoot returns the top-level `khbb` command.
func NewCmdRoot(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "khbb <command> <subcommand> [flags]",
		Short:         "Bitbucket Cloud CLI",
		Long:          "Work with Bitbucket Cloud pull requests and pipelines from the command line.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       f.AppVersion,
	}
	cmd.SetVersionTemplate(versionCmd.Format(f.AppVersion, f.BuildCommit, f.BuildDate))
	cmd.SetOut(f.IOStreams.Out)
	cmd.SetErr(f.IOStreams.ErrOut)
	cmd.CompletionOptions.HiddenDefaultCmd = true

	cmd.AddCommand(versionCmd.NewCmdVersion(f))
	return cmd
}
```

`cmd/khbb/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/root"
)

// Set with -ldflags at build time.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	ios := iostreams.System()
	f := &cmdutil.Factory{AppVersion: version, BuildCommit: commit, BuildDate: date, IOStreams: ios}
	if err := root.NewCmdRoot(f).Execute(); err != nil {
		fmt.Fprintln(ios.ErrOut, "error:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 10: Run all tests and the binary**

Run: `go mod tidy && go test ./... && go run ./cmd/khbb version`
Expected: all packages `ok`; last line `khbb version dev`.

- [ ] **Step 11: Add CI**

Check the current major versions first and use them in place of the ones below if newer:

```bash
for r in actions/checkout actions/setup-go golangci/golangci-lint-action; do printf "%s " $r; gh api repos/$r/releases/latest --jq .tag_name; done
```

`.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  test:
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
      - run: go vet ./...
      - name: Test
        shell: bash
        run: |
          if [ "$RUNNER_OS" = "Windows" ]; then go test ./...; else go test -race ./...; fi

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
      - uses: golangci/golangci-lint-action@v8
        with:
          version: latest
```

- [ ] **Step 12: Commit**

```bash
git add go.mod go.sum .gitignore .golangci.yml Makefile .github cmd internal pkg
git commit -m "feat: scaffold khbb with version command and CI"
```

---

### Task 2: HTTP mock registry and Bitbucket client core

**Files:**
- Create: `internal/httpmock/httpmock.go`
- Create: `internal/bitbucket/client.go`, `internal/bitbucket/errors.go`, `internal/bitbucket/users.go`
- Test: `internal/bitbucket/client_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `httpmock.New(t testing.TB) *Registry`; `(*Registry).Register(method, path string, r Responder)`; `(*Registry).Client() *http.Client`; `(*Registry).Calls []httpmock.Call` with `Call{Method string; URL *url.URL; Header http.Header; Body []byte}`; responders `StringResponse(status int, body string)`, `JSONResponse(status int, body string)`, `WithHeader(r Responder, key, value string)`. Path matching is exact on `req.URL.Path` (e.g. `/2.0/user`); each stub matches once, in registration order; unused stubs and unmatched requests fail the test.
  - `bitbucket.DefaultBaseURL`; `bitbucket.Options{BaseURL, Email, Token, UserAgent string; HTTPClient *http.Client}` (more fields in Task 3); `bitbucket.New(opts Options) *Client`.
  - `(*Client).URL(path string) (string, error)`; `(*Client).Request(ctx, method, path string, header http.Header, body []byte) (*http.Response, error)` — returns the raw response for any status, caller closes the body; `(*Client).Do(ctx, method, path string, in, out any) error` — JSON in/out, non-2xx → `*HTTPError`.
  - `bitbucket.HTTPError{StatusCode int; Method, URL, Message, Detail string; RequiredScopes []string}`; `bitbucket.NetworkError{Err error}`; `bitbucket.ParseHTTPError(resp *http.Response, body []byte) *HTTPError`.
  - `bitbucket.User{DisplayName, Nickname, UUID, AccountID string}` (JSON tags `display_name`, `nickname`, `uuid`, `account_id`); `(*Client).CurrentUser(ctx) (*User, error)`.

- [ ] **Step 1: Write the HTTP mock registry**

This is test infrastructure used by every later task; it has no test of its own beyond being exercised by the client tests.

`internal/httpmock/httpmock.go`:

```go
// Package httpmock is an http.RoundTripper that serves registered stubs in tests.
package httpmock

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// Responder builds the response for a matched request.
type Responder func(*http.Request) (*http.Response, error)

// Call records a request the registry received.
type Call struct {
	Method string
	URL    *url.URL
	Header http.Header
	Body   []byte
}

// Registry matches requests against stubs by method and exact URL path.
// Each stub answers once, in registration order. When the test ends, unused stubs fail it.
type Registry struct {
	t     testing.TB
	mu    sync.Mutex
	stubs []*stub
	Calls []Call
}

type stub struct {
	method  string
	path    string
	respond Responder
	used    bool
}

// New returns a Registry bound to t.
func New(t testing.TB) *Registry {
	r := &Registry{t: t}
	t.Cleanup(r.verify)
	return r
}

// Register adds a stub for method and path (e.g. "GET", "/2.0/user").
func (r *Registry) Register(method, path string, respond Responder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stubs = append(r.stubs, &stub{method: method, path: path, respond: respond})
}

// Client returns an *http.Client that sends every request to the registry.
func (r *Registry) Client() *http.Client {
	return &http.Client{Transport: r}
}

// RoundTrip implements http.RoundTripper.
func (r *Registry) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body.Close()
	}
	r.mu.Lock()
	r.Calls = append(r.Calls, Call{Method: req.Method, URL: req.URL, Header: req.Header.Clone(), Body: body})
	var match *stub
	for _, s := range r.stubs {
		if !s.used && s.method == req.Method && s.path == req.URL.Path {
			s.used = true
			match = s
			break
		}
	}
	r.mu.Unlock()

	if match == nil {
		r.t.Errorf("httpmock: no stub for %s %s", req.Method, req.URL)
		return nil, fmt.Errorf("httpmock: no stub for %s %s", req.Method, req.URL)
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	resp, err := match.respond(req)
	if resp != nil && resp.Request == nil {
		resp.Request = req
	}
	return resp, err
}

func (r *Registry) verify() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.stubs {
		if !s.used {
			r.t.Errorf("httpmock: stub never called: %s %s", s.method, s.path)
		}
	}
}

// StringResponse responds with status and a plain body.
func StringResponse(status int, body string) Responder {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	}
}

// JSONResponse responds with status and a JSON body.
func JSONResponse(status int, body string) Responder {
	return WithHeader(StringResponse(status, body), "Content-Type", "application/json")
}

// WithHeader adds a response header to r.
func WithHeader(r Responder, key, value string) Responder {
	return func(req *http.Request) (*http.Response, error) {
		resp, err := r(req)
		if resp != nil {
			resp.Header.Set(key, value)
		}
		return resp, err
	}
}
```

- [ ] **Step 2: Write the failing client tests**

`internal/bitbucket/client_test.go`:

```go
package bitbucket_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newTestClient(t *testing.T, opts bitbucket.Options) (*bitbucket.Client, *httpmock.Registry) {
	t.Helper()
	reg := httpmock.New(t)
	opts.HTTPClient = reg.Client()
	return bitbucket.New(opts), reg
}

const userJSON = `{"display_name":"Ada Example","nickname":"ada","uuid":"{00000000-0000-0000-0000-000000000001}","account_id":"000000:aaaa"}`

func TestURL(t *testing.T) {
	c := bitbucket.New(bitbucket.Options{})
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "user", want: "https://api.bitbucket.org/2.0/user"},
		{in: "/user", want: "https://api.bitbucket.org/2.0/user"},
		{in: "/2.0/user", want: "https://api.bitbucket.org/2.0/user"},
		{in: "repositories/acme/widgets/pullrequests?state=OPEN", want: "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests?state=OPEN"},
		{in: "https://api.bitbucket.org/2.0/user?page=2", want: "https://api.bitbucket.org/2.0/user?page=2"},
		{in: "https://evil.example.com/2.0/user", wantErr: true},
		{in: "http://api.bitbucket.org/2.0/user", wantErr: true},
	}
	for _, tc := range cases {
		got, err := c.URL(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("URL(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("URL(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("URL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCurrentUser_SendsBasicAuthAndDecodes(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{Email: "dev@example.com", Token: "s3cret", UserAgent: "khbb/test"})
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	u, err := c.CurrentUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Nickname != "ada" || u.DisplayName != "Ada Example" || u.AccountID != "000000:aaaa" {
		t.Errorf("unexpected user: %+v", u)
	}
	call := reg.Calls[0]
	user, pass, ok := (&http.Request{Header: call.Header}).BasicAuth()
	if !ok || user != "dev@example.com" || pass != "s3cret" {
		t.Errorf("basic auth = %q / %q / %v", user, pass, ok)
	}
	if got := call.Header.Get("User-Agent"); got != "khbb/test" {
		t.Errorf("User-Agent = %q", got)
	}
	if got := call.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q", got)
	}
}

func TestDo_ReturnsHTTPErrorWithBitbucketMessage(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/repositories/acme/nope", httpmock.JSONResponse(404,
		`{"type":"error","error":{"message":"Repository acme/nope not found"}}`))

	err := c.Do(context.Background(), "GET", "repositories/acme/nope", nil, nil)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != 404 || httpErr.Message != "Repository acme/nope not found" {
		t.Errorf("unexpected error fields: %+v", httpErr)
	}
	if httpErr.Method != "GET" || httpErr.URL != "https://api.bitbucket.org/2.0/repositories/acme/nope" {
		t.Errorf("unexpected request fields: %+v", httpErr)
	}
	if got, want := err.Error(), "Repository acme/nope not found (HTTP 404)"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestDo_HTTPErrorWithRequiredScopes(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/repositories/acme/widgets/pipelines", httpmock.JSONResponse(403,
		`{"type":"error","error":{"message":"Your credentials lack one or more required privilege scopes.","detail":{"required":["read:pipeline:bitbucket"],"granted":["read:user:bitbucket"]}}}`))

	err := c.Do(context.Background(), "GET", "repositories/acme/widgets/pipelines", nil, nil)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if !slices.Equal(httpErr.RequiredScopes, []string{"read:pipeline:bitbucket"}) {
		t.Errorf("RequiredScopes = %v", httpErr.RequiredScopes)
	}
}

func TestDo_NonJSONErrorBody(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", "/2.0/repositories/acme/widgets/pullrequests", httpmock.StringResponse(502, "<html><body>Bad Gateway</body></html>"))

	err := c.Do(context.Background(), "POST", "repositories/acme/widgets/pullrequests", map[string]string{"title": "x"}, nil)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if got, want := err.Error(), "Bad Gateway (HTTP 502)"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestDo_EncodesBodyAndHandlesNoContent(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", "/2.0/repositories/acme/widgets/pullrequests/1/approve", httpmock.StringResponse(204, ""))

	out := map[string]any{"untouched": true}
	err := c.Do(context.Background(), "POST", "repositories/acme/widgets/pullrequests/1/approve", map[string]string{"a": "b"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if out["untouched"] != true {
		t.Errorf("out modified on 204: %v", out)
	}
	call := reg.Calls[0]
	var sent map[string]string
	if err := json.Unmarshal(call.Body, &sent); err != nil || sent["a"] != "b" {
		t.Errorf("body = %s (%v)", call.Body, err)
	}
	if got := call.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestRequest_WrapsNetworkErrors(t *testing.T) {
	boom := errors.New("connection reset")
	c := bitbucket.New(bitbucket.Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, boom
	})}})

	_, err := c.Request(context.Background(), "GET", "user", nil, nil)
	var netErr *bitbucket.NetworkError
	if !errors.As(err, &netErr) || !errors.Is(err, boom) {
		t.Fatalf("expected NetworkError wrapping %v, got %T: %v", boom, err, err)
	}
}
```

- [ ] **Step 3: Run them to verify they fail**

Run: `go test ./internal/bitbucket/`
Expected: FAIL — `undefined: bitbucket.New` (and other undefined symbols).

- [ ] **Step 4: Implement the client core**

`internal/bitbucket/errors.go`:

```go
package bitbucket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// HTTPError is a non-2xx response from the Bitbucket API.
type HTTPError struct {
	StatusCode     int
	Method         string
	URL            string
	Message        string
	Detail         string
	RequiredScopes []string
}

func (e *HTTPError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	if msg == "" {
		msg = "request failed"
	}
	return fmt.Sprintf("%s (HTTP %d)", msg, e.StatusCode)
}

// NetworkError wraps a transport failure (DNS, TLS, timeout, connection reset).
type NetworkError struct{ Err error }

func (e *NetworkError) Error() string { return "network error: " + e.Err.Error() }
func (e *NetworkError) Unwrap() error { return e.Err }

// ParseHTTPError builds an HTTPError from a response and its already-read body.
func ParseHTTPError(resp *http.Response, body []byte) *HTTPError {
	e := &HTTPError{StatusCode: resp.StatusCode}
	if resp.Request != nil {
		e.Method = resp.Request.Method
		e.URL = resp.Request.URL.String()
	}
	var payload struct {
		Error struct {
			Message string          `json:"message"`
			Detail  json.RawMessage `json:"detail"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil {
		e.Message = payload.Error.Message
		e.Detail, e.RequiredScopes = parseDetail(payload.Error.Detail)
	}
	return e
}

func readHTTPError(resp *http.Response) *HTTPError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return ParseHTTPError(resp, body)
}

// parseDetail accepts Bitbucket's `detail`, which is either a string or an object
// such as {"required": [...], "granted": [...]}.
func parseDetail(raw json.RawMessage) (string, []string) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var obj struct {
		Required []string `json:"required"`
	}
	_ = json.Unmarshal(raw, &obj)
	return string(raw), obj.Required
}
```

`internal/bitbucket/client.go`:

```go
// Package bitbucket is a minimal client for the Bitbucket Cloud REST API 2.0.
package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the Bitbucket Cloud REST API root.
const DefaultBaseURL = "https://api.bitbucket.org/2.0/"

// Options configures a Client. Zero values select defaults.
type Options struct {
	BaseURL    string
	Email      string
	Token      string
	UserAgent  string
	HTTPClient *http.Client
}

// Client talks to the Bitbucket Cloud REST API 2.0.
type Client struct {
	base *url.URL
	opts Options
}

// New returns a Client. It panics only if BaseURL is not a valid URL.
func New(opts Options) *Client {
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if !strings.HasSuffix(opts.BaseURL, "/") {
		opts.BaseURL += "/"
	}
	base, err := url.Parse(opts.BaseURL)
	if err != nil {
		panic(fmt.Sprintf("bitbucket: invalid base URL %q: %v", opts.BaseURL, err))
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "khbb"
	}
	return &Client{base: base, opts: opts}
}

// URL resolves path against the API root. Absolute URLs must point at the API host,
// so credentials are never sent elsewhere (for example through a crafted `next` link).
func (c *Client) URL(path string) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		u, err := url.Parse(path)
		if err != nil {
			return "", fmt.Errorf("invalid URL %q: %w", path, err)
		}
		if u.Scheme != c.base.Scheme || !strings.EqualFold(u.Host, c.base.Host) {
			return "", fmt.Errorf("refusing to send credentials to %s://%s: only %s is allowed", u.Scheme, u.Host, c.base.Host)
		}
		return u.String(), nil
	}
	rel := strings.TrimPrefix(path, "/")
	rel = strings.TrimPrefix(rel, strings.TrimPrefix(c.base.Path, "/"))
	full := c.base.String() + rel
	if _, err := url.Parse(full); err != nil {
		return "", fmt.Errorf("invalid path %q: %w", path, err)
	}
	return full, nil
}

// Request sends a request and returns the raw response for any HTTP status.
// The caller must close the response body.
func (c *Client) Request(ctx context.Context, method, path string, header http.Header, body []byte) (*http.Response, error) {
	u, err := c.URL(path)
	if err != nil {
		return nil, err
	}
	return c.send(ctx, method, u, header, body)
}

// Do sends in (JSON-encoded, if non-nil) and decodes a 2xx JSON response into out (if non-nil).
// Non-2xx responses become *HTTPError.
func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		body = b
	}
	resp, err := c.Request(ctx, method, path, nil, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return readHTTPError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response of %s %s: %w", method, resp.Request.URL, err)
	}
	return nil
}

func (c *Client) newRequest(ctx context.Context, method, u string, header http.Header, body []byte) (*http.Request, error) {
	var rdr io.Reader = http.NoBody
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	if c.opts.Token != "" {
		req.SetBasicAuth(c.opts.Email, c.opts.Token)
	}
	return req, nil
}

func (c *Client) send(ctx context.Context, method, u string, header http.Header, body []byte) (*http.Response, error) {
	req, err := c.newRequest(ctx, method, u, header, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.opts.HTTPClient.Do(req)
	if err != nil {
		return nil, &NetworkError{Err: err}
	}
	if resp.Request == nil {
		resp.Request = req
	}
	return resp, nil
}
```

`internal/bitbucket/users.go`:

```go
package bitbucket

import (
	"context"
	"net/http"
)

// User is a Bitbucket account as returned by the API.
type User struct {
	DisplayName string `json:"display_name"`
	Nickname    string `json:"nickname"`
	UUID        string `json:"uuid"`
	AccountID   string `json:"account_id"`
}

// CurrentUser returns the account the client authenticates as.
func (c *Client) CurrentUser(ctx context.Context) (*User, error) {
	var u User
	if err := c.Do(ctx, http.MethodGet, "user", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/bitbucket/ ./internal/httpmock/`
Expected: `ok  github.com/khipu/khbb/internal/bitbucket` (httpmock: `no test files`).

- [ ] **Step 6: Commit**

```bash
git add internal/httpmock internal/bitbucket
git commit -m "feat(bitbucket): add REST client core and HTTP test registry"
```

---

### Task 3: Client resilience — retries, dry-run, debug logging, pagination, scope hints

**Files:**
- Modify: `internal/bitbucket/client.go` (extend `Options`, `New`, `Request`; replace `send`; add `isMutating`, `isRetryable`, `retryDelay`)
- Create: `internal/bitbucket/dryrun.go`, `internal/bitbucket/debug.go`, `internal/bitbucket/pagination.go`, `internal/bitbucket/scopes.go`
- Modify: `internal/bitbucket/client_test.go` (`newTestClient` gets a no-op `Sleep`)
- Test: `internal/bitbucket/resilience_test.go`, `internal/bitbucket/scopes_test.go`

**Interfaces:**
- Consumes: Task 2 client (`Options`, `New`, `Request`, `Do`, `newRequest`, `send`), `httpmock`.
- Produces:
  - `Options` gains `DryRun bool; DryRunOut io.Writer; Debug io.Writer; MaxAttempts int /*default 3*/; Sleep func(time.Duration) /*default time.Sleep*/`.
  - `bitbucket.ErrDryRun` — returned by `Request`/`Do` for mutating methods (anything but GET/HEAD/OPTIONS) when `DryRun` is set; the request is printed to `DryRunOut` as an indented JSON object and not sent.
  - `bitbucket.List[T any](ctx context.Context, c *Client, path string, limit int) ([]T, error)` — follows `next`; `limit <= 0` means all; adds `pagelen=min(limit,50)` (or 50) when the path has none.
  - `bitbucket.RequiredScopes []string`; `bitbucket.ScopeFor(method, rawURL string) string`.

- [ ] **Step 1: Write the failing tests**

In `internal/bitbucket/client_test.go`, replace `newTestClient` with:

```go
func newTestClient(t *testing.T, opts bitbucket.Options) (*bitbucket.Client, *httpmock.Registry) {
	t.Helper()
	reg := httpmock.New(t)
	opts.HTTPClient = reg.Client()
	if opts.Sleep == nil {
		opts.Sleep = func(time.Duration) {}
	}
	return bitbucket.New(opts), reg
}
```

and add `"time"` to its imports.

`internal/bitbucket/resilience_test.go`:

```go
package bitbucket_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

func TestRequest_RetriesGETOn429HonoringRetryAfter(t *testing.T) {
	var slept []time.Duration
	c, reg := newTestClient(t, bitbucket.Options{Sleep: func(d time.Duration) { slept = append(slept, d) }})
	reg.Register("GET", "/2.0/user", httpmock.WithHeader(httpmock.JSONResponse(429, `{}`), "Retry-After", "2"))
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	u, err := c.CurrentUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Nickname != "ada" {
		t.Errorf("Nickname = %q", u.Nickname)
	}
	if !slices.Equal(slept, []time.Duration{2 * time.Second}) {
		t.Errorf("slept %v, want [2s]", slept)
	}
}

func TestRequest_GivesUpAfterMaxAttempts(t *testing.T) {
	var slept []time.Duration
	c, reg := newTestClient(t, bitbucket.Options{Sleep: func(d time.Duration) { slept = append(slept, d) }})
	for range 3 {
		reg.Register("GET", "/2.0/user", httpmock.StringResponse(503, "unavailable"))
	}

	err := c.Do(context.Background(), "GET", "user", nil, nil)
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 503 {
		t.Fatalf("expected HTTP 503 error, got %v", err)
	}
	if !slices.Equal(slept, []time.Duration{time.Second, 2 * time.Second}) {
		t.Errorf("slept %v, want [1s 2s]", slept)
	}
}

func TestRequest_DoesNotRetryMutating(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{Sleep: func(time.Duration) { t.Error("mutating requests must not be retried") }})
	reg.Register("POST", "/2.0/repositories/acme/widgets/pullrequests", httpmock.StringResponse(503, "unavailable"))

	resp, err := c.Request(context.Background(), "POST", "repositories/acme/widgets/pullrequests", nil, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 503 || len(reg.Calls) != 1 {
		t.Errorf("status %d after %d calls", resp.StatusCode, len(reg.Calls))
	}
}

func TestRequest_DryRunSkipsMutatingRequests(t *testing.T) {
	var out bytes.Buffer
	c, _ := newTestClient(t, bitbucket.Options{Email: "dev@example.com", Token: "s3cret", DryRun: true, DryRunOut: &out})

	_, err := c.Request(context.Background(), "POST", "repositories/acme/widgets/pullrequests", nil, []byte(`{"title":"Add widgets"}`))
	if !errors.Is(err, bitbucket.ErrDryRun) {
		t.Fatalf("expected ErrDryRun, got %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("dry-run output is not JSON: %v\n%s", err, out.String())
	}
	if got["dryRun"] != true || got["method"] != "POST" || got["url"] != "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests" {
		t.Errorf("unexpected dry-run output: %v", got)
	}
	if body, _ := got["body"].(map[string]any); body["title"] != "Add widgets" {
		t.Errorf("body = %v", got["body"])
	}
	if strings.Contains(out.String(), "s3cret") || strings.Contains(out.String(), "Authorization") {
		t.Error("dry-run output leaks credentials")
	}
}

func TestRequest_DryRunStillSendsGET(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{DryRun: true})
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))
	if _, err := c.CurrentUser(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDryRun_MasksSecuredValues(t *testing.T) {
	var out bytes.Buffer
	c, _ := newTestClient(t, bitbucket.Options{DryRun: true, DryRunOut: &out})
	body := `{"variables":[{"key":"PLAIN","value":"visible"},{"key":"SECRET","value":"hunter2","secured":true}]}`

	_, err := c.Request(context.Background(), "POST", "repositories/acme/widgets/pipelines", nil, []byte(body))
	if !errors.Is(err, bitbucket.ErrDryRun) {
		t.Fatalf("expected ErrDryRun, got %v", err)
	}
	s := out.String()
	if strings.Contains(s, "hunter2") || !strings.Contains(s, `"****"`) || !strings.Contains(s, "visible") {
		t.Errorf("secured value not masked correctly:\n%s", s)
	}
}

func TestDebug_RedactsAuthorization(t *testing.T) {
	var debug bytes.Buffer
	c, reg := newTestClient(t, bitbucket.Options{Email: "dev@example.com", Token: "s3cret", Debug: &debug})
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if _, err := c.CurrentUser(context.Background()); err != nil {
		t.Fatal(err)
	}
	log := debug.String()
	for _, want := range []string{"> GET https://api.bitbucket.org/2.0/user", "> Authorization: [REDACTED]", "< 200 OK"} {
		if !strings.Contains(log, want) {
			t.Errorf("debug log missing %q:\n%s", want, log)
		}
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("dev@example.com:s3cret"))
	if strings.Contains(log, "s3cret") || strings.Contains(log, encoded) {
		t.Errorf("debug log leaks credentials:\n%s", log)
	}
}

func TestList_FollowsNextUntilLimit(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/items", httpmock.JSONResponse(200, `{"values":[1,2],"next":"https://api.bitbucket.org/2.0/items?page=2&pagelen=3"}`))
	reg.Register("GET", "/2.0/items", httpmock.JSONResponse(200, `{"values":[3,4]}`))

	got, err := bitbucket.List[int](context.Background(), c, "items", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []int{1, 2, 3}) {
		t.Errorf("got %v, want [1 2 3]", got)
	}
	if q := reg.Calls[0].URL.Query().Get("pagelen"); q != "3" {
		t.Errorf("pagelen = %q, want 3", q)
	}
}

func TestList_AllPagesWhenNoLimit(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/items", httpmock.JSONResponse(200, `{"values":[1,2],"next":"https://api.bitbucket.org/2.0/items?page=2"}`))
	reg.Register("GET", "/2.0/items", httpmock.JSONResponse(200, `{"values":[3]}`))

	got, err := bitbucket.List[int](context.Background(), c, "items?sort=-created_on", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []int{1, 2, 3}) {
		t.Errorf("got %v", got)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("pagelen") != "50" || q.Get("sort") != "-created_on" {
		t.Errorf("first query = %v", q)
	}
}

func TestList_RefusesForeignNextHost(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/items", httpmock.JSONResponse(200, `{"values":[1],"next":"https://evil.example.com/2.0/items?page=2"}`))

	if _, err := bitbucket.List[int](context.Background(), c, "items", 0); err == nil {
		t.Fatal("expected an error for a next link on another host")
	}
	if len(reg.Calls) != 1 {
		t.Errorf("made %d calls, want 1", len(reg.Calls))
	}
}
```

`internal/bitbucket/scopes_test.go`:

```go
package bitbucket_test

import (
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
)

func TestScopeFor(t *testing.T) {
	cases := []struct{ method, url, want string }{
		{"GET", "user", "read:user:bitbucket"},
		{"GET", "workspaces/acme/members", "read:workspace:bitbucket"},
		{"GET", "repositories/acme/widgets/pullrequests/1", "read:pullrequest:bitbucket"},
		{"POST", "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests/1/merge", "write:pullrequest:bitbucket"},
		{"GET", "repositories/acme/widgets/effective-default-reviewers", "read:pullrequest:bitbucket"},
		{"GET", "https://api.bitbucket.org/2.0/repositories/acme/widgets/pipelines/", "read:pipeline:bitbucket"},
		{"POST", "repositories/acme/widgets/pipelines/", "write:pipeline:bitbucket"},
		{"GET", "repositories/acme/widgets", "read:repository:bitbucket"},
		{"PUT", "repositories/acme/widgets", "admin:repository:bitbucket"},
		{"POST", "repositories/acme/widgets/src", "write:repository:bitbucket"},
		{"GET", "snippets/acme", ""},
	}
	for _, tc := range cases {
		if got := bitbucket.ScopeFor(tc.method, tc.url); got != tc.want {
			t.Errorf("ScopeFor(%s, %s) = %q, want %q", tc.method, tc.url, got, tc.want)
		}
	}
}

func TestRequiredScopes(t *testing.T) {
	want := []string{
		"read:user:bitbucket",
		"read:workspace:bitbucket",
		"read:repository:bitbucket",
		"read:pullrequest:bitbucket",
		"write:pullrequest:bitbucket",
		"read:pipeline:bitbucket",
		"write:pipeline:bitbucket",
	}
	if len(bitbucket.RequiredScopes) != len(want) {
		t.Fatalf("RequiredScopes = %v", bitbucket.RequiredScopes)
	}
	for i := range want {
		if bitbucket.RequiredScopes[i] != want[i] {
			t.Errorf("RequiredScopes[%d] = %q, want %q", i, bitbucket.RequiredScopes[i], want[i])
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/bitbucket/`
Expected: FAIL — `unknown field Sleep in struct literal`, `undefined: bitbucket.ErrDryRun`, `undefined: bitbucket.List`, `undefined: bitbucket.ScopeFor`.

- [ ] **Step 3: Extend the client**

In `internal/bitbucket/client.go`:

Add `"errors"` and `"strconv"` to the imports.

Below `DefaultBaseURL`, add:

```go
// ErrDryRun is returned instead of sending a mutating request when Options.DryRun is set.
var ErrDryRun = errors.New("dry run: request not sent")
```

Replace the `Options` struct with:

```go
// Options configures a Client. Zero values select defaults.
type Options struct {
	BaseURL    string
	Email      string
	Token      string
	UserAgent  string
	HTTPClient *http.Client

	// DryRun prints mutating requests to DryRunOut instead of sending them.
	DryRun    bool
	DryRunOut io.Writer
	// Debug receives a log of every request and response, with credentials redacted.
	Debug io.Writer
	// MaxAttempts bounds retries of idempotent requests (default 3).
	MaxAttempts int
	// Sleep waits between retries (default time.Sleep).
	Sleep func(time.Duration)
}
```

In `New`, before `return &Client{...}`, add:

```go
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 3
	}
	if opts.Sleep == nil {
		opts.Sleep = time.Sleep
	}
	if opts.DryRunOut == nil {
		opts.DryRunOut = io.Discard
	}
```

Replace `Request` with:

```go
// Request sends a request and returns the raw response for any HTTP status.
// The caller must close the response body. With DryRun set, mutating requests
// are printed and ErrDryRun is returned instead.
func (c *Client) Request(ctx context.Context, method, path string, header http.Header, body []byte) (*http.Response, error) {
	u, err := c.URL(path)
	if err != nil {
		return nil, err
	}
	if c.opts.DryRun && isMutating(method) {
		if err := writeDryRun(c.opts.DryRunOut, method, u, body); err != nil {
			return nil, err
		}
		return nil, ErrDryRun
	}
	return c.send(ctx, method, u, header, body)
}
```

Replace `send` with:

```go
func (c *Client) send(ctx context.Context, method, u string, header http.Header, body []byte) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		req, err := c.newRequest(ctx, method, u, header, body)
		if err != nil {
			return nil, err
		}
		c.debugRequest(req)
		start := time.Now()
		resp, err := c.opts.HTTPClient.Do(req)
		if err != nil {
			return nil, &NetworkError{Err: err}
		}
		if resp.Request == nil {
			resp.Request = req
		}
		c.debugResponse(resp, time.Since(start))
		if attempt >= c.opts.MaxAttempts || !isRetryable(method, resp.StatusCode) {
			return resp, nil
		}
		wait := retryDelay(resp, attempt)
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		c.opts.Sleep(wait)
	}
}

func isMutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// isRetryable reports whether a response is a transient failure of an idempotent request.
func isRetryable(method string, status int) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryDelay honors Retry-After (in seconds, capped at one minute), else backs off 1s, 2s, 4s…
func retryDelay(resp *http.Response, attempt int) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return min(time.Duration(n)*time.Second, time.Minute)
		}
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}
```

`internal/bitbucket/dryrun.go`:

```go
package bitbucket

import (
	"encoding/json"
	"io"
)

// writeDryRun prints the request that would have been sent. Headers are never
// included, and any object with "secured": true has its "value" masked.
func writeDryRun(w io.Writer, method, url string, body []byte) error {
	out := map[string]any{"dryRun": true, "method": method, "url": url}
	if len(body) > 0 {
		var v any
		if err := json.Unmarshal(body, &v); err == nil {
			out["body"] = maskSecured(v)
		} else {
			out["body"] = string(body)
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func maskSecured(v any) any {
	switch t := v.(type) {
	case map[string]any:
		if secured, _ := t["secured"].(bool); secured {
			if _, ok := t["value"]; ok {
				t["value"] = "****"
			}
		}
		for k, child := range t {
			t[k] = maskSecured(child)
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = maskSecured(child)
		}
		return t
	}
	return v
}
```

`internal/bitbucket/debug.go`:

```go
package bitbucket

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"
)

func (c *Client) debugRequest(req *http.Request) {
	if c.opts.Debug == nil {
		return
	}
	fmt.Fprintf(c.opts.Debug, "> %s %s\n", req.Method, req.URL)
	for _, k := range slices.Sorted(maps.Keys(req.Header)) {
		v := strings.Join(req.Header.Values(k), ", ")
		if strings.EqualFold(k, "Authorization") {
			v = "[REDACTED]"
		}
		fmt.Fprintf(c.opts.Debug, "> %s: %s\n", k, v)
	}
}

func (c *Client) debugResponse(resp *http.Response, elapsed time.Duration) {
	if c.opts.Debug == nil {
		return
	}
	fmt.Fprintf(c.opts.Debug, "< %s (%s)\n", resp.Status, elapsed.Round(time.Millisecond))
}
```

`internal/bitbucket/pagination.go`:

```go
package bitbucket

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type page[T any] struct {
	Values []T    `json:"values"`
	Next   string `json:"next"`
}

// List follows Bitbucket's `next` links and collects up to limit values (limit <= 0 means all).
func List[T any](ctx context.Context, c *Client, path string, limit int) ([]T, error) {
	next := withPagelen(path, limit)
	all := []T{}
	for next != "" {
		var p page[T]
		if err := c.Do(ctx, http.MethodGet, next, nil, &p); err != nil {
			return nil, err
		}
		all = append(all, p.Values...)
		if limit > 0 && len(all) >= limit {
			return all[:limit], nil
		}
		next = p.Next
	}
	return all, nil
}

func withPagelen(path string, limit int) string {
	if strings.Contains(path, "pagelen=") {
		return path
	}
	n := 50
	if limit > 0 && limit < n {
		n = limit
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%spagelen=%d", path, sep, n)
}
```

`internal/bitbucket/scopes.go`:

```go
package bitbucket

import (
	"net/url"
	"strings"
)

// RequiredScopes lists the API token scopes khbb v1 needs.
var RequiredScopes = []string{
	"read:user:bitbucket",
	"read:workspace:bitbucket",
	"read:repository:bitbucket",
	"read:pullrequest:bitbucket",
	"write:pullrequest:bitbucket",
	"read:pipeline:bitbucket",
	"write:pipeline:bitbucket",
}

// ScopeFor returns the token scope a request most likely needs, or "" when unknown.
// It only feeds hints on 403 responses; Bitbucket's own "required" list takes precedence.
func ScopeFor(method, rawURL string) string {
	p := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		p = u.Path
	}
	p = strings.TrimPrefix(strings.TrimPrefix(p, "/"), "2.0/")
	write := isMutating(method)
	access := func(resource string) string {
		if write {
			return "write:" + resource + ":bitbucket"
		}
		return "read:" + resource + ":bitbucket"
	}
	repo := strings.HasPrefix(p, "repositories/")
	switch {
	case p == "user" || strings.HasPrefix(p, "user/"):
		return access("user")
	case strings.HasPrefix(p, "workspaces/") && strings.Contains(p, "/members"):
		return "read:workspace:bitbucket"
	case repo && (strings.Contains(p, "/pullrequests") || strings.Contains(p, "default-reviewers")):
		return access("pullrequest")
	case repo && strings.Contains(p, "/pipelines"):
		return access("pipeline")
	case repo && !write:
		return "read:repository:bitbucket"
	case repo && strings.Count(strings.TrimSuffix(p, "/"), "/") <= 2:
		return "admin:repository:bitbucket"
	case repo:
		return "write:repository:bitbucket"
	}
	return ""
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/bitbucket/`
Expected: `ok  github.com/khipu/khbb/internal/bitbucket`

- [ ] **Step 5: Commit**

```bash
git add internal/bitbucket
git commit -m "feat(bitbucket): add retries, dry-run, debug log, pagination and scope hints"
```

---

### Task 4: Config file and credentials

**Files:**
- Create: `internal/config/config.go`, `internal/config/credentials.go`
- Test: `internal/config/config_test.go`, `internal/config/credentials_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `config.Config{Email, Username, GitProtocol, Editor, Pager, Browser, InsecureToken string}` (YAML keys `email`, `username`, `git_protocol`, `editor`, `pager`, `browser`, `token`); `config.Dir() (string, error)`; `config.Load() (*Config, error)`; `config.LoadFile(path string) (*Config, error)`; `(*Config).Save() error`; `(*Config).Path() string`.
  - `config.KeyringService = "khbb:bitbucket.org"`; `config.TokenSource` with `SourceEnv = "env"`, `SourceKeyring = "keyring"`, `SourceFile = "file"`; `config.Credentials{Email, Token string; Source TokenSource}`; `config.ErrNoCredentials`; `config.ResolveCredentials(cfg *Config) (Credentials, error)` (env → keyring → file); `config.StoreToken(email, token string) error`; `config.DeleteToken(email string) error` (missing entry is not an error).

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/zalando/go-keyring@v0.2.8 gopkg.in/yaml.v3@v3.0.1
```

- [ ] **Step 2: Write the failing tests**

`internal/config/config_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/config"
)

func TestDir_Precedence(t *testing.T) {
	t.Setenv("KHBB_CONFIG_DIR", filepath.Join("x", "explicit"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join("x", "xdg"))
	if got, _ := config.Dir(); got != filepath.Join("x", "explicit") {
		t.Errorf("with KHBB_CONFIG_DIR: %q", got)
	}
	t.Setenv("KHBB_CONFIG_DIR", "")
	if got, _ := config.Dir(); got != filepath.Join("x", "xdg", "khbb") {
		t.Errorf("with XDG_CONFIG_HOME: %q", got)
	}
}

func TestLoadFile_MissingReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Email != "" || cfg.Path() != path {
		t.Errorf("unexpected config: %+v (path %q)", cfg, cfg.Path())
	}
}

func TestSave_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yml")
	cfg, _ := config.LoadFile(path)
	cfg.Email = "dev@example.com"
	cfg.Username = "ada"
	cfg.GitProtocol = "ssh"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "dev@example.com" || got.Username != "ada" || got.GitProtocol != "ssh" {
		t.Errorf("round trip lost data: %+v", got)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "token") {
		t.Errorf("token key written without insecure storage:\n%s", data)
	}
}

func TestSave_OverwritesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	cfg, _ := config.LoadFile(path)
	cfg.Email = "first@example.com"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg.Email = "second@example.com"
	if err := cfg.Save(); err != nil {
		t.Fatalf("second save failed: %v", err)
	}
	got, _ := config.LoadFile(path)
	if got.Email != "second@example.com" {
		t.Errorf("Email = %q", got.Email)
	}
}

func TestSave_FileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions only")
	}
	path := filepath.Join(t.TempDir(), "config.yml")
	cfg, _ := config.LoadFile(path)
	cfg.Email = "dev@example.com"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

func TestLoadFile_InvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("email: [unclosed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadFile(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("expected parse error naming the file, got %v", err)
	}
}
```

`internal/config/credentials_test.go`:

```go
package config_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/khipu/khbb/internal/config"
)

func newConfig(t *testing.T) *config.Config {
	t.Helper()
	keyring.MockInit()
	t.Setenv("KHBB_TOKEN", "")
	t.Setenv("KHBB_EMAIL", "")
	cfg, err := config.LoadFile(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestResolve_EnvWins(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "stored@example.com"
	_ = config.StoreToken("stored@example.com", "from-keyring")
	t.Setenv("KHBB_TOKEN", "from-env")
	t.Setenv("KHBB_EMAIL", "env@example.com")

	got, err := config.ResolveCredentials(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := config.Credentials{Email: "env@example.com", Token: "from-env", Source: config.SourceEnv}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestResolve_EnvTokenFallsBackToConfigEmail(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "stored@example.com"
	t.Setenv("KHBB_TOKEN", "from-env")

	got, err := config.ResolveCredentials(cfg)
	if err != nil || got.Email != "stored@example.com" || got.Source != config.SourceEnv {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolve_EnvTokenWithoutEmailFails(t *testing.T) {
	cfg := newConfig(t)
	t.Setenv("KHBB_TOKEN", "from-env")
	if _, err := config.ResolveCredentials(cfg); err == nil {
		t.Fatal("expected an error when no email is available")
	}
}

func TestResolve_Keyring(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "dev@example.com"
	if err := config.StoreToken("dev@example.com", "s3cret"); err != nil {
		t.Fatal(err)
	}
	got, err := config.ResolveCredentials(cfg)
	if err != nil || got.Token != "s3cret" || got.Source != config.SourceKeyring {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolve_InsecureFile(t *testing.T) {
	cfg := newConfig(t)
	cfg.Email = "dev@example.com"
	cfg.InsecureToken = "plain"
	got, err := config.ResolveCredentials(cfg)
	if err != nil || got.Token != "plain" || got.Source != config.SourceFile {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolve_None(t *testing.T) {
	cfg := newConfig(t)
	if _, err := config.ResolveCredentials(cfg); !errors.Is(err, config.ErrNoCredentials) {
		t.Errorf("expected ErrNoCredentials, got %v", err)
	}
	cfg.Email = "dev@example.com"
	if _, err := config.ResolveCredentials(cfg); !errors.Is(err, config.ErrNoCredentials) {
		t.Errorf("email without token: expected ErrNoCredentials, got %v", err)
	}
}

func TestDeleteToken(t *testing.T) {
	newConfig(t)
	if err := config.DeleteToken("nobody@example.com"); err != nil {
		t.Errorf("deleting a missing token must succeed: %v", err)
	}
	_ = config.StoreToken("dev@example.com", "s3cret")
	if err := config.DeleteToken("dev@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := keyring.Get(config.KeyringService, "dev@example.com"); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("token still present: %v", err)
	}
}
```

- [ ] **Step 3: Run them to verify they fail**

Run: `go test ./internal/config/`
Expected: FAIL — `undefined: config.Dir` and other undefined symbols.

- [ ] **Step 4: Implement config and credentials**

`internal/config/config.go`:

```go
// Package config reads and writes khbb's configuration file and stored credentials.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

const fileName = "config.yml"

// Config is the content of config.yml.
type Config struct {
	Email         string `yaml:"email,omitempty"`
	Username      string `yaml:"username,omitempty"`
	GitProtocol   string `yaml:"git_protocol,omitempty"`
	Editor        string `yaml:"editor,omitempty"`
	Pager         string `yaml:"pager,omitempty"`
	Browser       string `yaml:"browser,omitempty"`
	InsecureToken string `yaml:"token,omitempty"` // only with `auth login --insecure-storage`

	path string
}

// Dir returns the directory that holds config.yml.
func Dir() (string, error) {
	if d := os.Getenv("KHBB_CONFIG_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "khbb"), nil
	}
	if runtime.GOOS == "windows" {
		if d := os.Getenv("AppData"); d != "" {
			return filepath.Join(d, "khbb"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".config", "khbb"), nil
}

// Load reads config.yml from Dir(). A missing file yields an empty Config.
func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	return LoadFile(filepath.Join(dir, fileName))
}

// LoadFile reads the config at path. A missing file yields an empty Config bound to path.
func LoadFile(path string) (*Config, error) {
	cfg := &Config{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

// Path returns the file this Config is read from and saved to.
func (c *Config) Path() string { return c.path }

// Save writes the config atomically with owner-only permissions.
func (c *Config) Save() error {
	if c.path == "" {
		return errors.New("config has no file path")
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
```

`internal/config/credentials.go`:

```go
package config

import (
	"errors"
	"os"

	"github.com/zalando/go-keyring"
)

// KeyringService is the system keyring service under which tokens are stored (account = email).
const KeyringService = "khbb:bitbucket.org"

// TokenSource says where a token came from.
type TokenSource string

const (
	SourceEnv     TokenSource = "env"
	SourceKeyring TokenSource = "keyring"
	SourceFile    TokenSource = "file"
)

// Credentials authenticate API requests.
type Credentials struct {
	Email  string
	Token  string
	Source TokenSource
}

// ErrNoCredentials means the user is not logged in.
var ErrNoCredentials = errors.New("no credentials found")

// ResolveCredentials finds credentials in KHBB_TOKEN/KHBB_EMAIL, then the keyring, then the config file.
func ResolveCredentials(cfg *Config) (Credentials, error) {
	if token := os.Getenv("KHBB_TOKEN"); token != "" {
		email := os.Getenv("KHBB_EMAIL")
		if email == "" {
			email = cfg.Email
		}
		if email == "" {
			return Credentials{}, errors.New("KHBB_EMAIL must be set when KHBB_TOKEN is set")
		}
		return Credentials{Email: email, Token: token, Source: SourceEnv}, nil
	}
	if cfg.Email == "" {
		return Credentials{}, ErrNoCredentials
	}
	// Keyring errors (for example no Secret Service on a headless Linux box) fall through to the file.
	if token, err := keyring.Get(KeyringService, cfg.Email); err == nil && token != "" {
		return Credentials{Email: cfg.Email, Token: token, Source: SourceKeyring}, nil
	}
	if cfg.InsecureToken != "" {
		return Credentials{Email: cfg.Email, Token: cfg.InsecureToken, Source: SourceFile}, nil
	}
	return Credentials{}, ErrNoCredentials
}

// StoreToken saves token in the system keyring.
func StoreToken(email, token string) error {
	return keyring.Set(KeyringService, email, token)
}

// DeleteToken removes the keyring entry for email. A missing entry is not an error.
func DeleteToken(email string) error {
	err := keyring.Delete(KeyringService, email)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/config/`
Expected: `ok  github.com/khipu/khbb/internal/config`

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/config
git commit -m "feat(config): add config file and keyring-backed credentials"
```

---

### Task 5: Git context — repository and branch resolution

**Files:**
- Create: `internal/gitctx/repo.go`, `internal/gitctx/remote.go`, `internal/gitctx/resolver.go`
- Test: `internal/gitctx/remote_test.go` (package `gitctx`), `internal/gitctx/resolver_test.go` (package `gitctx_test`)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `gitctx.Repo{Workspace, Slug string}` with `FullName() string`; `gitctx.ParseRepo(s string) (Repo, error)` for `WORKSPACE/REPO`.
  - `gitctx.Remote{Name, URL string}`; `gitctx.ErrNoRepo` (message `could not determine repository; use -R WORKSPACE/REPO`).
  - `gitctx.Resolver{Git func(args ...string) (string, error); SSHHostname func(alias string) (string, error)}`; `gitctx.NewResolver(dir string) *Resolver` (`dir == ""` = current directory); methods `Remotes() ([]Remote, error)`, `BaseRepo() (Repo, error)`, `CurrentBranch() (string, error)`.

- [ ] **Step 1: Add dependency**

```bash
go get github.com/cli/safeexec@v1.0.1
```

- [ ] **Step 2: Write the failing tests**

`internal/gitctx/remote_test.go`:

```go
package gitctx

import "testing"

func TestParseRemoteURL(t *testing.T) {
	cases := []struct {
		in        string
		host      string
		repo      Repo
		ssh       bool
		expectErr bool
	}{
		{in: "git@bitbucket.org:acme/widgets.git", host: "bitbucket.org", repo: Repo{"acme", "widgets"}, ssh: true},
		{in: "ssh://git@bitbucket.org/acme/widgets.git", host: "bitbucket.org", repo: Repo{"acme", "widgets"}, ssh: true},
		{in: "ssh://git@bitbucket.org:22/acme/widgets.git", host: "bitbucket.org", repo: Repo{"acme", "widgets"}, ssh: true},
		{in: "https://dev@bitbucket.org/acme/widgets.git", host: "bitbucket.org", repo: Repo{"acme", "widgets"}},
		{in: "https://bitbucket.org/acme/widgets", host: "bitbucket.org", repo: Repo{"acme", "widgets"}},
		{in: "https://bitbucket.org/acme/widgets/", host: "bitbucket.org", repo: Repo{"acme", "widgets"}},
		{in: "git@Bitbucket.org:acme/widgets.git", host: "bitbucket.org", repo: Repo{"acme", "widgets"}, ssh: true},
		{in: "git@bb-work:acme/widgets.git", host: "bb-work", repo: Repo{"acme", "widgets"}, ssh: true},
		{in: "git@github.com:acme/widgets.git", host: "github.com", repo: Repo{"acme", "widgets"}, ssh: true},
		{in: "/srv/git/widgets.git", expectErr: true},
		{in: "https://bitbucket.org/acme", expectErr: true},
		{in: `C:\repos\widgets`, expectErr: true},
	}
	for _, tc := range cases {
		host, repo, ssh, err := parseRemoteURL(tc.in)
		if tc.expectErr {
			if err == nil {
				t.Errorf("parseRemoteURL(%q): expected error, got %s %v", tc.in, host, repo)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseRemoteURL(%q): %v", tc.in, err)
			continue
		}
		if host != tc.host || repo != tc.repo || ssh != tc.ssh {
			t.Errorf("parseRemoteURL(%q) = %q %v ssh=%v, want %q %v ssh=%v", tc.in, host, repo, ssh, tc.host, tc.repo, tc.ssh)
		}
	}
}

func TestParseRepo(t *testing.T) {
	r, err := ParseRepo("acme/widgets")
	if err != nil || r.FullName() != "acme/widgets" {
		t.Errorf("ParseRepo = %v, %v", r, err)
	}
	for _, bad := range []string{"", "acme", "acme/", "/widgets", "a/b/c"} {
		if _, err := ParseRepo(bad); err == nil {
			t.Errorf("ParseRepo(%q): expected error", bad)
		}
	}
}
```

`internal/gitctx/resolver_test.go`:

```go
package gitctx_test

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/gitctx"
)

func fakeResolver(remotes string, branch string, branchErr error) *gitctx.Resolver {
	return &gitctx.Resolver{
		Git: func(args ...string) (string, error) {
			switch args[0] {
			case "remote":
				return remotes, nil
			case "symbolic-ref":
				return branch, branchErr
			}
			return "", errors.New("unexpected git call: " + strings.Join(args, " "))
		},
	}
}

func TestBaseRepo_PrefersOrigin(t *testing.T) {
	r := fakeResolver(strings.Join([]string{
		"upstream\tgit@bitbucket.org:acme/upstream.git (fetch)",
		"upstream\tgit@bitbucket.org:acme/upstream.git (push)",
		"origin\thttps://dev@bitbucket.org/acme/widgets.git (fetch)",
		"origin\thttps://dev@bitbucket.org/acme/widgets.git (push)",
	}, "\n"), "", nil)
	repo, err := r.BaseRepo()
	if err != nil || repo.FullName() != "acme/widgets" {
		t.Errorf("BaseRepo = %v, %v", repo, err)
	}
}

func TestBaseRepo_SkipsNonBitbucketRemotes(t *testing.T) {
	r := fakeResolver("origin\tgit@github.com:acme/mirror.git (fetch)\nbb\tgit@bitbucket.org:acme/widgets.git (fetch)", "", nil)
	repo, err := r.BaseRepo()
	if err != nil || repo.FullName() != "acme/widgets" {
		t.Errorf("BaseRepo = %v, %v", repo, err)
	}
}

func TestBaseRepo_ResolvesSSHAlias(t *testing.T) {
	r := fakeResolver("origin\tgit@bb-work:acme/widgets.git (fetch)", "", nil)
	var asked []string
	r.SSHHostname = func(alias string) (string, error) {
		asked = append(asked, alias)
		return "bitbucket.org", nil
	}
	repo, err := r.BaseRepo()
	if err != nil || repo.FullName() != "acme/widgets" {
		t.Errorf("BaseRepo = %v, %v", repo, err)
	}
	if len(asked) != 1 || asked[0] != "bb-work" {
		t.Errorf("SSHHostname called with %v", asked)
	}
}

func TestBaseRepo_NoBitbucketRemote(t *testing.T) {
	r := fakeResolver("origin\tgit@github.com:acme/widgets.git (fetch)", "", nil)
	if _, err := r.BaseRepo(); !errors.Is(err, gitctx.ErrNoRepo) {
		t.Errorf("expected ErrNoRepo, got %v", err)
	}
}

func TestBaseRepo_NotAGitRepository(t *testing.T) {
	r := &gitctx.Resolver{Git: func(...string) (string, error) { return "", errors.New("not a git repository") }}
	if _, err := r.BaseRepo(); !errors.Is(err, gitctx.ErrNoRepo) {
		t.Errorf("expected ErrNoRepo, got %v", err)
	}
}

func TestCurrentBranch(t *testing.T) {
	r := fakeResolver("", "feature/widgets", nil)
	if b, err := r.CurrentBranch(); err != nil || b != "feature/widgets" {
		t.Errorf("CurrentBranch = %q, %v", b, err)
	}
	r = fakeResolver("", "", errors.New("fatal: ref HEAD is not a symbolic ref"))
	if _, err := r.CurrentBranch(); err == nil || !strings.Contains(err.Error(), "not on a branch") {
		t.Errorf("expected detached-HEAD error, got %v", err)
	}
}

func TestResolver_RealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"remote", "add", "origin", "git@bitbucket.org:acme/widgets.git"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	r := gitctx.NewResolver(dir)
	if repo, err := r.BaseRepo(); err != nil || repo.FullName() != "acme/widgets" {
		t.Errorf("BaseRepo = %v, %v", repo, err)
	}
	if b, err := r.CurrentBranch(); err != nil || b != "main" {
		t.Errorf("CurrentBranch = %q, %v", b, err)
	}
}
```

- [ ] **Step 3: Run them to verify they fail**

Run: `go test ./internal/gitctx/`
Expected: FAIL — `undefined: parseRemoteURL`, `undefined: Repo`, …

- [ ] **Step 4: Implement git context**

`internal/gitctx/repo.go`:

```go
// Package gitctx works out which Bitbucket repository and branch a command targets.
package gitctx

import (
	"fmt"
	"strings"
)

// Repo identifies a Bitbucket repository.
type Repo struct {
	Workspace string
	Slug      string
}

// FullName returns WORKSPACE/REPO.
func (r Repo) FullName() string { return r.Workspace + "/" + r.Slug }

// ParseRepo parses WORKSPACE/REPO.
func ParseRepo(s string) (Repo, error) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Repo{}, fmt.Errorf("invalid repository %q: expected WORKSPACE/REPO", s)
	}
	return Repo{Workspace: parts[0], Slug: parts[1]}, nil
}
```

`internal/gitctx/remote.go`:

```go
package gitctx

import (
	"fmt"
	"net/url"
	"strings"
)

const bitbucketHost = "bitbucket.org"

// Remote is a git remote and its fetch URL.
type Remote struct {
	Name string
	URL  string
}

// parseRemoteURL extracts the lowercase host and WORKSPACE/REPO from a git remote URL.
// isSSH is true for SSH URLs, whose host may be an alias from ~/.ssh/config.
func parseRemoteURL(raw string) (host string, repo Repo, isSSH bool, err error) {
	raw = strings.TrimSpace(raw)
	var path string
	switch {
	case strings.Contains(raw, "://"):
		u, perr := url.Parse(raw)
		if perr != nil {
			return "", Repo{}, false, perr
		}
		host, path = u.Hostname(), u.Path
		isSSH = u.Scheme == "ssh" || u.Scheme == "git+ssh"
	case strings.Index(raw, ":") > 1: // scp-like user@host:path; index 1 would be a Windows drive letter
		i := strings.Index(raw, ":")
		hostPart := raw[:i]
		if at := strings.LastIndex(hostPart, "@"); at >= 0 {
			hostPart = hostPart[at+1:]
		}
		host, path, isSSH = hostPart, raw[i+1:], true
	default:
		return "", Repo{}, false, fmt.Errorf("unsupported remote URL %q", raw)
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	repo, err = ParseRepo(path)
	if err != nil {
		return "", Repo{}, false, err
	}
	return strings.ToLower(host), repo, isSSH, nil
}
```

`internal/gitctx/resolver.go`:

```go
package gitctx

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cli/safeexec"
)

// ErrNoRepo means no Bitbucket repository could be inferred.
var ErrNoRepo = errors.New("could not determine repository; use -R WORKSPACE/REPO")

// Resolver inspects a git working copy. Its functions are fields so tests can fake them.
type Resolver struct {
	// Git runs git with args and returns trimmed stdout.
	Git func(args ...string) (string, error)
	// SSHHostname resolves an SSH host alias to its real hostname. Nil disables alias resolution.
	SSHHostname func(alias string) (string, error)
}

// NewResolver returns a Resolver for the git working copy at dir ("" = current directory).
func NewResolver(dir string) *Resolver {
	return &Resolver{
		Git:         func(args ...string) (string, error) { return runGit(dir, args...) },
		SSHHostname: sshHostname,
	}
}

// Remotes lists remotes with their fetch URLs, in `git remote -v` order.
func (r *Resolver) Remotes() ([]Remote, error) {
	out, err := r.Git("remote", "-v")
	if err != nil {
		return nil, err
	}
	var remotes []Remote
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || f[2] != "(fetch)" || seen[f[0]] {
			continue
		}
		seen[f[0]] = true
		remotes = append(remotes, Remote{Name: f[0], URL: f[1]})
	}
	return remotes, nil
}

// BaseRepo returns the Bitbucket repository of the working copy, preferring the "origin" remote.
func (r *Resolver) BaseRepo() (Repo, error) {
	remotes, err := r.Remotes()
	if err != nil {
		return Repo{}, ErrNoRepo
	}
	var found []Repo
	for _, rem := range remotes {
		host, repo, isSSH, err := parseRemoteURL(rem.URL)
		if err != nil {
			continue
		}
		if host != bitbucketHost && isSSH && r.SSHHostname != nil {
			if real, err := r.SSHHostname(host); err == nil {
				host = real
			}
		}
		if host != bitbucketHost {
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

// CurrentBranch returns the checked-out branch name.
func (r *Resolver) CurrentBranch() (string, error) {
	b, err := r.Git("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || b == "" {
		return "", errors.New("could not determine current branch: not on a branch (detached HEAD?)")
	}
	return b, nil
}

func runGit(dir string, args ...string) (string, error) {
	exe, err := safeexec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git not found in PATH: %w", err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(string(out)), nil
}

func sshHostname(alias string) (string, error) {
	exe, err := safeexec.LookPath("ssh")
	if err != nil {
		return "", err
	}
	out, err := exec.Command(exe, "-G", alias).Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) == 2 && strings.EqualFold(f[0], "hostname") {
			return strings.ToLower(f[1]), nil
		}
	}
	return "", fmt.Errorf("ssh -G %s: no hostname", alias)
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/gitctx/`
Expected: `ok  github.com/khipu/khbb/internal/gitctx`

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/gitctx
git commit -m "feat(gitctx): resolve Bitbucket repo and branch from git"
```

---

### Task 6: Error types, classification and exit codes

**Files:**
- Create: `internal/cmdutil/errors.go`
- Test: `internal/cmdutil/errors_test.go`

**Interfaces:**
- Consumes: `bitbucket.HTTPError`, `bitbucket.NetworkError`, `bitbucket.ErrDryRun`, `bitbucket.ScopeFor` (Tasks 2–3); `iostreams.IOStreams` (Task 1).
- Produces:
  - `cmdutil.FlagError{Err error}` + `cmdutil.FlagErrorf(format string, args ...any) error`.
  - `cmdutil.ErrCancel`, `cmdutil.ErrConfirmationRequired`.
  - `cmdutil.AuthError{Msg string}`.
  - `cmdutil.ExitError{Code int}` — ends the command with `Code`, printing nothing (Plan 2 uses it for `pr checks` exit 8).
  - `cmdutil.ErrorInfo{Code string; Status int; Message, Hint string; Exit int; Silent bool}`; `cmdutil.Classify(err error) ErrorInfo`; `cmdutil.PrintError(ios *iostreams.IOStreams, err error, asJSON bool) int`.

- [ ] **Step 1: Write the failing tests**

`internal/cmdutil/errors_test.go`:

```go
package cmdutil_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

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
		{"confirm", cmdutil.ErrConfirmationRequired, "confirmation_required", 1, 0, "", false},
		{"flag", cmdutil.FlagErrorf("required flag --title not set"), "usage", 1, 0, "--help", false},
		{"auth", &cmdutil.AuthError{Msg: "not logged in to bitbucket.org"}, "auth_required", 4, 0, "khbb auth login", false},
		{"401", &bitbucket.HTTPError{StatusCode: 401}, "auth_required", 4, 401, "khbb auth login", false},
		{"403 reported", &bitbucket.HTTPError{StatusCode: 403, RequiredScopes: []string{"read:pipeline:bitbucket"}}, "forbidden", 1, 403, "read:pipeline:bitbucket", false},
		{"403 inferred", &bitbucket.HTTPError{StatusCode: 403, Method: "POST", URL: "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests/1/merge"}, "forbidden", 1, 403, "write:pullrequest:bitbucket", false},
		{"404", &bitbucket.HTTPError{StatusCode: 404}, "not_found", 1, 404, "private repositories", false},
		{"409", &bitbucket.HTTPError{StatusCode: 409}, "conflict", 1, 409, "", false},
		{"400", &bitbucket.HTTPError{StatusCode: 400, Detail: "title is required"}, "validation", 1, 400, "title is required", false},
		{"429", &bitbucket.HTTPError{StatusCode: 429}, "rate_limited", 1, 429, "", false},
		{"555", &bitbucket.HTTPError{StatusCode: 555}, "server_error", 1, 555, "retry later", false},
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

func TestPrintError_Human(t *testing.T) {
	ios, _, out, errOut := iostreams.Test()
	code := cmdutil.PrintError(ios, &bitbucket.HTTPError{StatusCode: 404, Message: "Repository acme/nope not found"}, false)
	if code != 1 {
		t.Errorf("exit = %d", code)
	}
	want := "error: Repository acme/nope not found (HTTP 404)\nhint: private repositories return 404 when you lack access\n"
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", out.String())
	}
}

func TestPrintError_JSON(t *testing.T) {
	ios, _, _, errOut := iostreams.Test()
	cmdutil.PrintError(ios, &bitbucket.HTTPError{StatusCode: 404, Message: "Repository acme/nope not found"}, true)
	want := `{"error":{"code":"not_found","status":404,"message":"Repository acme/nope not found (HTTP 404)","hint":"private repositories return 404 when you lack access"}}` + "\n"
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
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cmdutil/`
Expected: FAIL — `undefined: cmdutil.Classify` and other undefined symbols.

- [ ] **Step 3: Implement errors**

`internal/cmdutil/errors.go`:

```go
package cmdutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/iostreams"
)

// FlagError is a usage error: wrong or missing flags or arguments.
type FlagError struct{ Err error }

func (e *FlagError) Error() string { return e.Err.Error() }
func (e *FlagError) Unwrap() error { return e.Err }

// FlagErrorf returns a *FlagError.
func FlagErrorf(format string, args ...any) error {
	return &FlagError{Err: fmt.Errorf(format, args...)}
}

// ErrCancel means the user declined a prompt.
var ErrCancel = errors.New("cancelled")

// ErrConfirmationRequired means a destructive action ran without a terminal and without --yes.
var ErrConfirmationRequired = errors.New("this action needs confirmation: rerun with --yes")

// AuthError means credentials are missing or were rejected.
type AuthError struct{ Msg string }

func (e *AuthError) Error() string { return e.Msg }

// ExitError ends the command with Code without printing anything.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// ErrorInfo is the classified form of an error, as printed to stderr.
type ErrorInfo struct {
	Code    string `json:"code"`
	Status  int    `json:"status,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	Exit    int    `json:"-"`
	Silent  bool   `json:"-"`
}

// Classify maps an error to its code, message, hint and process exit code.
func Classify(err error) ErrorInfo {
	var (
		flagErr *FlagError
		authErr *AuthError
		exitErr *ExitError
		httpErr *bitbucket.HTTPError
		netErr  *bitbucket.NetworkError
	)
	switch {
	case errors.Is(err, bitbucket.ErrDryRun):
		return ErrorInfo{Exit: 0, Silent: true}
	case errors.As(err, &exitErr):
		return ErrorInfo{Exit: exitErr.Code, Silent: true}
	case errors.Is(err, ErrCancel):
		return ErrorInfo{Code: "cancelled", Message: "cancelled", Exit: 2}
	case errors.Is(err, ErrConfirmationRequired):
		return ErrorInfo{Code: "confirmation_required", Message: err.Error(), Exit: 1}
	case errors.As(err, &flagErr):
		return ErrorInfo{Code: "usage", Message: err.Error(), Hint: "see `--help` for usage", Exit: 1}
	case errors.As(err, &authErr):
		return ErrorInfo{Code: "auth_required", Message: err.Error(), Hint: "run `khbb auth login`", Exit: 4}
	case errors.As(err, &httpErr):
		return classifyHTTP(httpErr)
	case errors.As(err, &netErr):
		return ErrorInfo{Code: "network", Message: err.Error(), Exit: 1}
	}
	return ErrorInfo{Code: "error", Message: err.Error(), Exit: 1}
}

func classifyHTTP(e *bitbucket.HTTPError) ErrorInfo {
	info := ErrorInfo{Status: e.StatusCode, Message: e.Error(), Exit: 1}
	switch {
	case e.StatusCode == 401:
		info.Code, info.Exit = "auth_required", 4
		info.Hint = "the token is invalid or expired; run `khbb auth login`"
	case e.StatusCode == 403:
		info.Code = "forbidden"
		if len(e.RequiredScopes) > 0 {
			info.Hint = "the token is missing scope(s) " + strings.Join(e.RequiredScopes, ", ") + "; create a token with them and run `khbb auth login`"
		} else if scope := bitbucket.ScopeFor(e.Method, e.URL); scope != "" {
			info.Hint = "this request needs the " + scope + " scope, or repository permissions you may not have"
		}
	case e.StatusCode == 404:
		info.Code = "not_found"
		info.Hint = "private repositories return 404 when you lack access"
	case e.StatusCode == 409:
		info.Code = "conflict"
	case e.StatusCode == 400 || e.StatusCode == 422:
		info.Code = "validation"
	case e.StatusCode == 429:
		info.Code = "rate_limited"
	case e.StatusCode == 555:
		info.Code = "server_error"
		info.Hint = "Bitbucket timed out; retry later"
	case e.StatusCode >= 500:
		info.Code = "server_error"
	default:
		info.Code = "error"
	}
	if info.Hint == "" && e.Detail != "" {
		info.Hint = e.Detail
	}
	return info
}

// PrintError reports err on stderr (as one JSON line when asJSON) and returns the exit code.
func PrintError(ios *iostreams.IOStreams, err error, asJSON bool) int {
	info := Classify(err)
	if info.Silent {
		return info.Exit
	}
	if asJSON {
		b, _ := json.Marshal(map[string]ErrorInfo{"error": info})
		fmt.Fprintln(ios.ErrOut, string(b))
		return info.Exit
	}
	fmt.Fprintf(ios.ErrOut, "error: %s\n", info.Message)
	if info.Hint != "" {
		fmt.Fprintf(ios.ErrOut, "hint: %s\n", info.Hint)
	}
	return info.Exit
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/cmdutil/`
Expected: `ok  github.com/khipu/khbb/internal/cmdutil`

- [ ] **Step 5: Commit**

```bash
git add internal/cmdutil
git commit -m "feat(cmdutil): classify errors into codes, hints and exit codes"
```

---

### Task 7: Factory, JSON output, confirmation and shared flags

**Files:**
- Modify: `internal/cmdutil/factory.go` (replace whole file)
- Create: `internal/cmdutil/json.go`, `internal/cmdutil/confirm.go`, `internal/cmdutil/flags.go`
- Test: `internal/cmdutil/json_test.go`, `internal/cmdutil/confirm_test.go`, `internal/cmdutil/factory_test.go`

**Interfaces:**
- Consumes: `iostreams` (Task 1), `bitbucket.Client` (Task 2), `config.Config` (Task 4), `gitctx` (Task 5), errors (Task 6).
- Produces:
  - `cmdutil.Prompter` interface: `Input(prompt, defaultValue string) (string, error)`, `Password(prompt string) (string, error)`, `Confirm(prompt string, defaultValue bool) (bool, error)` — satisfied by go-gh `*prompter.Prompter` and `*prompter.PrompterMock`.
  - `cmdutil.Factory` fields: `AppVersion, BuildCommit, BuildDate string; IOStreams *iostreams.IOStreams; Prompter Prompter; Git *gitctx.Resolver; Config func() (*config.Config, error); HTTPClient func() (*bitbucket.Client, error); RepoOverride string; DryRun bool`. Methods: `BaseRepo() (gitctx.Repo, error)` (flag → `KHBB_REPO` → git), `Branch() (string, error)`.
  - `cmdutil.Exporter` interface: `Fields() []string`, `Write(ios *iostreams.IOStreams, data any) error`; `cmdutil.AddJSONFlags(cmd *cobra.Command, exporter *Exporter, fields []string)` — `*exporter` is non-nil after parsing iff `--json` was given. Output values are the requested JSON keys of `data` (a struct, pointer, or slice of either), taken from its `json` tags.
  - `cmdutil.ConfirmDestructive(ios *iostreams.IOStreams, p Prompter, yes bool, question string) error` — callers pass `yes || dryRun`.
  - `cmdutil.EnableRepoOverride(cmd *cobra.Command, f *Factory)` (persistent `-R/--repo`), `cmdutil.AddDryRunFlag(cmd *cobra.Command, f *Factory)`, `cmdutil.AddYesFlag(cmd *cobra.Command, yes *bool)`.

- [ ] **Step 1: Write the failing tests**

`internal/cmdutil/json_test.go`:

```go
package cmdutil_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
)

type sample struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
}

var samples = []sample{{1, "Add widgets", "OPEN"}, {2, "Fix gears", "MERGED"}}

func runJSON(t *testing.T, data any, args ...string) (string, error) {
	t.Helper()
	ios, _, out, _ := iostreams.Test()
	var exporter cmdutil.Exporter
	cmd := &cobra.Command{
		Use: "sample",
		RunE: func(*cobra.Command, []string) error {
			if exporter == nil {
				return errors.New("exporter not set")
			}
			return exporter.Write(ios, data)
		},
	}
	cmdutil.AddJSONFlags(cmd, &exporter, []string{"id", "title", "state"})
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	return out.String(), err
}

func TestJSON_FiltersFieldsOnList(t *testing.T) {
	out, err := runJSON(t, samples, "--json", "id,state")
	if err != nil {
		t.Fatal(err)
	}
	if want := `[{"id":1,"state":"OPEN"},{"id":2,"state":"MERGED"}]` + "\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

func TestJSON_SingleObject(t *testing.T) {
	out, err := runJSON(t, &samples[0], "--json", "title,id")
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"id":1,"title":"Add widgets"}` + "\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

func TestJSON_EmptyListIsArray(t *testing.T) {
	for _, data := range []any{[]sample{}, []sample(nil)} {
		out, err := runJSON(t, data, "--json", "id")
		if err != nil {
			t.Fatal(err)
		}
		if out != "[]\n" {
			t.Errorf("out = %q, want []", out)
		}
	}
}

func TestJSON_JQ(t *testing.T) {
	out, err := runJSON(t, samples, "--json", "title", "--jq", ".[].title")
	if err != nil {
		t.Fatal(err)
	}
	if out != "Add widgets\nFix gears\n" {
		t.Errorf("out = %q", out)
	}
}

func TestJSON_Template(t *testing.T) {
	out, err := runJSON(t, samples, "--json", "id,title", "--template", `{{range .}}#{{.id}} {{.title}}{{"\n"}}{{end}}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "#1 Add widgets\n#2 Fix gears\n" {
		t.Errorf("out = %q", out)
	}
}

func TestJSON_UnknownFieldListsAvailable(t *testing.T) {
	_, err := runJSON(t, samples, "--json", "author")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) {
		t.Fatalf("expected FlagError, got %v", err)
	}
	if !strings.Contains(err.Error(), `unknown JSON field: "author"`) || !strings.Contains(err.Error(), "  state") {
		t.Errorf("message = %q", err.Error())
	}
}

func TestJSON_NoFieldsListsAvailable(t *testing.T) {
	_, err := runJSON(t, samples, "--json")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "specify one or more comma-separated fields") || !strings.Contains(err.Error(), "  title") {
		t.Errorf("err = %v", err)
	}
}

func TestJSON_FilterFlagsRequireJSON(t *testing.T) {
	_, err := runJSON(t, samples, "--jq", ".")
	if err == nil || !strings.Contains(err.Error(), "without --json") {
		t.Errorf("err = %v", err)
	}
}

func TestJSON_NotRequestedLeavesExporterNil(t *testing.T) {
	_, err := runJSON(t, samples)
	if err == nil || err.Error() != "exporter not set" {
		t.Errorf("err = %v", err)
	}
}
```

`internal/cmdutil/confirm_test.go`:

```go
package cmdutil_test

import (
	"errors"
	"testing"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
)

func TestConfirmDestructive_YesSkipsPrompt(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if err := cmdutil.ConfirmDestructive(ios, nil, true, "Merge #42?"); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmDestructive_NoTerminalRequiresYes(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if err := cmdutil.ConfirmDestructive(ios, nil, false, "Merge #42?"); !errors.Is(err, cmdutil.ErrConfirmationRequired) {
		t.Fatalf("expected ErrConfirmationRequired, got %v", err)
	}
}

func TestConfirmDestructive_NeverPrompt(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	ios.SetNeverPrompt(true)
	if err := cmdutil.ConfirmDestructive(ios, prompter.NewMock(t), false, "Merge #42?"); !errors.Is(err, cmdutil.ErrConfirmationRequired) {
		t.Fatalf("expected ErrConfirmationRequired, got %v", err)
	}
}

func TestConfirmDestructive_Prompt(t *testing.T) {
	for _, answer := range []bool{true, false} {
		ios, _, _, _ := iostreams.Test()
		ios.SetStdinTTY(true)
		ios.SetStdoutTTY(true)
		pm := prompter.NewMock(t)
		pm.RegisterConfirm("Merge #42?", func(_ string, defaultValue bool) (bool, error) {
			if defaultValue {
				t.Error("the default answer must be No")
			}
			return answer, nil
		})
		err := cmdutil.ConfirmDestructive(ios, pm, false, "Merge #42?")
		if answer && err != nil {
			t.Errorf("accepted: %v", err)
		}
		if !answer && !errors.Is(err, cmdutil.ErrCancel) {
			t.Errorf("declined: expected ErrCancel, got %v", err)
		}
	}
}
```

`internal/cmdutil/factory_test.go`:

```go
package cmdutil_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
)

func fakeGit(remotes string) *gitctx.Resolver {
	return &gitctx.Resolver{Git: func(args ...string) (string, error) {
		if args[0] == "remote" {
			return remotes, nil
		}
		return "main", nil
	}}
}

func TestBaseRepo_Precedence(t *testing.T) {
	git := fakeGit("origin\tgit@bitbucket.org:acme/from-git.git (fetch)")

	t.Setenv("KHBB_REPO", "acme/from-env")
	f := &cmdutil.Factory{RepoOverride: "acme/from-flag", Git: git}
	if r, err := f.BaseRepo(); err != nil || r.FullName() != "acme/from-flag" {
		t.Errorf("flag: %v, %v", r, err)
	}
	f.RepoOverride = ""
	if r, err := f.BaseRepo(); err != nil || r.FullName() != "acme/from-env" {
		t.Errorf("env: %v, %v", r, err)
	}
	t.Setenv("KHBB_REPO", "")
	if r, err := f.BaseRepo(); err != nil || r.FullName() != "acme/from-git" {
		t.Errorf("git: %v, %v", r, err)
	}
}

func TestBaseRepo_InvalidFlagIsUsageError(t *testing.T) {
	t.Setenv("KHBB_REPO", "")
	f := &cmdutil.Factory{RepoOverride: "widgets"}
	_, err := f.BaseRepo()
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "WORKSPACE/REPO") {
		t.Errorf("err = %v", err)
	}
}

func TestBaseRepo_NoGit(t *testing.T) {
	t.Setenv("KHBB_REPO", "")
	if _, err := (&cmdutil.Factory{}).BaseRepo(); !errors.Is(err, gitctx.ErrNoRepo) {
		t.Errorf("err = %v", err)
	}
}

func TestSharedFlagsBindToFactory(t *testing.T) {
	f := &cmdutil.Factory{}
	var yes bool
	cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	cmdutil.EnableRepoOverride(cmd, f)
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddYesFlag(cmd, &yes)
	cmd.SetArgs([]string{"-R", "acme/widgets", "--dry-run", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if f.RepoOverride != "acme/widgets" || !f.DryRun || !yes {
		t.Errorf("RepoOverride=%q DryRun=%v yes=%v", f.RepoOverride, f.DryRun, yes)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cmdutil/`
Expected: FAIL — `undefined: cmdutil.Exporter`, `undefined: cmdutil.ConfirmDestructive`, `unknown field RepoOverride`, …

- [ ] **Step 3: Implement**

Replace `internal/cmdutil/factory.go` with:

```go
// Package cmdutil holds what every khbb command shares: the Factory, flag helpers and error types.
package cmdutil

import (
	"errors"
	"fmt"
	"os"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
)

// Prompter asks the user questions on a terminal.
type Prompter interface {
	Input(prompt, defaultValue string) (string, error)
	Password(prompt string) (string, error)
	Confirm(prompt string, defaultValue bool) (bool, error)
}

// Factory provides commands with their dependencies. Function fields are lazy so that
// commands which do not need the network or a repository never touch them.
type Factory struct {
	AppVersion  string
	BuildCommit string
	BuildDate   string

	IOStreams *iostreams.IOStreams
	Prompter  Prompter
	Git       *gitctx.Resolver

	Config     func() (*config.Config, error)
	HTTPClient func() (*bitbucket.Client, error)

	// Bound to flags by EnableRepoOverride and AddDryRunFlag.
	RepoOverride string
	DryRun       bool
}

// BaseRepo resolves the target repository: -R flag, then KHBB_REPO, then git remotes.
func (f *Factory) BaseRepo() (gitctx.Repo, error) {
	if f.RepoOverride != "" {
		r, err := gitctx.ParseRepo(f.RepoOverride)
		if err != nil {
			return gitctx.Repo{}, &FlagError{Err: err}
		}
		return r, nil
	}
	if env := os.Getenv("KHBB_REPO"); env != "" {
		r, err := gitctx.ParseRepo(env)
		if err != nil {
			return gitctx.Repo{}, fmt.Errorf("KHBB_REPO: %w", err)
		}
		return r, nil
	}
	if f.Git == nil {
		return gitctx.Repo{}, gitctx.ErrNoRepo
	}
	return f.Git.BaseRepo()
}

// Branch returns the current git branch.
func (f *Factory) Branch() (string, error) {
	if f.Git == nil {
		return "", errors.New("could not determine current branch: git is not available")
	}
	return f.Git.CurrentBranch()
}
```

`internal/cmdutil/json.go`:

```go
package cmdutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/jq"
	"github.com/cli/go-gh/v2/pkg/jsonpretty"
	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/iostreams"
)

// Exporter writes command output as JSON, optionally filtered by --jq or --template.
type Exporter interface {
	Fields() []string
	Write(ios *iostreams.IOStreams, data any) error
}

type jsonExporter struct {
	fields   []string
	jq       string
	template string
}

// AddJSONFlags registers --json, --jq and --template on cmd. After flag parsing,
// *exporter is non-nil if and only if --json was given.
func AddJSONFlags(cmd *cobra.Command, exporter *Exporter, fields []string) {
	flags := cmd.Flags()
	flags.StringSlice("json", nil, "Output JSON with the specified `fields`")
	flags.StringP("jq", "q", "", "Filter JSON output using a jq `expression`")
	flags.StringP("template", "t", "", "Format JSON output using a Go `template`")

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		jqExpr, _ := flags.GetString("jq")
		tmpl, _ := flags.GetString("template")
		if !flags.Changed("json") {
			if jqExpr != "" || tmpl != "" {
				return FlagErrorf("cannot use --jq or --template without --json")
			}
			return nil
		}
		if jqExpr != "" && tmpl != "" {
			return FlagErrorf("cannot use --jq and --template together")
		}
		requested, _ := flags.GetStringSlice("json")
		if len(requested) == 0 {
			return FlagErrorf("specify one or more comma-separated fields for `--json`:\n%s", fieldList(fields))
		}
		for _, r := range requested {
			if !slices.Contains(fields, r) {
				return FlagErrorf("unknown JSON field: %q\navailable fields:\n%s", r, fieldList(fields))
			}
		}
		*exporter = &jsonExporter{fields: requested, jq: jqExpr, template: tmpl}
		return nil
	}

	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		if c == cmd && strings.Contains(err.Error(), "flag needs an argument") && strings.Contains(err.Error(), "--json") {
			return FlagErrorf("specify one or more comma-separated fields for `--json`:\n%s", fieldList(fields))
		}
		if p := c.Parent(); p != nil {
			return p.FlagErrorFunc()(c, err)
		}
		return &FlagError{Err: err}
	})
}

func fieldList(fields []string) string {
	return "  " + strings.Join(slices.Sorted(slices.Values(fields)), "\n  ")
}

func (e *jsonExporter) Fields() []string { return e.fields }

func (e *jsonExporter) Write(ios *iostreams.IOStreams, data any) error {
	filtered, err := filterFields(data, e.fields)
	if err != nil {
		return err
	}
	buf, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	switch {
	case e.jq != "":
		return jq.EvaluateFormatted(bytes.NewReader(buf), ios.Out, e.jq, "  ", ios.ColorEnabled())
	case e.template != "":
		t := template.New(ios.Out, ios.TerminalWidth(), ios.ColorEnabled())
		if err := t.Parse(e.template); err != nil {
			return err
		}
		if err := t.Execute(bytes.NewReader(buf)); err != nil {
			return err
		}
		return t.Flush()
	case ios.IsStdoutTTY():
		return jsonpretty.Format(ios.Out, bytes.NewReader(buf), "  ", ios.ColorEnabled())
	default:
		_, err := fmt.Fprintf(ios.Out, "%s\n", buf)
		return err
	}
}

// filterFields turns data (a struct, a pointer, or a slice of them) into maps holding only fields.
// Slices always become JSON arrays, never null.
func filterFields(data any, fields []string) (any, error) {
	v := reflect.ValueOf(data)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Slice {
		out := make([]map[string]any, v.Len())
		for i := range v.Len() {
			m, err := pick(v.Index(i).Interface(), fields)
			if err != nil {
				return nil, err
			}
			out[i] = m
		}
		return out, nil
	}
	return pick(v.Interface(), fields)
}

func pick(item any, fields []string) (map[string]any, error) {
	b, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var all map[string]any
	if err := dec.Decode(&all); err != nil {
		return nil, err
	}
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		out[f] = all[f]
	}
	return out, nil
}
```

`internal/cmdutil/confirm.go`:

```go
package cmdutil

import "github.com/khipu/khbb/internal/iostreams"

// ConfirmDestructive guards irreversible actions. With yes it proceeds; on a terminal it asks
// question (default No); otherwise it refuses with ErrConfirmationRequired. Callers pass
// yes || dryRun, since a dry run sends nothing.
func ConfirmDestructive(ios *iostreams.IOStreams, p Prompter, yes bool, question string) error {
	if yes {
		return nil
	}
	if !ios.CanPrompt() || p == nil {
		return ErrConfirmationRequired
	}
	ok, err := p.Confirm(question, false)
	if err != nil {
		return err
	}
	if !ok {
		return ErrCancel
	}
	return nil
}
```

`internal/cmdutil/flags.go`:

```go
package cmdutil

import "github.com/spf13/cobra"

// EnableRepoOverride adds a persistent -R/--repo flag bound to f.RepoOverride.
func EnableRepoOverride(cmd *cobra.Command, f *Factory) {
	cmd.PersistentFlags().StringVarP(&f.RepoOverride, "repo", "R", "", "Select a repository using the `WORKSPACE/REPO` format")
}

// AddDryRunFlag adds --dry-run bound to f.DryRun.
func AddDryRunFlag(cmd *cobra.Command, f *Factory) {
	cmd.Flags().BoolVar(&f.DryRun, "dry-run", false, "Print the request that would be sent without sending it")
}

// AddYesFlag adds --yes for destructive commands.
func AddYesFlag(cmd *cobra.Command, yes *bool) {
	cmd.Flags().BoolVar(yes, "yes", false, "Skip the confirmation prompt")
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/cmdutil/ && go build ./...`
Expected: `ok  github.com/khipu/khbb/internal/cmdutil`; build succeeds (`cmd/khbb/main.go` still compiles because it only sets fields that still exist).

- [ ] **Step 5: Commit**

```bash
git add internal/cmdutil
git commit -m "feat(cmdutil): add factory, JSON exporter, confirmation and shared flags"
```

---

### Task 8: Production factory and entrypoint wiring

**Files:**
- Create: `pkg/cmd/factory/default.go`
- Modify: `cmd/khbb/main.go` (replace whole file), `pkg/cmd/root/root.go` (add flag error func)
- Test: `pkg/cmd/factory/default_test.go`, `cmd/khbb/main_test.go`, `pkg/cmd/root/root_test.go` (add a test)

**Interfaces:**
- Consumes: everything from Tasks 1–7.
- Produces: `factory.New(version, commit, date string) *cmdutil.Factory`. Root flag-parsing errors become `*cmdutil.FlagError`. `main.run(args []string) int`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/factory/default_test.go`:

```go
package factory_test

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/factory"
)

func TestHTTPClient_NotLoggedIn(t *testing.T) {
	keyring.MockInit()
	t.Setenv("KHBB_CONFIG_DIR", t.TempDir())
	t.Setenv("KHBB_TOKEN", "")
	f := factory.New("dev", "", "")
	_, err := f.HTTPClient()
	var authErr *cmdutil.AuthError
	if !errors.As(err, &authErr) || authErr.Msg != "not logged in to bitbucket.org" {
		t.Fatalf("expected AuthError, got %v", err)
	}
}

func TestHTTPClient_FromEnvironment(t *testing.T) {
	keyring.MockInit()
	t.Setenv("KHBB_CONFIG_DIR", t.TempDir())
	t.Setenv("KHBB_TOKEN", "s3cret")
	t.Setenv("KHBB_EMAIL", "dev@example.com")
	f := factory.New("dev", "", "")
	c, err := f.HTTPClient()
	if err != nil || c == nil {
		t.Fatalf("HTTPClient() = %v, %v", c, err)
	}
}

func TestConfigIsCached(t *testing.T) {
	t.Setenv("KHBB_CONFIG_DIR", t.TempDir())
	f := factory.New("dev", "", "")
	a, err := f.Config()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := f.Config()
	if a != b {
		t.Error("Config() must return the same instance")
	}
}
```

`cmd/khbb/main_test.go`:

```go
package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestWantsJSON(t *testing.T) {
	if wantsJSON(nil) {
		t.Error("nil command")
	}
	cmd := &cobra.Command{Use: "x"}
	if wantsJSON(cmd) {
		t.Error("no --json flag registered")
	}
	cmd.Flags().StringSlice("json", nil, "")
	if wantsJSON(cmd) {
		t.Error("--json not set")
	}
	if err := cmd.Flags().Set("json", "id"); err != nil {
		t.Fatal(err)
	}
	if !wantsJSON(cmd) {
		t.Error("--json set")
	}
}
```

Append to `pkg/cmd/root/root_test.go` (add `"errors"` to its imports):

```go
func TestRootUnknownFlagIsUsageError(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{"--nope"})
	err := cmd.Execute()
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) {
		t.Fatalf("expected FlagError, got %T: %v", err, err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/factory/ ./cmd/khbb/ ./pkg/cmd/root/`
Expected: FAIL — `package …/pkg/cmd/factory` has no Go files; `undefined: wantsJSON`; root test gets a plain error instead of `*FlagError`.

- [ ] **Step 3: Implement**

`pkg/cmd/factory/default.go`:

```go
// Package factory builds the Factory used by the khbb binary.
package factory

import (
	"errors"
	"os"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
)

// New returns the production Factory.
func New(version, commit, date string) *cmdutil.Factory {
	ios := iostreams.System()
	if os.Getenv("KHBB_PROMPT_DISABLED") != "" {
		ios.SetNeverPrompt(true)
	}
	f := &cmdutil.Factory{
		AppVersion:  version,
		BuildCommit: commit,
		BuildDate:   date,
		IOStreams:   ios,
		// Prompts render on stderr so that stdout carries only data.
		Prompter: prompter.New(os.Stdin, os.Stderr, os.Stderr),
		Git:      gitctx.NewResolver(""),
	}
	f.Config = cachedConfig()
	f.HTTPClient = func() (*bitbucket.Client, error) { return newClient(f) }
	return f
}

func cachedConfig() func() (*config.Config, error) {
	var cfg *config.Config
	return func() (*config.Config, error) {
		if cfg != nil {
			return cfg, nil
		}
		c, err := config.Load()
		if err != nil {
			return nil, err
		}
		cfg = c
		return cfg, nil
	}
}

func newClient(f *cmdutil.Factory) (*bitbucket.Client, error) {
	cfg, err := f.Config()
	if err != nil {
		return nil, err
	}
	creds, err := config.ResolveCredentials(cfg)
	if errors.Is(err, config.ErrNoCredentials) {
		return nil, &cmdutil.AuthError{Msg: "not logged in to bitbucket.org"}
	}
	if err != nil {
		return nil, err
	}
	opts := bitbucket.Options{
		Email:     creds.Email,
		Token:     creds.Token,
		UserAgent: "khbb/" + f.AppVersion,
		DryRun:    f.DryRun,
		DryRunOut: f.IOStreams.Out,
	}
	if os.Getenv("KHBB_DEBUG") != "" {
		opts.Debug = f.IOStreams.ErrOut
	}
	return bitbucket.New(opts), nil
}
```

Replace `cmd/khbb/main.go` with:

```go
package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/factory"
	"github.com/khipu/khbb/pkg/cmd/root"
)

// Set with -ldflags at build time.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	f := factory.New(version, commit, date)
	rootCmd := root.NewCmdRoot(f)
	rootCmd.SetArgs(args)
	cmd, err := rootCmd.ExecuteC()
	if err != nil {
		return cmdutil.PrintError(f.IOStreams, err, wantsJSON(cmd))
	}
	return 0
}

// wantsJSON reports whether the command that failed was asked for JSON output.
func wantsJSON(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	fl := cmd.Flags().Lookup("json")
	return fl != nil && fl.Changed
}
```

In `pkg/cmd/root/root.go`, after `cmd.CompletionOptions.HiddenDefaultCmd = true`, add:

```go
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &cmdutil.FlagError{Err: err}
	})
```

- [ ] **Step 4: Run tests and a smoke check**

Run: `go test ./... && make build && ./bin/khbb version && ./bin/khbb --nope; echo "exit=$?"`
Expected: all `ok`; `khbb version <describe>`; then `error: unknown flag: --nope`, `hint: see `--help` for usage`, `exit=1`.

- [ ] **Step 5: Commit**

```bash
git add cmd pkg
git commit -m "feat: wire production factory and exit-code mapping"
```

---

### Task 9: `khbb auth login`

**Files:**
- Create: `pkg/cmd/auth/auth.go`, `pkg/cmd/auth/login/login.go`
- Modify: `pkg/cmd/root/root.go` (register `auth`)
- Test: `pkg/cmd/auth/login/login_test.go`

**Interfaces:**
- Consumes: `cmdutil.Factory`, `cmdutil.Prompter`, `cmdutil.FlagErrorf`, `cmdutil.AuthError` (Tasks 6–7); `bitbucket.New`, `bitbucket.RequiredScopes`, `(*Client).CurrentUser`, `bitbucket.HTTPError` (Tasks 2–3); `config.Config`, `config.StoreToken`, `config.DeleteToken` (Task 4); `httpmock` (Task 2).
- Produces: `auth.NewCmdAuth(f *cmdutil.Factory) *cobra.Command` (Task 10 adds `status` and `logout` to it); `login.LoginOptions`; `login.NewCmdLogin(f *cmdutil.Factory, runF func(*LoginOptions) error) *cobra.Command`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/auth/login/login_test.go`:

```go
package login

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/internal/iostreams"
)

const userJSON = `{"display_name":"Ada Example","nickname":"ada","uuid":"{00000000-0000-0000-0000-000000000001}","account_id":"000000:aaaa"}`

type fixture struct {
	opts    *LoginOptions
	ios     *iostreams.IOStreams
	cfg     *config.Config
	reg     *httpmock.Registry
	stdin   *bytes.Buffer
	stderr  *bytes.Buffer
	stored  map[string]string
	deleted []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("KHBB_TOKEN", "")
	ios, stdin, _, stderr := iostreams.Test()
	cfg, err := config.LoadFile(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	fx := &fixture{ios: ios, cfg: cfg, reg: httpmock.New(t), stdin: stdin, stderr: stderr, stored: map[string]string{}}
	fx.opts = &LoginOptions{
		IO:     ios,
		Config: func() (*config.Config, error) { return cfg, nil },
		NewClient: func(email, token string) *bitbucket.Client {
			return bitbucket.New(bitbucket.Options{Email: email, Token: token, HTTPClient: fx.reg.Client()})
		},
		StoreToken:  func(email, token string) error { fx.stored[email] = token; return nil },
		DeleteToken: func(email string) error { fx.deleted = append(fx.deleted, email); return nil },
	}
	return fx
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestNewCmdLogin_ParsesFlags(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	var got *LoginOptions
	cmd := NewCmdLogin(&cmdutil.Factory{IOStreams: ios}, func(o *LoginOptions) error { got = o; return nil })
	cmd.SetArgs([]string{"--email", "dev@example.com", "--with-token", "--insecure-storage"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Email != "dev@example.com" || !got.WithToken || !got.InsecureStorage {
		t.Errorf("parsed %+v", got)
	}
}

func TestLogin_WithTokenFromStdin(t *testing.T) {
	fx := newFixture(t)
	fx.stdin.WriteString("s3cret\r\n")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if err := loginRun(context.Background(), fx.opts); err != nil {
		t.Fatal(err)
	}
	if fx.stored["dev@example.com"] != "s3cret" {
		t.Errorf("stored = %v", fx.stored)
	}
	user, pass, _ := (&http.Request{Header: fx.reg.Calls[0].Header}).BasicAuth()
	if user != "dev@example.com" || pass != "s3cret" {
		t.Errorf("validated with %q/%q", user, pass)
	}
	saved, _ := config.LoadFile(fx.cfg.Path())
	if saved.Email != "dev@example.com" || saved.Username != "ada" || saved.InsecureToken != "" {
		t.Errorf("saved config = %+v", saved)
	}
	if !strings.Contains(fx.stderr.String(), "Logged in to bitbucket.org as ada") {
		t.Errorf("stderr = %q", fx.stderr.String())
	}
}

func TestLogin_NonInteractiveRequiresEmail(t *testing.T) {
	fx := newFixture(t)
	fx.opts.WithToken = true
	err := loginRun(context.Background(), fx.opts)
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "--email") {
		t.Errorf("err = %v", err)
	}
}

func TestLogin_NonInteractiveRequiresWithToken(t *testing.T) {
	fx := newFixture(t)
	fx.opts.Email = "dev@example.com"
	err := loginRun(context.Background(), fx.opts)
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "--with-token") {
		t.Errorf("err = %v", err)
	}
}

func TestLogin_InteractivePromptsAndListsScopes(t *testing.T) {
	fx := newFixture(t)
	fx.ios.SetStdinTTY(true)
	fx.ios.SetStdoutTTY(true)
	pm := prompter.NewMock(t)
	pm.RegisterInput("Atlassian account email:", func(_, _ string) (string, error) { return " dev@example.com ", nil })
	pm.RegisterPassword("API token:", func(string) (string, error) { return "s3cret", nil })
	fx.opts.Prompter = pm
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if err := loginRun(context.Background(), fx.opts); err != nil {
		t.Fatal(err)
	}
	if fx.stored["dev@example.com"] != "s3cret" {
		t.Errorf("stored = %v", fx.stored)
	}
	for _, want := range []string{tokenURL, "write:pipeline:bitbucket", "read:workspace:bitbucket"} {
		if !strings.Contains(fx.stderr.String(), want) {
			t.Errorf("stderr missing %q:\n%s", want, fx.stderr.String())
		}
	}
}

func TestLogin_RejectsInvalidToken(t *testing.T) {
	fx := newFixture(t)
	fx.stdin.WriteString("wrong")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(401, `{"type":"error","error":{"message":"Unauthorized"}}`))

	err := loginRun(context.Background(), fx.opts)
	var authErr *cmdutil.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthError, got %v", err)
	}
	if len(fx.stored) != 0 || fileExists(fx.cfg.Path()) {
		t.Error("an invalid token must not be stored")
	}
}

func TestLogin_KeyringFailureSuggestsAlternatives(t *testing.T) {
	fx := newFixture(t)
	fx.stdin.WriteString("s3cret")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.opts.StoreToken = func(string, string) error { return errors.New("no secret service") }
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	err := loginRun(context.Background(), fx.opts)
	if err == nil || !strings.Contains(err.Error(), "--insecure-storage") || !strings.Contains(err.Error(), "KHBB_TOKEN") {
		t.Fatalf("err = %v", err)
	}
	if fileExists(fx.cfg.Path()) {
		t.Error("config must not be written when storing the token failed")
	}
}

func TestLogin_InsecureStorage(t *testing.T) {
	fx := newFixture(t)
	fx.stdin.WriteString("s3cret")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.opts.InsecureStorage = true
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if err := loginRun(context.Background(), fx.opts); err != nil {
		t.Fatal(err)
	}
	if len(fx.stored) != 0 {
		t.Error("keyring must not be used with --insecure-storage")
	}
	saved, _ := config.LoadFile(fx.cfg.Path())
	if saved.InsecureToken != "s3cret" {
		t.Errorf("InsecureToken = %q", saved.InsecureToken)
	}
	if !strings.Contains(fx.stderr.String(), "plain text") {
		t.Errorf("missing plain-text warning: %q", fx.stderr.String())
	}
}

func TestLogin_SwitchingAccountsDeletesOldToken(t *testing.T) {
	fx := newFixture(t)
	fx.cfg.Email = "old@example.com"
	fx.stdin.WriteString("s3cret")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if err := loginRun(context.Background(), fx.opts); err != nil {
		t.Fatal(err)
	}
	if len(fx.deleted) != 1 || fx.deleted[0] != "old@example.com" {
		t.Errorf("deleted = %v", fx.deleted)
	}
}

func TestLogin_WarnsWhenEnvTokenSet(t *testing.T) {
	fx := newFixture(t)
	t.Setenv("KHBB_TOKEN", "from-env")
	fx.stdin.WriteString("s3cret")
	fx.opts.Email = "dev@example.com"
	fx.opts.WithToken = true
	fx.reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))

	if err := loginRun(context.Background(), fx.opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fx.stderr.String(), "KHBB_TOKEN is set") {
		t.Errorf("stderr = %q", fx.stderr.String())
	}
}
```

Add `"net/http"` to the test imports.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/auth/...`
Expected: FAIL — `undefined: LoginOptions`, `undefined: loginRun`, `undefined: tokenURL`.

- [ ] **Step 3: Implement**

`pkg/cmd/auth/login/login.go`:

```go
// Package login implements `khbb auth login`.
package login

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/iostreams"
)

const tokenURL = "https://id.atlassian.com/manage-profile/security/api-tokens"

// LoginOptions holds the inputs and dependencies of `khbb auth login`.
type LoginOptions struct {
	IO          *iostreams.IOStreams
	Config      func() (*config.Config, error)
	Prompter    cmdutil.Prompter
	NewClient   func(email, token string) *bitbucket.Client
	StoreToken  func(email, token string) error
	DeleteToken func(email string) error

	Email           string
	WithToken       bool
	InsecureStorage bool
}

// NewCmdLogin returns `khbb auth login`.
func NewCmdLogin(f *cmdutil.Factory, runF func(*LoginOptions) error) *cobra.Command {
	opts := &LoginOptions{
		IO:       f.IOStreams,
		Config:   f.Config,
		Prompter: f.Prompter,
		NewClient: func(email, token string) *bitbucket.Client {
			return bitbucket.New(bitbucket.Options{Email: email, Token: token, UserAgent: "khbb/" + f.AppVersion})
		},
		StoreToken:  config.StoreToken,
		DeleteToken: config.DeleteToken,
	}
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to Bitbucket Cloud with an Atlassian API token",
		Long: fmt.Sprintf(`Authenticate with Bitbucket Cloud using an Atlassian API token.

Create a token at %s
("Create API token with scopes", app: Bitbucket) with these scopes:
  %s

The token is stored in the system keyring. To use credentials from the
environment instead, set KHBB_EMAIL and KHBB_TOKEN.`, tokenURL, strings.Join(bitbucket.RequiredScopes, "\n  ")),
		Example: `  # Interactive
  $ khbb auth login

  # Non-interactive (scripts, agents)
  $ echo "$TOKEN" | khbb auth login --email you@example.com --with-token`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return loginRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Email, "email", "", "Atlassian account `email`")
	cmd.Flags().BoolVar(&opts.WithToken, "with-token", false, "Read the API token from standard input")
	cmd.Flags().BoolVar(&opts.InsecureStorage, "insecure-storage", false, "Store the token in plain text in the config file instead of the system keyring")
	return cmd
}

func loginRun(ctx context.Context, opts *LoginOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	if os.Getenv("KHBB_TOKEN") != "" {
		fmt.Fprintln(opts.IO.ErrOut, "warning: KHBB_TOKEN is set and takes precedence over stored credentials")
	}

	email, err := readEmail(opts, cfg.Email)
	if err != nil {
		return err
	}
	token, err := readToken(opts)
	if err != nil {
		return err
	}

	user, err := opts.NewClient(email, token).CurrentUser(ctx)
	if err != nil {
		var httpErr *bitbucket.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized {
			return &cmdutil.AuthError{Msg: "authentication failed: check the email and the API token"}
		}
		return err
	}

	previous := cfg.Email
	if opts.InsecureStorage {
		cfg.InsecureToken = token
	} else {
		if err := opts.StoreToken(email, token); err != nil {
			return fmt.Errorf("could not store the token in the system keyring: %w\n"+
				"set KHBB_EMAIL and KHBB_TOKEN instead, or rerun with --insecure-storage", err)
		}
		cfg.InsecureToken = ""
	}
	if previous != "" && previous != email {
		_ = opts.DeleteToken(previous)
	}
	cfg.Email = email
	cfg.Username = user.Nickname
	if err := cfg.Save(); err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "Logged in to bitbucket.org as %s\n", user.Nickname)
	if opts.InsecureStorage {
		fmt.Fprintf(opts.IO.ErrOut, "warning: the token is stored in plain text in %s\n", cfg.Path())
	}
	return nil
}

func readEmail(opts *LoginOptions, current string) (string, error) {
	email := strings.TrimSpace(opts.Email)
	if email != "" {
		return email, nil
	}
	if !opts.IO.CanPrompt() {
		return "", cmdutil.FlagErrorf("--email is required when not running interactively")
	}
	answer, err := opts.Prompter.Input("Atlassian account email:", current)
	if err != nil {
		return "", err
	}
	if email = strings.TrimSpace(answer); email == "" {
		return "", cmdutil.FlagErrorf("email cannot be empty")
	}
	return email, nil
}

func readToken(opts *LoginOptions) (string, error) {
	var token string
	switch {
	case opts.WithToken:
		b, err := io.ReadAll(opts.IO.In)
		if err != nil {
			return "", fmt.Errorf("reading the token from standard input: %w", err)
		}
		token = string(b)
	case opts.IO.CanPrompt():
		fmt.Fprintf(opts.IO.ErrOut, "Create an API token at %s\nwith these scopes:\n", tokenURL)
		for _, s := range bitbucket.RequiredScopes {
			fmt.Fprintf(opts.IO.ErrOut, "  - %s\n", s)
		}
		answer, err := opts.Prompter.Password("API token:")
		if err != nil {
			return "", err
		}
		token = answer
	default:
		return "", cmdutil.FlagErrorf("--with-token is required when not running interactively")
	}
	if token = strings.TrimSpace(token); token == "" {
		return "", cmdutil.FlagErrorf("the API token cannot be empty")
	}
	return token, nil
}
```

`pkg/cmd/auth/auth.go`:

```go
// Package auth groups the `khbb auth` commands.
package auth

import (
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/auth/login"
)

// NewCmdAuth returns `khbb auth`.
func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth <command>",
		Short: "Authenticate khbb with Bitbucket Cloud",
	}
	cmd.AddCommand(login.NewCmdLogin(f, nil))
	return cmd
}
```

In `pkg/cmd/root/root.go`, add the import `authCmd "github.com/khipu/khbb/pkg/cmd/auth"` and change the `AddCommand` line to:

```go
	cmd.AddCommand(
		authCmd.NewCmdAuth(f),
		versionCmd.NewCmdVersion(f),
	)
```

- [ ] **Step 4: Run the tests**

Run: `go test ./pkg/... && go build ./...`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add pkg
git commit -m "feat(auth): add khbb auth login"
```

---

### Task 10: `khbb auth status` and `khbb auth logout`

**Files:**
- Create: `pkg/cmd/auth/status/status.go`, `pkg/cmd/auth/logout/logout.go`
- Modify: `pkg/cmd/auth/auth.go` (register both)
- Test: `pkg/cmd/auth/status/status_test.go`, `pkg/cmd/auth/logout/logout_test.go`

**Interfaces:**
- Consumes: as in Task 9, plus `config.ResolveCredentials`, `config.Credentials`, `config.ErrNoCredentials`.
- Produces: `status.NewCmdStatus(f, runF)`, `status.StatusOptions{IO; Config; NewClient func(email, token string) *bitbucket.Client; Resolve func(*config.Config) (config.Credentials, error)}`; `logout.NewCmdLogout(f, runF)`, `logout.LogoutOptions{IO; Config; DeleteToken func(email string) error}`.
- Behavior: `auth status` exits 0 and prints to stdout when the token works; exits 4 (`AuthError`) when not logged in or on 401.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/auth/status/status_test.go`:

```go
package status

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/internal/iostreams"
)

func newOpts(t *testing.T, creds config.Credentials, credsErr error) (*StatusOptions, *httpmock.Registry, func() string) {
	t.Helper()
	ios, _, out, _ := iostreams.Test()
	cfg, _ := config.LoadFile(filepath.Join(t.TempDir(), "config.yml"))
	reg := httpmock.New(t)
	return &StatusOptions{
		IO:     ios,
		Config: func() (*config.Config, error) { return cfg, nil },
		NewClient: func(email, token string) *bitbucket.Client {
			return bitbucket.New(bitbucket.Options{Email: email, Token: token, HTTPClient: reg.Client()})
		},
		Resolve: func(*config.Config) (config.Credentials, error) { return creds, credsErr },
	}, reg, out.String
}

var keyringCreds = config.Credentials{Email: "dev@example.com", Token: "s3cret", Source: config.SourceKeyring}

func TestStatus_LoggedIn(t *testing.T) {
	opts, reg, out := newOpts(t, keyringCreds, nil)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, `{"display_name":"Ada Example","nickname":"ada"}`))

	if err := statusRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	want := "bitbucket.org\n  Logged in as ada (Ada Example)\n  Email: dev@example.com\n  Token source: keyring\n"
	if out() != want {
		t.Errorf("out = %q, want %q", out(), want)
	}
}

func TestStatus_NotLoggedIn(t *testing.T) {
	opts, _, _ := newOpts(t, config.Credentials{}, config.ErrNoCredentials)
	err := statusRun(context.Background(), opts)
	var authErr *cmdutil.AuthError
	if !errors.As(err, &authErr) || cmdutil.Classify(err).Exit != 4 {
		t.Errorf("err = %v", err)
	}
}

func TestStatus_InvalidToken(t *testing.T) {
	opts, reg, _ := newOpts(t, keyringCreds, nil)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(401, `{}`))
	err := statusRun(context.Background(), opts)
	var authErr *cmdutil.AuthError
	if !errors.As(err, &authErr) || !strings.Contains(err.Error(), "keyring") {
		t.Errorf("err = %v", err)
	}
}

func TestStatus_CredentialErrorPassesThrough(t *testing.T) {
	boom := errors.New("KHBB_EMAIL must be set when KHBB_TOKEN is set")
	opts, _, _ := newOpts(t, config.Credentials{}, boom)
	if err := statusRun(context.Background(), opts); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
}
```

`pkg/cmd/auth/logout/logout_test.go`:

```go
package logout

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/iostreams"
)

func newOpts(t *testing.T) (*LogoutOptions, *config.Config, *[]string, func() string) {
	t.Helper()
	t.Setenv("KHBB_TOKEN", "")
	ios, _, _, errOut := iostreams.Test()
	cfg, _ := config.LoadFile(filepath.Join(t.TempDir(), "config.yml"))
	var deleted []string
	return &LogoutOptions{
		IO:          ios,
		Config:      func() (*config.Config, error) { return cfg, nil },
		DeleteToken: func(email string) error { deleted = append(deleted, email); return nil },
	}, cfg, &deleted, errOut.String
}

func TestLogout_RemovesTokenAndClearsConfig(t *testing.T) {
	opts, cfg, deleted, errOut := newOpts(t)
	cfg.Email, cfg.Username, cfg.GitProtocol = "dev@example.com", "ada", "ssh"

	if err := logoutRun(opts); err != nil {
		t.Fatal(err)
	}
	if len(*deleted) != 1 || (*deleted)[0] != "dev@example.com" {
		t.Errorf("deleted = %v", *deleted)
	}
	saved, _ := config.LoadFile(cfg.Path())
	if saved.Email != "" || saved.Username != "" || saved.GitProtocol != "ssh" {
		t.Errorf("saved = %+v (preferences must be kept)", saved)
	}
	if !strings.Contains(errOut(), "Logged out of bitbucket.org (ada)") {
		t.Errorf("stderr = %q", errOut())
	}
}

func TestLogout_NotLoggedIn(t *testing.T) {
	opts, _, _, _ := newOpts(t)
	if err := logoutRun(opts); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("err = %v", err)
	}
}

func TestLogout_InsecureStorageIgnoresKeyringErrors(t *testing.T) {
	opts, cfg, _, _ := newOpts(t)
	cfg.Email, cfg.InsecureToken = "dev@example.com", "plain"
	opts.DeleteToken = func(string) error { return errors.New("no secret service") }

	if err := logoutRun(opts); err != nil {
		t.Fatal(err)
	}
	saved, _ := config.LoadFile(cfg.Path())
	if saved.InsecureToken != "" {
		t.Error("plain-text token must be removed")
	}
}

func TestLogout_KeyringErrorFails(t *testing.T) {
	opts, cfg, _, _ := newOpts(t)
	cfg.Email = "dev@example.com"
	opts.DeleteToken = func(string) error { return errors.New("no secret service") }
	if err := logoutRun(opts); err == nil || !strings.Contains(err.Error(), "keyring") {
		t.Errorf("err = %v", err)
	}
}

func TestLogout_WarnsWhenEnvTokenSet(t *testing.T) {
	opts, cfg, _, errOut := newOpts(t)
	cfg.Email = "dev@example.com"
	t.Setenv("KHBB_TOKEN", "from-env")
	if err := logoutRun(opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut(), "KHBB_TOKEN is still set") {
		t.Errorf("stderr = %q", errOut())
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/auth/...`
Expected: FAIL — `undefined: StatusOptions`, `undefined: LogoutOptions`, …

- [ ] **Step 3: Implement**

`pkg/cmd/auth/status/status.go`:

```go
// Package status implements `khbb auth status`.
package status

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/iostreams"
)

// StatusOptions holds the dependencies of `khbb auth status`.
type StatusOptions struct {
	IO        *iostreams.IOStreams
	Config    func() (*config.Config, error)
	NewClient func(email, token string) *bitbucket.Client
	Resolve   func(*config.Config) (config.Credentials, error)
}

// NewCmdStatus returns `khbb auth status`.
func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{
		IO:     f.IOStreams,
		Config: f.Config,
		NewClient: func(email, token string) *bitbucket.Client {
			return bitbucket.New(bitbucket.Options{Email: email, Token: token, UserAgent: "khbb/" + f.AppVersion})
		},
		Resolve: config.ResolveCredentials,
	}
	return &cobra.Command{
		Use:   "status",
		Short: "Show and verify the current authentication",
		Long:  "Show which account khbb uses and verify the token. Exits with status 4 when not logged in or when the token is rejected.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return statusRun(cmd.Context(), opts)
		},
	}
}

func statusRun(ctx context.Context, opts *StatusOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	creds, err := opts.Resolve(cfg)
	if errors.Is(err, config.ErrNoCredentials) {
		return &cmdutil.AuthError{Msg: "not logged in to bitbucket.org"}
	}
	if err != nil {
		return err
	}
	user, err := opts.NewClient(creds.Email, creds.Token).CurrentUser(ctx)
	if err != nil {
		var httpErr *bitbucket.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized {
			return &cmdutil.AuthError{Msg: fmt.Sprintf("the token for %s (from %s) is invalid or expired", creds.Email, creds.Source)}
		}
		return err
	}
	fmt.Fprintf(opts.IO.Out, "bitbucket.org\n  Logged in as %s (%s)\n  Email: %s\n  Token source: %s\n",
		user.Nickname, user.DisplayName, creds.Email, creds.Source)
	return nil
}
```

`pkg/cmd/auth/logout/logout.go`:

```go
// Package logout implements `khbb auth logout`.
package logout

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/iostreams"
)

// LogoutOptions holds the dependencies of `khbb auth logout`.
type LogoutOptions struct {
	IO          *iostreams.IOStreams
	Config      func() (*config.Config, error)
	DeleteToken func(email string) error
}

// NewCmdLogout returns `khbb auth logout`.
func NewCmdLogout(f *cmdutil.Factory, runF func(*LogoutOptions) error) *cobra.Command {
	opts := &LogoutOptions{IO: f.IOStreams, Config: f.Config, DeleteToken: config.DeleteToken}
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored Bitbucket credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return logoutRun(opts)
		},
	}
}

func logoutRun(opts *LogoutOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	if cfg.Email == "" && cfg.InsecureToken == "" {
		return errors.New("not logged in to bitbucket.org")
	}
	if cfg.Email != "" {
		// With insecure storage the keyring may be unavailable altogether; the file token is what matters.
		if err := opts.DeleteToken(cfg.Email); err != nil && cfg.InsecureToken == "" {
			return fmt.Errorf("removing the token from the system keyring: %w", err)
		}
	}
	name := cfg.Username
	if name == "" {
		name = cfg.Email
	}
	cfg.Email, cfg.Username, cfg.InsecureToken = "", "", ""
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "Logged out of bitbucket.org (%s)\n", name)
	if os.Getenv("KHBB_TOKEN") != "" {
		fmt.Fprintln(opts.IO.ErrOut, "warning: KHBB_TOKEN is still set and will keep authenticating khbb")
	}
	return nil
}
```

In `pkg/cmd/auth/auth.go`, add the imports `"github.com/khipu/khbb/pkg/cmd/auth/logout"` and `"github.com/khipu/khbb/pkg/cmd/auth/status"` and change the `AddCommand` line to:

```go
	cmd.AddCommand(
		login.NewCmdLogin(f, nil),
		logout.NewCmdLogout(f, nil),
		status.NewCmdStatus(f, nil),
	)
```

- [ ] **Step 4: Run tests and a smoke check**

Run: `go test ./... && make build && KHBB_CONFIG_DIR="$(mktemp -d)" KHBB_TOKEN= ./bin/khbb auth status; echo "exit=$?"`
Expected: all `ok`; then `error: not logged in to bitbucket.org`, `hint: run `khbb auth login``, `exit=4`.

- [ ] **Step 5: Commit**

```bash
git add pkg
git commit -m "feat(auth): add khbb auth status and logout"
```

---

### Task 11: `khbb api`

**Files:**
- Create: `pkg/cmd/api/api.go`
- Modify: `pkg/cmd/root/root.go` (register `api`)
- Test: `pkg/cmd/api/api_test.go`

**Interfaces:**
- Consumes: `cmdutil.Factory` (`HTTPClient`, `BaseRepo`, `Branch`), `cmdutil.EnableRepoOverride`, `cmdutil.AddDryRunFlag`, `cmdutil.FlagErrorf` (Task 7); `bitbucket.Client.Request`, `bitbucket.ParseHTTPError`, `bitbucket.ErrDryRun` (Tasks 2–3); `gitctx.Repo`, `gitctx.ErrNoRepo` (Task 5).
- Produces: `api.APIOptions`, `api.NewCmdAPI(f *cmdutil.Factory, runF func(*APIOptions) error) *cobra.Command`.

- [ ] **Step 1: Write the failing tests**

`pkg/cmd/api/api_test.go`:

```go
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
		{"--input", "body.json", "-f", "a=b", "user"},
		{"--jq", ".", "--template", "{{.}}", "user"},
		{"--include", "--paginate", "user"},
		{"-f", "novalue", "user"},
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
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/cmd/api/`
Expected: FAIL — `undefined: APIOptions`, `undefined: apiRun`, …

- [ ] **Step 3: Implement**

`pkg/cmd/api/api.go`:

```go
// Package api implements `khbb api`.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cli/go-gh/v2/pkg/jq"
	"github.com/cli/go-gh/v2/pkg/jsonpretty"
	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
)

// APIOptions holds the inputs and dependencies of `khbb api`.
type APIOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)

	Path        string
	Method      string
	MethodSet   bool
	RawFields   []string
	TypedFields []string
	Headers     []string
	Input       string
	Paginate    bool
	Include     bool
	Silent      bool
	JQ          string
	Template    string
}

// NewCmdAPI returns `khbb api`.
func NewCmdAPI(f *cmdutil.Factory, runF func(*APIOptions) error) *cobra.Command {
	opts := &APIOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch}
	cmd := &cobra.Command{
		Use:   "api <path>",
		Short: "Make an authenticated Bitbucket API request",
		Long: `Make an authenticated HTTP request to the Bitbucket Cloud REST API and print the response.

The path is relative to https://api.bitbucket.org/2.0/. The placeholders {workspace},
{repo} and {branch} are replaced with values from --repo, KHBB_REPO or the current git
repository.

The default method is GET, or POST when fields or --input are given. With GET, fields
are sent as query parameters; otherwise they form a JSON object body.

  -f key=value   adds a string field
  -F key=value   adds a typed field: true, false, null and integers are converted;
                 @file reads the value from a file, @- from standard input`,
		Example: `  $ khbb api user
  $ khbb api 'repositories/{workspace}/{repo}/pullrequests' --paginate --jq '.[].title'
  $ khbb api 'repositories/{workspace}/{repo}/pullrequests' -X GET -f q='state="OPEN"'
  $ khbb api -X POST 'repositories/{workspace}/{repo}/pullrequests/42/comments' --input comment.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Path = args[0]
			opts.MethodSet = cmd.Flags().Changed("method")
			if err := validateFlags(opts); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return apiRun(cmd.Context(), opts)
		},
	}
	cmdutil.EnableRepoOverride(cmd, f)
	cmdutil.AddDryRunFlag(cmd, f)
	fl := cmd.Flags()
	fl.StringVarP(&opts.Method, "method", "X", "GET", "The HTTP `method` for the request")
	fl.StringArrayVarP(&opts.RawFields, "raw-field", "f", nil, "Add a string parameter in `key=value` format")
	fl.StringArrayVarP(&opts.TypedFields, "field", "F", nil, "Add a typed parameter in `key=value` format")
	fl.StringArrayVarP(&opts.Headers, "header", "H", nil, "Add a HTTP request header in `key:value` format")
	fl.StringVar(&opts.Input, "input", "", "The `file` to use as body for the request (use \"-\" for standard input)")
	fl.BoolVar(&opts.Paginate, "paginate", false, "Follow `next` links and print all values as one JSON array")
	fl.BoolVarP(&opts.Include, "include", "i", false, "Include the HTTP response status line and headers in the output")
	fl.BoolVar(&opts.Silent, "silent", false, "Do not print the response body")
	fl.StringVarP(&opts.JQ, "jq", "q", "", "Filter the response using a jq `expression`")
	fl.StringVarP(&opts.Template, "template", "t", "", "Format the response using a Go `template`")
	return cmd
}

func validateFlags(opts *APIOptions) error {
	switch {
	case opts.Paginate && opts.MethodSet && !strings.EqualFold(opts.Method, http.MethodGet):
		return cmdutil.FlagErrorf("--paginate only works with GET requests")
	case opts.Paginate && opts.Include:
		return cmdutil.FlagErrorf("--include cannot be combined with --paginate")
	case opts.Input != "" && (len(opts.RawFields) > 0 || len(opts.TypedFields) > 0):
		return cmdutil.FlagErrorf("--input cannot be combined with -f/-F")
	case opts.JQ != "" && opts.Template != "":
		return cmdutil.FlagErrorf("cannot use --jq and --template together")
	}
	return validateFields(opts)
}

// validateFields checks key=value syntax without reading files or stdin.
func validateFields(opts *APIOptions) error {
	for _, f := range append(slices.Clone(opts.RawFields), opts.TypedFields...) {
		if k, _, ok := strings.Cut(f, "="); !ok || k == "" {
			return cmdutil.FlagErrorf("field %q must be in key=value format", f)
		}
	}
	return nil
}

func apiRun(ctx context.Context, opts *APIOptions) error {
	path, err := fillPlaceholders(opts.Path, opts.BaseRepo, opts.Branch)
	if err != nil {
		return err
	}
	params, err := parseFields(opts.RawFields, opts.TypedFields, opts.IO.In)
	if err != nil {
		return err
	}
	method := strings.ToUpper(opts.Method)
	if !opts.MethodSet && (len(params) > 0 || opts.Input != "") {
		method = http.MethodPost
	}
	var body []byte
	switch {
	case opts.Input != "":
		body, err = readInput(opts.Input, opts.IO.In)
	case len(params) > 0 && method == http.MethodGet:
		path = addQuery(path, params)
	case len(params) > 0:
		body, err = json.Marshal(params)
	}
	if err != nil {
		return err
	}
	header, err := parseHeaders(opts.Headers)
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	if opts.Paginate {
		return paginate(ctx, client, path, header, opts)
	}

	resp, err := client.Request(ctx, method, path, header, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if opts.Include {
		writeHeaders(opts.IO.Out, resp)
	}
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if err := writeBody(opts, resp.Header.Get("Content-Type"), data, ok); err != nil {
		return err
	}
	if !ok {
		return bitbucket.ParseHTTPError(resp, data)
	}
	return nil
}

func paginate(ctx context.Context, client *bitbucket.Client, path string, header http.Header, opts *APIOptions) error {
	all := []json.RawMessage{}
	for next := path; next != ""; {
		resp, err := client.Request(ctx, http.MethodGet, next, header, nil)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			_ = writeBody(opts, resp.Header.Get("Content-Type"), data, false)
			return bitbucket.ParseHTTPError(resp, data)
		}
		var page struct {
			Values []json.RawMessage `json:"values"`
			Next   string            `json:"next"`
		}
		if err := json.Unmarshal(data, &page); err != nil || page.Values == nil {
			// Not a paginated collection: print it as is.
			return writeBody(opts, resp.Header.Get("Content-Type"), data, true)
		}
		all = append(all, page.Values...)
		next = page.Next
	}
	merged, err := json.Marshal(all)
	if err != nil {
		return err
	}
	return writeBody(opts, "application/json", append(merged, '\n'), true)
}

var placeholderRE = regexp.MustCompile(`\{(workspace|repo|branch)\}`)

func fillPlaceholders(path string, baseRepo func() (gitctx.Repo, error), branch func() (string, error)) (string, error) {
	var (
		repo     *gitctx.Repo
		firstErr error
	)
	result := placeholderRE.ReplaceAllStringFunc(path, func(m string) string {
		if firstErr != nil {
			return m
		}
		switch m {
		case "{workspace}", "{repo}":
			if repo == nil {
				r, err := baseRepo()
				if err != nil {
					firstErr = err
					return m
				}
				repo = &r
			}
			if m == "{workspace}" {
				return repo.Workspace
			}
			return repo.Slug
		default: // {branch}
			b, err := branch()
			if err != nil {
				firstErr = err
				return m
			}
			return b
		}
	})
	return result, firstErr
}

func parseFields(raw, typed []string, stdin io.Reader) (map[string]any, error) {
	params := map[string]any{}
	for _, f := range raw {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, cmdutil.FlagErrorf("field %q must be in key=value format", f)
		}
		params[k] = v
	}
	for _, f := range typed {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, cmdutil.FlagErrorf("field %q must be in key=value format", f)
		}
		val, err := typedValue(v, stdin)
		if err != nil {
			return nil, err
		}
		params[k] = val
	}
	return params, nil
}

func typedValue(v string, stdin io.Reader) (any, error) {
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n, nil
	}
	if strings.HasPrefix(v, "@") {
		b, err := readInput(v[1:], stdin)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	}
	return v, nil
}

func readInput(name string, stdin io.Reader) ([]byte, error) {
	if name == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(name)
}

func addQuery(path string, params map[string]any) string {
	q := url.Values{}
	for k, v := range params {
		if v == nil {
			q.Set(k, "")
			continue
		}
		q.Set(k, fmt.Sprint(v))
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + q.Encode()
}

func parseHeaders(values []string) (http.Header, error) {
	h := http.Header{}
	for _, s := range values {
		k, v, ok := strings.Cut(s, ":")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, cmdutil.FlagErrorf("header %q must be in key:value format", s)
		}
		h.Add(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	return h, nil
}

func writeHeaders(w io.Writer, resp *http.Response) {
	proto := resp.Proto
	if proto == "" {
		proto = "HTTP/1.1"
	}
	fmt.Fprintf(w, "%s %s\n", proto, resp.Status)
	for _, k := range slices.Sorted(maps.Keys(resp.Header)) {
		fmt.Fprintf(w, "%s: %s\n", k, strings.Join(resp.Header.Values(k), ", "))
	}
	fmt.Fprintln(w)
}

// writeBody prints a response body; filter applies --jq/--template (never to error bodies).
func writeBody(opts *APIOptions, contentType string, data []byte, filter bool) error {
	if opts.Silent || len(data) == 0 {
		return nil
	}
	switch {
	case filter && opts.JQ != "":
		return jq.EvaluateFormatted(bytes.NewReader(data), opts.IO.Out, opts.JQ, "  ", opts.IO.ColorEnabled())
	case filter && opts.Template != "":
		t := template.New(opts.IO.Out, opts.IO.TerminalWidth(), opts.IO.ColorEnabled())
		if err := t.Parse(opts.Template); err != nil {
			return err
		}
		if err := t.Execute(bytes.NewReader(data)); err != nil {
			return err
		}
		return t.Flush()
	case strings.Contains(contentType, "json") && opts.IO.IsStdoutTTY():
		return jsonpretty.Format(opts.IO.Out, bytes.NewReader(data), "  ", opts.IO.ColorEnabled())
	default:
		_, err := opts.IO.Out.Write(data)
		return err
	}
}
```

In `pkg/cmd/root/root.go`, add the import `apiCmd "github.com/khipu/khbb/pkg/cmd/api"` and change the `AddCommand` call to:

```go
	cmd.AddCommand(
		apiCmd.NewCmdAPI(f, nil),
		authCmd.NewCmdAuth(f),
		versionCmd.NewCmdVersion(f),
	)
```

- [ ] **Step 4: Run tests, lint and a smoke check**

Run: `go test ./... && go vet ./... && make build && KHBB_CONFIG_DIR="$(mktemp -d)" KHBB_TOKEN= ./bin/khbb api user; echo "exit=$?"`
Expected: all `ok`; then `error: not logged in to bitbucket.org` and `exit=4`.

If `golangci-lint` is installed locally, also run `make lint` and fix any finding.

- [ ] **Step 5: Commit**

```bash
git add pkg
git commit -m "feat(api): add khbb api"
```

---

### Task 12: Verify the remaining API facts against Bitbucket (human in the loop)

This task needs a real Atlassian API token, so it **must run in the main session with the human**, not in a subagent. Every request is a read-only `GET`. Record only shapes, field names, status codes and enum values in the findings file — never names, emails, UUIDs or repository content.

**Files:**
- Modify: `docs/superpowers/specs/2026-10-05-bitbucket-api-findings.md`

**Interfaces:**
- Consumes: the `khbb` binary from Task 11.
- Produces: resolved rows for items #5, #9 and maximum `pagelen` in the findings file, consumed by Plans 2 and 3.

- [ ] **Step 1: Build**

Run: `make build`
Expected: `bin/khbb` exists.

- [ ] **Step 2: Human logs in**

Ask the human to create a token at `https://id.atlassian.com/manage-profile/security/api-tokens` ("Create API token with scopes", app Bitbucket) with the seven scopes in `bitbucket.RequiredScopes`, and then to run in **their own terminal** (the token must never be pasted into the chat):

```bash
cd ~/git/khbb && ./bin/khbb auth login
```

Then run here: `./bin/khbb auth status; echo "exit=$?"`
Expected: `Logged in as …`, `exit=0`. If this shell cannot read the system keyring (exit 4 although the human is logged in), ask the human to run Steps 3–6 in their terminal and paste only the filtered outputs.

- [ ] **Step 3: Pick a repository with pipelines**

Ask the human which `khipu/<repo>` to use (one with recent pipelines and pull requests). Use it as `R` below:

```bash
R=khipu/<repo>
```

- [ ] **Step 4: Item #5 — build number in the pipeline path**

```bash
N=$(./bin/khbb api "repositories/$R/pipelines?sort=-created_on&pagelen=1" --jq '.values[0].build_number')
./bin/khbb api "repositories/$R/pipelines/$N" --jq '{build_number, state: .state.name}'; echo "exit=$?"
```

Expected: either a JSON object and `exit=0` (build numbers accepted), or a 404 error and `exit=1` (UUID required). Also record the real state shape:

```bash
./bin/khbb api "repositories/$R/pipelines?sort=-created_on&pagelen=5" --jq '.values[].state | {name, stage: .stage.name, result: .result.name}'
```

- [ ] **Step 5: Item #9 — does the API expose granted scopes?**

```bash
./bin/khbb api user -i --silent | grep -i -E 'scope|oauth' ; echo "exit=$?"
```

Expected: either scope-related headers (record only the header names) or no match (`exit=1`).

- [ ] **Step 6: Maximum `pagelen`**

```bash
for p in "repositories/$R/pullrequests" "repositories/$R/pipelines" "workspaces/khipu/members" "repositories/$R/pullrequests?state=MERGED"; do
  sep='?'; case "$p" in *\?*) sep='&';; esac
  printf '%s -> ' "$p"; ./bin/khbb api "$p${sep}pagelen=100" --jq '.pagelen' 2>&1 | tail -1
done
```

Expected: `100` where accepted; an error message where the endpoint caps lower (then retry with 50 and record the cap).

- [ ] **Step 7: Record the findings**

In `docs/superpowers/specs/2026-10-05-bitbucket-api-findings.md`, move rows #5, #9 and "Maximum `pagelen`" from **Still open** into **Resolved**, with Source B, the observed result (status code, shape, header names, caps) and the decision:

- #5 → "use build numbers directly" or "resolve build number to UUID via `GET …/pipelines?q=build_number=N`" (Plan 3).
- #9 → "`auth status` prints granted scopes from header X" or "scope problems surface through 403 hints only".
- pagelen → the cap per endpoint; if any cap is below 50, Plan 2/3 must pass an explicit lower `pagelen` for that endpoint.

Leave #1 in **Still open** (Plan 2 verifies it against the sandbox repository).

- [ ] **Step 8: Commit**

```bash
git add docs/superpowers/specs/2026-10-05-bitbucket-api-findings.md
git commit -m "docs: record live Bitbucket API findings"
```

---

## Spec Coverage (Plan 1)

| Spec section | Covered by |
|---|---|
| §4 Architecture (layout, command pattern, data flow, dependencies) | Tasks 1, 7, 8 |
| §5.1 Tokens and scopes | Tasks 3 (`RequiredScopes`), 9 |
| §5.2 `auth login/status/logout`, no `auth token`, insecure storage | Tasks 9, 10 |
| §5.3 Environment variables (except `KHBB_PAGER`) | Tasks 4, 7, 8 |
| §5.4 Config file | Task 4 |
| §6 Repository and branch resolution | Tasks 5, 7 |
| §7.3 `khbb api` | Task 11 |
| §7.4 `khbb version` | Task 1 |
| §8.1 JSON output contract (mechanism) | Task 7 |
| §8.2 Streams and TTY behavior | Tasks 1, 7, 8 |
| §8.3 Errors, §8.4 Exit codes | Tasks 6, 8 |
| §9 Safety: confirmation, dry-run, secrets | Tasks 3, 7 |
| §10 Error handling (401/403/404/409/429/5xx, retries, timeouts) | Tasks 2, 3, 6 |
| §11 Testing (mock registry, fixtures, TTY simulation, CI matrix) | Tasks 1–11 |
| §14 Items to verify | findings doc + Task 12 |
| §7.1 PR commands, §8.1 PR field sets, `KHBB_PAGER`, `--web` | Plan 2 |
| §7.2 Pipeline commands, status normalization | Plan 3 |
| §12 Distribution, §13 Agent skill, `khbb skill install` | Plan 4 |
