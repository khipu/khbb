# khbb Plan 4 — Agent skill, release and README

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make khbb installable and usable by people and agents: the embedded agent skill and `khbb skill install`, tests that keep the skill and README in step with the commands, opt-in end-to-end tests against the sandbox, GoReleaser releases with Homebrew and Scoop, a README, and the small hardening items deferred from earlier plans.

**Architecture:** `skills/khbb/SKILL.md` is embedded by a tiny `skills` package and written out by `pkg/cmd/skill/install` (same `XxxOptions` + `runF` pattern as every command). Documentation tests in `pkg/cmd/root` walk the cobra tree and check every `khbb …` line in the code blocks of SKILL.md and README.md. End-to-end tests live in `e2e/` and skip unless `KHBB_E2E=1`. Releases are configuration only: `.goreleaser.yaml` plus `.github/workflows/release.yml` on `v*` tags; CI gains a pinned linter, gofmt and `goreleaser check`.

**Tech Stack:** Go 1.26, cobra v1.10.2, go-gh v2.16.1, GoReleaser v2 (≥ 2.15, verified with 2.18.2), goreleaser-action v7, golangci-lint v2.14.0, Homebrew casks (Homebrew 6). No new Go dependencies.

**Spec:** `docs/superpowers/specs/2026-10-05-khbb-cli-design.md` (§7.4 `skill install`, §11 testing/E2E, §12 distribution, §13 agent skill; §5.3 and §8 for the README). Earlier plans: `docs/superpowers/plans/2026-10-0{5,6}-khbb-plan-*.md`.

**Roadmap:** Plans 1, 2a, 2b, 3 (done) → **Plan 4 (this)** → first release `v0.1.0`.

## Global Constraints

Everything in the earlier plans' Global Constraints still applies. The ones that matter most here:

- Module `github.com/khipu/khbb`; all code, help text, messages and docs in English. Khipu's voice for prose (README, SKILL.md): close, clear and direct.
- Must build and pass `go test -race ./...` on macOS and Linux and `go test ./...` on Windows; `golangci-lint run` (v2.14.0, config `.golangci.yml`) must report 0 issues; `gofmt -l` on changed files prints nothing. The controller gives implementers the paths of the `golangci-lint` and `goreleaser` binaries.
- stdout carries data only; confirmations, notices, warnings and errors go to stderr.
- Exit codes: `0` success, `1` error (including `conflict`), `2` cancelled, `4` auth required, `8` checks pending.
- Spec §7.4: "`khbb skill install [--dir <path>]` — writes the embedded `SKILL.md` to `~/.claude/skills/khbb/SKILL.md` by default; overwrites only with `--force`."
- Spec §12: GoReleaser on tags `v*`; targets `darwin`, `linux`, `windows` × `amd64`, `arm64`; archives (`tar.gz`, `zip` for Windows) + `checksums.txt` on GitHub Releases of `khipu/khbb`; `brew install khipu/tap/khbb` (tap repo `khipu/homebrew-tap`); `scoop bucket add khipu https://github.com/khipu/scoop-bucket` then `scoop install khbb`; version info via `-ldflags` (`version`, `commit`, `date`); semantic versioning, JSON contract changes are major.
- Spec §13 (the skill must cover): when to use khbb and checking `khbb auth status` first; `--json` + `--jq`, never scraping tables; never `pr merge`, `pr decline` or `pipeline stop` without explicit human approval in the conversation, `--dry-run` shown first; `khbb api` GET only unless the human approved the specific mutating call; never print tokens, never pass secrets as plain `--var`; the exit-code table (4 → ask the human to log in, 8 → keep waiting); recipes: PR from the current branch, `khbb pipeline watch --exit-status`, `khbb pipeline logs --failed`, list review comments, address and reply.
- Commits: each task's "Message:" line is the subject. Write it, a blank line and the attribution lines the controller gives you to a file and run `git commit -F <that file>`.
- Test fixtures and examples use fake identities only (`@example.com`, `ada`, workspace `acme`, repository `acme/widgets`). The e2e tests commit as `khbb-e2e <e2e@example.invalid>`.
- Nothing outside this repository is created by implementers: no GitHub repositories, secrets, tags or releases, and no writes to `~/.claude`. Only the controller runs the e2e tests (sandbox writes are approved) and only the controller cuts a release, after the user's explicit go-ahead (Task 9).

## Decisions taken for this plan (rulings)

