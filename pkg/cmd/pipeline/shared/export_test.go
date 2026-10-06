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

func decodeStep(t *testing.T, s string) *bitbucket.PipelineStep {
	t.Helper()
	var step bitbucket.PipelineStep
	if err := json.Unmarshal([]byte(s), &step); err != nil {
		t.Fatal(err)
	}
	return &step
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
