package merge

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

const (
	taskURL    = "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests/42/merge/task-status/t1"
	taskPath   = prtest.PRs + "/42/merge/task-status/t1"
	strategies = `{"destination":{"branch":{"name":"main","merge_strategies":["merge_commit","squash"],"default_merge_strategy":"squash"}}}`
	clean      = `{"size":1,"values":[{"type":"git_mergeability_check","status":"PASSED","required":true,"blocking":false,"reason":"clean"}]}`
	conflicts  = `{"size":1,"values":[{"type":"git_mergeability_check","status":"FAILED","required":true,"blocking":true,"reason":"conflicts"}]}`
	pending    = `{"task_status":"PENDING","links":{}}`
)

var merged = strings.Replace(prtest.WithState(prtest.PR42, "MERGED"), `"merge_commit":null`, `"merge_commit":{"hash":"fff9999"}`, 1)

// prechecks registers the requests made before the merge itself.
func prechecks(reg *httpmock.Registry, checks string) {
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, strategies))
	reg.Register("GET", prtest.PRs+"/42/mergeability/checks", httpmock.JSONResponse(200, checks))
}

func accepted() httpmock.Responder {
	return httpmock.WithHeader(httpmock.JSONResponse(202, `""`), "Location", taskURL)
}

type result struct {
	out, errOut string
	slept       []time.Duration
	err         error
}

func run(t *testing.T, reg *httpmock.Registry, setup func(*cmdutil.Factory), args ...string) result {
	t.Helper()
	f, _, out, errOut := prtest.NewFactory(reg)
	if setup != nil {
		setup(f)
	}
	clock := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	var slept []time.Duration
	cmd := NewCmdMerge(f, func(o *MergeOptions) error {
		o.Now = func() time.Time { return clock }
		o.Sleep = func(d time.Duration) { slept = append(slept, d); clock = clock.Add(d) }
		return mergeRun(context.Background(), o)
	})
	err := prtest.Run(cmd, append([]string{"42"}, args...)...)
	return result{out.String(), errOut.String(), slept, err}
}

func TestMerge_AsyncWithDefaultStrategy(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)
	reg.Register("POST", prtest.PRs+"/42/merge", accepted())
	reg.Register("GET", taskPath, httpmock.JSONResponse(200, pending))
	reg.Register("GET", taskPath, httpmock.JSONResponse(200, `{"task_status":"SUCCESS","merge_result":`+merged+`}`))

	r := run(t, reg, nil, "--yes")
	if r.err != nil {
		t.Fatal(r.err)
	}
	post := reg.Calls[3]
	if post.URL.Query().Get("async") != "true" {
		t.Errorf("merge URL = %s", post.URL)
	}
	prtest.AssertJSONBody(t, post, `{"type":"pullrequest","merge_strategy":"squash","close_source_branch":false}`)
	want := "note: keeping branch feature/widgets; pass --delete-branch to delete it\n" +
		"Merged pull request #42 (feature/widgets → main) with squash\n"
	if r.out != "" || r.errOut != want || !slices.Equal(r.slept, []time.Duration{2 * time.Second}) {
		t.Errorf("out %q stderr %q slept %v", r.out, r.errOut, r.slept)
	}
}

func TestMerge_FinishedAtOnceWithFlagsAndJSON(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)
	reg.Register("POST", prtest.PRs+"/42/merge", httpmock.JSONResponse(200, merged))

	r := run(t, reg, nil, "--merge", "-d", "-m", "Release widgets", "--yes", "--json", "state,mergeCommit")
	if r.err != nil {
		t.Fatal(r.err)
	}
	prtest.AssertJSONBody(t, reg.Calls[3], `{"type":"pullrequest","merge_strategy":"merge_commit","message":"Release widgets","close_source_branch":true}`)
	if r.out != `{"mergeCommit":"fff9999","state":"MERGED"}`+"\n" {
		t.Errorf("out = %q", r.out)
	}
	if r.errOut != "Merged pull request #42 (feature/widgets → main) with a merge commit and deleted branch feature/widgets\n" {
		t.Errorf("stderr = %q", r.errOut)
	}
}

func TestMerge_StrategyNotAllowed(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.PR42))
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, strategies))

	r := run(t, reg, nil, "--fast-forward", "--yes")
	var flagErr *cmdutil.FlagError
	if !errors.As(r.err, &flagErr) || r.err.Error() != "merge strategy fast_forward is not allowed into main; allowed: merge_commit, squash" {
		t.Errorf("err = %v", r.err)
	}
}

