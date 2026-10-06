package view

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const pipelineURL = "https://bitbucket.org/acme/widgets/pipelines/results/42"

type fakeBrowser struct{ urls []string }

func (b *fakeBrowser) Browse(u string) error {
	b.urls = append(b.urls, u)
	return nil
}

func failedPipeline(reg *httpmock.Registry) {
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, ptest.StateFailed)))
	reg.Register("GET", ptest.Pipelines+"/42/steps", httpmock.JSONResponse(200, ptest.Steps(
		ptest.Step("{s1}", "Build", ptest.StateSuccessful), ptest.Step("{s2}", "Test", ptest.StateFailed))))
}

func run(reg *httpmock.Registry, tty bool, args ...string) (string, string, error) {
	f, ios, out, errOut := prtest.NewFactory(reg)
	ios.SetStdoutTTY(tty)
	err := prtest.Run(NewCmdView(f, nil), args...)
	return out.String(), errOut.String(), err
}

func TestView_TTY(t *testing.T) {
	reg := httpmock.New(t)
	failedPipeline(reg)
	out, _, err := run(reg, true, "42")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Pipeline #42 X failed", "branch main · push by ada · commit abc1234", "Build", "✓ successful", "Test", "X failed", "12s"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestView_NonTTY(t *testing.T) {
	reg := httpmock.New(t)
	failedPipeline(reg)
	out, _, err := run(reg, false, "#42")
	want := "number:\t42\nstatus:\tfailed\nrun on:\tbranch main\ntrigger:\tpush\ncreator:\tada\ncommit:\t" + ptest.Commit +
		"\nduration:\t62\nurl:\t" + pipelineURL + "\n--\nBuild\tsuccessful\t12\nTest\tfailed\t12\n"
	if err != nil || out != want {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestView_Verbose(t *testing.T) {
	reg := httpmock.New(t)
	failedPipeline(reg)
	out, _, err := run(reg, false, "42", "-v")
	if err != nil || !strings.Contains(out, "Build\tsuccessful\t12\t{s1}\t2026-10-06T12:00:05Z\t2026-10-06T12:00:17Z\n") {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestView_JSONWithSteps(t *testing.T) {
	reg := httpmock.New(t)
	failedPipeline(reg)
	out, _, err := run(reg, false, "42", "--json", "number,status,steps")
	want := `{"number":42,"status":"failed","steps":[` +
		`{"completedOn":"2026-10-06T12:00:17Z","durationSeconds":12,"name":"Build","startedOn":"2026-10-06T12:00:05Z","status":"successful","uuid":"{s1}"},` +
		`{"completedOn":"2026-10-06T12:00:17Z","durationSeconds":12,"name":"Test","startedOn":"2026-10-06T12:00:05Z","status":"failed","uuid":"{s2}"}]}` + "\n"
	if err != nil || out != want {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestView_NewestOfTheCurrentBranch(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page(ptest.Pipeline(43, ptest.StateRunning))))
	reg.Register("GET", ptest.Pipelines+"/43/steps", httpmock.JSONResponse(200, ptest.Steps(ptest.Step("{s1}", "Build", ptest.StepInProgress))))
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")

	if err := prtest.Run(NewCmdView(f, nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "number:\t43\nstatus:\trunning\n") || reg.Calls[0].URL.Query().Get("target.branch") != "feature/widgets" {
		t.Errorf("out %q", out.String())
	}
}

func TestView_Web(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines+"/42", httpmock.JSONResponse(200, ptest.Pipeline(42, ptest.StateFailed)))
	f, ios, out, errOut := prtest.NewFactory(reg)
	ios.SetStderrTTY(true)
	b := &fakeBrowser{}
	f.Browser = b

	if err := prtest.Run(NewCmdView(f, nil), "42", "--web"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.urls, []string{pipelineURL}) || out.Len() != 0 || errOut.String() != "Opening "+pipelineURL+" in your browser.\n" {
		t.Errorf("urls %v out %q stderr %q", b.urls, out.String(), errOut.String())
	}
}

func TestView_FlagErrors(t *testing.T) {
	for _, args := range [][]string{{"42", "--web", "--json", "number"}, {"1", "2"}, {"abc"}} {
		f, _, _, _ := prtest.NewFactory(httpmock.New(t))
		f.Browser = &fakeBrowser{}
		err := prtest.Run(NewCmdView(f, nil), args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
