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
