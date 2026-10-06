// Package watch implements `khbb pipeline watch`.
package watch

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pipeline/shared"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// headWait is how long watch waits, after a push, for a pipeline of the local HEAD commit to start.
const headWait = 60 * time.Second

// WatchOptions holds the inputs and dependencies of `khbb pipeline watch`.
type WatchOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Git        *gitctx.Resolver
	Sleep      func(time.Duration)

	Selector   string
	Interval   int
	ExitStatus bool
}

// NewCmdWatch returns `khbb pipeline watch`.
func NewCmdWatch(f *cmdutil.Factory, runF func(*WatchOptions) error) *cobra.Command {
	opts := &WatchOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Git: f.Git, Sleep: time.Sleep}
	cmd := &cobra.Command{
		Use:   "watch [<number> | <url>]",
		Short: "Watch a pipeline until it finishes",
		Long: `Follow a pipeline until it finishes or pauses on a manual step. On a terminal the summary is
redrawn every interval; otherwise one line is printed per pipeline or step status change.

Without an argument, watch the current branch's pipeline for your local HEAD commit, waiting up
to 60 seconds for it to start after a push.

With --exit-status, exit with status 1 when the pipeline ends failed, error, stopped or expired.`,
		Example: `  $ git push && khbb pipeline watch --exit-status
  $ khbb pipeline watch 42 --interval 10`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prshared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			if _, _, err := shared.ParseSelector(opts.Selector); err != nil {
				return err
			}
			if opts.Interval < 1 {
				return cmdutil.FlagErrorf("invalid --interval %d: must be at least 1 second", opts.Interval)
			}
			if runF != nil {
				return runF(opts)
			}
			return watchRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().IntVarP(&opts.Interval, "interval", "i", 5, "Refresh interval in `seconds`")
	cmd.Flags().BoolVar(&opts.ExitStatus, "exit-status", false, "Exit with status 1 if the pipeline does not succeed")
	return cmd
}

func watchRun(ctx context.Context, opts *WatchOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	interval := time.Duration(opts.Interval) * time.Second
	number, repo, err := resolve(ctx, client, opts, interval)
	if err != nil {
		return err
	}
	p, _, err := shared.Watch(ctx, shared.WatchOptions{
		IO: opts.IO, Client: client, Repo: repo, Number: number, Interval: interval, Sleep: opts.Sleep,
	})
	if err != nil {
		return err
	}
	if opts.ExitStatus {
		return shared.ExitStatus(p)
	}
	return nil
}

// resolve returns the pipeline to watch: the selector's, or the current branch's pipeline for the
// local HEAD commit — right after a push, the newest pipeline of the branch is often the previous one.
// A pipeline named by number is not fetched here: Watch fetches it.
func resolve(ctx context.Context, client *bitbucket.Client, opts *WatchOptions, interval time.Duration) (int, gitctx.Repo, error) {
	n, urlRepo, err := shared.ParseSelector(opts.Selector)
	if err != nil {
		return 0, gitctx.Repo{}, err
	}
	if n > 0 {
		if urlRepo != nil {
			return n, *urlRepo, nil
		}
		repo, err := opts.BaseRepo()
		return n, repo, err
	}
	head := localHead(opts.Git)
	if head == "" {
		finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
		p, repo, err := finder.Find(ctx, "")
		if err != nil {
			return 0, repo, err
		}
		return p.BuildNumber, repo, nil
	}
	repo, err := opts.BaseRepo()
	if err != nil {
		return 0, repo, err
	}
	branch, err := opts.Branch()
	if err != nil {
		return 0, repo, err
	}
	for waited := time.Duration(0); ; waited += interval {
		ps, err := client.ListPipelines(ctx, repo.Workspace, repo.Slug, bitbucket.PipelineListOptions{Branch: branch, CommitHash: head}, 1)
		if err != nil {
			return 0, repo, err
		}
		if len(ps) > 0 {
			return ps[0].BuildNumber, repo, nil
		}
		if waited >= headWait {
			return 0, repo, &cmdutil.NotFoundError{Msg: fmt.Sprintf("no pipeline started for commit %s on branch %q within %s; push it, or name a pipeline number",
				shared.ShortHash(head), branch, headWait)}
		}
		if waited == 0 {
			fmt.Fprintf(opts.IO.ErrOut, "Waiting for a pipeline for commit %s on branch %s…\n", shared.ShortHash(head), branch)
		}
		opts.Sleep(interval)
	}
}

// localHead returns the full hash of the local HEAD commit, or "" without git.
func localHead(git *gitctx.Resolver) string {
	if git == nil {
		return ""
	}
	head, err := git.Git("rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return head
}
