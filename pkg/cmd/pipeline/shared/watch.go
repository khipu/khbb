package shared

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
)

// maxConsecutiveErrors is how many failed polls in a row Watch accepts: it gives up on the 5th
// consecutive transient failure (spec §10).
const maxConsecutiveErrors = 5

const clearScreen = "\x1b[H\x1b[2J"

// WatchOptions configures Watch.
type WatchOptions struct {
	IO       *iostreams.IOStreams
	Client   *bitbucket.Client
	Repo     gitctx.Repo
	Number   int
	Interval time.Duration
	Sleep    func(time.Duration)
}

// Watch polls pipeline o.Number until it completes or pauses on a manual step, and returns its last
// state. On a terminal it redraws the summary on every poll; otherwise it prints one line per
// pipeline or step status change. Transient errors (429, 5xx, network) are retried; others end it.
func Watch(ctx context.Context, o WatchOptions) (Pipeline, []Step, error) {
	live := o.IO.IsStdoutTTY()
	seen := map[string]string{}
	errorsInARow := 0
	for {
		raw, steps, err := fetch(ctx, o)
		if err != nil {
			errorsInARow++
			if !bitbucket.IsTransient(err) || errorsInARow >= maxConsecutiveErrors {
				return Pipeline{}, nil, err
			}
			o.Sleep(o.Interval)
			continue
		}
		errorsInARow = 0
		p := NewPipeline(raw, o.Repo)
		done := raw.State.Name == "COMPLETED" || p.Status == StatusPaused
		if live {
			fmt.Fprint(o.IO.Out, clearScreen)
			if err := RenderSummary(o.IO, p, steps, false); err != nil {
				return Pipeline{}, nil, err
			}
			if name := PausedStep(steps); p.Status == StatusPaused && name != "" {
				fmt.Fprintf(o.IO.Out, "\nWaiting for manual step %q: start it in Bitbucket.\n", name)
			}
			if !done {
				fmt.Fprintf(o.IO.Out, "\nRefreshing every %s; press Ctrl-C to stop.\n", o.Interval)
			}
		} else {
			printChanges(o.IO.Out, p, steps, seen)
		}
		if done {
			return p, steps, nil
		}
		o.Sleep(o.Interval)
	}
}

// ExitStatus turns an unsuccessful final status into exit code 1 (`watch --exit-status`, `run --watch`).
func ExitStatus(p Pipeline) error {
	if Unsuccessful(p.Status) {
		return &cmdutil.ExitError{Code: 1}
	}
	return nil
}

// PausedStep returns the name of the step waiting to be started by hand, or "".
func PausedStep(steps []Step) string {
	for _, s := range steps {
		if s.Status == StatusPaused {
			return s.Name
		}
	}
	return ""
}

func fetch(ctx context.Context, o WatchOptions) (*bitbucket.Pipeline, []Step, error) {
	raw, err := o.Client.GetPipeline(ctx, o.Repo.Workspace, o.Repo.Slug, o.Number)
	if err != nil {
		return nil, nil, err
	}
	rawSteps, err := o.Client.ListPipelineSteps(ctx, o.Repo.Workspace, o.Repo.Slug, o.Number)
	if err != nil {
		return nil, nil, err
	}
	steps := make([]Step, len(rawSteps))
	for i := range rawSteps {
		steps[i] = NewStep(&rawSteps[i])
	}
	return raw, steps, nil
}

// printChanges writes a line for each step, then the pipeline, whose status changed since the
// last poll; seen keeps the last status printed per step UUID ("" for the pipeline).
func printChanges(w io.Writer, p Pipeline, steps []Step, seen map[string]string) {
	for _, s := range steps {
		if seen[s.UUID] == s.Status {
			continue
		}
		seen[s.UUID] = s.Status
		line := fmt.Sprintf("#%d step %q: %s", p.Number, s.Name, s.Status)
		if s.CompletedOn != nil && s.StartedOn != nil {
			line += " (" + FormatDuration(s.DurationSeconds) + ")"
		}
		fmt.Fprintln(w, line)
	}
	if seen[""] == p.Status {
		return
	}
	seen[""] = p.Status
	line := fmt.Sprintf("#%d %s", p.Number, p.Status)
	switch {
	case p.Status == StatusPaused && PausedStep(steps) != "":
		line += fmt.Sprintf(" (waiting for manual step %q)", PausedStep(steps))
	case p.CompletedOn != nil:
		line += " (" + FormatDuration(p.DurationSeconds) + ")"
	}
	fmt.Fprintln(w, line)
}
