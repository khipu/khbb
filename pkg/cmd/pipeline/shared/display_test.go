package shared_test

import (
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
)

func TestFormatDuration(t *testing.T) {
	for seconds, want := range map[int]string{0: "0s", 45: "45s", 60: "1m00s", 185: "3m05s", 3725: "1h02m"} {
		if got := shared.FormatDuration(seconds); got != want {
			t.Errorf("FormatDuration(%d) = %q, want %q", seconds, got, want)
		}
	}
}

func TestRefLabel(t *testing.T) {
	cases := []struct {
		p    shared.Pipeline
		want string
	}{
		{shared.Pipeline{RefType: "branch", RefName: "main"}, "branch main"},
		{shared.Pipeline{RefType: "tag", RefName: "v1.2.0"}, "tag v1.2.0"},
		{shared.Pipeline{RefType: "pullrequest", RefName: "feature/widgets"}, "pull request from feature/widgets"},
		{shared.Pipeline{RefType: "commit", Commit: "abc1234def"}, "commit abc1234"},
		{shared.Pipeline{RefType: "branch", RefName: "main", Selector: shared.Selector{Type: "custom", Pattern: "deploy"}}, "custom pipeline deploy on branch main"},
	}
	for _, tc := range cases {
		if got := shared.RefLabel(tc.p); got != tc.want {
			t.Errorf("RefLabel(%+v) = %q, want %q", tc.p, got, tc.want)
		}
	}
}

func TestStatusLabel(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	for status, want := range map[string]string{"successful": "✓ successful", "failed": "X failed", "error": "X error",
		"stopped": "- stopped", "skipped": "- skipped", "paused": "‖ paused", "running": "* running", "pending": "* pending"} {
		if got := shared.StatusLabel(ios, status); got != want {
			t.Errorf("StatusLabel(%s) = %q, want %q", status, got, want)
		}
	}
}

func TestRenderSummary(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	p := shared.NewPipeline(decodePipeline(t, ptest.Pipeline(42, ptest.StateFailed)), repo)
	steps := []shared.Step{
		shared.NewStep(decodeStep(t, ptest.Step("{s1}", "Build", ptest.StateSuccessful))),
		shared.NewStep(decodeStep(t, ptest.Step("{s2}", "Deploy", ptest.StepNotRun))),
	}
	if err := shared.RenderSummary(ios, p, steps, true); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Pipeline #42 X failed\n", "branch main · push by ada · commit abc1234\n",
		"Started 2026-10-06 12:00 UTC · took 1m02s\n", "STEP", "Build", "✓ successful", "12s", "{s1}", "Deploy", "- skipped"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
}
