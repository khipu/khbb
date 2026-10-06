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
- **Retries and timeouts:** reads that hit a rate limit (429) or a 502, 503 or 504 are retried up
  to three times, and each wait is announced on stderr. A response that sends nothing for 60
  seconds fails with a `network` error; long logs that keep arriving are never cut.
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
