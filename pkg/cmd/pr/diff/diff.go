// Package diff implements `khbb pr diff`.
package diff

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// DiffOptions holds the inputs and dependencies of `khbb pr diff`.
type DiffOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)

	Selector string
	NameOnly bool
	Patch    bool
	Color    string
}

// NewCmdDiff returns `khbb pr diff`.
func NewCmdDiff(f *cmdutil.Factory, runF func(*DiffOptions) error) *cobra.Command {
	opts := &DiffOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch}
	cmd := &cobra.Command{
		Use:   "diff [<number> | <url>]",
		Short: "Show the changes in a pull request",
		Example: `  $ khbb pr diff 42
  $ khbb pr diff --name-only
  $ khbb pr diff 42 --patch > changes.patch`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			switch opts.Color {
			case "auto", "always", "never":
			default:
				return cmdutil.FlagErrorf("invalid --color %q: use auto, always or never", opts.Color)
			}
			if opts.NameOnly && opts.Patch {
				return cmdutil.FlagErrorf("--name-only cannot be combined with --patch")
			}
			if runF != nil {
				return runF(opts)
			}
			return diffRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.NameOnly, "name-only", false, "Show only the names of changed files")
	cmd.Flags().BoolVar(&opts.Patch, "patch", false, "Show the changes as git patches")
	cmd.Flags().StringVar(&opts.Color, "color", "auto", "Use color in diff output: auto, always or never")
	return cmd
}

func diffRun(ctx context.Context, opts *DiffOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if opts.NameOnly {
		stats, err := client.PRDiffStat(ctx, repo.Workspace, repo.Slug, pr.ID)
		if err != nil {
			return err
		}
		for _, s := range stats {
			switch {
			case s.New != nil:
				fmt.Fprintln(opts.IO.Out, s.New.Path)
			case s.Old != nil:
				fmt.Fprintln(opts.IO.Out, s.Old.Path)
			}
		}
		return nil
	}

	var text string
	if opts.Patch {
		text, err = client.PRPatch(ctx, repo.Workspace, repo.Slug, pr.ID)
	} else {
		text, err = client.PRDiff(ctx, repo.Workspace, repo.Slug, pr.ID)
	}
	if err != nil {
		return err
	}
	colorize := opts.Color == "always" || (opts.Color == "auto" && opts.IO.ColorEnabled())
	if !colorize {
		_, err := io.WriteString(opts.IO.Out, text)
		return err
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		content := strings.TrimSuffix(line, "\n")
		if _, err := io.WriteString(opts.IO.Out, colorLine(content)+line[len(content):]); err != nil {
			return err
		}
	}
	return nil
}

const (
	ansiBold  = "\x1b[1m"
	ansiRed   = "\x1b[31m"
	ansiGreen = "\x1b[32m"
	ansiCyan  = "\x1b[36m"
	ansiReset = "\x1b[m"
)

func colorLine(line string) string {
	switch {
	case strings.HasPrefix(line, "diff --git"), strings.HasPrefix(line, "index "),
		strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return ansiBold + line + ansiReset
	case strings.HasPrefix(line, "@@"):
		return ansiCyan + line + ansiReset
	case strings.HasPrefix(line, "+"):
		return ansiGreen + line + ansiReset
	case strings.HasPrefix(line, "-"):
		return ansiRed + line + ansiReset
	}
	return line
}
