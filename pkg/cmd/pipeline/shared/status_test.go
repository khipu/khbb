package shared_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
)

func state(name, stage, result string) bitbucket.PipelineState {
	s := bitbucket.PipelineState{Name: name}
	if stage != "" {
		s.Stage = &bitbucket.Named{Name: stage}
	}
	if result != "" {
		s.Result = &bitbucket.Named{Name: result}
	}
	return s
}

func TestStatus(t *testing.T) {
	cases := []struct{ name, stage, result, want string }{
		{"PARSING", "", "", "pending"},
		{"PENDING", "PENDING", "", "pending"},
		{"READY", "", "", "pending"},
		{"PENDING", "PAUSED", "", "paused"},
		{"IN_PROGRESS", "RUNNING", "", "running"},
		{"IN_PROGRESS", "", "", "running"},
		{"IN_PROGRESS", "PAUSED", "", "paused"},
		{"COMPLETED", "", "SUCCESSFUL", "successful"},
		{"COMPLETED", "", "FAILED", "failed"},
		{"COMPLETED", "", "ERROR", "error"},
		{"COMPLETED", "", "STOPPED", "stopped"},
		{"COMPLETED", "", "EXPIRED", "expired"},
		{"COMPLETED", "", "NOT_RUN", "skipped"},
		{"COMPLETED", "", "SOMETHING_NEW", "something_new"},
		{"HALTED", "", "", "halted"},
	}
	for _, tc := range cases {
		if got := shared.Status(state(tc.name, tc.stage, tc.result)); got != tc.want {
			t.Errorf("Status(%s/%s/%s) = %q, want %q", tc.name, tc.stage, tc.result, got, tc.want)
		}
	}
}

func TestFinishedAndUnsuccessful(t *testing.T) {
	cases := []struct {
		status                 string
		finished, unsuccessful bool
	}{
		{"pending", false, false}, {"running", false, false}, {"paused", false, false},
		{"successful", true, false}, {"skipped", true, false},
		{"failed", true, true}, {"error", true, true}, {"stopped", true, true}, {"expired", true, true},
	}
	for _, tc := range cases {
		if shared.Finished(tc.status) != tc.finished || shared.Unsuccessful(tc.status) != tc.unsuccessful {
			t.Errorf("%s: Finished %v Unsuccessful %v", tc.status, shared.Finished(tc.status), shared.Unsuccessful(tc.status))
		}
	}
}

func TestListStatuses(t *testing.T) {
	want := []string{"error", "failed", "paused", "pending", "running", "stopped", "successful"}
	if got := slices.Sorted(maps.Keys(shared.ListStatuses)); !slices.Equal(got, want) {
		t.Errorf("keys = %v", got)
	}
	if !slices.Equal(shared.ListStatuses["successful"], []string{"PASSED"}) || !slices.Equal(shared.ListStatuses["running"], []string{"BUILDING"}) ||
		!slices.Equal(shared.ListStatuses["pending"], []string{"PENDING", "PARSING"}) || !slices.Equal(shared.ListStatuses["paused"], []string{"PAUSED", "HALTED"}) {
		t.Errorf("ListStatuses = %v", shared.ListStatuses)
	}
}
