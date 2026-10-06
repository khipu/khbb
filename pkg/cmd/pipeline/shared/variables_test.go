package shared_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
)

func TestParseVariables(t *testing.T) {
	vars, err := shared.ParseVariables([]string{"ENV=staging", "EMPTY=", "URL=https://example.com/?a=b"}, []string{"TOKEN=s3cret"})
	want := []bitbucket.PipelineVariable{
		{Key: "ENV", Value: "staging"}, {Key: "EMPTY", Value: ""}, {Key: "URL", Value: "https://example.com/?a=b"},
		{Key: "TOKEN", Value: "s3cret", Secured: true},
	}
	if err != nil || len(vars) != len(want) {
		t.Fatalf("vars %v err %v", vars, err)
	}
	for i := range want {
		if vars[i] != want[i] {
			t.Errorf("vars[%d] = %+v, want %+v", i, vars[i], want[i])
		}
	}
	var flagErr *cmdutil.FlagError
	if _, err := shared.ParseVariables([]string{"novalue"}, nil); !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "novalue") {
		t.Errorf("--var without '=': err = %v", err)
	}
	_, err = shared.ParseVariables(nil, []string{"ok=1", "pasted-s3cret-without-key"})
	if !errors.As(err, &flagErr) || strings.Contains(err.Error(), "s3cret") || !strings.Contains(err.Error(), "--secret-var number 2") {
		t.Errorf("a bad --secret-var must not be echoed: err = %v", err)
	}
	if vars, err := shared.ParseVariables(nil, nil); err != nil || vars == nil || len(vars) != 0 {
		t.Errorf("no variables: %#v %v", vars, err)
	}
}
