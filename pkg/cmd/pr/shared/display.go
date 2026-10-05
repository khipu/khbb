package shared

import (
	"fmt"
	"strings"

	"github.com/khipu/khbb/internal/iostreams"
)

// StateColor returns the color for a pull request's state: green open, gray draft, cyan merged, red otherwise.
func StateColor(ios *iostreams.IOStreams, pr PullRequest) func(string) string {
	switch {
	case pr.State == "OPEN" && pr.Draft:
		return ios.Gray
	case pr.State == "OPEN":
		return ios.Green
	case pr.State == "MERGED":
		return ios.Cyan
	}
	return ios.Red
}

// StateLabel renders the state for humans; open drafts read DRAFT.
func StateLabel(ios *iostreams.IOStreams, pr PullRequest) string {
	label := pr.State
	if pr.State == "OPEN" && pr.Draft {
		label = "DRAFT"
	}
	return StateColor(ios, pr)(label)
}

// AuthorName returns the author's nickname, or "unknown" when Bitbucket sends no author.
func AuthorName(pr PullRequest) string {
	if pr.Author == nil || pr.Author.Nickname == "" {
		return "unknown"
	}
	return pr.Author.Nickname
}

// ReviewSummary lists reviewers and their decisions, e.g. "bob (approved), cy (pending)".
func ReviewSummary(pr PullRequest) string {
	if len(pr.Reviewers) == 0 {
		return "none"
	}
	parts := make([]string, len(pr.Reviewers))
	for i, r := range pr.Reviewers {
		name := r.User.Nickname
		if name == "" {
			name = r.User.DisplayName
		}
		parts[i] = fmt.Sprintf("%s (%s)", name, strings.ReplaceAll(r.State, "_", " "))
	}
	return strings.Join(parts, ", ")
}

// ApprovalCount counts approving reviewers and reports whether any requested changes.
func ApprovalCount(pr PullRequest) (approved, total int, changesRequested bool) {
	for _, r := range pr.Reviewers {
		switch r.State {
		case "approved":
			approved++
		case "changes_requested":
			changesRequested = true
		}
	}
	return approved, len(pr.Reviewers), changesRequested
}
