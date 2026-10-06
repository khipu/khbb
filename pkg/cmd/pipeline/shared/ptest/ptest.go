// Package ptest holds Bitbucket Pipelines fixtures for `khbb pipeline` tests. Every identity is fake.
package ptest

import (
	"fmt"
	"strings"

	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

// Pipelines is the request path of acme/widgets pipelines.
const Pipelines = "/2.0/repositories/acme/widgets/pipelines"

// Commit is the full hash every fixture pipeline runs on.
const Commit = "abc1234def5678abc1234def5678abc1234def56"

// Pipeline and step states, as Bitbucket sends them.
const (
	StatePending    = `{"name":"PENDING","stage":{"name":"PENDING"}}`
	StateRunning    = `{"name":"IN_PROGRESS","stage":{"name":"RUNNING"}}`
	StatePaused     = `{"name":"IN_PROGRESS","stage":{"name":"PAUSED"}}`
	StateSuccessful = `{"name":"COMPLETED","result":{"name":"SUCCESSFUL"}}`
	StateFailed     = `{"name":"COMPLETED","result":{"name":"FAILED"}}`
	StateStopped    = `{"name":"COMPLETED","result":{"name":"STOPPED"}}`
	StepInProgress  = `{"name":"IN_PROGRESS"}`
	StepWaiting     = `{"name":"PENDING","stage":{"name":"PAUSED"}}` // a manual step not started yet
	StepNotRun      = `{"name":"COMPLETED","result":{"name":"NOT_RUN"}}`
)

// Pipeline returns build n on branch main at Commit, pushed by ada, in state.
func Pipeline(n int, state string) string {
	return pipeline(n, state, `{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main",`+
		`"commit":{"type":"commit","hash":"`+Commit+`"},"selector":{"type":"branches","pattern":"main"}}`, "PUSH")
}

// CustomPipeline returns build n of the custom pipeline name on main, started by hand.
func CustomPipeline(n int, state, name string) string {
	return pipeline(n, state, `{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main",`+
		`"commit":{"type":"commit","hash":"`+Commit+`"},"selector":{"type":"custom","pattern":"`+name+`"}}`, "MANUAL")
}

// PullRequestPipeline returns build n of pull request 7 (feature/widgets → main).
func PullRequestPipeline(n int, state string) string {
	return pipeline(n, state, `{"type":"pipeline_pullrequest_target","source":"feature/widgets","destination":"main",`+
		`"destination_commit":{"type":"commit","hash":"def5678"},"commit":{"type":"commit","hash":"`+Commit+`"},`+
		`"pullrequest":{"id":7,"title":"Add widgets","draft":false},"selector":{"type":"pull-requests","pattern":"**"}}`, "PUSH")
}

func pipeline(n int, state, target, trigger string) string {
	completed, duration := "null", 0
	if strings.Contains(state, "COMPLETED") {
		completed, duration = `"2026-10-06T12:01:02.000000+00:00"`, 62
	}
	return fmt.Sprintf(`{"type":"pipeline","uuid":"{00000000-0000-0000-0000-%012d}","build_number":%d,"state":%s,"target":%s,`+
		`"trigger":{"name":"%s"},"creator":%s,"created_on":"2026-10-06T12:00:00.000000+00:00","completed_on":%s,"duration_in_seconds":%d}`,
		n, n, state, target, trigger, prtest.Ada, completed, duration)
}

// Step returns a step with the given UUID, name and state. Steps that ran took 12 s. A NOT_RUN step
// gets completed_on set but no started_on, matching what Bitbucket sends for a skipped step.
func Step(uuid, name, state string) string {
	notRun := strings.Contains(state, "NOT_RUN")
	ran := !strings.Contains(state, `"PENDING"`) && !notRun
	started, completed, duration := "null", "null", 0
	if ran {
		started = `"2026-10-06T12:00:05.000000+00:00"`
	}
	if ran && strings.Contains(state, "COMPLETED") {
		completed, duration = `"2026-10-06T12:00:17.000000+00:00"`, 12
	}
	if notRun {
		completed = `"2026-10-06T12:00:17.000000+00:00"`
	}
	return fmt.Sprintf(`{"uuid":"%s","name":"%s","state":%s,"started_on":%s,"completed_on":%s,"duration_in_seconds":%d,`+
		`"trigger":{"type":"pipeline_step_trigger_automatic"}}`, uuid, name, state, started, completed, duration)
}

// Steps wraps step fixtures in a one-page collection.
func Steps(steps ...string) string {
	return prtest.Page(steps...)
}
