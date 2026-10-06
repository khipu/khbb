package shared

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
)

// RequireOpen refuses to act on a pull request that is no longer open. Bitbucket itself accepts
// some of these actions, such as approving a merged pull request.
func RequireOpen(pr *bitbucket.PullRequest, action string) error {
	if pr.State == "OPEN" {
		return nil
	}
	return &cmdutil.ConflictError{Msg: fmt.Sprintf("pull request #%d is %s; only open pull requests can be %s",
		pr.ID, strings.ToLower(pr.State), action)}
}

// Describe names a pull request with its branches, such as "#42 (feature/widgets → main)".
func Describe(pr *bitbucket.PullRequest) string {
	return fmt.Sprintf("#%d (%s → %s)", pr.ID, pr.Source.Branch.Name, pr.Destination.Branch.Name)
}

// ReadBody returns the text given with --body (when bodySet, even if empty) or --body-file, where
// "-" reads standard input. provided is false when neither flag was used.
func ReadBody(ios *iostreams.IOStreams, body string, bodySet bool, bodyFile string) (text string, provided bool, err error) {
	switch {
	case bodySet && bodyFile != "":
		return "", false, cmdutil.FlagErrorf("specify only one of --body and --body-file")
	case bodySet:
		return body, true, nil
	case bodyFile == "-":
		b, err := io.ReadAll(ios.In)
		if err != nil {
			return "", false, fmt.Errorf("reading the body from standard input: %w", err)
		}
		return string(b), true, nil
	case bodyFile != "":
		b, err := os.ReadFile(bodyFile)
		if err != nil {
			return "", false, fmt.Errorf("reading --body-file: %w", err)
		}
		return string(b), true, nil
	}
	return "", false, nil
}

// PrintSuccess reports a completed action on stderr, with a check mark on a terminal.
func PrintSuccess(ios *iostreams.IOStreams, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if ios.IsStderrTTY() {
		msg = ios.Green("✓") + " " + msg
	}
	fmt.Fprintln(ios.ErrOut, msg)
}
