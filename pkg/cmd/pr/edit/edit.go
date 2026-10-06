// Package edit implements `khbb pr edit`.
package edit

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

// EditOptions holds the inputs and dependencies of `khbb pr edit`.
type EditOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Exporter   cmdutil.Exporter

	Selector        string
	Title           string
	TitleSet        bool
	Body            string
	BodySet         bool
	BodyFile        string
	Base            string
	AddReviewers    []string
	RemoveReviewers []string
	Draft           bool
	Ready           bool
}

// NewCmdEdit returns `khbb pr edit`.
func NewCmdEdit(f *cmdutil.Factory, runF func(*EditOptions) error) *cobra.Command {
	opts := &EditOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch}
	cmd := &cobra.Command{
		Use:   "edit [<number> | <url>]",
		Short: "Edit a pull request",
		Long: `Change the title, description, base branch, reviewers or draft state of an open pull request.
Only the fields you name are changed. Without an argument, edit the open pull request of the
current branch. Reviewers accept the same forms as in pr create; you are never added as a
reviewer of your own pull request.`,
		Example: `  $ khbb pr edit 42 --title "Add widgets and gears"
  $ khbb pr edit 42 --add-reviewer bob --remove-reviewer cy
  $ khbb pr edit --ready`,
		Args: cmdutil.MaximumNArgs(1, "[<number> | <url>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shared.CheckRepoSelector(cmd, args); err != nil {
				return err
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			fl := cmd.Flags()
			opts.TitleSet, opts.BodySet = fl.Changed("title"), fl.Changed("body")
			changed := opts.Draft || opts.Ready
			for _, name := range []string{"title", "body", "body-file", "base", "add-reviewer", "remove-reviewer"} {
				changed = changed || fl.Changed(name)
			}
			switch {
			case !changed:
				return cmdutil.FlagErrorf("specify at least one change: --title, --body, --body-file, --base, --add-reviewer, --remove-reviewer, --draft or --ready")
			case opts.Draft && opts.Ready:
				return cmdutil.FlagErrorf("--draft and --ready cannot be used together")
			case opts.TitleSet && strings.TrimSpace(opts.Title) == "":
				return cmdutil.FlagErrorf("the title cannot be empty")
			case opts.BodySet && opts.BodyFile != "":
				return cmdutil.FlagErrorf("specify only one of --body and --body-file")
			case fl.Changed("base") && strings.TrimSpace(opts.Base) == "":
				return cmdutil.FlagErrorf("--base cannot be empty")
			}
			if runF != nil {
				return runF(opts)
			}
			return editRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.Title, "title", "t", "", "Set the title")
	fl.StringVarP(&opts.Body, "body", "b", "", "Set the description")
	fl.StringVarP(&opts.BodyFile, "body-file", "F", "", "Read the description from `file` (use \"-\" to read from standard input)")
	fl.StringVarP(&opts.Base, "base", "B", "", "Change the `branch` to merge into")
	fl.StringSliceVar(&opts.AddReviewers, "add-reviewer", nil, "Add these `users` as reviewers")
	fl.StringSliceVar(&opts.RemoveReviewers, "remove-reviewer", nil, "Remove these `users` from the reviewers")
	fl.BoolVar(&opts.Draft, "draft", false, "Mark the pull request as a draft")
	fl.BoolVar(&opts.Ready, "ready", false, "Mark the pull request as ready for review")
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PullRequestFields)
	return cmd
}

func editRun(ctx context.Context, opts *EditOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	finder := &shared.Finder{Client: client, BaseRepo: opts.BaseRepo, Branch: opts.Branch}
	pr, repo, err := finder.Find(ctx, opts.Selector)
	if err != nil {
		return err
	}
	if err := shared.RequireOpen(pr, "edited"); err != nil {
		return err
	}
	var upd bitbucket.PRUpdate
	if opts.TitleSet {
		upd.Title = &opts.Title
	}
	body, bodyGiven, err := shared.ReadBody(opts.IO, opts.Body, opts.BodySet, opts.BodyFile)
	if err != nil {
		return err
	}
	if bodyGiven {
		upd.Description = &body
	}
	if opts.Base != "" {
		upd.Destination = &opts.Base
	}
	if opts.Draft || opts.Ready {
		draft := opts.Draft
		upd.Draft = &draft
	}
	if len(opts.AddReviewers) > 0 || len(opts.RemoveReviewers) > 0 {
		if upd.Reviewers, err = editedReviewers(ctx, client, repo, pr, opts); err != nil {
			return err
		}
	} else {
		current := make([]string, 0, len(pr.Reviewers))
		for _, r := range pr.Reviewers {
			current = append(current, r.UUID)
		}
		var author string
		if pr.Author != nil {
			author = pr.Author.UUID
		}
		upd.Reviewers = shared.UniqueUUIDs(current, author)
	}
	updated, err := client.UpdatePullRequest(ctx, repo.Workspace, repo.Slug, pr.ID, upd)
	if err != nil {
		return err
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, shared.NewPullRequest(updated))
	}
	fmt.Fprintln(opts.IO.Out, updated.Links.HTML.Href)
	return nil
}

// editedReviewers returns the pull request's reviewers with --add-reviewer and --remove-reviewer
// applied. The author is never added: Bitbucket rejects that.
func editedReviewers(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, pr *bitbucket.PullRequest, opts *EditOptions) ([]string, error) {
	resolver := &shared.ReviewerResolver{Client: client, Workspace: repo.Workspace}
	add, err := resolver.ResolveAll(ctx, opts.AddReviewers)
	if err != nil {
		return nil, err
	}
	remove, err := resolver.ResolveAll(ctx, opts.RemoveReviewers)
	if err != nil {
		return nil, err
	}
	uuids := make([]string, 0, len(pr.Reviewers)+len(add))
	for _, r := range pr.Reviewers {
		uuids = append(uuids, r.UUID)
	}
	uuids = append(uuids, add...)
	if pr.Author != nil {
		remove = append(remove, pr.Author.UUID)
	}
	return shared.UniqueUUIDs(uuids, remove...), nil
}