func TestMerge_BlockedByConflicts(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, conflicts)

	r := run(t, reg, nil, "--yes")
	var conflict *cmdutil.ConflictError
	if !errors.As(r.err, &conflict) || r.err.Error() != "pull request #42 cannot be merged: it has merge conflicts" || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", r.err)
	}
}

func TestMerge_RefusesClosedPullRequest(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.PRs+"/42", httpmock.JSONResponse(200, prtest.WithState(prtest.PR42, "MERGED")))

	r := run(t, reg, nil, "--yes")
	var conflict *cmdutil.ConflictError
	if !errors.As(r.err, &conflict) || !strings.Contains(r.err.Error(), "is merged") {
		t.Errorf("err = %v", r.err)
	}
}

func TestMerge_NeedsConfirmationWithoutATerminal(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)

	if r := run(t, reg, nil); !errors.Is(r.err, cmdutil.ErrConfirmationRequired) || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err = %v", r.err)
	}
}

func TestMerge_PromptsOnATerminal(t *testing.T) {
	for _, answer := range []bool{false, true} {
		reg := httpmock.New(t)
		prechecks(reg, clean)
		if answer {
			reg.Register("POST", prtest.PRs+"/42/merge", httpmock.JSONResponse(200, merged))
		}
		r := run(t, reg, func(f *cmdutil.Factory) {
			prtest.SetTTY(f.IOStreams)
			pm := prompter.NewMock(t)
			pm.RegisterConfirm("Merge #42 (feature/widgets → main) with squash and delete the source branch?", func(_ string, def bool) (bool, error) {
				if def {
					t.Error("the default answer must be No")
				}
				return answer, nil
			})
			f.Prompter = pm
		}, "-d")
		if !answer && !errors.Is(r.err, cmdutil.ErrCancel) {
			t.Errorf("declined: err = %v", r.err)
		}
		if answer && r.err != nil {
			t.Errorf("accepted: err = %v", r.err)
		}
	}
}

func TestMerge_DryRun(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)

	r := run(t, reg, nil, "--dry-run")
	if !errors.Is(r.err, bitbucket.ErrDryRun) || !strings.Contains(r.out, `"method": "POST"`) ||
		!strings.Contains(r.out, "/pullrequests/42/merge?async=true") || len(prtest.Writes(reg)) != 0 {
		t.Errorf("err %v out %q", r.err, r.out)
	}
}

func TestMerge_Timeout(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)
	reg.Register("POST", prtest.PRs+"/42/merge", accepted())
	// Polls at 0 s, 2 s, …, 120 s: 61 polls and 60 sleeps before giving up.
	for range 61 {
		reg.Register("GET", taskPath, httpmock.JSONResponse(200, pending))
	}

	r := run(t, reg, nil, "--yes")
	if r.err == nil || r.err.Error() != "the merge of pull request #42 is still running after 2m0s; check "+taskURL || len(r.slept) != 60 {
		t.Errorf("err %v sleeps %d", r.err, len(r.slept))
	}
}

func TestMerge_TransientErrorWhilePolling(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)
	reg.Register("POST", prtest.PRs+"/42/merge", accepted())
	reg.Register("GET", taskPath, httpmock.JSONResponse(503, `{"type":"error","error":{"message":"Service unavailable"}}`))
	reg.Register("GET", taskPath, httpmock.JSONResponse(200, `{"task_status":"SUCCESS","merge_result":`+merged+`}`))

	if r := run(t, reg, nil, "--yes"); r.err != nil || len(r.slept) != 1 {
		t.Errorf("err %v slept %v", r.err, r.slept)
	}
}

func TestMerge_FailedTask(t *testing.T) {
	reg := httpmock.New(t)
	prechecks(reg, clean)
	reg.Register("POST", prtest.PRs+"/42/merge", accepted())
	reg.Register("GET", taskPath, httpmock.JSONResponse(400,
		`{"type":"error","error":{"message":"You can't merge until you resolve all merge conflicts."}}`))

	r := run(t, reg, nil, "--yes")
	var httpErr *bitbucket.HTTPError
	if !errors.As(r.err, &httpErr) || httpErr.StatusCode != 400 {
		t.Errorf("err = %v", r.err)
	}
}

func TestMerge_OneStrategyFlag(t *testing.T) {
	r := run(t, httpmock.New(t), nil, "--merge", "--squash", "--yes")
	var flagErr *cmdutil.FlagError
	if !errors.As(r.err, &flagErr) {
		t.Errorf("err = %v", r.err)
	}
}
