# Bitbucket Cloud API findings

Resolves the "items to verify" list (§14) of
[`2026-10-05-khbb-cli-design.md`](2026-10-05-khbb-cli-design.md).

- **Source A:** Bitbucket Cloud OpenAPI document,
  `https://dac-static.atlassian.com/cloud/bitbucket/swagger.v3.json`
  (OpenAPI 3.0.0, API 2.0), downloaded 2026-10-05.
- **Source B:** live, read-only requests with `khbb api` (Plan 1, Task 11).
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
  `squash_fast_forward`, `rebase_fast_forward`, `rebase_merge`. v1 exposes
  `--merge`, `--squash`, `--fast-forward`; Plan 2 adds `--strategy <name>` for the
  other three.
- **Commit status** `state`: `SUCCESSFUL`, `FAILED`, `INPROGRESS`, `STOPPED`.
- **Participant** `state`: `approved`, `changes_requested`, `null` (→ khbb `pending`).

## Still open

| # | Question | How to resolve | When |
|---|---|---|---|
| 1 | Does `POST /pullrequests` add default reviewers automatically? | Create a PR in the sandbox repo without `reviewers`; inspect the response. | Plan 2 (Source C) |
| 5 | Do pipeline endpoints accept a build number in place of `{pipeline_uuid}`? | `khbb api 'repositories/<ws>/<repo>/pipelines/<build_number>'` | Plan 1, Task 11 |
| 9 | Does the API expose a token's granted scopes? | `khbb api user -i --silent`; look for scope-related response headers. | Plan 1, Task 11 |
| 10 | Bearer vs Basic | Basic (`email:token`) is documented and sufficient. | Closed: keep Basic, no verification needed |
| — | Maximum `pagelen` per endpoint | `khbb api '…/pullrequests?pagelen=100'` and `…/pipelines?pagelen=100` | Plan 1, Task 11 |
