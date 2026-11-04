// Package checks implements `khbb pr checks`.
package checks

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// ChecksOptions holds the inputs and dependencies of `khbb pr checks`.
type ChecksOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Exporter   cmdutil.Exporter
	Sleep      func(time.Duration)
	Now        func() time.Time // default time.Now

	Selector string
	Watch    bool
	Interval int
	FailFast bool
}

const clearScreen = "\x1b[H\x1b[2J"

// NewCmdChecks returns `khbb pr checks`.
func NewCmdChecks(f *cmdutil.Factory, runF func(*ChecksOptions) error) *cobra.Command {
	opts := &ChecksOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Sleep: time.Sleep, Now: time.Now}
	cmd := &cobra.Command{
		Use:   "checks [<number> | <url>]",
		Short: "Show the build statuses of a pull request",
		Long: `Show the commit statuses (pipelines and external builds) reported on a pull request.

Exit status: 0 when every check passed or none are reported, 1 when any check failed or was
stopped, 8 when any check is still in progress, 3 when Bitbucket did not answer (with --watch,
after 5 minutes of retries).`,
		Example: `  $ khbb pr checks
  $ khbb pr checks 42 --watch --fail-fast
  $ khbb pr checks 42 --json name,state,url`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			if opts.Interval < 1 {
				return cmdutil.FlagErrorf("invalid --interval %d: must be at least 1 second", opts.Interval)
			}
			if opts.FailFast && !opts.Watch {
				return cmdutil.FlagErrorf("--fail-fast requires --watch")
			}
			if runF != nil {
				return runF(opts)
			}
			return checksRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Watch, "watch", false, "Wait until no check is in progress")
	cmd.Flags().IntVarP(&opts.Interval, "interval", "i", 5, "Refresh interval in seconds while watching")
	cmd.Flags().BoolVar(&opts.FailFast, "fail-fast", false, "Stop watching as soon as a check fails")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.CheckFields)
	return cmd
}

type summary struct{ passed, failed, pending int }

func checksRun(ctx context.Context, opts *ChecksOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	interval := time.Duration(opts.Interval) * time.Second
	live := opts.Watch && opts.IO.IsStdoutTTY() && opts.Exporter == nil
	retry := cmdutil.PollRetry{Interval: interval, Notices: opts.IO.ErrOut, Now: opts.Now}
	for {
		statuses, err := client.ListPRStatuses(ctx, repo.Workspace, repo.Slug, pr.ID)
		if err != nil {
			if !opts.Watch {
				return err
			}
			wait, err := retry.Failed(err)
			if err != nil {
				return err
			}
			opts.Sleep(wait)
			continue
		}
		retry.Succeeded()
		checks := make([]shared.Check, len(statuses))
		for i := range statuses {
			checks[i] = shared.NewCheck(&statuses[i])
		}
		sortChecks(checks)
		sum := summarize(checks)
		done := !opts.Watch || sum.pending == 0 || (opts.FailFast && sum.failed > 0)
		if live {
			fmt.Fprint(opts.IO.Out, clearScreen)
		}
		if done {
			if err := render(opts, pr.ID, checks, sum); err != nil {
				return err
			}
			return exitStatus(sum)
		}
		if live {
			if err := render(opts, pr.ID, checks, sum); err != nil {
				return err
			}
			fmt.Fprintf(opts.IO.Out, "\nRefreshing every %ds; press Ctrl-C to stop.\n", opts.Interval)
		}
		opts.Sleep(interval)
	}
}

func summarize(checks []shared.Check) summary {
	var s summary
	for _, c := range checks {
		switch c.State {
		case "successful":
			s.passed++
		case "failed", "stopped":
			s.failed++
		default:
			s.pending++
		}
	}
	return s
}

func exitStatus(s summary) error {
	switch {
	case s.failed > 0:
		return &cmdutil.ExitError{Code: 1}
	case s.pending > 0:
		return &cmdutil.ExitError{Code: 8}
	}
	return nil
}

// sortChecks puts failures first, then checks in progress, then the rest; each group by name.
func sortChecks(checks []shared.Check) {
	rank := func(state string) int {
		switch state {
		case "failed", "stopped":
			return 0
		case "inprogress":
			return 1
		}
		return 2
	}
	slices.SortStableFunc(checks, func(a, b shared.Check) int {
		if d := rank(a.State) - rank(b.State); d != 0 {
			return d
		}
		return strings.Compare(a.Name, b.Name)
	})
}

func render(opts *ChecksOptions, id int, checks []shared.Check, s summary) error {
	if len(checks) == 0 {
		fmt.Fprintf(opts.IO.ErrOut, "no checks reported on pull request #%d\n", id)
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, checks)
	}
	if len(checks) == 0 {
		return nil
	}
	ios := opts.IO
	tty := ios.IsStdoutTTY()
	if tty {
		fmt.Fprintf(ios.Out, "%s\n%d failed, %d successful, %d pending\n\n", headline(ios, s), s.failed, s.passed, s.pending)
	}
	tp := tableprinter.New(ios.Out, tty, ios.TerminalWidth())
	for _, c := range checks {
		if tty {
			symbol, color := symbolFor(ios, c.State)
			tp.AddField(symbol, tableprinter.WithColor(color))
			tp.AddField(c.Name)
			tp.AddField(c.URL)
		} else {
			tp.AddField(c.Name)
			tp.AddField(c.State)
			tp.AddField(c.URL)
		}
		tp.EndRow()
	}
	return tp.Render()
}

func headline(ios *iostreams.IOStreams, s summary) string {
	switch {
	case s.failed > 0:
		return ios.Bold("Some checks were not successful")
	case s.pending > 0:
		return ios.Bold("Some checks are still in progress")
	}
	return ios.Bold("All checks were successful")
}

func symbolFor(ios *iostreams.IOStreams, state string) (string, func(string) string) {
	switch state {
	case "successful":
		return "✓", ios.Green
	case "failed":
		return "X", ios.Red
	case "stopped":
		return "-", ios.Gray
	}
	return "*", ios.Yellow
}
