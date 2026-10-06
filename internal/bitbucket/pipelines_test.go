package bitbucket_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

const pipelinesPath = "/2.0/repositories/acme/widgets/pipelines"

const pipelineJSON = `{"type":"pipeline","uuid":"{00000000-0000-0000-0000-000000000042}","build_number":42,` +
	`"state":{"name":"COMPLETED","type":"pipeline_state_completed","result":{"name":"FAILED","type":"pipeline_state_completed_failed"}},` +
	`"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main",` +
	`"commit":{"type":"commit","hash":"abc1234def5678abc1234def5678abc1234def56","links":{}},"selector":{"type":"branches","pattern":"main"}},` +
	`"trigger":{"name":"PUSH","type":"pipeline_trigger_push"},"creator":` + userJSON + `,` +
	`"created_on":"2026-10-06T12:00:00.000000+00:00","completed_on":"2026-10-06T12:01:02.000000+00:00","duration_in_seconds":62}`

func TestListPipelines(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", pipelinesPath, httpmock.JSONResponse(200, `{"values":[`+pipelineJSON+`]}`))

	ps, err := c.ListPipelines(context.Background(), "acme", "widgets", bitbucket.PipelineListOptions{
		Branch: "feature/widgets", Statuses: []string{"FAILED", "STOPPED"}, CreatorUUID: "{a}", CommitHash: "abc",
	}, 20)
	if err != nil || len(ps) != 1 {
		t.Fatalf("pipelines %v err %v", ps, err)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("sort") != "-created_on" || q.Get("target.branch") != "feature/widgets" || !slices.Equal(q["status"], []string{"FAILED", "STOPPED"}) ||
		q.Get("creator.uuid") != "{a}" || q.Get("target.commit.hash") != "abc" || q.Get("pagelen") != "20" {
		t.Errorf("query = %v", q)
	}
	p := ps[0]
	if p.BuildNumber != 42 || p.State.Name != "COMPLETED" || p.State.Result.Name != "FAILED" || p.State.Stage != nil ||
		p.Target.RefName != "main" || p.Target.Commit.Hash[:7] != "abc1234" || p.Target.Selector.Type != "branches" ||
		p.Trigger.Name != "PUSH" || p.Creator.Nickname != "ada" || p.CompletedOn == nil || p.DurationInSeconds != 62 {
		t.Errorf("pipeline = %+v", p)
	}
}

func TestListPipelines_NoFilters(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", pipelinesPath, httpmock.JSONResponse(200, `{"values":[]}`))

	if _, err := c.ListPipelines(context.Background(), "acme", "widgets", bitbucket.PipelineListOptions{}, 5); err != nil {
		t.Fatal(err)
	}
	q := reg.Calls[0].URL.Query()
	if q.Get("sort") != "-created_on" || q.Has("status") || q.Has("target.branch") || q.Has("creator.uuid") || q.Has("target.commit.hash") {
		t.Errorf("query = %v", q)
	}
}

func TestGetPipelineAndSteps(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", pipelinesPath+"/42", httpmock.JSONResponse(200, pipelineJSON))
	reg.Register("GET", pipelinesPath+"/42/steps", httpmock.JSONResponse(200, `{"values":[`+
		`{"uuid":"{s1}","name":"Build","state":{"name":"COMPLETED","result":{"name":"SUCCESSFUL"}},"started_on":"2026-10-06T12:00:05.000000+00:00","completed_on":"2026-10-06T12:00:17.000000+00:00","duration_in_seconds":12,"trigger":{"type":"pipeline_step_trigger_automatic"}},`+
		`{"uuid":"{s2}","name":"Deploy","state":{"name":"PENDING","stage":{"name":"PAUSED"}},"started_on":null,"completed_on":null,"trigger":{"type":"pipeline_step_trigger_manual"}}]}`))

	p, err := c.GetPipeline(context.Background(), "acme", "widgets", 42)
	if err != nil || p.UUID != "{00000000-0000-0000-0000-000000000042}" {
		t.Fatalf("pipeline %+v err %v", p, err)
	}
	steps, err := c.ListPipelineSteps(context.Background(), "acme", "widgets", 42)
	if err != nil || len(steps) != 2 {
		t.Fatalf("steps %+v err %v", steps, err)
	}
	if steps[0].Name != "Build" || steps[0].DurationInSeconds != 12 || steps[0].StartedOn == nil ||
		steps[1].State.Stage.Name != "PAUSED" || steps[1].StartedOn != nil || steps[1].Trigger.Type != "pipeline_step_trigger_manual" {
		t.Errorf("steps = %+v", steps)
	}
}

func TestStepLog(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", pipelinesPath+"/42/steps/{s1}/log", httpmock.StringResponse(200, "line 1\nline 2\n"))
	reg.Register("GET", pipelinesPath+"/42/steps/{s2}/log", httpmock.JSONResponse(404,
		`{"error":{"message":"Not Found","detail":"Log in step {s2} does not exist."}}`))

	log, err := c.StepLog(context.Background(), "acme", "widgets", 42, "{s1}")
	if err != nil || log != "line 1\nline 2\n" {
		t.Errorf("log %q err %v", log, err)
	}
	if reg.Calls[0].Header.Get("Accept") != "*/*" {
		t.Errorf("Accept = %q", reg.Calls[0].Header.Get("Accept"))
	}
	_, err = c.StepLog(context.Background(), "acme", "widgets", 42, "{s2}")
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 {
		t.Errorf("err = %v", err)
	}
}

func TestRunPipeline(t *testing.T) {
	cases := []struct {
		name   string
		target bitbucket.PipelineTarget
		vars   []bitbucket.PipelineVariable
		want   string
	}{
		{"branch", bitbucket.PipelineTarget{Type: "pipeline_ref_target", RefType: "branch", RefName: "main"}, nil,
			`{"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main"}}`},
		{"custom with variables", bitbucket.PipelineTarget{Type: "pipeline_ref_target", RefType: "branch", RefName: "main",
			Selector: &bitbucket.PipelineSelector{Type: "custom", Pattern: "deploy"}},
			[]bitbucket.PipelineVariable{{Key: "ENV", Value: "staging"}, {Key: "TOKEN", Value: "s3cret", Secured: true}},
			`{"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main","selector":{"type":"custom","pattern":"deploy"}},` +
				`"variables":[{"key":"ENV","value":"staging"},{"key":"TOKEN","value":"s3cret","secured":true}]}`},
		{"commit", bitbucket.PipelineTarget{Type: "pipeline_commit_target", Commit: &bitbucket.Commit{Hash: "abc"},
			Selector: &bitbucket.PipelineSelector{Type: "default"}}, nil,
			`{"target":{"type":"pipeline_commit_target","commit":{"type":"commit","hash":"abc"},"selector":{"type":"default"}}}`},
		{"pull request (rerun)", bitbucket.PipelineTarget{Type: "pipeline_pullrequest_target", Source: "feature/widgets", Destination: "main",
			DestinationCommit: &bitbucket.Commit{Hash: "def"}, Commit: &bitbucket.Commit{Hash: "abc"},
			PullRequest: &bitbucket.PipelinePullRequest{ID: 7}, Selector: &bitbucket.PipelineSelector{Type: "pull-requests", Pattern: "**"}}, nil,
			`{"target":{"type":"pipeline_pullrequest_target","source":"feature/widgets","destination":"main","destination_commit":{"hash":"def"},` +
				`"commit":{"type":"commit","hash":"abc"},"pullrequest":{"id":7},"selector":{"type":"pull-requests","pattern":"**"}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, reg := newTestClient(t, bitbucket.Options{})
			reg.Register("POST", pipelinesPath, httpmock.JSONResponse(201, pipelineJSON))
			p, err := c.RunPipeline(context.Background(), "acme", "widgets", tc.target, tc.vars)
			if err != nil || p.BuildNumber != 42 {
				t.Fatalf("pipeline %+v err %v", p, err)
			}
			assertBody(t, reg.Calls[0], tc.want)
		})
	}
}

func TestRunPipeline_TargetReadBackIsAccepted(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", pipelinesPath+"/42", httpmock.JSONResponse(200, pipelineJSON))
	reg.Register("POST", pipelinesPath, httpmock.JSONResponse(201, pipelineJSON))

	p, err := c.GetPipeline(context.Background(), "acme", "widgets", 42)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.RunPipeline(context.Background(), "acme", "widgets", p.Target, nil); err != nil {
		t.Fatal(err)
	}
	assertBody(t, reg.Calls[1], `{"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main",`+
		`"commit":{"type":"commit","hash":"abc1234def5678abc1234def5678abc1234def56"},"selector":{"type":"branches","pattern":"main"}}}`)
}

func TestStopPipeline(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("POST", pipelinesPath+"/42/stopPipeline", httpmock.StringResponse(204, ""))

	if err := c.StopPipeline(context.Background(), "acme", "widgets", 42); err != nil {
		t.Fatal(err)
	}
	if len(reg.Calls[0].Body) != 0 {
		t.Errorf("body = %q", reg.Calls[0].Body)
	}
}
