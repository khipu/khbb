# Bitbucket Cloud API findings

Resolves the "items to verify" list (§14) of
[`2026-10-05-khbb-cli-design.md`](2026-10-05-khbb-cli-design.md).

- **Source A:** Bitbucket Cloud OpenAPI document,
  `https://dac-static.atlassian.com/cloud/bitbucket/swagger.v3.json`
  (OpenAPI 3.0.0, API 2.0), downloaded 2026-10-05.
- **Source B:** live, read-only requests with `khbb api` (Plan 1, Task 12).
- **Source C:** write requests against the sandbox repo (Plan 2).

No personal data is recorded here: only endpoint shapes, field names and enum values.

## Resolved

| # | Question | Finding | Source | Decision |
|---|---|---|---|---|
| 2 | PR `draft` support | `pullrequest.draft` exists ("A boolean flag indicating whether the pull request is a draft"). | A | Keep `pr create --draft`, `pr edit --draft/--ready`. |
| 3 | Can declined PRs be reopened? | No reopen endpoint exists under `/pullrequests/{id}`. | A | No `pr reopen`; `pr decline` stays destructive. |
| 4 | Merge async flow | `POST …/merge` accepts `?async=true`; returns `200` (PR) when done, `202` with the task-status URL in `Location`, `409` if refs changed during merge, `555` if the merge timed out (retry later). `GET …/merge/task-status/{task_id}` returns the task status and, once successful, the PR. | A | Always call with `async=true` and poll `Location`. Map `555` to `server_error` with hint "merge timed out on Bitbucket; retry later". |
| 6 | Pipeline rerun endpoint | None. Pipeline paths: list/create, get, steps, step, step log, step logs/{log_uuid}, `stopPipeline`. | A | `pipeline rerun` = new `POST /pipelines` with the original target + selector (spec fallback). |
| 7 | Pipeline/step state values | Pipeline `state.name`: `PENDING`, `IN_PROGRESS` (`stage.name`: `RUNNING`, `PAUSED`), `COMPLETED` (`result.name`: `SUCCESSFUL`, `FAILED`, `ERROR`, `STOPPED`, `EXPIRED`). Step `state.name`: `PENDING`, `READY`, `IN_PROGRESS`, `COMPLETED` (`result.name`: `SUCCESSFUL`, `FAILED`, `ERROR`, `STOPPED`, `EXPIRED`, `NOT_RUN`). | A | Spec's `status` enum gains `expired` (pipelines and steps) and `skipped` (steps, from `NOT_RUN`); `READY` maps to `pending`. To apply in Plan 3. |
| 8 | Scope for workspace members | `GET /workspaces/{workspace}/members` → `read:workspace:bitbucket`. | A | Keep in required scopes. |
| 11 | `go-gh` reuse | v2.16.1 (MIT): `pkg/jq` (`Evaluate`, `EvaluateFormatted`), `pkg/template` (`New(w, width, color)`, `Parse`, `Execute`, `Flush`), `pkg/tableprinter` (`New(w, isTTY, maxWidth)`), `pkg/term` (`FromEnv`), `pkg/prompter` (`New`, `NewMock`), `pkg/browser` (`New`, `Browse`), `pkg/jsonpretty` (`Format`). All GitHub-agnostic. | probe module | Use as planned. |

### Extra findings (not in §14)

- **Per-operation API-token scopes** are published in the OpenAPI document as
  `x-atlassian-oauth2-scopes`. The ones khbb v1 uses:

  | Operation | Scopes |
  |---|---|
  | `GET /user` | `read:user` |
  | `GET /workspaces/{ws}/members` | `read:workspace` |
  | `GET /repositories/{ws}/{repo}` | `read:repository` |
  | `GET …/effective-default-reviewers` | `read:pullrequest` |
  | `GET` any `…/pullrequests…` | `read:pullrequest` |
  | `POST/PUT …/pullrequests…` (create, update, approve, request-changes, decline, merge) | `read:pullrequest` + `write:pullrequest` |
  | `POST …/pullrequests/{id}/comments` | `read:pullrequest` |
  | `GET` any `…/pipelines…` | `read:pipeline` |
  | `POST …/pipelines` | `read:pipeline` + `write:pipeline` |
  | `POST …/pipelines/{uuid}/stopPipeline` | `write:pipeline` |
  | `POST/PUT /repositories/{ws}/{repo}` | `admin:repository` |

  (All scope names carry the `:bitbucket` suffix.) The v1 required-scope list in the
  spec (§5.1) is confirmed complete.
