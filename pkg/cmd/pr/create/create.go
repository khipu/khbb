// Package create implements `khbb pr create`.
package create

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// CreateOptions holds the inputs and dependencies of `khbb pr create`.
type CreateOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)
	Prompter   cmdutil.Prompter
	Exporter   cmdutil.Exporter

	Title              string
	Body               string
	BodySet            bool
	BodyFile           string
	Base               string
	Head               string
	Reviewers          []string
	Draft              bool
	NoDefaultReviewers bool
	DeleteBranch       bool
}

// NewCmdCreate returns `khbb pr create`.
func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Prompter: f.Prompter}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		Long: `Create a pull request from a branch that is already on Bitbucket. khbb never pushes:
run git push first.

The head branch defaults to the current branch and the base branch to the repository's main
branch. The repository's default reviewers are added unless --no-default-reviewers is given, and
you are never made a reviewer of your own pull request. --reviewer accepts a nickname, display
name, account ID, {uuid} or @me, matched against the members of the workspace.

On a terminal, a missing title or body is asked for. Otherwise --title and --body (or
--body-file) are required.`,
		Example: `  $ khbb pr create --title "Add widgets" --body "Adds the widget factory."
  $ khbb pr create -t "Fix gears" -b "" -r bob -r "Cy Example" --draft
  $ khbb pr create -R acme/widgets -H feature/widgets -B develop -t "Add widgets" -F body.md`,
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fl := cmd.Flags()
			opts.BodySet = fl.Changed("body")
			switch {
			case fl.Changed("repo") && opts.Head == "":
				return cmdutil.FlagErrorf("--head required when using the --repo flag")
			case opts.BodySet && opts.BodyFile != "":
				return cmdutil.FlagErrorf("specify only one of --body and --body-file")
			case opts.Head != "" && opts.Head == opts.Base:
				return cmdutil.FlagErrorf("the head and base branches are both %q", opts.Head)
			}
			if !opts.IO.CanPrompt() {
				if opts.Title == "" {
					return cmdutil.FlagErrorf("--title required when not running interactively")
				}
				if !opts.BodySet && opts.BodyFile == "" {
					return cmdutil.FlagErrorf("--body or --body-file required when not running interactively")
				}
			}
			if runF != nil {
				return runF(opts)
			}
			return createRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.Title, "title", "t", "", "Title of the pull request")
	fl.StringVarP(&opts.Body, "body", "b", "", "Description of the pull request")
	fl.StringVarP(&opts.BodyFile, "body-file", "F", "", "Read the description from `file` (use \"-\" to read from standard input)")
	fl.StringVarP(&opts.Base, "base", "B", "", "The `branch` to merge into (default: the repository's main branch)")
	fl.StringVarP(&opts.Head, "head", "H", "", "The `branch` that contains the changes (default: the current branch)")
	fl.StringSliceVarP(&opts.Reviewers, "reviewer", "r", nil, "Request a review from these `users`")
	fl.BoolVar(&opts.Draft, "draft", false, "Create a draft pull request")
	fl.BoolVar(&opts.NoDefaultReviewers, "no-default-reviewers", false, "Do not add the repository's default reviewers")
	fl.BoolVarP(&opts.DeleteBranch, "delete-branch", "d", false, "Close the head branch when the pull request is merged on Bitbucket")
	cmdutil.AddDryRunFlag(cmd, f)
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PullRequestFields)
	return cmd
}

