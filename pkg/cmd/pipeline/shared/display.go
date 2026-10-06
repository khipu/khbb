package shared

import (
	"fmt"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"

	"github.com/khipu/khbb/internal/iostreams"
)

// StatusLabel renders a status for humans: a symbol and the status, colored.
func StatusLabel(ios *iostreams.IOStreams, status string) string {
	switch status {
	case StatusSuccessful:
		return ios.Green("✓ " + status)
	case StatusFailed, StatusError:
		return ios.Red("X " + status)
	case StatusStopped, StatusExpired, StatusSkipped:
		return ios.Gray("- " + status)
	case StatusPaused:
		return ios.Yellow("‖ " + status)
	}
	return ios.Yellow("* " + status)
}

// FormatDuration renders seconds as 45s, 3m05s or 1h02m.
func FormatDuration(seconds int) string {
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh%02dm", seconds/3600, seconds%3600/60)
}

// ShortHash abbreviates a commit hash to 7 characters.
func ShortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// RefLabel says what a pipeline ran on, such as "branch main", "pull request from feature/x" or
// "custom pipeline deploy on branch main".
func RefLabel(p Pipeline) string {
	var label string
	switch p.RefType {
	case "pullrequest":
		label = "pull request from " + p.RefName
	case "commit":
		label = "commit " + ShortHash(p.Commit)
	default:
		label = p.RefType + " " + p.RefName
	}
	if p.Selector.Type == "custom" {
		label = "custom pipeline " + p.Selector.Pattern + " on " + label
	}
	return label
}

// CreatorName returns the creator's nickname, or "unknown" when Bitbucket sends none.
func CreatorName(p Pipeline) string {
	if p.Creator == nil || p.Creator.Nickname == "" {
		return "unknown"
	}
	return p.Creator.Nickname
}

// RenderSummary prints a pipeline's summary and step table for humans; verbose adds each step's
// UUID and timestamps.
func RenderSummary(ios *iostreams.IOStreams, p Pipeline, steps []Step, verbose bool) error {
	w := ios.Out
	fmt.Fprintf(w, "%s %s\n", ios.Bold(fmt.Sprintf("Pipeline #%d", p.Number)), StatusLabel(ios, p.Status))
	fmt.Fprintf(w, "%s · %s by %s · commit %s\n", RefLabel(p), p.Trigger, CreatorName(p), ShortHash(p.Commit))
	started := "Started " + p.CreatedOn.Format("2006-01-02 15:04 MST")
	if p.CompletedOn != nil {
		started += " · took " + FormatDuration(p.DurationSeconds)
	}
	fmt.Fprintf(w, "%s\n\n", started)
	tp := tableprinter.New(w, true, ios.TerminalWidth())
	header := []string{"STEP", "STATUS", "DURATION"}
	if verbose {
		header = append(header, "UUID", "STARTED", "COMPLETED")
	}
	tp.AddHeader(header)
	for _, s := range steps {
		tp.AddField(s.Name)
		tp.AddField(StatusLabel(ios, s.Status))
		tp.AddField(stepDuration(s))
		if verbose {
			tp.AddField(s.UUID)
			tp.AddField(timeOrDash(s.StartedOn))
			tp.AddField(timeOrDash(s.CompletedOn))
		}
		tp.EndRow()
	}
	return tp.Render()
}

func stepDuration(s Step) string {
	if s.StartedOn == nil {
		return "-"
	}
	return FormatDuration(s.DurationSeconds)
}

func timeOrDash(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
}
