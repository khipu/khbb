package shared_test

import (
	"testing"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
)

func TestStarted(t *testing.T) {
	repo := gitctx.Repo{Workspace: "acme", Slug: "widgets"}
	requestedSel := &bitbucket.PipelineSelector{Type: "custom", Pattern: "deploy"}

	// A raw pipeline with nil selector gets the requested one
	now := time.Now()
	raw := &bitbucket.Pipeline{
		UUID:        "{00000000-0000-0000-0000-000000000042}",
		BuildNumber: 42,
		State: bitbucket.PipelineState{
			Name: "COMPLETED",
			Result: &bitbucket.Named{
				Name: "SUCCESSFUL",
			},
		},
		Target: bitbucket.PipelineTarget{
			Type:     "pipeline_ref_target",
			RefType:  "branch",
			RefName:  "main",
			Selector: nil, // Will be filled in by Started
		},
		Trigger: bitbucket.Named{
			Name: "MANUAL",
		},
		Creator: &bitbucket.User{
			DisplayName: "Test User",
		},
		CreatedOn: now,
	}
	p := shared.Started(raw, requestedSel, repo)
	if p.Selector.Type != "custom" || p.Selector.Pattern != "deploy" {
		t.Errorf("selector = %+v", p.Selector)
	}
	refLabel := shared.RefLabel(p)
	if refLabel != "custom pipeline deploy on branch main" {
		t.Errorf("RefLabel = %q", refLabel)
	}

	// A raw pipeline that already has a selector keeps it
	raw2 := &bitbucket.Pipeline{
		UUID:        "{00000000-0000-0000-0000-000000000043}",
		BuildNumber: 43,
		State: bitbucket.PipelineState{
			Name: "COMPLETED",
			Result: &bitbucket.Named{
				Name: "SUCCESSFUL",
			},
		},
		Target: bitbucket.PipelineTarget{
			Type:     "pipeline_ref_target",
			RefType:  "branch",
			RefName:  "main",
			Selector: &bitbucket.PipelineSelector{Type: "branches", Pattern: "main"},
		},
		Trigger: bitbucket.Named{
			Name: "MANUAL",
		},
		Creator: &bitbucket.User{
			DisplayName: "Test User",
		},
		CreatedOn: now,
	}
	p2 := shared.Started(raw2, requestedSel, repo)
	if p2.Selector.Type != "branches" || p2.Selector.Pattern != "main" {
		t.Errorf("selector = %+v, want branches main", p2.Selector)
	}
}
