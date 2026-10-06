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
