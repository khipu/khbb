package status

import (
	"encoding/json"
	"testing"

	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func registerLists(reg *httpmock.Registry, pages ...string) {
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))
	for _, p := range pages {
		reg.Register("GET", prtest.PRs, httpmock.JSONResponse(200, p))
	}
}

func TestStatus_HumanOutput(t *testing.T) {
	reg := httpmock.New(t)
	registerLists(reg, prtest.Page(prtest.PR42), prtest.Page(prtest.PR42), prtest.Page(prtest.PR9, prtest.PR42))
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")

	if err := prtest.Run(NewCmdStatus(f, nil)); err != nil {
		t.Fatal(err)
	}
	want := "Relevant pull requests in acme/widgets\n\n" +
		"Current branch\n  #42  Add widgets [feature/widgets]  1/2 approved\n\n" +
		"Created by you\n  #42  Add widgets [feature/widgets]  1/2 approved\n\n" +
		"Requesting a code review from you\n  #9  Update docs [feature/docs]  1/2 approved\n"
	if out.String() != want {
		t.Errorf("out = %q\nwant %q", out.String(), want)
	}
	wantQueries := []string{
		`state = "OPEN" AND (source.branch.name = "feature/widgets")`,
		`state = "OPEN" AND (author.uuid = "` + prtest.AdaUUID + `")`,
		`state = "OPEN" AND (reviewers.uuid = "` + prtest.AdaUUID + `")`,
	}
	for i, want := range wantQueries {
		q := reg.Calls[i+1].URL.Query()
		if q.Get("q") != want || q.Get("state") != "" || q.Get("fields") == "" {
			t.Errorf("call %d query = %v", i+1, q)
		}
	}
}

func TestStatus_NotOnBranchAndNothingToShow(t *testing.T) {
	reg := httpmock.New(t)
	registerLists(reg, prtest.Page(), prtest.Page())
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "")

	if err := prtest.Run(NewCmdStatus(f, nil)); err != nil {
		t.Fatal(err)
	}
	want := "Relevant pull requests in acme/widgets\n\n" +
		"Current branch\n  Not on a branch\n\n" +
		"Created by you\n  You have no open pull requests\n\n" +
		"Requesting a code review from you\n  You have no pull requests to review\n"
	if out.String() != want {
		t.Errorf("out = %q\nwant %q", out.String(), want)
	}
}

func TestStatus_JSON(t *testing.T) {
	reg := httpmock.New(t)
	registerLists(reg, prtest.Page(prtest.PR42), prtest.Page(prtest.PR42), prtest.Page(prtest.PR9, prtest.PR42))
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "feature/widgets")

	if err := prtest.Run(NewCmdStatus(f, nil), "--json", "currentBranch,createdByMe,needsMyReview"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		CurrentBranch *struct{ ID int }  `json:"currentBranch"`
		CreatedByMe   []struct{ ID int } `json:"createdByMe"`
		NeedsMyReview []struct{ ID int } `json:"needsMyReview"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if got.CurrentBranch == nil || got.CurrentBranch.ID != 42 || len(got.CreatedByMe) != 1 ||
		len(got.NeedsMyReview) != 1 || got.NeedsMyReview[0].ID != 9 {
		t.Errorf("got %+v", got)
	}
}

func TestStatus_JSONEmptyArrays(t *testing.T) {
	reg := httpmock.New(t)
	registerLists(reg, prtest.Page(), prtest.Page())
	f, _, out, _ := prtest.NewFactory(reg)
	prtest.SetBranch(f, "")

	if err := prtest.Run(NewCmdStatus(f, nil), "--json", "currentBranch,createdByMe,needsMyReview"); err != nil {
		t.Fatal(err)
	}
	if want := `{"createdByMe":[],"currentBranch":null,"needsMyReview":[]}` + "\n"; out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
}
