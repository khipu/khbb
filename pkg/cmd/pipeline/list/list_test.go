package list

import (
	"errors"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared/ptest"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func run(reg *httpmock.Registry, tty bool, args ...string) (string, string, error) {
	f, ios, out, errOut := prtest.NewFactory(reg)
	ios.SetStdoutTTY(tty)
	err := prtest.Run(NewCmdList(f, nil), args...)
	return out.String(), errOut.String(), err
}

func twoPipelines(reg *httpmock.Registry) {
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200,
		prtest.Page(ptest.Pipeline(43, ptest.StateRunning), ptest.Pipeline(42, ptest.StateFailed))))
}

func TestList_TTYTable(t *testing.T) {
	reg := httpmock.New(t)
	twoPipelines(reg)
	out, _, err := run(reg, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"STATUS", "RUN ON", "* running", "#43", "branch main", "push", "X failed", "#42", "1m02s", "2026-10-06 12:00"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestList_NonTTYIsTabSeparated(t *testing.T) {
	reg := httpmock.New(t)
	twoPipelines(reg)
	out, _, err := run(reg, false)
	want := "43\trunning\tbranch\tmain\tpush\t0\t2026-10-06T12:00:00Z\n" +
		"42\tfailed\tbranch\tmain\tpush\t62\t2026-10-06T12:00:00Z\n"
	if err != nil || out != want {
		t.Errorf("out %q err %v", out, err)
	}
	if q := reg.Calls[0].URL.Query(); q.Get("sort") != "-created_on" || q.Get("pagelen") != "20" || q.Has("status") || q.Has("target.branch") {
		t.Errorf("query = %v", q)
	}
}

func TestList_Filters(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))
	if _, _, err := run(reg, false, "-b", "feature/widgets", "-s", "Successful", "--mine", "-L", "5"); err != nil {
		t.Fatal(err)
	}
	q := reg.Calls[1].URL.Query()
	if q.Get("target.branch") != "feature/widgets" || q.Get("status") != "PASSED" || q.Get("creator.uuid") != prtest.AdaUUID || q.Get("pagelen") != "5" {
		t.Errorf("query = %v", q)
	}
}

func TestList_PendingIsTwoFilterValues(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))
	if _, _, err := run(reg, false, "--status", "pending"); err != nil {
		t.Fatal(err)
	}
	if got := reg.Calls[0].URL.Query()["status"]; len(got) != 2 || got[0] != "PENDING" || got[1] != "PARSING" {
		t.Errorf("status = %v", got)
	}
}

func TestList_JSON(t *testing.T) {
	reg := httpmock.New(t)
	twoPipelines(reg)
	out, _, err := run(reg, false, "--json", "number,status,url")
	want := `[{"number":43,"status":"running","url":"https://bitbucket.org/acme/widgets/pipelines/results/43"},` +
		`{"number":42,"status":"failed","url":"https://bitbucket.org/acme/widgets/pipelines/results/42"}]` + "\n"
	if err != nil || out != want {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestList_EmptyResults(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))
	reg.Register("GET", ptest.Pipelines, httpmock.JSONResponse(200, prtest.Page()))
	out, errOut, err := run(reg, true)
	if err != nil || out != "" || errOut != "No pipelines match your filters in acme/widgets\n" {
		t.Errorf("tty: out %q stderr %q err %v", out, errOut, err)
	}
	out, errOut, err = run(reg, false, "--json", "number")
	if err != nil || out != "[]\n" || errOut != "" {
		t.Errorf("json: out %q stderr %q err %v", out, errOut, err)
	}
}

func TestList_FlagErrors(t *testing.T) {
	for _, args := range [][]string{{"-s", "done"}, {"-L", "0"}, {"extra"}} {
		_, _, err := run(httpmock.New(t), false, args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
