package shared_test

import (
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func TestReviewSummaryAndApprovals(t *testing.T) {
	pr := shared.NewPullRequest(decode[bitbucket.PullRequest](t, prtest.PR42))
	if got := shared.ReviewSummary(pr); got != "bob (approved), cy (pending)" {
		t.Errorf("ReviewSummary = %q", got)
	}
	if a, n, changes := shared.ApprovalCount(pr); a != 1 || n != 2 || changes {
		t.Errorf("ApprovalCount = %d %d %v", a, n, changes)
	}
	pr.Reviewers[1].State = "changes_requested"
	if got := shared.ReviewSummary(pr); got != "bob (approved), cy (changes requested)" {
		t.Errorf("ReviewSummary = %q", got)
	}
	if _, _, changes := shared.ApprovalCount(pr); !changes {
		t.Error("changes requested not reported")
	}
	if got := shared.ReviewSummary(shared.PullRequest{}); got != "none" {
		t.Errorf("empty ReviewSummary = %q", got)
	}
}

func TestAuthorAndStateLabel(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	pr := shared.NewPullRequest(decode[bitbucket.PullRequest](t, prtest.PR42))
	if shared.AuthorName(pr) != "ada" || shared.StateLabel(ios, pr) != "OPEN" {
		t.Errorf("author %q state %q", shared.AuthorName(pr), shared.StateLabel(ios, pr))
	}
	pr.Author = nil
	pr.Draft = true
	if shared.AuthorName(pr) != "unknown" || shared.StateLabel(ios, pr) != "DRAFT" {
		t.Errorf("author %q state %q", shared.AuthorName(pr), shared.StateLabel(ios, pr))
	}
}