func createRun(ctx context.Context, opts *CreateOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	head := opts.Head
	if head == "" {
		if head, err = opts.Branch(); err != nil {
			return fmt.Errorf("%w; name the branch with --head", err)
		}
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	base := opts.Base
	if base == "" {
		if base, err = client.RepositoryMainBranch(ctx, repo.Workspace, repo.Slug); err != nil {
			return err
		}
	}
	if head == base {
		return cmdutil.FlagErrorf("the head and base branches are both %q", head)
	}
	title, body, err := titleAndBody(opts)
	if err != nil {
		return err
	}
	if err := checkNoOpenPR(ctx, client, repo, head, base); err != nil {
		return err
	}
	reviewers, err := reviewerUUIDs(ctx, client, repo, opts)
	if err != nil {
		return err
	}
	if opts.IO.IsStderrTTY() {
		fmt.Fprintf(opts.IO.ErrOut, "Creating pull request for %s into %s in %s\n\n", head, base, repo.FullName())
	}
	pr, err := client.CreatePullRequest(ctx, repo.Workspace, repo.Slug, bitbucket.PRCreate{
		Title: title, Description: body, Source: head, Destination: base,
		Draft: opts.Draft, CloseSourceBranch: opts.DeleteBranch, Reviewers: reviewers,
	})
	if err != nil {
		return explainCreateError(err, head)
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, shared.NewPullRequest(pr))
	}
	fmt.Fprintln(opts.IO.Out, pr.Links.HTML.Href)
	return nil
}

// titleAndBody returns the title and body from the flags and asks on a terminal for what is missing.
func titleAndBody(opts *CreateOptions) (string, string, error) {
	body, bodyGiven, err := shared.ReadBody(opts.IO, opts.Body, opts.BodySet, opts.BodyFile)
	if err != nil {
		return "", "", err
	}
	title := opts.Title
	if title == "" || !bodyGiven {
		if !opts.IO.CanPrompt() || opts.Prompter == nil {
			return "", "", cmdutil.FlagErrorf("--title and --body required when not running interactively")
		}
		if title == "" {
			if title, err = opts.Prompter.Input("Title", ""); err != nil {
				return "", "", err
			}
		}
		if !bodyGiven {
			if body, err = opts.Prompter.Input("Body", ""); err != nil {
				return "", "", err
			}
		}
	}
	if strings.TrimSpace(title) == "" {
		return "", "", cmdutil.FlagErrorf("the title cannot be empty")
	}
	return title, body, nil
}

// checkNoOpenPR refuses to continue when head already has an open pull request into base:
// Bitbucket would retitle that pull request instead of creating a new one.
func checkNoOpenPR(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, head, base string) error {
	prs, err := client.ListPullRequests(ctx, repo.Workspace, repo.Slug, bitbucket.PRListOptions{
		Query: "source.branch.name = " + bitbucket.QuoteBBQL(head) + " AND destination.branch.name = " + bitbucket.QuoteBBQL(base),
	}, 1)
	if err != nil {
		return err
	}
	if len(prs) > 0 {
		return &cmdutil.ConflictError{Msg: fmt.Sprintf("a pull request for %s into %s already exists: %s", head, base, prs[0].Links.HTML.Href)}
	}
	return nil
}

// reviewerUUIDs resolves --reviewer and adds the default reviewers, without duplicates and without
// the author: Bitbucket rejects a pull request whose author is among its reviewers.
func reviewerUUIDs(ctx context.Context, client *bitbucket.Client, repo gitctx.Repo, opts *CreateOptions) ([]string, error) {
	resolver := &shared.ReviewerResolver{Client: client, Workspace: repo.Workspace}
	uuids, err := resolver.ResolveAll(ctx, opts.Reviewers)
	if err != nil {
		return nil, err
	}
	if !opts.NoDefaultReviewers {
		defaults, err := client.EffectiveDefaultReviewers(ctx, repo.Workspace, repo.Slug)
		if err != nil {
			return nil, err
		}
		for _, u := range defaults {
			uuids = append(uuids, u.UUID)
		}
	}
	if len(uuids) == 0 {
		return []string{}, nil
	}
	me, err := client.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	return shared.UniqueUUIDs(uuids, me.UUID), nil
}

// explainCreateError turns Bitbucket's "branch not found" for the source into advice: khbb never pushes.
func explainCreateError(err error, head string) error {
	var httpErr *bitbucket.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusBadRequest {
		for _, msg := range httpErr.Fields["source"] {
			if strings.Contains(msg, "branch not found") {
				return &cmdutil.NotFoundError{Msg: fmt.Sprintf("branch %q is not on Bitbucket; push it first, for example with `git push -u origin %s`", head, head)}
			}
		}
	}
	return err
}
