// Package merge implements `khbb pr merge`.
package merge

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

const (
	pollInterval = 2 * time.Second
	mergeTimeout = 2 * time.Minute
)

// MergeOptions holds the inputs and dependencies of `khbb pr merge`.
type MergeOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Prompter   cmdutil.Prompter
	Exporter   cmdutil.Exporter
	Sleep      func(time.Duration)
	Now        func() time.Time

	Selector     string
	Strategy     string // merge_commit, squash or fast_forward; "" means the destination's default
	DeleteBranch bool
	Message      string
	Yes          bool
	DryRun       bool
}

// NewCmdMerge returns `khbb pr merge`.
func NewCmdMerge(f *cmdutil.Factory, runF func(*MergeOptions) error) *cobra.Command {
	opts := &MergeOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch,
		Prompter: f.Prompter, Sleep: time.Sleep, Now: time.Now}
	var mergeCommit, squash, fastForward bool
	cmd := &cobra.Command{
		Use:   "merge [<number> | <url>]",
		Short: "Merge a pull request",
		Long: `Merge an open pull request. Without a strategy flag, the default strategy of the destination
branch is used. The source branch is kept unless --delete-branch is given.

Merging cannot be undone: on a terminal you are asked to confirm; otherwise --yes is required.
Blocking merge checks, such as conflicts, stop the merge before anything is sent. khbb waits up
to 2 minutes for Bitbucket to finish the merge.`,
		Example: `  $ khbb pr merge 42 --squash --delete-branch
  $ khbb pr merge 42 --yes --json state,mergeCommit
  $ khbb pr merge --dry-run`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			chosen := 0
			for strategy, set := range map[string]bool{"merge_commit": mergeCommit, "squash": squash, "fast_forward": fastForward} {
				if set {
					opts.Strategy = strategy
					chosen++
				}
			}
			if chosen > 1 {
				return cmdutil.FlagErrorf("specify only one of --merge, --squash or --fast-forward")
			}
			opts.DryRun = f.DryRun
			if runF != nil {
				return runF(opts)
			}
			return mergeRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.BoolVar(&mergeCommit, "merge", false, "Merge with a merge commit")
	fl.BoolVar(&squash, "squash", false, "Squash the commits into one")
	fl.BoolVar(&fastForward, "fast-forward", false, "Fast-forward the destination branch")
	fl.BoolVarP(&opts.DeleteBranch, "delete-branch", "d", false, "Delete the source branch on Bitbucket after merging")
	fl.StringVarP(&opts.Message, "message", "m", "", "The `text` of the merge commit message")
	cmdutil.AddYesFlag(cmd, &opts.Yes)
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PullRequestFields)
	return cmd
}

func mergeRun(ctx context.Context, opts *MergeOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if err := shared.RequireOpen(pr, "merged"); err != nil {
		return err
	}
	strategy, err := chooseStrategy(ctx, client, repo, pr, opts.Strategy)
	if err != nil {
		return err
	}
	if err := checkMergeable(ctx, client, repo, pr); err != nil {
		return err
	}
	if pr.CloseSourceBranch && !opts.DeleteBranch {
		fmt.Fprintf(opts.IO.ErrOut, "note: keeping branch %s; pass --delete-branch to delete it\n", pr.Source.Branch.Name)
	}
	deleteNote := ""
	if opts.DeleteBranch {
		deleteNote = " and delete the source branch"
	}
	question := fmt.Sprintf("Merge %s with %s%s?", shared.Describe(pr), strategyName(strategy), deleteNote)
	if err := cmdutil.ConfirmDestructive(opts.IO, opts.Prompter, opts.Yes || opts.DryRun, question); err != nil {
		return err
	}
	merged, taskURL, err := client.StartMerge(ctx, repo.Workspace, repo.Slug, pr.ID, bitbucket.PRMerge{
		Strategy: strategy, Message: opts.Message, CloseSourceBranch: opts.DeleteBranch,
	})
	if err != nil {
		return err
	}
	if merged == nil {
		if merged, err = waitForMerge(ctx, client, opts, pr.ID, taskURL); err != nil {
			return err
		}
	}
	deleted := ""
	if opts.DeleteBranch {
		deleted = " and deleted branch " + pr.Source.Branch.Name
	}
	shared.PrintSuccess(opts.IO, "Merged pull request %s with %s%s", shared.Describe(pr), strategyName(strategy), deleted)
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, shared.NewPullRequest(merged))
	}
	return nil
}

// chooseStrategy returns the requested strategy after checking that the destination branch allows
// it, or the branch's default strategy when none was requested.
func chooseStrategy(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, pr *bitbucket.PullRequest, requested string) (string, error) {
	allowed, err := client.PRMergeStrategies(ctx, repo.Workspace, repo.Slug, pr.ID)
	if err != nil {
		return "", err
	}
	if requested == "" {
		return allowed.Default, nil
	}
	if len(allowed.Allowed) > 0 && !slices.Contains(allowed.Allowed, requested) {
		return "", cmdutil.FlagErrorf("merge strategy %s is not allowed into %s; allowed: %s",
			requested, pr.Destination.Branch.Name, strings.Join(allowed.Allowed, ", "))
	}
	return requested, nil
}

// checkMergeable fails on a blocking merge check, such as conflicts, with Bitbucket's reason.
func checkMergeable(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, pr *bitbucket.PullRequest) error {
	checks, err := client.PRMergeabilityChecks(ctx, repo.Workspace, repo.Slug, pr.ID)
	if err != nil {
		return err
	}
	var problems []string
	for _, c := range checks {
		if c.Status == "FAILED" && c.Blocking {
			problems = append(problems, describeCheck(c))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return &cmdutil.ConflictError{Msg: fmt.Sprintf("pull request #%d cannot be merged: %s", pr.ID, strings.Join(problems, "; "))}
}

func describeCheck(c bitbucket.MergeCheck) string {
	switch {
	case c.Type == "git_mergeability_check" && c.Reason == "conflicts":
		return "it has merge conflicts"
	case c.Reason != "":
		return c.Type + ": " + c.Reason
	}
	return c.Type + " failed"
}

// waitForMerge polls the merge task until it finishes, for at most mergeTimeout. Transient errors
// keep it polling; an unknown task stays PENDING forever, hence the deadline.
func waitForMerge(ctx context.Context, client *bitbucket.Client, opts *MergeOptions, id int, taskURL string) (*bitbucket.PullRequest, error) {
	deadline := opts.Now().Add(mergeTimeout)
	for {
		pr, done, err := client.MergeTaskStatus(ctx, taskURL)
		if err != nil && !bitbucket.IsTransient(err) {
			return nil, err
		}
		if done {
			return pr, nil
		}
		if !opts.Now().Before(deadline) {
			return nil, fmt.Errorf("the merge of pull request #%d is still running after %s; check %s", id, mergeTimeout, taskURL)
		}
		opts.Sleep(pollInterval)
	}
}

// strategyName describes a merge strategy in messages.
func strategyName(strategy string) string {
	switch strategy {
	case "merge_commit":
		return "a merge commit"
	case "fast_forward":
		return "fast-forward"
	case "":
		return "the default strategy"
	}
	return strategy
}