- **`--dir` names the directory that receives `SKILL.md`** (default `~/.claude/skills/khbb`), e.g. `--dir .claude/skills/khbb` for a repository. — Why: literal and predictable; the path written is always printed. Cost if wrong: users pass a parent directory and get `SKILL.md` one level up; the message shows where it went.
- **`skill install` outcomes:** no file → write it (`Installed the khbb skill at <path>`); identical content → nothing written, `The khbb skill at <path> is already up to date`, exit 0; different content without `--force` → `conflict` error, exit 1, file untouched; with `--force` → replace (`Updated …`). All messages on stderr; stdout stays empty. Directories are created with `0755`, the file with `0644`.
- **SKILL.md is the single source of the skill.** It is embedded with `//go:embed` and pinned to LF line endings by `.gitattributes`, so every build ships the same bytes.
- **Documentation tests** (Task 4): every visible leaf command must appear in SKILL.md as `` `khbb <path>` ``; every code-block line part (split at `&&`, `||`, `;`, `|`) that starts with `khbb ` must name a leaf command and use only flags that command accepts (own or inherited, long or short). README.md gets the same line check in Task 7. Lines are split on spaces, so docs must not quote values that start with `-`.
- **Version string:** `buildVersion` fills what `-ldflags` left unset from `debug.ReadBuildInfo` (module version for `go install …@vX`, `vcs.revision`/`vcs.time` for builds in a git checkout) and drops a leading `v`, so GoReleaser (`{{ .Version }}` = `1.2.3`), `make build` (`git describe` = `v1.2.3-…`) and `go install` all print `khbb version 1.2.3…`.
- **Environment switches:** `KHBB_DEBUG` and `KHBB_PROMPT_DISABLED` are off when unset or set to `0`, `false`, `no` or `off` (any case), on for anything else. — Cost if wrong: someone who set `KHBB_PROMPT_DISABLED=0` to disable prompts (unlikely) now gets prompts.
- **Remotes:** `altssh.bitbucket.org` (SSH over port 443) counts as Bitbucket, directly or as the hostname an SSH alias resolves to. `ssh -G` is never called with a host that starts with `-` or is empty.
- **E2E (spec §11):** one sequential test, `TestPullRequestAndPipelineLifecycle`, in `e2e/` (package `e2e`, test files only). It clones the sandbox over SSH, pushes `khbb-probe/e2e-<UTC stamp>`, and runs khbb inside the clone (`KHBB_REPO` cleared, so remote resolution is exercised). It refuses any repository whose name lacks "sandbox". Default repository `khipu/khipubb-sandbox` (spec says `khipu/khbb-sandbox`; the real, user-approved sandbox is `khipubb`), overridable with `KHBB_E2E_REPO` and `KHBB_E2E_REMOTE`. `make e2e` runs it with a 15-minute timeout. Cleanup declines the pull request and deletes the branch even on failure. It is never part of CI.
- **Homebrew:** a cask (`homebrew_casks`; `brews` is deprecated in GoReleaser v2) in `khipu/homebrew-tap`, directory `Casks`, with the post-install hook that clears the quarantine attribute (binaries are not signed — spec §3 non-goal) and `generate_completions_from_executable` with `shell_parameter_format: cobra` for bash, zsh and fish (Homebrew 6 runs `khbb completion <shell>`; verified in Homebrew's source). Prerelease tags (`v1.0.0-rc1`) skip the tap and the bucket (`skip_upload: auto`) and mark the GitHub release as a prerelease.
- **One token for both publishing repositories:** the Actions secret `TAP_GITHUB_TOKEN` (fine-grained, Contents read/write on `khipu/homebrew-tap` and `khipu/scoop-bucket`). The release itself uses the workflow's `GITHUB_TOKEN`.
- **Builds:** `CGO_ENABLED=0`, `-trimpath`, `mod_timestamp` and `date` from the commit (reproducible). Archives keep GoReleaser's default name (`khbb_<version>_<os>_<arch>`) and default files (binary, README.md, LICENSE).
- **CI:** golangci-lint pinned to `v2.14.0`; gofmt runs as a golangci-lint formatter (verified: `golangci-lint run` reports "File is not properly formatted (gofmt)"); a `goreleaser` job runs `goreleaser check`. The release workflow runs `go test ./...` before GoReleaser.
- **Not in this plan** (stays in the backlog): a pager (`KHBB_PAGER`, spec §5.3) and the `editor` setting, nested `khbb api -f 'a[b]=x'` keys, `GH_FORCE_TTY` leaking through go-gh's `term`, and the Plan 2a/2b/3 review backlogs. The README documents only what works.

## Review Focus

1. **The skill drifting from the binary** — a renamed flag or a new command would silently mislead every agent: tests `TestSkillCommandLinesAreValid`, `TestSkillMentionsEveryCommand`, `TestCheckCommandLine` (Task 4) and `TestReadmeCommandLinesAreValid` (Task 7).
2. **`skill install` over a SKILL.md someone edited** must keep it unless `--force`, and a repeat install must not rewrite an identical file: `TestInstall_KeepsADifferentFileWithoutForce`, `TestInstall_AlreadyUpToDate` (Task 3).
3. **A released binary that reports the wrong version or will not start on macOS:** `TestBuildVersion` (Task 2); the snapshot inspection of the cask's quarantine hook and `khbb version` in Task 6 and Task 8.
4. **E2E tests writing to a real repository** because `KHBB_REPO` is exported or `KHBB_E2E_REPO` is wrong: `TestCheckSandboxRepo` and the cleared `KHBB_REPO` in `env()` (Task 5).
5. **Remotes on `altssh.bitbucket.org:443`** not recognized, and option-like SSH hosts reaching `ssh -G`: `TestBaseRepo_AltSSHHost`, `TestBaseRepo_AliasForAltSSHHost`, `TestSSHHostname_RefusesOptionLikeHosts` (Task 1).

---

## File Map

| File | Responsibility | Task |
|---|---|---|
| `internal/gitctx/remote.go`, `internal/gitctx/resolver.go` | altssh host; refuse option-like SSH hosts | 1 |
| `pkg/cmd/factory/default.go` | `envEnabled` for `KHBB_DEBUG`, `KHBB_PROMPT_DISABLED` | 1 |
| `cmd/khbb/main.go` | `buildVersion` from build info | 2 |
| `skills/khbb/SKILL.md` | The agent skill | 3 |
| `skills/embed.go` | Embeds SKILL.md as `skills.Khbb` | 3 |
| `.gitattributes` | LF endings for SKILL.md | 3 |
| `pkg/cmd/skill/skill.go` | `khbb skill` group | 3 |
| `pkg/cmd/skill/install/install.go` | `khbb skill install` | 3 |
| `pkg/cmd/root/root.go` | Register the group | 3 |
| `pkg/cmd/root/docs_test.go` | Docs ↔ command tree checks | 4, 7 |
| `e2e/e2e_test.go` | Opt-in end-to-end test | 5 |
| `Makefile` | `make e2e` | 5 |
| `.goreleaser.yaml` | Builds, archives, release, cask, Scoop | 6 |
| `.github/workflows/release.yml` | Release on `v*` tags | 6 |
| `.github/workflows/ci.yml`, `.golangci.yml` | Pinned lint, gofmt, `goreleaser check` | 6 |
| `README.md` | Install, log in, usage, scripting, agents, config, development | 7 |
| — | Live verification (controller only) | 8 |
| — | First release (controller only, after the user's go-ahead) | 9 |

---

### Task 1: Hardening — altssh remotes, option-like SSH hosts, environment switches

**Files:**
- Modify: `internal/gitctx/remote.go` (replace the `bitbucketHost` constant), `internal/gitctx/resolver.go` (`bitbucketRepo`, `sshHostname`), `pkg/cmd/factory/default.go` (`New`, `newClient`, new `envEnabled`)
- Test: `internal/gitctx/resolver_test.go`, `internal/gitctx/ssh_test.go` (new), `pkg/cmd/factory/env_test.go` (new)

**Interfaces:**
- Produces: `gitctx.isBitbucketHost(host string) bool` (unexported); `factory.envEnabled(name string) bool` (unexported). No exported API changes.

- [ ] **Step 1: Write the failing tests**

Append to `internal/gitctx/resolver_test.go` (package `gitctx_test`; it already imports `errors`, `strings`, `testing` and has `fakeResolver`):

```go
func TestBaseRepo_AltSSHHost(t *testing.T) {
	r := fakeResolver("origin\tssh://git@altssh.bitbucket.org:443/acme/widgets.git (fetch)", "", nil)
	r.SSHHostname = func(alias string) (string, error) {
		t.Errorf("altssh.bitbucket.org needs no alias lookup, asked for %q", alias)
		return "", errors.New("unexpected lookup")
	}
	repo, err := r.BaseRepo()
	if err != nil || repo.FullName() != "acme/widgets" {
		t.Errorf("BaseRepo = %v, %v", repo, err)
	}
}

func TestBaseRepo_AliasForAltSSHHost(t *testing.T) {
	r := fakeResolver("origin\tgit@bb-443:acme/widgets.git (fetch)", "", nil)
	r.SSHHostname = func(string) (string, error) { return "altssh.bitbucket.org", nil }
	repo, err := r.BaseRepo()
	if err != nil || repo.FullName() != "acme/widgets" {
		t.Errorf("BaseRepo = %v, %v", repo, err)
	}
}
```

Create `internal/gitctx/ssh_test.go` (internal package, to reach `sshHostname`):

```go
package gitctx

import (
	"strings"
	"testing"
)

func TestSSHHostname_RefusesOptionLikeHosts(t *testing.T) {
	for _, alias := range []string{"-oProxyCommand=touch /tmp/pwned", "-G", ""} {
		_, err := sshHostname(alias)
		if err == nil || !strings.Contains(err.Error(), "invalid ssh host") {
			t.Errorf("sshHostname(%q) err = %v", alias, err)
		}
	}
}
```

Create `pkg/cmd/factory/env_test.go` (internal package, like `browser_test.go`):

```go
package factory

import "testing"

func TestEnvEnabled(t *testing.T) {
	cases := map[string]bool{
		"": false, "0": false, "false": false, "FALSE": false, "no": false, "off": false, " 0 ": false,
		"1": true, "true": true, "yes": true, "api": true,
	}
	for value, want := range cases {
		t.Setenv("KHBB_TEST_FLAG", value)
		if got := envEnabled("KHBB_TEST_FLAG"); got != want {
			t.Errorf("envEnabled with %q = %v, want %v", value, got, want)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/gitctx/ ./pkg/cmd/factory/`
Expected: FAIL — `TestBaseRepo_AltSSHHost` and `TestBaseRepo_AliasForAltSSHHost` report `ErrNoRepo` (and the alias lookup error), `TestSSHHostname_RefusesOptionLikeHosts` gets ssh's own error or none, and the factory package does not compile (`undefined: envEnabled`).

- [ ] **Step 3: Implement**

In `internal/gitctx/remote.go`, replace

```go
const bitbucketHost = "bitbucket.org"
```

with

```go
// isBitbucketHost reports whether host is bitbucket.org or altssh.bitbucket.org, which serves SSH on
// port 443 for networks that block port 22.
func isBitbucketHost(host string) bool {
	return host == "bitbucket.org" || host == "altssh.bitbucket.org"
}
```

In `internal/gitctx/resolver.go`, `bitbucketRepo` becomes:

```go
// bitbucketRepo returns the repository a remote URL points at; ok is false for hosts other than
// bitbucket.org and altssh.bitbucket.org, after resolving SSH host aliases.
func (r *Resolver) bitbucketRepo(rawURL string) (Repo, bool) {
	host, repo, isSSH, err := parseRemoteURL(rawURL)
	if err != nil {
		return Repo{}, false
	}
	if !isBitbucketHost(host) && isSSH && r.SSHHostname != nil {
		if real, err := r.SSHHostname(host); err == nil {
			host = real
		}
	}
	return repo, isBitbucketHost(host)
}
```

and `sshHostname` gets a doc comment and a guard at the top (the rest is unchanged):

```go
// sshHostname asks ssh which hostname an alias from ~/.ssh/config stands for.
func sshHostname(alias string) (string, error) {
	// ssh would read a host that starts with "-" as an option, such as -oProxyCommand=….
	if alias == "" || strings.HasPrefix(alias, "-") {
		return "", fmt.Errorf("invalid ssh host %q", alias)
	}
	exe, err := safeexec.LookPath("ssh")
	// … unchanged …
```

`grep -n bitbucketHost internal/gitctx/*.go` must print nothing afterwards.

In `pkg/cmd/factory/default.go`, add `"strings"` to the imports, replace `os.Getenv("KHBB_PROMPT_DISABLED") != ""` with `envEnabled("KHBB_PROMPT_DISABLED")` and `os.Getenv("KHBB_DEBUG") != ""` with `envEnabled("KHBB_DEBUG")`, and append:

```go
// envEnabled reports whether the environment variable name is set to anything but "", "0",
// "false", "no" or "off" (in any case).
func envEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/gitctx/ ./pkg/cmd/factory/ && go test ./...`
Expected: PASS.

- [ ] **Step 5: Lint and commit**

```bash
gofmt -l internal pkg
golangci-lint run ./...
git add internal/gitctx pkg/cmd/factory
git commit -F <message file>
```

Message: `fix(gitctx): accept altssh.bitbucket.org and refuse option-like ssh hosts; off values for KHBB_DEBUG and KHBB_PROMPT_DISABLED`

---

### Task 2: Version from build information

**Files:**
- Modify: `cmd/khbb/main.go`
- Test: `cmd/khbb/main_test.go`

**Interfaces:**
- Produces: `buildVersion(version, commit, date string, info *debug.BuildInfo) (string, string, string)` in package `main`. `factory.New` receives its three results.

- [ ] **Step 1: Write the failing test**

Add `"runtime/debug"` to the imports of `cmd/khbb/main_test.go` and append:

```go
func TestBuildVersion(t *testing.T) {
	vcs := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "64b0e38c0ffee0123456789abcdef0123456789a"},
		{Key: "vcs.time", Value: "2026-10-06T12:00:00Z"},
	}
	cases := []struct {
		name                  string
		version, commit, date string
		info                  *debug.BuildInfo
		want                  [3]string
	}{
		{"ldflags win", "v1.2.3", "abc1234", "2026-10-01T00:00:00Z",
			&debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}, Settings: vcs},
			[3]string{"1.2.3", "abc1234", "2026-10-01T00:00:00Z"}},
		{"goreleaser", "1.2.3", "abc1234", "2026-10-01T00:00:00Z", nil,
			[3]string{"1.2.3", "abc1234", "2026-10-01T00:00:00Z"}},
		{"go install", "dev", "", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0"}},
			[3]string{"0.2.0", "", ""}},
		{"git checkout", "dev", "", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: vcs},
			[3]string{"dev", "64b0e38", "2026-10-06T12:00:00Z"}},
		{"no build info", "dev", "", "", nil, [3]string{"dev", "", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, c, d := buildVersion(tc.version, tc.commit, tc.date, tc.info)
			if got := [3]string{v, c, d}; got != tc.want {
				t.Errorf("buildVersion = %q, want %q", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/khbb/`
Expected: FAIL to compile — `undefined: buildVersion`.

- [ ] **Step 3: Implement**

In `cmd/khbb/main.go`: add `"runtime/debug"` and `"strings"` to the imports; change the comment above the variables to `// Set with -ldflags at build time; buildVersion fills in what is missing.`; start `run` with

```go
func run(args []string) int {
	info, _ := debug.ReadBuildInfo()
	f := factory.New(buildVersion(version, commit, date, info))
```

(the rest of `run` is unchanged), and append:

```go
// buildVersion completes the version details that -ldflags did not set with the build information
// the Go toolchain embeds: the module version for `go install …@v1.2.3`, and the commit and its
// time for a build from a git checkout. The leading "v" of a tag is dropped, so every kind of build
// prints "khbb version 1.2.3".
func buildVersion(version, commit, date string, info *debug.BuildInfo) (string, string, string) {
	if info != nil {
		if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
		for _, s := range info.Settings {
			switch {
			case s.Key == "vcs.revision" && commit == "":
				commit = s.Value
				if len(commit) > 7 {
					commit = commit[:7]
				}
			case s.Key == "vcs.time" && date == "":
				date = s.Value
			}
		}
	}
	return strings.TrimPrefix(version, "v"), commit, date
}
```

- [ ] **Step 4: Run the tests and try a build**

Run: `go test ./cmd/khbb/ && go build -o bin/khbb ./cmd/khbb && ./bin/khbb version`
Expected: PASS; from a git checkout `khbb version` prints a Go pseudo-version and the commit, e.g. `khbb version 0.0.0-20261006134754-64b0e38f9ff6 (64b0e38, 2026-10-06T13:47:54Z)` (with `+dirty` when the tree has changes). `make build && ./bin/khbb version` still prints the `git describe` value without a leading `v`.

- [ ] **Step 5: Lint and commit**

```bash
gofmt -l cmd
golangci-lint run ./...
git add cmd/khbb
git commit -F <message file>
```

Message: `feat(version): fall back to Go build information for go install and plain builds`

---

### Task 3: The agent skill and `khbb skill install`

**Files:**
- Create: `skills/khbb/SKILL.md`, `skills/embed.go`, `.gitattributes`, `pkg/cmd/skill/skill.go`, `pkg/cmd/skill/install/install.go`
- Modify: `pkg/cmd/root/root.go` (register the group)
- Test: `pkg/cmd/skill/install/install_test.go`, `pkg/cmd/root/root_test.go`

**Interfaces:**
- Consumes: `cmdutil.Factory` (`IOStreams`), `cmdutil.NoArgs`, `cmdutil.GroupRunE`, `cmdutil.ConflictError{Msg}`, `iostreams.Test()`.
- Produces: `skills.Khbb string` (the embedded SKILL.md); `skill.NewCmdSkill(f *cmdutil.Factory) *cobra.Command`; `install.NewCmdInstall(f *cmdutil.Factory, runF func(*install.InstallOptions) error) *cobra.Command`; `install.InstallOptions{IO, HomeDir func() (string, error), Content string, Dir string, Force bool}`. Task 4 tests SKILL.md against the command tree, so every command and flag it names must exist — they do as of this plan.

- [ ] **Step 1: Write the skill**

Create `skills/khbb/SKILL.md` with exactly this content (it is product copy: keep the wording; every `khbb …` line in its code blocks is checked against the command tree in Task 4):

````markdown
---
name: khbb
description: Work with Bitbucket Cloud pull requests and pipelines through the khbb CLI. Use it for any task on a bitbucket.org repository, such as opening, reviewing, commenting on or merging pull requests, waiting for CI, reading pipeline logs, running pipelines, or calling the Bitbucket REST API.
---

# khbb: Bitbucket Cloud from the command line

`khbb` is a `gh`-style CLI for Bitbucket Cloud. Use it for every Bitbucket pull request and
pipeline task instead of a browser or hand-written API calls. This skill matches the khbb version
that installed it; after upgrading khbb, `khbb skill install --force` refreshes it.

## Before you start

1. Run `khbb auth status`. Exit status 4 means khbb is not logged in or its token was rejected: ask
   the human to run `khbb auth login` in their own terminal. Never ask for the token, never put it
   in a command, and never read it from the keyring or from khbb's config file.
2. khbb finds the repository from the git remotes of the current directory (`origin` first).
   Anywhere else, pass `-R workspace/repo`.
3. Without a number, commands act on the current branch: its open pull request (`pr view`,
   `pr checks`, `pr merge`, ...) or its newest pipeline (`pipeline view`, `pipeline logs`, ...).
   Pull requests and pipelines are otherwise named by number (`42` or `#42`) or by their URL.

## Rules

- **Read with JSON.** Use `--json <fields>` and filter with `--jq`; never parse the tables meant for
  people. `--json` without fields lists the available ones. Ask only for the fields you need.
- **Check the exit status before you use stdout.** stdout carries only data; notices and errors go
  to stderr. With `--json`, an error is one JSON line on stderr, such as
  `{"error":{"code":"not_found","status":404,"message":"...","hint":"..."}}`. `khbb api` also
  prints Bitbucket's JSON error body on stdout when a request fails, so a failed call can look
  like data.
- **Never merge, decline or stop without the human's explicit approval** of that specific action in
  this conversation, even though `--yes` exists. This covers `pr merge`, `pr decline` (a declined
  pull request cannot be reopened) and `pipeline stop`. Run the command with `--dry-run` first,
  show the human what it would send, wait for a yes, then run it with `--yes`.
- **Approving or requesting changes speaks for the human** whose token khbb uses. Do it only when
  they ask you to.
- **`khbb api` is read-only for you.** Use GET unless the human approved that exact mutating call;
  show it with `--dry-run` first.
- **Keep secrets out of commands.** Pass secret pipeline variables with
  `--secret-var NAME="$ENV_VAR"`, never with `--var` and never as a literal value. khbb never
  prints them.
- **No hidden side effects.** khbb never pushes: push the branch yourself before `pr create`.
  `pr checkout --force` discards local changes, so ask first. Starting a pipeline uses build
  minutes and may deploy: ask first when it deploys anything.

## Exit status

| Status | Meaning | What to do |
|---|---|---|
| 0 | Success, also after `--dry-run` | Use stdout. |
| 1 | Error, including usage errors and `confirmation_required`. Also: `pr checks` found a failed check; `pipeline watch --exit-status` or `--watch` saw the pipeline end failed, error, stopped or expired | Read stderr. For a failed pipeline, read its logs. |
| 2 | Cancelled at a prompt | Stop and ask the human. |
| 4 | Not logged in, or the token is invalid | Ask the human to run `khbb auth login`. |
| 8 | `pr checks`: some checks are still in progress | Wait with `khbb pr checks --watch`. |

JSON error codes: `auth_required`, `forbidden`, `not_found`, `conflict`, `validation`,
`rate_limited`, `server_error`, `network`, `confirmation_required`, `cancelled`, `usage` and
`error`. A `forbidden` error's hint names the token scope that is missing.

## Recipes

### Open a pull request from the current branch

```bash
git push -u origin HEAD
khbb pr create --title "Add the widget factory" --body "Adds the factory and its tests." --json id,url
```

`--fill` takes the title and description from the branch's commits, `--draft` opens a draft,
`--base develop` targets another branch and `--reviewer nickname` asks someone for a review. If the
branch already has an open pull request into the same base, khbb fails with `conflict` and prints
its URL.

### Find the pull request of the current branch

```bash
khbb pr view --json id,title,state,url,reviewers
khbb pr status --json currentBranch,needsMyReview
```

`state` is `OPEN`, `MERGED`, `DECLINED` or `SUPERSEDED`. A reviewer's `state` is `approved`,
`changes_requested` or `pending`.

### Wait for CI after a push

```bash
git push
khbb pipeline watch --exit-status
```

Without a number, `pipeline watch` waits up to 60 seconds for the pipeline of your local HEAD
commit to start, then follows it until it ends. Exit 0 means it succeeded or paused on a manual
step; `khbb pipeline view --json status` tells which. Prefer it to `pr checks` right after a push:
the new commit may have no checks reported yet, and `pr checks` exits 0 when none are reported.
The command runs as long as the pipeline does, so give it a generous timeout.

To wait for every check on a pull request (pipelines and external builds), use
`khbb pr checks --watch`: exit 0 when all passed, 1 when one failed.

### Find out why a pipeline failed

```bash
khbb pipeline view --json number,status,steps --jq '{number, status, steps: [.steps[] | {name, status}]}'
khbb pipeline logs --failed --tail 200
```

Pipeline and step statuses are `pending`, `running`, `paused`, `successful`, `failed`, `error`,
`stopped` and `expired`; steps can also be `skipped`.

### Read and answer review comments

```bash
khbb pr view 42 --json comments --jq '.comments[] | select(.deleted | not) | {id, path, line, parentId, body}'
khbb pr comment 42 --reply-to 101 --body "Fixed in the latest commit."
khbb pr comment 42 --file src/widget.go --line 12 --body "This is where the cache is reset."
```

`path` is empty and `line` is null for general comments; `parentId` is set on replies.

### Merge after approval

```bash
khbb pr checks 42
khbb pr merge 42 --squash --delete-branch --dry-run
```

Show the dry run to the human. Only after they approve:

```bash
khbb pr merge 42 --squash --delete-branch --yes --json state,mergeCommit
```

Without a strategy flag, the destination branch's default strategy is used. Blocking merge checks,
such as conflicts, stop the merge before anything is sent.

### Run or rerun a pipeline

```bash
khbb pipeline run --custom deploy-staging --var ENV=staging --secret-var API_KEY="$API_KEY" --watch
khbb pipeline rerun 42 --watch
```

`pipeline run` prints the new pipeline's URL, or its JSON with `--json number,url`. Bitbucket never
returns a pipeline's variables, so `pipeline rerun` cannot copy them: pass them again.

### Anything else

```bash
khbb api 'repositories/{workspace}/{repo}/pullrequests/42/activity' --paginate --jq 'length'
```

The path is relative to `https://api.bitbucket.org/2.0/`; `{workspace}`, `{repo}` and `{branch}`
are filled in from the current repository and branch. `--paginate` follows every page and prints
their `values` as one JSON array.

## Commands

- `khbb auth status`, `khbb auth login` (the human runs it), `khbb auth logout`
- `khbb pr create`, `khbb pr list`, `khbb pr view`, `khbb pr status`, `khbb pr diff`,
  `khbb pr checks`, `khbb pr checkout`
- `khbb pr comment`, `khbb pr edit`, `khbb pr approve`, `khbb pr unapprove`,
  `khbb pr request-changes`
- `khbb pr merge` and `khbb pr decline`: only with the human's approval
- `khbb pipeline list`, `khbb pipeline view`, `khbb pipeline logs`, `khbb pipeline watch`,
  `khbb pipeline run`, `khbb pipeline rerun`
- `khbb pipeline stop`: only with the human's approval
- `khbb api`, `khbb skill install`, `khbb version`

Every command has `--help` with its flags and examples.
````

Create `.gitattributes` so Windows checkouts embed the same bytes:

```
skills/**/SKILL.md text eol=lf
```

Create `skills/embed.go`:

```go
// Package skills holds the agent skills that ship inside the khbb binary.
package skills

import _ "embed" // for go:embed

// Khbb is the khbb agent skill (skills/khbb/SKILL.md), written out by `khbb skill install`.
//
//go:embed khbb/SKILL.md
var Khbb string
```

- [ ] **Step 2: Write the failing tests**

Create `pkg/cmd/skill/install/install_test.go`:

```go
package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/skills"
)

func newOpts(t *testing.T) (*InstallOptions, string, func() string, func() string) {
	t.Helper()
	ios, _, out, errOut := iostreams.Test()
	home := t.TempDir()
	return &InstallOptions{
		IO:      ios,
		HomeDir: func() (string, error) { return home, nil },
		Content: "---\nname: khbb\n---\nskill v2\n",
	}, home, out.String, errOut.String
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInstall_WritesToTheClaudeSkillsDirectory(t *testing.T) {
	opts, home, out, errOut := newOpts(t)
	if err := installRun(opts); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "skills", "khbb", "SKILL.md")
	if got := readFile(t, path); got != opts.Content {
		t.Errorf("SKILL.md = %q", got)
	}
	if out() != "" {
		t.Errorf("stdout = %q, want nothing", out())
	}
	if want := "Installed the khbb skill at " + path; !strings.Contains(errOut(), want) {
		t.Errorf("stderr = %q, want %q", errOut(), want)
	}
}

func TestInstall_Dir(t *testing.T) {
	opts, _, _, _ := newOpts(t)
	opts.Dir = filepath.Join(t.TempDir(), "repo", ".claude", "skills", "khbb")
	if err := installRun(opts); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(opts.Dir, "SKILL.md")); got != opts.Content {
		t.Errorf("SKILL.md = %q", got)
	}
}

func TestInstall_AlreadyUpToDate(t *testing.T) {
	opts, _, _, errOut := newOpts(t)
	opts.Dir = t.TempDir()
	path := filepath.Join(opts.Dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(opts.Content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installRun(opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut(), "already up to date") {
		t.Errorf("stderr = %q", errOut())
	}
}

func TestInstall_KeepsADifferentFileWithoutForce(t *testing.T) {
	opts, _, _, _ := newOpts(t)
	opts.Dir = t.TempDir()
	path := filepath.Join(opts.Dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("my own notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := installRun(opts)
	var conflict *cmdutil.ConflictError
	if !errors.As(err, &conflict) || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("err = %v, want a conflict that mentions --force", err)
	}
	if got := readFile(t, path); got != "my own notes\n" {
		t.Errorf("SKILL.md was changed to %q", got)
	}
}

func TestInstall_ForceReplaces(t *testing.T) {
	opts, _, _, errOut := newOpts(t)
	opts.Dir, opts.Force = t.TempDir(), true
	path := filepath.Join(opts.Dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("skill v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installRun(opts); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != opts.Content {
		t.Errorf("SKILL.md = %q", got)
	}
	if !strings.Contains(errOut(), "Updated the khbb skill at "+path) {
		t.Errorf("stderr = %q", errOut())
	}
}

func TestInstall_NoHomeDirectory(t *testing.T) {
	opts, _, _, _ := newOpts(t)
	opts.HomeDir = func() (string, error) { return "", errors.New("$HOME is not defined") }
	if err := installRun(opts); err == nil || !strings.Contains(err.Error(), "use --dir") {
		t.Errorf("err = %v", err)
	}
}

func TestInstall_DirIsAFile(t *testing.T) {
	opts, _, _, _ := newOpts(t)
	opts.Dir = filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(opts.Dir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installRun(opts); err == nil {
		t.Error("expected an error when --dir is a file")
	}
}

func TestNewCmdInstall(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	var got *InstallOptions
	cmd := NewCmdInstall(&cmdutil.Factory{IOStreams: ios}, func(o *InstallOptions) error { got = o; return nil })
	cmd.SetArgs([]string{"--dir", "skills/khbb", "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Dir != "skills/khbb" || !got.Force {
		t.Errorf("opts = %+v", got)
	}
	if got.Content != skills.Khbb || !strings.HasPrefix(got.Content, "---\nname: khbb\n") {
		t.Errorf("the embedded skill must be installed, got %.40q", got.Content)
	}

	cmd = NewCmdInstall(&cmdutil.Factory{IOStreams: ios}, func(*InstallOptions) error { return nil })
	cmd.SetArgs([]string{"extra"})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	var flagErr *cmdutil.FlagError
	if err := cmd.Execute(); !errors.As(err, &flagErr) {
		t.Errorf("an argument must be a usage error, got %v", err)
	}
}
```

Append to `pkg/cmd/root/root_test.go`:

```go
func TestRootRegistersSkillGroup(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "1.2.3", IOStreams: ios})
	cmd.SetArgs([]string{"skill", "instal"})
	var flagErr *cmdutil.FlagError
	if err := cmd.Execute(); !errors.As(err, &flagErr) || !strings.Contains(err.Error(), `unknown command "instal" for "khbb skill"`) {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./pkg/cmd/skill/... ./pkg/cmd/root/`
Expected: FAIL — the `install` package does not exist yet; the root test gets `unknown command "skill" for "khbb"`.

- [ ] **Step 4: Implement the command**

Create `pkg/cmd/skill/install/install.go`:

```go
// Package install implements `khbb skill install`.
package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/skills"
)

// InstallOptions holds the dependencies and flags of `khbb skill install`.
type InstallOptions struct {
	IO      *iostreams.IOStreams
	HomeDir func() (string, error)
	Content string

	Dir   string
	Force bool
}

// NewCmdInstall returns `khbb skill install`.
func NewCmdInstall(f *cmdutil.Factory, runF func(*InstallOptions) error) *cobra.Command {
	opts := &InstallOptions{IO: f.IOStreams, HomeDir: os.UserHomeDir, Content: skills.Khbb}
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the khbb skill for AI coding agents",
		Long: `Write the khbb agent skill, a SKILL.md file that teaches AI coding agents such as Claude Code
to use khbb: JSON output, exit statuses, and which commands need a human's approval. The skill
matches this version of khbb.

The skill is written to ~/.claude/skills/khbb/SKILL.md, or into the directory given with --dir,
such as a repository's .claude/skills/khbb. An existing SKILL.md with other content is only
replaced with --force: run "khbb skill install --force" after upgrading khbb.`,
		Example: `  $ khbb skill install
  $ khbb skill install --force
  $ khbb skill install --dir .claude/skills/khbb`,
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return installRun(opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", "", "Write SKILL.md into this `directory` (default: ~/.claude/skills/khbb)")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Replace an existing SKILL.md that differs")
	return cmd
}

func installRun(opts *InstallOptions) error {
	dir := opts.Dir
	if dir == "" {
		home, err := opts.HomeDir()
		if err != nil {
			return fmt.Errorf("could not find your home directory (%w); use --dir", err)
		}
		dir = filepath.Join(home, ".claude", "skills", "khbb")
	}
	path := filepath.Join(dir, "SKILL.md")

	existing, err := os.ReadFile(path)
	exists := err == nil
	switch {
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("reading %s: %w", path, err)
	case exists && string(existing) == opts.Content:
		fmt.Fprintf(opts.IO.ErrOut, "The khbb skill at %s is already up to date\n", path)
		return nil
	case exists && !opts.Force:
		return &cmdutil.ConflictError{Msg: fmt.Sprintf("%s already exists and differs from this version's skill; rerun with --force to replace it", path)}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(opts.Content), 0o644); err != nil {
		return err
	}
	verb := "Installed"
	if exists {
		verb = "Updated"
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s the khbb skill at %s\n", verb, path)
	return nil
}
```

Create `pkg/cmd/skill/skill.go`:

```go
// Package skill groups the `khbb skill` commands.
package skill

import (
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/skill/install"
)

// NewCmdSkill returns `khbb skill`.
func NewCmdSkill(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill <command>",
		Short: "Install the khbb skill for AI coding agents",
		Args:  cobra.ArbitraryArgs,
		RunE:  cmdutil.GroupRunE,
	}
	cmd.AddCommand(install.NewCmdInstall(f, nil))
	return cmd
}
```

In `pkg/cmd/root/root.go`, import `skillCmd "github.com/khipu/khbb/pkg/cmd/skill"` (after the `prCmd` import) and add `skillCmd.NewCmdSkill(f),` after `prCmd.NewCmdPR(f),` in `cmd.AddCommand`.

- [ ] **Step 5: Run the tests and try it**

Run: `go test ./... && go build -o bin/khbb ./cmd/khbb`
Expected: PASS. Then, in a temporary directory (never `~/.claude`):

```bash
D=$(mktemp -d)
./bin/khbb skill install --dir "$D/khbb"; echo "exit=$?"            # Installed the khbb skill at …/SKILL.md, exit 0
./bin/khbb skill install --dir "$D/khbb"; echo "exit=$?"            # … is already up to date, exit 0
echo "local edit" >> "$D/khbb/SKILL.md"
./bin/khbb skill install --dir "$D/khbb"; echo "exit=$?"            # error: … rerun with --force …, exit 1
./bin/khbb skill install --dir "$D/khbb" --force; echo "exit=$?"    # Updated the khbb skill at …, exit 0
cmp "$D/khbb/SKILL.md" skills/khbb/SKILL.md && echo identical
```

- [ ] **Step 6: Lint and commit**

```bash
gofmt -l skills pkg
golangci-lint run ./...
git add .gitattributes skills pkg/cmd/skill pkg/cmd/root/root.go pkg/cmd/root/root_test.go
git commit -F <message file>
```

Message: `feat(skill): embed the khbb agent skill and add khbb skill install`

---

### Task 4: Keep the skill in step with the commands

**Files:**
- Create: `pkg/cmd/root/docs_test.go`

**Interfaces:**
- Consumes: `root.NewCmdRoot`, `skills.Khbb` (Task 3), cobra's `Command.Find`, `Command.InitDefaultCompletionCmd`, `Command.Flags`, `Command.InheritedFlags`, pflag's `FlagSet.Lookup`/`ShorthandLookup`.
- Produces (test helpers in package `root_test`, used again by Task 7): `newDocsRoot() *cobra.Command`, `commandLines(doc string) []string`, `checkCommandLine(rootCmd *cobra.Command, line string) string`, `checkDocCommands(t *testing.T, name, doc string)`.

- [ ] **Step 1: Write the tests**

Create `pkg/cmd/root/docs_test.go`:

```go
package root_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/root"
	"github.com/khipu/khbb/skills"
)

// The documentation tests keep the agent skill (and the README) in step with the command tree:
// every command is mentioned in the skill, and every khbb command line in their code blocks names
// a real command and only flags it accepts.

func newDocsRoot() *cobra.Command {
	ios, _, _, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "test", IOStreams: ios})
	cmd.InitDefaultCompletionCmd()
	return cmd
}

// leafCommands returns every visible command that has no subcommands, such as `khbb pr create`.
func leafCommands(c *cobra.Command) []*cobra.Command {
	if c.Hidden || c.Name() == "help" {
		return nil
	}
	if !c.HasSubCommands() {
		return []*cobra.Command{c}
	}
	var out []*cobra.Command
	for _, sub := range c.Commands() {
		out = append(out, leafCommands(sub)...)
	}
	return out
}

// codeLines returns the lines inside the fenced code blocks of a Markdown document, without a
// leading "$ " prompt.
func codeLines(doc string) []string {
	var lines []string
	inCode := false
	for _, line := range strings.Split(doc, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			lines = append(lines, strings.TrimPrefix(t, "$ "))
		}
	}
	return lines
}

var shellSeparator = regexp.MustCompile(`&&|\|\||;|\|`)

// commandLines returns the khbb invocations in a document's code blocks: each part of a code line,
// split at &&, ||, ; and |, that starts with "khbb ".
func commandLines(doc string) []string {
	var out []string
	for _, line := range codeLines(doc) {
		for _, part := range shellSeparator.Split(line, -1) {
			if part = strings.TrimSpace(part); strings.HasPrefix(part, "khbb ") {
				out = append(out, part)
			}
		}
	}
	return out
}

// checkCommandLine returns what is wrong with a khbb command line, or "" when it names a command
// and only flags that command accepts. Arguments are split on spaces, so quoted values must not
// start with "-".
func checkCommandLine(rootCmd *cobra.Command, line string) string {
	cmd, rest, err := rootCmd.Find(strings.Fields(line)[1:])
	if err != nil {
		return err.Error()
	}
	if cmd == rootCmd || cmd.HasSubCommands() {
		return "names no khbb command"
	}
	for _, arg := range rest {
		if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
			continue
		}
		if !hasFlag(cmd, arg) {
			return fmt.Sprintf("%s has no flag %s", cmd.CommandPath(), arg)
		}
	}
	return ""
}

func hasFlag(cmd *cobra.Command, arg string) bool {
	name, _, _ := strings.Cut(arg, "=")
	if name == "--help" || name == "-h" {
		return true
	}
	for _, set := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags()} {
		if long, ok := strings.CutPrefix(name, "--"); ok {
			if set.Lookup(long) != nil {
				return true
			}
		} else if short := name[1:]; len(short) == 1 && set.ShorthandLookup(short) != nil {
			return true
		}
	}
	return false
}

// checkDocCommands fails the test for every khbb command line in doc that checkCommandLine rejects.
func checkDocCommands(t *testing.T, name, doc string) {
	t.Helper()
	rootCmd := newDocsRoot()
	lines := commandLines(doc)
	if len(lines) == 0 {
		t.Fatalf("%s: no khbb command lines in code blocks", name)
	}
	for _, line := range lines {
		if problem := checkCommandLine(rootCmd, line); problem != "" {
			t.Errorf("%s: %q: %s", name, line, problem)
		}
	}
}

func TestCheckCommandLine(t *testing.T) {
	rootCmd := newDocsRoot()
	cases := map[string]string{
		"khbb pr list --state merged -L 5 --json id,title":    "",
		"khbb pr ls -R acme/widgets":                          "",
		"khbb pr view 42 --json comments --jq '.comments[]'":  "",
		"khbb pipeline logs --failed --tail 200 --step=Build": "",
		"khbb api user -X GET --help":                         "",
		"khbb nope":                                           "names no khbb command",
		"khbb pr nope":                                        "names no khbb command",
		"khbb pr":                                             "names no khbb command",
		"khbb pr list --nope":                                 "khbb pr list has no flag --nope",
		"khbb pr view -Z":                                     "khbb pr view has no flag -Z",
	}
	for line, want := range cases {
		if got := checkCommandLine(rootCmd, line); got != want {
			t.Errorf("%q: got %q, want %q", line, got, want)
		}
	}
}

func TestCommandLines(t *testing.T) {
	doc := "Run `khbb pr list` first.\n\n```bash\n$ git push && khbb pipeline watch --exit-status\nkhbb pr view --json id | jq .\n# khbb in a comment\n```\nkhbb outside a block\n"
	got := commandLines(doc)
	want := []string{"khbb pipeline watch --exit-status", "khbb pr view --json id"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("commandLines = %q, want %q", got, want)
	}
}

func TestSkillMentionsEveryCommand(t *testing.T) {
	for _, c := range leafCommands(newDocsRoot()) {
		mention := regexp.MustCompile("`" + regexp.QuoteMeta(c.CommandPath()) + "[ `]")
		if !mention.MatchString(skills.Khbb) {
			t.Errorf("skills/khbb/SKILL.md does not mention `%s`", c.CommandPath())
		}
	}
}

func TestSkillCommandLinesAreValid(t *testing.T) {
	checkDocCommands(t, "skills/khbb/SKILL.md", skills.Khbb)
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./pkg/cmd/root/ -run 'CheckCommandLine|CommandLines|Skill' -v`
Expected: PASS for all four. If `TestSkillCommandLinesAreValid` or `TestSkillMentionsEveryCommand` fails, the skill from Task 3 has drifted: fix SKILL.md (not the test) and say so in your report.

- [ ] **Step 3: Prove the checks bite**

Temporarily change `--tail 200` to `--last 200` in SKILL.md and delete `` `khbb pr unapprove`, `` from its command list, then run `go test ./pkg/cmd/root/ -run Skill`. Expected: FAIL with `khbb pipeline logs has no flag --last` and ``does not mention `khbb pr unapprove` ``. Restore the file (`git checkout skills/khbb/SKILL.md`) and rerun: PASS.

- [ ] **Step 4: Lint and commit**

```bash
gofmt -l pkg
golangci-lint run ./...
git add pkg/cmd/root/docs_test.go
git commit -F <message file>
```

Message: `test(skill): check that SKILL.md names every command and only real flags`

---

### Task 5: Opt-in end-to-end tests

**Files:**
- Create: `e2e/e2e_test.go`
- Modify: `Makefile` (target `e2e`)

**Interfaces:**
- Consumes: the khbb command line only (the test builds `github.com/khipu/khbb/cmd/khbb` and runs it), plus `git`.
- Produces: `make e2e`; environment `KHBB_E2E=1` (required), `KHBB_E2E_REPO` (default `khipu/khipubb-sandbox`), `KHBB_E2E_REMOTE` (default `git@bitbucket.org:<repo>.git`). README (Task 7) documents them.

The test needs the sandbox's `bitbucket-pipelines.yml` (on its `main`): branches `khbb-probe/*` run steps `Build` (`echo "build"`) and `Slow` (`sleep 60`); every pull request runs `PR check`; the custom pipeline `khbb-fail` runs `Fails` (`echo "about to fail"`, `exit 3`) and then `Never`.

**Implementers do not run the live test** (it writes to Bitbucket with the user's login); the controller runs it in Task 8. Implementers check that it compiles, vets, lints and skips.

- [ ] **Step 1: Write the test**

Create `e2e/e2e_test.go`:

```go
// Package e2e runs khbb against a real Bitbucket repository. The tests are skipped unless
// KHBB_E2E=1. They push a khbb-probe/e2e-* branch to the sandbox repository, open a pull request,
// comment on and approve it, follow its pipelines, run a custom pipeline, then decline the pull
// request and delete the branch.
//
// Requirements: khbb logged in (keyring or KHBB_EMAIL/KHBB_TOKEN), git push access to the sandbox
// over SSH (or KHBB_E2E_REMOTE), and the sandbox's test bitbucket-pipelines.yml.
package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const defaultRepo = "khipu/khipubb-sandbox"

// khbbBin is the khbb binary built by TestMain.
var khbbBin string

func TestMain(m *testing.M) {
	if os.Getenv("KHBB_E2E") != "1" {
		os.Exit(m.Run()) // every test skips itself
	}
	dir, err := os.MkdirTemp("", "khbb-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	khbbBin = filepath.Join(dir, "khbb")
	if runtime.GOOS == "windows" {
		khbbBin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", khbbBin, "github.com/khipu/khbb/cmd/khbb").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building khbb: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// checkSandboxRepo refuses repositories that are not sandboxes: the tests write to the repository.
func checkSandboxRepo(repo string) error {
	if !strings.Contains(strings.ToLower(repo), "sandbox") {
		return fmt.Errorf("KHBB_E2E_REPO=%q: the end-to-end tests open pull requests and run pipelines, so they only run against a repository whose name contains \"sandbox\"", repo)
	}
	return nil
}

func TestCheckSandboxRepo(t *testing.T) {
	if err := checkSandboxRepo("khipu/khipubb-sandbox"); err != nil {
		t.Error(err)
	}
	if err := checkSandboxRepo("khipu/payments"); err == nil {
		t.Error("a repository that is not a sandbox must be refused")
	}
}

// sandbox skips the test unless KHBB_E2E=1 and returns the sandbox repository.
func sandbox(t *testing.T) string {
	t.Helper()
	if os.Getenv("KHBB_E2E") != "1" {
		t.Skip("set KHBB_E2E=1 to run the end-to-end tests against the sandbox repository")
	}
	repo := os.Getenv("KHBB_E2E_REPO")
	if repo == "" {
		repo = defaultRepo
	}
	if err := checkSandboxRepo(repo); err != nil {
		t.Fatal(err)
	}
	return repo
}

// runner runs khbb and git in a clone of the sandbox.
type runner struct {
	t   *testing.T
	dir string
}

// env is the environment of every command: no prompts, no color, no git credential prompts, and
// KHBB_REPO cleared so that khbb resolves the repository from the clone's remote.
func env() []string {
	return append(os.Environ(), "KHBB_PROMPT_DISABLED=1", "NO_COLOR=1", "GIT_TERMINAL_PROMPT=0", "KHBB_REPO=")
}

// run executes khbb and returns its trimmed stdout, its stderr and its exit status.
func (r *runner) run(args ...string) (string, string, int) {
	r.t.Helper()
	cmd := exec.Command(khbbBin, args...)
	cmd.Dir, cmd.Env = r.dir, env()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return strings.TrimSpace(stdout.String()), stderr.String(), 0
	case errors.As(err, &exitErr):
		return strings.TrimSpace(stdout.String()), stderr.String(), exitErr.ExitCode()
	}
	r.t.Fatalf("khbb %s: %v", strings.Join(args, " "), err)
	return "", "", 0
}

// out runs khbb, requires exit status 0 and returns stdout.
func (r *runner) out(args ...string) string {
	r.t.Helper()
	stdout, stderr, code := r.run(args...)
	if code != 0 {
		r.t.Fatalf("khbb %s: exit status %d\n%s", strings.Join(args, " "), code, stderr)
	}
	return stdout
}

// equal runs khbb and requires exit status 0 and the given stdout.
func (r *runner) equal(want string, args ...string) {
	r.t.Helper()
	if got := r.out(args...); got != want {
		r.t.Errorf("khbb %s = %q, want %q", strings.Join(args, " "), got, want)
	}
}

// exits runs khbb, requires the given exit status and returns stderr.
func (r *runner) exits(want int, args ...string) string {
	r.t.Helper()
	_, stderr, code := r.run(args...)
	if code != want {
		r.t.Errorf("khbb %s: exit status %d, want %d\n%s", strings.Join(args, " "), code, want, stderr)
	}
	return stderr
}

// git runs git in dir and fails the test on error.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, env()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestPullRequestAndPipelineLifecycle(t *testing.T) {
	repo := sandbox(t)
	remote := os.Getenv("KHBB_E2E_REMOTE")
	if remote == "" {
		remote = "git@bitbucket.org:" + repo + ".git"
	}
	clone := filepath.Join(t.TempDir(), "sandbox")
	git(t, "", "clone", "-q", "--depth", "1", remote, clone)
	k := &runner{t: t, dir: clone}
	k.out("auth", "status")

	stamp := time.Now().UTC().Format("20060102-150405")
	branch := "khbb-probe/e2e-" + stamp
	git(t, clone, "checkout", "-q", "-b", branch)
	if err := os.MkdirAll(filepath.Join(clone, "probe"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, "probe", "e2e-"+stamp+".txt"), []byte("khbb e2e\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, clone, "add", "probe")
	git(t, clone, "-c", "user.name=khbb-e2e", "-c", "user.email=e2e@example.invalid", "commit", "-q", "-m", "probe: khbb end-to-end test")
	git(t, clone, "push", "-q", "origin", branch)
	t.Cleanup(func() {
		cmd := exec.Command("git", "push", "-q", "origin", "--delete", branch)
		cmd.Dir, cmd.Env = clone, env()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("deleting %s: %v\n%s", branch, err, out)
		}
	})

	// The push starts the branch pipeline: Build, then a 60-second Slow step.
	k.out("pipeline", "watch", "--exit-status")
	k.equal("successful", "pipeline", "view", "--json", "status", "--jq", ".status")
	if logs := k.out("pipeline", "logs", "--step", "Build"); !strings.Contains(logs, "build") {
		t.Errorf("Build step log = %q", logs)
	}

	id := k.out("pr", "create", "--title", "khbb e2e "+stamp, "--body", "Opened by khbb's end-to-end tests; safe to decline.",
		"--no-default-reviewers", "--json", "id", "--jq", ".id")
	declined := false
	t.Cleanup(func() {
		if !declined {
			k.run("pr", "decline", id, "--yes", "--message", "End-to-end test cleanup")
		}
	})
	k.equal(branch, "pr", "view", "--json", "sourceBranch", "--jq", ".sourceBranch")

	commentID := k.out("pr", "comment", id, "--body", "e2e comment", "--json", "id", "--jq", ".id")
	k.out("pr", "comment", id, "--reply-to", commentID, "--body", "e2e reply")
	k.equal("2", "pr", "view", id, "--json", "comments", "--jq", "[.comments[] | select(.deleted | not)] | length")

	k.out("pr", "approve", id)
	k.equal("1", "pr", "view", id, "--json", "participants", "--jq", "[.participants[] | select(.approved)] | length")
	k.out("pr", "unapprove", id)

	// The pull-request pipeline may still be running.
	k.out("pr", "checks", id, "--watch")

	k.out("pr", "merge", id, "--dry-run")
	if stderr := k.exits(1, "pr", "merge", id, "--json", "state"); !strings.Contains(stderr, `"code":"confirmation_required"`) {
		t.Errorf("merge without --yes: stderr = %q", stderr)
	}

	number := k.out("pipeline", "run", "--branch", branch, "--custom", "khbb-fail", "--json", "number", "--jq", ".number")
	k.exits(1, "pipeline", "watch", number, "--exit-status")
	if logs := k.out("pipeline", "logs", number, "--failed"); !strings.Contains(logs, "about to fail") {
		t.Errorf("failed step log = %q", logs)
	}

	k.equal("DECLINED", "pr", "decline", id, "--yes", "--message", "End-to-end test done", "--json", "state", "--jq", ".state")
	declined = true
}
```

- [ ] **Step 2: Run it without `KHBB_E2E`**

Run: `go vet ./e2e && go test ./e2e -v`
Expected: `TestCheckSandboxRepo` PASS; `TestPullRequestAndPipelineLifecycle` SKIP with `set KHBB_E2E=1 to run the end-to-end tests against the sandbox repository`; the package passes. Nothing is built or sent.

- [ ] **Step 3: Add the Makefile target**

In `Makefile`, change `.PHONY: build test lint` to `.PHONY: build test lint e2e` and append:

```make
e2e:
	KHBB_E2E=1 go test ./e2e -count=1 -v -timeout 15m
```

(The recipe line starts with a tab.) Run `make -n e2e`; expected: `KHBB_E2E=1 go test ./e2e -count=1 -v -timeout 15m`.

- [ ] **Step 4: Lint and commit**

```bash
gofmt -l e2e
golangci-lint run ./...
go test ./...
git add e2e Makefile
git commit -F <message file>
```

Message: `test(e2e): opt-in end-to-end test of pull requests and pipelines against the sandbox`

---

### Task 6: Release pipeline and CI

**Files:**
- Create: `.goreleaser.yaml`, `.github/workflows/release.yml`
- Modify: `.github/workflows/ci.yml`, `.golangci.yml`

**Interfaces:**
- Consumes: `main.version`, `main.commit`, `main.date` (`cmd/khbb/main.go`), the hidden cobra `completion` command, the Actions secret `TAP_GITHUB_TOKEN` (created by the user in Task 9; not needed for anything in this task).
- Produces: `goreleaser release --snapshot --clean` output in `dist/` (git-ignored): six archives, `checksums.txt`, `dist/homebrew/Casks/khbb.rb`, `dist/scoop/khbb.json`.

- [ ] **Step 1: Write the GoReleaser configuration**

Create `.goreleaser.yaml`:

```yaml
# GoReleaser configuration: builds khbb for macOS, Linux and Windows on amd64 and arm64, publishes
# the archives and checksums.txt to a GitHub release of khipu/khbb, and updates the Homebrew tap
# (khipu/homebrew-tap) and the Scoop bucket (khipu/scoop-bucket). Run by
# .github/workflows/release.yml when a v* tag is pushed.
version: 2

project_name: khbb

builds:
  - id: khbb
    main: ./cmd/khbb
    binary: khbb
    env:
      - CGO_ENABLED=0
    goos: [darwin, linux, windows]
    goarch: [amd64, arm64]
    flags:
      - -trimpath
    ldflags:
      - -s -w -X main.version={{ .Version }} -X main.commit={{ .ShortCommit }} -X main.date={{ .CommitDate }}
    mod_timestamp: "{{ .CommitTimestamp }}"

archives:
  - id: khbb
    formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]

checksum:
  name_template: checksums.txt

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs"
      - "^test"
      - "^chore"
      - "^ci"

release:
  github:
    owner: khipu
    name: khbb
  prerelease: auto
  footer: |
    Install with Homebrew: `brew install khipu/tap/khbb`. With Scoop: `scoop bucket add khipu https://github.com/khipu/scoop-bucket` then `scoop install khbb`. Or download an archive below and check it against `checksums.txt`.

homebrew_casks:
  - name: khbb
    repository:
      owner: khipu
      name: homebrew-tap
      token: "{{ .Env.TAP_GITHUB_TOKEN }}"
    directory: Casks
    homepage: https://github.com/khipu/khbb
    description: Bitbucket Cloud CLI for pull requests and pipelines
    license: MIT
    skip_upload: auto
    generate_completions_from_executable:
      shell_parameter_format: cobra
      shells: [bash, zsh, fish]
    hooks:
      post:
        # The binaries are not signed or notarized; without this, macOS refuses to run them.
        install: |
          if OS.mac?
            system_command "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "#{staged_path}/khbb"]
          end

scoops:
  - name: khbb
    repository:
      owner: khipu
      name: scoop-bucket
      token: "{{ .Env.TAP_GITHUB_TOKEN }}"
    homepage: https://github.com/khipu/khbb
    description: Bitbucket Cloud CLI for pull requests and pipelines
    license: MIT
    skip_upload: auto
```

- [ ] **Step 2: Check it and build a snapshot**

Run (with the `goreleaser` binary the controller provides; `--snapshot` skips validation and publishing, so an uncommitted tree is fine):

```bash
goreleaser check
goreleaser release --snapshot --clean
ls dist
tar -tzf dist/khbb_*_darwin_arm64.tar.gz
unzip -l dist/khbb_*_windows_amd64.zip
cat dist/homebrew/Casks/khbb.rb dist/scoop/khbb.json
./dist/khbb_$(go env GOOS)_$(go env GOARCH)*/khbb version
```

Expected: `1 configuration file(s) validated` with no deprecation warnings; four `.tar.gz` (darwin and linux × amd64, arm64), two `.zip` (windows × amd64, arm64) and `checksums.txt`; each archive holds `khbb` (or `khbb.exe`), `LICENSE` and `README.md` once Task 7 has added it; the cask has `binary "khbb"`, a `postflight` block running `/usr/bin/xattr -dr com.apple.quarantine "#{staged_path}/khbb"` on macOS, and `generate_completions_from_executable "khbb", shell_parameter_format: :cobra, shells: [:bash, :zsh, :fish]`; the Scoop manifest lists both Windows zips with `"bin": ["khbb.exe"]`; the local binary prints `khbb version 0.0.0-SNAPSHOT-<commit> (<commit>, <commit date>)`. `TAP_GITHUB_TOKEN` is not needed for a snapshot. Remove `dist/` afterwards (`rm -rf dist`).

- [ ] **Step 3: Write the release workflow**

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags: ["v*"]

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
      - run: go test ./...
      - uses: goreleaser/goreleaser-action@v7
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          # Fine-grained token with Contents read and write on khipu/homebrew-tap and khipu/scoop-bucket.
          TAP_GITHUB_TOKEN: ${{ secrets.TAP_GITHUB_TOKEN }}
```

- [ ] **Step 4: Pin the linter, add gofmt and `goreleaser check` to CI**

In `.github/workflows/ci.yml`, change the lint step's `version: latest` to `version: v2.14.0` and append a job after `lint` (same indentation as `lint:`):

```yaml
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: goreleaser/goreleaser-action@v7
        with:
          distribution: goreleaser
          version: "~> v2"
          args: check
```

Append to `.golangci.yml`:

```yaml
formatters:
  enable:
    - gofmt
```

Check that the formatter reports: create a throwaway `skills/probe.go` containing `package skills` and `var   Probe = 1`, run `golangci-lint run ./skills/` (expected: `File is not properly formatted (gofmt)`), delete the file, run `golangci-lint run ./...` (expected: `0 issues.`).

- [ ] **Step 5: Commit**

```bash
git add .goreleaser.yaml .github/workflows/release.yml .github/workflows/ci.yml .golangci.yml
git commit -F <message file>
```

Message: `build(release): GoReleaser with Homebrew cask and Scoop; pin golangci-lint, add gofmt and goreleaser check to CI`

---

### Task 7: README

**Files:**
- Create: `README.md`
- Modify: `pkg/cmd/root/docs_test.go` (README check)

**Interfaces:**
- Consumes: `checkDocCommands` (Task 4); `make e2e` and the `KHBB_E2E*` variables (Task 5); the install commands of Task 6.

- [ ] **Step 1: Write the failing test**

In `pkg/cmd/root/docs_test.go`, add `"os"` to the imports and append:

```go
func TestReadmeCommandLinesAreValid(t *testing.T) {
	readme, err := os.ReadFile("../../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	checkDocCommands(t, "README.md", string(readme))
}
```

Run: `go test ./pkg/cmd/root/ -run Readme`
Expected: FAIL — `open ../../../README.md: no such file or directory`.

- [ ] **Step 2: Write the README**

Create `README.md` with exactly this content:

````markdown
# khbb

Bitbucket Cloud from your terminal. `khbb` brings `gh`-style commands for pull requests and
pipelines to Bitbucket Cloud, for you and for the AI agents you work with.

```bash
khbb pr create --fill
khbb pipeline watch --exit-status
khbb pr checks --watch
khbb pr merge --squash --delete-branch
```

- **Pull requests:** create, list, view, diff, checks, comment (also inline and replies), edit,
  approve, request changes, merge, decline and check out.
- **Pipelines:** list, view, logs, watch, run (branch, tag, commit or custom, with variables),
  rerun and stop.
- **`khbb api`** for everything else in the Bitbucket REST API.
- **Made for scripts and agents:** stable JSON with `--json`, `--jq` and `--template`, clear exit
  statuses, no prompts without a terminal, `--dry-run` on every change to Bitbucket, and
  confirmation before anything that cannot be undone.

## Install

**Homebrew**

```bash
brew install khipu/tap/khbb
```

**Scoop (Windows)**

```bash
scoop bucket add khipu https://github.com/khipu/scoop-bucket
scoop install khbb
```

**With Go**

```bash
go install github.com/khipu/khbb/cmd/khbb@latest
```

**Binaries:** download an archive for macOS, Linux or Windows (amd64 or arm64) from the
[releases page](https://github.com/khipu/khbb/releases) and check it against `checksums.txt`. The
binaries are not signed: on macOS, run `xattr -d com.apple.quarantine khbb` once before the first
run (Homebrew does this for you).

## Log in

khbb uses an Atlassian API token with scopes; Bitbucket app passwords no longer work.

1. Create a token at <https://id.atlassian.com/manage-profile/security/api-tokens> with
   **Create API token with scopes**, app **Bitbucket**, and these scopes:
   `read:user:bitbucket`, `read:workspace:bitbucket`, `read:repository:bitbucket`,
   `read:pullrequest:bitbucket`, `write:pullrequest:bitbucket`, `read:pipeline:bitbucket` and
   `write:pipeline:bitbucket`.
2. Log in with your Atlassian email and the token:

```bash
khbb auth login
khbb auth status
```

The token is stored in your system keyring. On machines without one (CI, containers, headless
Linux), set `KHBB_EMAIL` and `KHBB_TOKEN` instead, or log in with
`echo "$TOKEN" | khbb auth login --email you@example.com --with-token --insecure-storage` to keep
it in khbb's config file (readable only by you). There is deliberately no command that prints the
token.

## Use it

khbb works out the repository from the git remotes of the current directory (`origin` first; SSH
host aliases are resolved). Anywhere else, pass `-R workspace/repo` or set `KHBB_REPO`. Commands
that take a pull request or pipeline number default to the current branch's open pull request or
newest pipeline.

### Pull requests

```bash
git push -u origin HEAD
khbb pr create --title "Add the widget factory" --body "Adds the factory and its tests." --reviewer ada
khbb pr list --author @me
khbb pr status
khbb pr view 42 --comments
khbb pr diff 42 --name-only
khbb pr checks 42 --watch
khbb pr comment 42 --file src/widget.go --line 12 --body "Can we cache this?"
khbb pr comment 42 --reply-to 101 --body "Done."
khbb pr approve 42
khbb pr merge 42 --squash --delete-branch
khbb pr checkout 42
```

khbb never pushes for you: push the branch before `pr create`. `pr merge`, `pr decline` and
`pipeline stop` ask for confirmation on a terminal and need `--yes` otherwise.

### Pipelines

```bash
khbb pipeline list --branch main --status failed
khbb pipeline watch --exit-status
khbb pipeline view 1234
khbb pipeline logs 1234 --failed --tail 100
khbb pipeline run --custom deploy-staging --var ENV=staging --secret-var API_KEY="$API_KEY" --watch
khbb pipeline rerun 1234
khbb pipeline stop 1234
```

Right after a `git push`, `khbb pipeline watch` waits for the pipeline of your HEAD commit and
follows it to the end. `--secret-var` values are sent as secured variables and never printed.

### Everything else

```bash
khbb api user --jq .display_name
khbb api 'repositories/{workspace}/{repo}/pullrequests' --paginate --jq '.[].title'
```

`{workspace}`, `{repo}` and `{branch}` are filled in from the current repository. Shell
completion: `khbb completion bash|zsh|fish|powershell` (Homebrew installs it for you).

## Scripting and agents

- **JSON:** `--json id,title,url` prints only those fields; `--json` alone lists them.
  `--jq` and `--template` filter and format the JSON. Field names are stable: renaming or
  removing one is a breaking change.
- **Streams:** stdout carries only data; prompts, progress, warnings and errors go to stderr.
  Without a terminal there are no prompts, colors or truncated tables.
- **Errors:** `error: <message>` on stderr, or one JSON line with `--json`:
  `{"error":{"code":"not_found","status":404,"message":"...","hint":"..."}}`.
- **Safety:** every command that changes something on Bitbucket takes `--dry-run`, which prints
  the request instead of sending it. Destructive commands need `--yes` without a terminal.

| Exit status | Meaning |
|---|---|
| 0 | Success |
| 1 | Error, a failed check (`pr checks`) or a failed pipeline (`pipeline watch --exit-status`) |
| 2 | Cancelled at a prompt |
| 4 | Not logged in, or the token was rejected |
| 8 | Checks still in progress (`pr checks`) |

### AI agents

khbb ships an agent skill that teaches AI coding agents such as Claude Code to use it well: read
JSON, check exit statuses, and never merge, decline or stop anything without your approval.

```bash
khbb skill install
```

This writes `~/.claude/skills/khbb/SKILL.md`. Use `--dir .claude/skills/khbb` to add it to a
repository instead, and run `khbb skill install --force` after upgrading khbb so the skill matches
the new version.

## Configuration

| Environment variable | Purpose |
|---|---|
| `KHBB_EMAIL`, `KHBB_TOKEN` | Credentials; they take precedence over the keyring |
| `KHBB_REPO` | Default `workspace/repo` |
| `KHBB_CONFIG_DIR` | Directory of `config.yml` |
| `KHBB_PROMPT_DISABLED` | Never prompt, even on a terminal |
| `KHBB_DEBUG` | Log HTTP requests and responses to stderr (the `Authorization` header is redacted) |
| `NO_COLOR` | No colors |
| `BROWSER` | Browser for `--web` |

`KHBB_PROMPT_DISABLED` and `KHBB_DEBUG` are off when unset or set to `0`, `false`, `no` or `off`.

`config.yml` lives in `$KHBB_CONFIG_DIR`, `$XDG_CONFIG_HOME/khbb`, `~/.config/khbb` (macOS and
Linux) or `%AppData%\khbb` (Windows). `khbb auth login` writes your email and username there. You
can also set `git_protocol` (`ssh` or `https`, used by `pr checkout` for pull requests from forks)
and `browser`.

## Development

```bash
make build
make test
make lint
```

`make build` writes `bin/khbb`. `make lint` needs [golangci-lint](https://golangci-lint.run) v2.
The design is in [`docs/superpowers/specs`](docs/superpowers/specs), and Bitbucket API behavior
that khbb relies on is recorded in the API findings document there.

**End-to-end tests** run khbb against a real sandbox repository: they push a `khbb-probe/e2e-*`
branch, open and decline a pull request, and run pipelines. They need a logged-in khbb, SSH push
access to the sandbox and its test `bitbucket-pipelines.yml`:

```bash
make e2e
```

They use `khipu/khipubb-sandbox` unless `KHBB_E2E_REPO` names another repository, whose name must
contain "sandbox".

**Releases** are cut by pushing a `v*` tag: GoReleaser builds the binaries, publishes the GitHub
release and updates the Homebrew tap and the Scoop bucket (see
[`.goreleaser.yaml`](.goreleaser.yaml)). Run `make e2e` first. JSON field changes need a new major
version.

## License

[MIT](LICENSE)
````

- [ ] **Step 3: Run the tests**

Run: `go test ./pkg/cmd/root/ && go test ./...`
Expected: PASS. A failing README line means the README is wrong, not the test.

- [ ] **Step 4: Commit**

```bash
git add README.md pkg/cmd/root/docs_test.go
git commit -F <message file>
```

Message: `docs: README with install, login, usage, scripting, agent skill and development`

---

### Task 8: Live verification (controller only)

Run by the controller, not a subagent: it uses the user's Bitbucket login and writes to `khipu/khipubb-sandbox` (user-approved: `khbb-probe/` branches, pull requests that are declined, test pipelines). Print only statuses, counts and khbb's own messages.

**Files:** none (results go into the SDD ledger; a mismatch becomes a fix task before the final review).

- [ ] **Step 1: Whole tree**

```bash
go test -race ./... && go vet ./...
golangci-lint run ./...
goreleaser check
goreleaser release --snapshot --clean
```

Inspect `dist/` as in Task 6 Step 2, now with `README.md` in every archive; run the snapshot binary for this machine (`khbb version`, `khbb skill install --dir <scratchpad>/skill`, `cmp` with `skills/khbb/SKILL.md`). Then `rm -rf dist`.

- [ ] **Step 2: End-to-end**

```bash
make e2e
```

Expected: both tests PASS in about 3–4 minutes (the plan's trial run took 197 s). Afterwards, with the repository's khbb build:

```bash
khbb api 'repositories/khipu/khipubb-sandbox/refs/branches' -X GET -f q='name ~ "khbb-probe/"' --jq '.values | length'   # 0
khbb pr list -R khipu/khipubb-sandbox --json sourceBranch --jq '[.[] | select(.sourceBranch | startswith("khbb-probe/"))] | length'   # 0
```

- [ ] **Step 3: `go install` and the version**

```bash
GOBIN=<scratchpad>/gobin go install ./cmd/khbb && <scratchpad>/gobin/khbb version   # pseudo-version and commit, not "dev"
make build && ./bin/khbb version                                                    # git describe value, no leading "v"
```

Record each result (expected vs. observed) in the ledger.

---

### Task 9: First release (controller only, after the user's explicit go-ahead)

Cutting a release publishes to GitHub, Homebrew and Scoop and cannot be fully undone (a tag and release can be deleted, but people may already have installed it). **Do not start this task without the user's explicit approval in the conversation**, and do not create repositories, tokens or secrets on their behalf.

**Prerequisites (the user does these; ask, never assume):**

1. Repositories `khipu/homebrew-tap` and `khipu/scoop-bucket` exist (public, default branch `main`, with at least a README so they have a first commit). Today both answer 404.
2. A fine-grained GitHub token with **Contents: read and write** on those two repositories (resource owner `khipu`; an organization admin may have to approve it) is stored as the Actions secret `TAP_GITHUB_TOKEN` of `khipu/khbb`, for example with `gh secret set TAP_GITHUB_TOKEN -R khipu/khbb`, typed in the user's own terminal. The token never appears in the conversation.
3. The user picks the version. Suggested: `v0.1.0` (pre-1.0: the JSON contract may still change).
4. `main` is pushed and its CI run is green; Task 8 passed.

- [ ] **Step 1: Check the prerequisites (read-only)**

```bash
gh api repos/khipu/homebrew-tap --jq .full_name
gh api repos/khipu/scoop-bucket --jq .full_name
gh secret list -R khipu/khbb        # TAP_GITHUB_TOKEN listed (names only)
gh run list -R khipu/khbb -b main -L 1 --json conclusion --jq '.[0].conclusion'   # success
```

- [ ] **Step 2: Tag and push (only after the user says go)**

```bash
git tag -a v0.1.0 -m "khbb v0.1.0"
git push origin v0.1.0
gh run watch -R khipu/khbb $(gh run list -R khipu/khbb -w Release -L 1 --json databaseId --jq '.[0].databaseId')
```

- [ ] **Step 3: Verify what was published**

```bash
gh release view v0.1.0 -R khipu/khbb --json assets --jq '[.assets[].name]'   # 6 archives + checksums.txt
gh api repos/khipu/homebrew-tap/contents/Casks/khbb.rb --jq .name             # khbb.rb
gh api repos/khipu/scoop-bucket/contents/khbb.json --jq .name                 # khbb.json
```

Then ask the user whether to try `brew install khipu/tap/khbb` on their machine (it installs software, so it is their call); if yes, `khbb version` must print `khbb version 0.1.0 (…)` and `khbb completion zsh` must work. If anything failed after the GitHub release was created, fix forward with `v0.1.1` rather than moving the tag.

---

## Spec Coverage (Plan 4)

| Spec item | Covered by |
|---|---|
| §7.4 `khbb skill install [--dir]`, default `~/.claude/skills/khbb/SKILL.md`, overwrite only with `--force` | Task 3 |
| §13 skill content (auth status first, JSON + jq, approval for merge/decline/stop with `--dry-run` first, `api` GET only, secrets, exit codes, recipes) | Task 3 (content), Task 4 (kept in step with the binary) |
| §11 E2E opt-in with `KHBB_E2E=1` against a sandbox: PR create → comment → approve → decline; pipeline run → watch → logs | Task 5, run in Task 8 |
| §11 CI: `go vet`, `golangci-lint`, `go test -race` matrix | existing CI; Task 6 pins lint and adds gofmt and `goreleaser check` |
| §12 GoReleaser on `v*`, six targets, archives + `checksums.txt`, Homebrew tap, Scoop bucket, `-ldflags` version info, semver | Tasks 2, 6, 9 |
| §5.3 environment variables (documented), `KHBB_DEBUG` | Tasks 1, 7 |
| §6 SSH aliases (hardening: altssh, option-like hosts) | Task 1 |
| §8 output contract, §9 safety (documented for users) | Task 7 |
| Memory backlog: pin golangci-lint + gofmt, `KHBB_DEBUG=0`, `ssh -G -…`, altssh, skill notes (exit status before stdout; `pipeline watch` after a push) | Tasks 1, 3, 6 |
| Deferred: `KHBB_PAGER` and `editor`, nested `api -f` keys, `GH_FORCE_TTY`, Plan 2a/2b/3 review backlogs | Not in this plan (rulings) |