- **Mergeability pre-check:** `GET …/pullrequests/{id}/mergeability/checks` lists the
  checks that decide whether a PR can be merged (types: `pullrequest_state_check`,
  `current_user_permission_check`, `git_mergeability_check`, `standard_merge_check`,
  `custom_merge_check`, `merge_queue_check`). Use it in `pr merge --dry-run` and to
  explain blocked merges (Plan 2).
- **Merge strategies:** `merge_commit`, `squash`, `fast_forward`,
  `squash_fast_forward`, `rebase_fast_forward`, `rebase_merge`. v1 ships only
  `--merge`, `--squash` and `--fast-forward` (spec §7.1); the other three
  (`squash_fast_forward`, `rebase_fast_forward`, `rebase_merge`) are reachable
  through `khbb api` for now.
- **Commit status** `state`: `SUCCESSFUL`, `FAILED`, `INPROGRESS`, `STOPPED`.
- **Participant** `state`: `approved`, `changes_requested`, `null` (→ khbb `pending`).

## Still open

| # | Question | How to resolve | When |
|---|---|---|---|
| 1 | Does `POST /pullrequests` add default reviewers automatically? | Not answerable in the sandbox: it has no default reviewers (`effective-default-reviewers` → 0), and PRs created there came back with `reviewers: []`. Plan 2b sidesteps the question: `pr create` reads `effective-default-reviewers` itself and sends the list explicitly (minus the author). | Plan 2b (decided, not verified) |

## Resolved with live requests (Source B)

Verified 2026-10-05 with `khbb api` (Plan 1, Task 12), read-only `GET`s against one
Khipu repository with recent pipelines and pull requests.

