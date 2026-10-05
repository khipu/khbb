// Package status implements `khbb pr status`.
package status

import (
	"context"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// StatusOptions holds the dependencies of `khbb pr status`.
type StatusOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Exporter   cmdutil.Exporter

	// RepoFlagSet is true when --repo was passed: the current branch belongs to the local
	// repository, not the one named by --repo, so its pull request is not looked up or shown.
	RepoFlagSet bool
}

var statusFields = []string{"currentBranch", "createdByMe", "needsMyReview"}

type statusExport struct {
	CurrentBranch *shared.PullRequest  `json:"currentBranch"`
	CreatedByMe   []shared.PullRequest `json:"createdByMe"`
	NeedsMyReview []shared.PullRequest `json:"needsMyReview"`
}

// NewCmdStatus returns `khbb pr status`.
func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the pull requests that involve you",
		Long:  "Show the open pull request of the current branch, your open pull requests, and those waiting for your review, in the current repository.",
		Args:  cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RepoFlagSet = cmd.Flags().Changed("repo")
			if runF != nil {
				return runF(opts)
			}
			return statusRun(cmd.Context(), opts)
		},
	}
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, statusFields)
	return cmd
}

func statusRun(ctx context.Context, opts *StatusOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	me, err := client.CurrentUser(ctx)
	if err != nil {
		return err
	}
	var branch string
	var branchErr error
	var current *shared.PullRequest
	if !opts.RepoFlagSet {
		branch, branchErr = opts.Branch()
		if branchErr == nil {
			prs, err := openPRs(ctx, client, repo, "source.branch.name = "+bitbucket.QuoteBBQL(branch), 1)
			if err != nil {
				return err
			}
			if len(prs) > 0 {
				current = &prs[0]
			}
		}
	}
	mine, err := openPRs(ctx, client, repo, "author.uuid = "+bitbucket.QuoteBBQL(me.UUID), 30)
	if err != nil {
		return err
	}
	toReview, err := openPRs(ctx, client, repo, "reviewers.uuid = "+bitbucket.QuoteBBQL(me.UUID), 30)
	if err != nil {
		return err
	}
	toReview = slices.DeleteFunc(toReview, func(pr shared.PullRequest) bool { return !awaitingReviewFrom(pr, me.UUID) })

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, statusExport{CurrentBranch: current, CreatedByMe: mine, NeedsMyReview: toReview})
	}
	printStatus(opts.IO, repo, branch, branchErr, current, mine, toReview, opts.RepoFlagSet)
	return nil
}

func openPRs(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, query string, limit int) ([]shared.PullRequest, error) {
	raws, err := client.ListPullRequests(ctx, repo.Workspace, repo.Slug, bitbucket.PRListOptions{
		States: []string{"OPEN"}, Query: query, WithParticipants: true,
	}, limit)
	if err != nil {
		return nil, err
	}
	prs := make([]shared.PullRequest, len(raws))
	for i := range raws {
		prs[i] = shared.NewPullRequest(&raws[i])
	}
	return prs, nil
}

func awaitingReviewFrom(pr shared.PullRequest, uuid string) bool {
	for _, r := range pr.Reviewers {
		if r.User.UUID == uuid {
			return r.State == "pending"
		}
	}
	return false
}

func printStatus(ios *iostreams.IOStreams, repo gitctx.Repo, branch string, branchErr error, current *shared.PullRequest, mine, toReview []shared.PullRequest, repoFlagSet bool) {
	w := ios.Out
	fmt.Fprintf(w, "Relevant pull requests in %s\n\n", repo.FullName())

	fmt.Fprintln(w, ios.Bold("Current branch"))
	switch {
	case repoFlagSet:
		fmt.Fprintln(w, ios.Gray("  Not shown when --repo is set"))
	case branchErr != nil:
		fmt.Fprintln(w, ios.Gray("  Not on a branch"))
	case current == nil:
		fmt.Fprintln(w, ios.Gray("  There is no open pull request for "+branch))
	default:
		printRow(ios, *current)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, ios.Bold("Created by you"))
	if len(mine) == 0 {
		fmt.Fprintln(w, ios.Gray("  You have no open pull requests"))
	}
	for _, pr := range mine {
		printRow(ios, pr)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, ios.Bold("Requesting a code review from you"))
	if len(toReview) == 0 {
		fmt.Fprintln(w, ios.Gray("  You have no pull requests to review"))
	}
	for _, pr := range toReview {
		printRow(ios, pr)
	}
}

func printRow(ios *iostreams.IOStreams, pr shared.PullRequest) {
	fmt.Fprintf(ios.Out, "  %s  %s [%s]  %s\n", ios.Green(fmt.Sprintf("#%d", pr.ID)), pr.Title, ios.Cyan(pr.SourceBranch), reviewText(ios, pr))
}

func reviewText(ios *iostreams.IOStreams, pr shared.PullRequest) string {
	approved, total, changes := shared.ApprovalCount(pr)
	switch {
	case changes:
		return ios.Red("changes requested")
	case total == 0:
		return ios.Gray("no reviewers")
	case approved == total:
		return ios.Green(fmt.Sprintf("%d/%d approved", approved, total))
	}
	return ios.Yellow(fmt.Sprintf("%d/%d approved", approved, total))
}
