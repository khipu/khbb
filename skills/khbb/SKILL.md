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
2. khbb finds the repository from the git remotes of the current directory (`origin` first), but
   `KHBB_REPO`, when set, wins. Anywhere else, or to be sure, pass `-R workspace/repo`.
3. Without a number, commands act on the current branch: its open pull request (`pr view`,
   `pr checks`, `pr merge`, ...) or its newest pipeline (`pipeline view`, `pipeline logs`, ...).
   Pull requests and pipelines are otherwise named by their number (`42`) or their URL. Write the
   plain number: unquoted, `#42` starts a shell comment, and the command would act on the current
   branch instead.

## Rules

- **Read with JSON.** Use `--json <fields>` and filter with `--jq`; never parse the tables meant for
  people. `--json` without fields lists the available ones on stderr and exits 1. Ask only for the
  fields you need.
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
  show it with `--dry-run` first. Fields (`-f`, `-F`) and `--input` turn a request into a POST: add
  `-X GET` to send fields as query parameters.
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
