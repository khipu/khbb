// Package comment implements `khbb pr comment`.
package comment

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// CommentOptions holds the inputs and dependencies of `khbb pr comment`.
type CommentOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Prompter   cmdutil.Prompter
	Exporter   cmdutil.Exporter

	Selector string
	Body     string
	BodySet  bool
	BodyFile string
	File     string
	Line     int
	ReplyTo  int
}

// NewCmdComment returns `khbb pr comment`.
func NewCmdComment(f *cmdutil.Factory, runF func(*CommentOptions) error) *cobra.Command {
	opts := &CommentOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Prompter: f.Prompter}
	cmd := &cobra.Command{
		Use:   "comment [<number> | <url>]",
		Short: "Comment on a pull request",
		Long: `Add a comment to a pull request: a general comment, a comment on a file (--file), on a line
of the new version of a file (--file with --line), or a reply to another comment (--reply-to).

Without an argument, comment on the open pull request of the current branch. Without --body or
--body-file, the comment is asked for on a terminal.`,
		Example: `  $ khbb pr comment 42 --body "Looks good"
  $ khbb pr comment 42 --file src/widget.go --line 12 --body "Rename this"
  $ khbb pr comment 42 --reply-to 101 --body-file reply.md`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			fl := cmd.Flags()
			opts.BodySet = fl.Changed("body")
			switch {
			case fl.Changed("line") && opts.File == "":
				return cmdutil.FlagErrorf("--line requires --file")
			case fl.Changed("line") && opts.Line < 1:
				return cmdutil.FlagErrorf("invalid --line %d: must be at least 1", opts.Line)
			case fl.Changed("reply-to") && opts.ReplyTo < 1:
				return cmdutil.FlagErrorf("invalid --reply-to %d: expected a comment ID", opts.ReplyTo)
			case opts.ReplyTo > 0 && opts.File != "":
				return cmdutil.FlagErrorf("--reply-to cannot be combined with --file or --line: a reply stays with its comment")
			}
			if runF != nil {
				return runF(opts)
			}
			return commentRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.Body, "body", "b", "", "The comment `text`")
	fl.StringVarP(&opts.BodyFile, "body-file", "F", "", "Read the comment from `file` (use \"-\" to read from standard input)")
	fl.StringVar(&opts.File, "file", "", "Comment on this `path` of the pull request")
	fl.IntVar(&opts.Line, "line", 0, "Comment on this `line` of the new version of --file")
	fl.IntVar(&opts.ReplyTo, "reply-to", 0, "Reply to the comment with this `id`")
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.CommentFields)
	return cmd
}

func commentRun(ctx context.Context, opts *CommentOptions) error {
	body, err := commentBody(opts)
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if opts.File != "" {
		if err := checkFile(ctx, client, repo, pr.ID, opts.File); err != nil {
			return err
		}
	}
	c, err := client.CreatePRComment(ctx, repo.Workspace, repo.Slug, pr.ID, bitbucket.PRCommentInput{
		Body: body, Path: opts.File, Line: opts.Line, ParentID: opts.ReplyTo,
	})
	if err != nil {
		return err
	}
	comment := shared.NewComment(c)
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, comment)
	}
	fmt.Fprintln(opts.IO.Out, comment.URL)
	return nil
}

// commentBody reads the comment from the flags, or asks for it on a terminal.
func commentBody(opts *CommentOptions) (string, error) {
	body, provided, err := shared.ReadBody(opts.IO, opts.Body, opts.BodySet, opts.BodyFile)
	if err != nil {
		return "", err
	}
	if !provided {
		if !opts.IO.CanPrompt() || opts.Prompter == nil {
			return "", cmdutil.FlagErrorf("--body or --body-file required when not running interactively")
		}
		if body, err = opts.Prompter.Input("Comment", ""); err != nil {
			return "", err
		}
	}
	if strings.TrimSpace(body) == "" {
		return "", cmdutil.FlagErrorf("the comment cannot be empty")
	}
	return body, nil
}

// checkFile makes sure path is part of the pull request: Bitbucket accepts comments on any path.
func checkFile(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, id int, path string) error {
	stats, err := client.PRDiffStat(ctx, repo.Workspace, repo.Slug, id)
	if err != nil {
		return err
	}
	for _, s := range stats {
		if (s.New != nil && s.New.Path == path) || (s.Old != nil && s.Old.Path == path) {
			return nil
		}
	}
	return &cmdutil.NotFoundError{Msg: fmt.Sprintf("%s is not changed in pull request #%d", path, id)}
}