| # | Question | Finding | Decision |
|---|---|---|---|
| 5 | Build number in place of `{pipeline_uuid}` | `GET …/pipelines/<build_number>` → 200 with the full pipeline (including `uuid`). `GET …/pipelines/<build_number>/steps` → 200. `GET …/pipelines/<build_number>/steps/<step_uuid>/log` → 200 (with the right `Accept`, see below). The list filter `?q=build_number=N` is **ignored** (returns the unfiltered list). | Plan 3 uses build numbers directly in paths; never look a build up through `q=`. |
| 9 | Does the API expose a token's granted scopes? | Every response carries `X-Oauth-Scopes` (comma-separated granted scopes), `X-Accepted-Oauth-Scopes` (what the endpoint accepts) and `X-Credential-Type: api_token`. | `auth status` can list granted scopes and flag missing `RequiredScopes`; 403 hints can compare accepted vs granted. Follow-up for Plan 2. |
| 10 | Bearer vs Basic | Basic (`email:token`) works for every request made. | Closed: keep Basic. |
| — | Maximum `pagelen` | `pullrequests`: 50 (`pagelen=100` → 400 `Invalid pagelen`). `pipelines`: 100. `workspaces/{ws}/members`: 100. `effective-default-reviewers`: 100. `pipelines/{n}/steps`: ignores `pagelen` (returns all steps). | Keep the client default of 50; Plans 2–3 need no per-endpoint override. |
| — | Pipeline/step state shape (complements #7) | Completed pipelines: `state.name = COMPLETED`, `state.result.name ∈ {SUCCESSFUL, FAILED}` observed, `state.stage = null`. Steps: `state.name` + `state.result.name`. | Matches the OpenAPI-derived mapping in #7. |
| — | Step log endpoint | With `Accept: application/json` or `text/plain` → **406 Not Acceptable**. With `Accept: application/octet-stream` or `*/*` → 200. The body is served through a redirect (final response over HTTP/1.1) and was ~1.2 MB for a three-step pipeline. | Plan 3: request logs with `Accept: application/octet-stream`; replace the 30 s whole-request `http.Client.Timeout` with transport-level timeouts so large logs are not cut off (final-review Minor #9). |

## Pull request API (Source B, for Plan 2)

Verified 2026-10-05 with read-only `khbb api` requests. Shapes and status codes only.

| Topic | Finding | Decision |
|---|---|---|
| List vs. single PR | `GET …/pullrequests` items omit `participants` and `reviewers`; `GET …/pullrequests/{id}` includes them. `fields=+values.participants,+values.reviewers` (the `+` URL-encoded as `%2B`) adds them to list items. | List commands request them only when the output needs them. |
| PR fields | `id, title, description, state, draft, author, source{branch{name}, commit{hash}, repository{full_name}}, destination{…}, merge_commit{hash}\|null, reviewers[], participants[{user, role: REVIEWER\|PARTICIPANT, approved, state: approved\|changes_requested\|null}], comment_count, task_count, close_source_branch, closed_by, created_on, updated_on, links.html.href, summary.raw`. | Mapped to the spec §8.1 PullRequest shape. |
| State filter | Repeated `state=` parameters are ORed (`state=OPEN&state=MERGED&…`) and default to OPEN — but only without q: whenever q= is present Bitbucket ignores state= entirely and returns every state. The state must then be part of the BBQL (`state = "OPEN" AND …`). | ListPullRequests sends state= params without a query and folds the states into the BBQL with one. |
| BBQL filters | Supported: `source.branch.name`, `destination.branch.name`, `author.uuid`, `author.nickname`, `author.account_id`, `reviewers.uuid`, `reviewers.nickname`, `reviewers.account_id`, `title ~`, `state`, `draft`. `participants.*` → 400 "does not support filtering". | `--author`/`--reviewer` accept `@me`, `{uuid}`, account ID or nickname without a member lookup. |
| Comments | Items: `id, content{raw}, user, inline{path, from, to}\|absent, parent{id}\|absent, deleted, pending, created_on, updated_on, links`. | Comment shape: `line` = `inline.to`, else `inline.from`. |
| Commit statuses | `GET …/pullrequests/{id}/statuses` items: `key, name, state (SUCCESSFUL observed), description, url, updated_on, refname, commit`. Older PRs may have none. | `pr checks` with no statuses exits 0 with a notice. |
| Diff / patch | `…/diff` and `…/patch` answer 200 with `Accept` `application/json`, `text/plain` or `*/*`. `…/diffstat` items: `status, lines_added, lines_removed, old{path}\|null, new{path}\|null`. | Text endpoints are fetched with `Accept: */*` (works here and for logs). |
| `pagelen` | `statuses`, `diffstat` and `comments` accept 50. | Client default 50 is safe. |

## Pull request write API (Source C, sandbox)

Verified 2026-10-05 in the sandbox repository `khipu/khipubb-sandbox` with `khbb api`,
using probe branches `khbb-probe/*` (all deleted afterwards; PRs #1–#2 merged, #3–#4
declined). Shapes and status codes only.

| Topic | Finding | Decision |
|---|---|---|
| Create | `POST …/pullrequests` with `title`, `source.branch.name`, optional `destination`, `description`, `draft`, `close_source_branch`, `reviewers` → **201**. `draft: true` is honored. Without `destination` the main branch is used. | `pr create` sends exactly these fields. |
| Create, branch already has an open PR | **No error**: Bitbucket returns the existing PR (same `id`) and **overwrites** its `title` and `close_source_branch` with the new values. | `pr create` must look up an open PR for the source branch first and refuse (exit 1, print its URL) instead of silently editing it. |
| Create, same source into another destination | A second `POST` from the same source branch into a **different** destination creates a new PR (verified: two open PRs, one per destination). The silent update only happens for the same source **and** destination. | `pr create` refuses only when an open PR exists for the same head and base; it always sends `destination` (the repository's `mainbranch.name` when `-B` is absent). |
| Create, missing source branch | **400** `error.fields.source = ["branch not found: <name>"]`. | Surface as-is; hint to push the branch. |
| Reviewers | Accepted keys: `uuid` and `account_id`. `nickname` → **400** "Malformed reviewers list". An unknown uuid/account ID → the same 400. The author as reviewer → **400** "`<name>` is the author and cannot be included as a reviewer." | Resolve every `--reviewer` to a uuid before the request; drop the author from default reviewers. |
| Member lookup | `GET workspaces/{ws}/members?q=…` filters server-side on `user.nickname`, `user.account_id` and `user.uuid` (0 or 1 result); `user.display_name` → **400** "does not support filtering" (also with `~`). Items carry `user{uuid, account_id, nickname, display_name}`. Needs `read:workspace`. | `--reviewer` resolves uuid/account ID/nickname with one filtered request; a name with no nickname match falls back to scanning every member's display name (case-insensitive). |
| Edit | `PUT …/pullrequests/{id}` is **partial**: a body with only `title` keeps `description`, `draft`, `close_source_branch`. `draft: false` turns a draft into a ready PR. `reviewers: []` clears reviewers. Editing a closed PR → **400** "Can only update an open pull request." | `pr edit` sends only the changed fields; `pr ready` = `PUT {draft:false}`. |
| Edit destination | `PUT {destination: {branch: {name: X}}}` with a missing branch → 400 `fields.destination = ["branch not found: X"]`. | Surface as-is. |
| Repository main branch | `GET repositories/{ws}/{slug}` → `mainbranch{name, type}`. | Default base for `pr create`. |
| Reopen | Not possible: `PUT {state: OPEN}` on a declined PR → 400 (above). | No `pr reopen`. |
| Comments | `POST …/comments` with `content.raw` → 201. Inline: `inline{path, to}` → 201, response adds `src_rev, dest_rev, context_lines, outdated`. Reply: `parent{id}` → 201. **Not validated**: a line outside the diff (`to: 99`) and a path not in the diff both → 201. Comments on a declined PR → 201. | `pr comment` validates `--file` against the diffstat itself; line numbers are not checked. |
| Comment variants | `inline{path}` without `to`/`from` → 201, a file-level comment. `inline{path, from}` → 201, an old-side comment. Reply to an unknown `parent.id` → 400 "Invalid parent ID". | `--file` alone makes a file comment; `--line` targets the new side only. |
| Approve / request changes | `POST …/approve` → 200 participant `{approved: true, role, state: approved}` — **allowed on your own PR**. `DELETE …/approve` → 204. `POST …/request-changes` → 200 `{approved: false, state: changes_requested}`; `DELETE` → 204. | No self-approval guard is needed. **Approving a merged PR also answers 200**, while unapproving one → 400 `CANNOT_UNAPPROVED_MERGED_PR`. | `pr approve`/`unapprove`/`request-changes` require an open PR and check it themselves. |
| Decline | `POST …/decline` with optional body `{"message": "…"}` → 200 `state: DECLINED`, the message lands in the PR's `reason`. Declining a closed PR → 400 `fields.newstatus = ["This pull request is already closed."]`. | `pr decline --message` maps to `message`. |
| Merge, sync | `POST …/merge` body `{type: "pullrequest", merge_strategy, message, close_source_branch}` → **200** with the merged PR (`state: MERGED`, `merge_commit.hash` 40 chars here but 12 chars on a later `GET`). `close_source_branch: true` deleted the branch. | Always send `close_source_branch` explicitly (`true` only with `-d`): when omitted Bitbucket falls back to the PR's create-time value, which would delete a branch implicitly (spec §9). |
| Merge, async | `?async=true` → **202**, body `""`, `Location: …/merge/task-status/<uuid>`. Polling gives `{task_status: PENDING\|SUCCESS, merge_result: <PR>}`. A merge that fails in the task (conflicts) → task-status **400** with the same error as sync. An **unknown task id → 200 PENDING forever**. | `pr merge` always calls `?async=true` and polls task-status with the 2-minute deadline of spec §7.1 — one code path for fast and slow merges, and a sync 555 timeout leaves the merge state unknown. Never poll without a deadline. |
| Merge errors | Invalid strategy → 400 `fields.merge_strategy`. Already merged/declined → 400 `fields.newstatus` "already closed". Fast-forward not possible → 400 "Unable to fast forward due to changes in the destination branch." Conflicts → 400 "You can't merge until you resolve all merge conflicts." | Exit 1 with the API message; check `mergeability/checks` first to give a better error. |
| Mergeability | `GET …/mergeability/checks` → `{size, values: [{type, status: PASSED\|FAILED, required, blocking, reason, state?}]}`. Types seen: `pullrequest_state_check`, `current_user_permission_check`, `git_mergeability_check` (`reason: clean\|conflicts`, `blocking: true` on conflict). | `pr merge` pre-checks it and fails fast on any `blocking` check. |
| Merge strategies | Not in the default PR payload. `GET …/pullrequests/{id}?fields=%2Bdestination.branch.merge_strategies,%2Bdestination.branch.default_merge_strategy` adds `merge_strategies` (allowed list) and `default_merge_strategy`. The merge body's own default is `merge_commit`, which may differ from the repository's default. `branching-model/settings` needs `repository:admin` (403). `GET …/pullrequests/{id}?fields=destination.branch.*` (the form khbb uses) also returns `destination.branch` with `default_merge_strategy`, `merge_strategies` and `name` (verified sandbox PRs #9 and #10, and earlier on another Khipu repository); without `fields` only `name` comes back. | `pr merge` without a strategy flag sends the PR's `default_merge_strategy`; a flag outside `merge_strategies` fails before the request with the allowed list. |
| HTML error pages | `GET repositories/{ws}/{missing-repo}/commits/<rev>` → **404 `text/html`**, a ~25 KB web page that embeds the caller's profile data and a short-lived web token. Other paths on a missing repo return JSON 404s. | **Never print a non-JSON error body** (Plan 2b, first task): `khbb api` and the error renderer drop `text/html` bodies and print a one-line note instead. |
| Error hint | For field errors the hint repeats the message verbatim. | Cosmetic; drop the hint when it equals the message. |


## Pipelines API (Sources B and C, for Plan 3)

Verified 2026-10-06. Read-only requests against one Khipu repository with ~3,000
pipelines (Source B), and runs/stops in the sandbox `khipu/khipubb-sandbox` with a
minimal `bitbucket-pipelines.yml` (`echo`/`sleep` steps on `alpine`; a branch
pipeline for `khbb-probe/*` with a 60 s step, a tag pipeline, a pull-request
pipeline, and custom pipelines `khbb-vars`, `khbb-manual` and `khbb-fail`)
(Source C). Shapes and status codes only.

| Topic | Finding | Decision |
|---|---|---|
| Addressing | Build numbers work in every path: `GET …/pipelines/{n}`, `…/{n}/steps`, `…/{n}/steps/{step_uuid}/log`, and `POST …/pipelines/{n}/stopPipeline` (204). A pipeline UUID works too. | khbb uses build numbers everywhere; no UUID lookup. |
| List order | `GET …/pipelines` without `sort` returns the **oldest** first (build 1, 2, 3…). `sort=-created_on` returns the newest first. `pagelen` up to 100. | Always send `sort=-created_on`. |
| List filters | Honored: `target.branch` and `target.ref_name` (branch names unencoded or encoded), `creator.uuid`, `trigger_type` (`PUSH`, `MANUAL`), `target.ref_type` (`BRANCH`, `TAG` — **uppercase only**; `branch` → 0 results), `target.selector.type` (`CUSTOM`, `PULLREQUESTS`, `BRANCH`… uppercase only), `target.commit.hash` (full 40-char hash only; a 12-char prefix → 0), `status` (see next row). Combinations AND together. `q=` (BBQL) is **ignored** (returns everything). Unknown enum values return 0 results, not an error. | Build filters from these params only; validate enum values locally. |
| `status` filter | Its own enum: `PARSING`, `PENDING`, `PAUSED`, `HALTED`, `BUILDING`, `ERROR`, `PASSED`, `FAILED`, `STOPPED`, `UNKNOWN`. Repeated `status=` params are ORed (FAILED + STOPPED = sum). Verified live: `BUILDING` = running, `PAUSED` = paused on a manual step, `PASSED` = successful, `FAILED`, `STOPPED`. `SUCCESSFUL` → 0 results (not an error). | `--status` maps: pending → `PENDING`+`PARSING`, running → `BUILDING`, paused → `PAUSED`+`HALTED`, successful → `PASSED`, failed → `FAILED`, error → `ERROR`, stopped → `STOPPED`. |
| Pipeline fields | `build_number, uuid, state, target, trigger{name: PUSH\|MANUAL…}, creator, created_on, completed_on, duration_in_seconds, build_seconds_used, run_number, expired (bool), has_variables, labels, links{self, steps}, repository`. No HTML link. `target` for branches/tags: `{type: pipeline_ref_target, ref_type: branch\|tag, ref_name, commit{hash}, selector{type: branches\|tags\|custom\|default, pattern}}`; for pull requests: `{type: pipeline_pullrequest_target, source, destination, destination_commit{hash}, commit{hash}, pullrequest{id, title, draft…}, selector{type: pull-requests, pattern}}`. | Web URL by convention: `https://bitbucket.org/{ws}/{repo}/pipelines/results/{n}` (not in the API). |
| Pipeline states seen | Created: `state.name = PARSING` (in the `POST` response), then `PENDING` with `stage PENDING`; `IN_PROGRESS` with `stage RUNNING` or `stage PAUSED` (manual step waiting); `COMPLETED` with `result SUCCESSFUL`, `FAILED`, `STOPPED`. `ERROR` and `EXPIRED` results come from the OpenAPI only. `expired: true` on old pipelines is not a state (their logs still download). | Normalized status: parsing/pending → `pending`; IN_PROGRESS+RUNNING (or no stage) → `running`; IN_PROGRESS+PAUSED → `paused`; COMPLETED+result → `successful`, `failed`, `error`, `stopped`, `expired`. |
| Steps | `GET …/{n}/steps` returns every step in one page. Fields: `uuid, name, state, started_on, completed_on, duration_in_seconds, build_seconds_used, run_number, trigger{type: pipeline_step_trigger_automatic\|pipeline_step_trigger_manual}, image, maxTime, script_commands…`. States seen: `PENDING` (with or without `stage PENDING`), `PENDING` + `stage PAUSED` (a manual step waiting to be started), `IN_PROGRESS`, `COMPLETED` with `result SUCCESSFUL`, `FAILED`, `STOPPED`, `NOT_RUN` (after a failure). | Step status: PENDING/READY → `pending`, PENDING+PAUSED → `paused`, IN_PROGRESS → `running`, COMPLETED+result as for pipelines, `NOT_RUN` → `skipped`. |
| Step logs | `GET …/steps/{uuid}/log` with `Accept: application/octet-stream` (or `*/*`) → 200 `application/octet-stream`, served through a redirect to a storage host (HTTP/1.1), `Accept-Ranges: bytes`; `Range: bytes=-N` → 206 with the last N bytes. A step that is running → 200 with the log so far. A step that has not started or did not run → **404** with `detail: "Log in step {uuid} does not exist."`. | `pipeline logs` skips steps without a log with a notice; `--tail` works on whole lines locally. |
| Run | `POST …/pipelines` → **201**, body is the new pipeline (`build_number`, `uuid`, `state PARSING`, `selector` null until parsed), `Location` with UUIDs. Bodies verified: branch `{target: {type: pipeline_ref_target, ref_type: branch, ref_name}}`; tag (`ref_type: tag`); custom (`selector: {type: custom, pattern}`); commit `{type: pipeline_commit_target, commit: {type: commit, hash}, selector: {type: default}\|{type: custom, pattern}}` — a commit target **without** `selector` → 400 "The request body contains invalid properties"; ref + commit + selector (the same commit on a branch); pull-request targets (`pipeline_pullrequest_target` with `source`, `destination`, `destination_commit`, `commit`, `pullrequest.id`, `selector`). | `pipeline run --commit` sends `selector {type: default}` unless `--custom`. |
| Variables | `variables: [{key, value, secured?}]` reach the build (verified in the step log; the secured value was not printed). They are **never returned**: no `variables` field on the pipeline (also not with `fields=+variables`), and `has_variables` stays `false`. | `pipeline rerun` cannot replay variables: it accepts `--var`/`--secret-var` and warns for custom pipelines. |
| Rerun | No rerun endpoint (OpenAPI). Posting the original `target` back (branch/tag: `ref_type`, `ref_name`, `commit.hash`, `selector`; pull request: `source`, `destination`, `destination_commit.hash`, `commit.hash`, `pullrequest.id`, `selector`) starts a new pipeline on the same commit (verified for selectors `default`, `branches`, `custom` and `pull-requests`; a missing tag → 404 `detail: "Could not find last reference for tag <name>"`). | `pipeline rerun` = POST of the original target, reduced to those fields. |
| Run errors | Unknown custom pipeline → 400, `message: "Bad request"`, `detail: "Requested selector is not found in bitbucket-pipelines.yml."`. Unknown branch → **404**, `message: "Not found"`, `detail: "Could not find last reference for branch <name>"`. Unknown build number → 404, `detail: "Pipeline with build number '<n>' not found in repository {uuid}"`. Pipelines errors put the useful text in `detail` and a generic `message`. | Show `detail` as the error message when `message` is generic; no generic 404 hint for pipeline paths. |
| Stop | `POST …/{n}/stopPipeline` → 204; the pipeline ends `COMPLETED/STOPPED` within seconds, the running step `STOPPED`. Stopping a completed pipeline → 400 `detail: "Cannot stop pipeline result that is already complete with status STOPPED"`. Stopping a pipeline **paused on a manual step → 204 but nothing changes** (still `IN_PROGRESS/PAUSED` after several minutes and a second stop). | `pipeline stop` refuses completed pipelines and paused ones (stop those in the web UI) before sending anything. |
| Timing | A one-`echo` pipeline on `alpine` finished in ~6 s; queued `PENDING` up to ~10 s. | Watch interval default 5 s is fine. |
