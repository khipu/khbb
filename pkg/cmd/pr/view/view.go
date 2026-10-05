// Package view implements `khbb pr view`.
package view

import (
	"context"
	"fmt"
	"io"
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

// ViewOptions holds the inputs and dependencies of `khbb pr view`.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Browser    cmdutil.Browser
	Exporter   cmdutil.Exporter

	Selector string
	Comments bool
	Web      bool
}

// viewFields are the pull request fields plus the PR's comments.
var viewFields = append(slices.Clone(shared.PullRequestFields), "comments")

type viewExport struct {
	shared.PullRequest
	Comments []shared.Comment `json:"comments"`
}

// NewCmdView returns `khbb pr view`.
func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Browser: f.Browser}
	cmd := &cobra.Command{
		Use:   "view [<number> | <url>]",
		Short: "Show a pull request",
		Long:  "Show a pull request's title, state, reviewers and description. Without an argument, show the open pull request of the current branch.",
		Example: `  $ khbb pr view 42
  $ khbb pr view --comments
  $ khbb pr view 42 --json title,state,reviewers,comments`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			if opts.Web && opts.Exporter != nil {
				return cmdutil.FlagErrorf("--web cannot be combined with --json")
			}
			if runF != nil {
				return runF(opts)
			}
			return viewRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.Comments, "comments", "c", false, "Show the pull request's comments")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open the pull request in the browser")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, viewFields)
	return cmd
}

func viewRun(ctx context.Context, opts *ViewOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	raw, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	pr := shared.NewPullRequest(raw)
	if opts.Web {
		if opts.IO.IsStderrTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", pr.URL)
		}
		return opts.Browser.Browse(pr.URL)
	}

	var comments []shared.Comment
	if opts.Comments || (opts.Exporter != nil && slices.Contains(opts.Exporter.Fields(), "comments")) {
		raws, err := client.ListPRComments(ctx, repo.Workspace, repo.Slug, pr.ID)
		if err != nil {
			return err
		}
		comments = make([]shared.Comment, len(raws))
		for i := range raws {
			comments[i] = shared.NewComment(&raws[i])
		}
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, viewExport{PullRequest: pr, Comments: comments})
	}
	if opts.IO.IsStdoutTTY() {
		printHuman(opts.IO, pr, comments)
	} else {
		printRaw(opts.IO.Out, pr, comments)
	}
	return nil
}

func printHuman(ios *iostreams.IOStreams, pr shared.PullRequest, comments []shared.Comment) {
	w := ios.Out
	fmt.Fprintf(w, "%s #%d\n", ios.Bold(pr.Title), pr.ID)
	fmt.Fprintf(w, "%s • %s wants to merge %s into %s • updated %s\n",
		shared.StateLabel(ios, pr), shared.AuthorName(pr), ios.Cyan(pr.SourceBranch), ios.Cyan(pr.DestinationBranch),
		pr.UpdatedOn.Format("2006-01-02"))
	fmt.Fprintf(w, "Reviewers: %s\n", shared.ReviewSummary(pr))
	fmt.Fprintf(w, "Comments: %d • Tasks: %d\n\n", pr.CommentCount, pr.TaskCount)
	body := strings.TrimSpace(pr.Body)
	if body == "" {
		body = ios.Gray("No description provided")
	}
	fmt.Fprintf(w, "%s\n", body)
	if comments != nil {
		fmt.Fprintf(w, "\n%s\n", ios.Bold("Comments"))
		for _, c := range comments {
			if c.Deleted {
				continue
			}
			fmt.Fprintf(w, "%s • %s%s\n", commentAuthor(c), c.CreatedOn.Format("2006-01-02"), commentContext(c))
			for _, line := range strings.Split(strings.TrimRight(c.Body, "\n"), "\n") {
				fmt.Fprintf(w, "  %s\n", line)
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintf(w, "\n%s\n", ios.Gray("View this pull request on Bitbucket: "+pr.URL))
}

func printRaw(w io.Writer, pr shared.PullRequest, comments []shared.Comment) {
	fmt.Fprintf(w, "title:\t%s\nnumber:\t%d\nstate:\t%s\ndraft:\t%t\nauthor:\t%s\nsource:\t%s\ndestination:\t%s\n"+
		"reviewers:\t%s\ncomments:\t%d\ntasks:\t%d\nurl:\t%s\n--\n%s\n",
		pr.Title, pr.ID, pr.State, pr.Draft, shared.AuthorName(pr), pr.SourceBranch, pr.DestinationBranch,
		shared.ReviewSummary(pr), pr.CommentCount, pr.TaskCount, pr.URL, pr.Body)
	for _, c := range comments {
		if c.Deleted {
			continue
		}
		fmt.Fprintf(w, "--\ncomment:\t%d\nauthor:\t%s\ncreated:\t%s\n", c.ID, commentAuthor(c), c.CreatedOn.Format(time.RFC3339))
		if c.Path != "" {
			fmt.Fprintf(w, "location:\t%s\n", location(c))
		}
		if c.ParentID != nil {
			fmt.Fprintf(w, "reply to:\t%d\n", *c.ParentID)
		}
		fmt.Fprintf(w, "--\n%s\n", c.Body)
	}
}

func commentAuthor(c shared.Comment) string {
	if c.Author == nil || c.Author.Nickname == "" {
		return "unknown"
	}
	return c.Author.Nickname
}

func location(c shared.Comment) string {
	if c.Line != nil {
		return fmt.Sprintf("%s:%d", c.Path, *c.Line)
	}
	return c.Path
}

func commentContext(c shared.Comment) string {
	switch {
	case c.Path != "":
		return " • " + location(c)
	case c.ParentID != nil:
		return fmt.Sprintf(" • reply to %d", *c.ParentID)
	}
	return ""
}
