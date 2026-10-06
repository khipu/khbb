// Package create implements `khbb pr create`.
package create

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	Browser    cmdutil.Browser
	Git        *gitctx.Resolver

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
	Fill               bool
	Web                bool
}

// NewCmdCreate returns `khbb pr create`.
func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch, Prompter: f.Prompter,
		Browser: f.Browser, Git: f.Git,
	}
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
--body-file) are required.

--fill takes the title and description from the commits that are on the head branch but not on the
base branch (one commit: its message; several: the branch name and a list of subjects); --title and
--body still win. --web opens Bitbucket's page for creating a pull request instead.`,
		Example: `  $ khbb pr create --title "Add widgets" --body "Adds the widget factory."
  $ khbb pr create -t "Fix gears" -b "" -r bob -r "Cy Example" --draft
  $ khbb pr create -R acme/widgets -H feature/widgets -B develop -t "Add widgets" -F body.md`,
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fl := cmd.Flags()
			opts.BodySet = fl.Changed("body")
			switch {
			case opts.Web && opts.Exporter != nil:
				return cmdutil.FlagErrorf("--web cannot be combined with --json")
			case opts.Web && f.DryRun:
				return cmdutil.FlagErrorf("--web cannot be combined with --dry-run")
			case fl.Changed("repo") && opts.Head == "":
				return cmdutil.FlagErrorf("--head required when using the --repo flag")
			case opts.BodySet && opts.BodyFile != "":
				return cmdutil.FlagErrorf("specify only one of --body and --body-file")
			case fl.Changed("title") && strings.TrimSpace(opts.Title) == "":
				return cmdutil.FlagErrorf("the title cannot be empty")
			case opts.Head != "" && opts.Head == opts.Base:
				return cmdutil.FlagErrorf("the head and base branches are both %q", opts.Head)
			}
			if !opts.IO.CanPrompt() && !opts.Fill && !opts.Web {
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
	fl.BoolVar(&opts.Fill, "fill", false, "Use commit messages for the title and description")
	fl.BoolVarP(&opts.Web, "web", "w", false, "Open the web page to create a pull request instead")
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
	if opts.Web {
		return openWeb(opts, repo, head)
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
	if err := checkNoOpenPR(ctx, client, repo, head, base); err != nil {
		return err
	}
	reviewers, err := reviewerUUIDs(ctx, client, repo, opts)
	if err != nil {
		return err
	}
	title, body, err := titleAndBody(opts, repo, head, base)
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
	warnUnpushed(opts, head, pr)
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, shared.NewPullRequest(pr))
	}
	fmt.Fprintln(opts.IO.Out, pr.Links.HTML.Href)
	return nil
}

// titleAndBody returns the title and body from the flags, then from --fill, and asks on a terminal
// for what is still missing.
func titleAndBody(opts *CreateOptions, repo gitctx.Repo, head, base string) (string, string, error) {
	body, bodyGiven, err := shared.ReadBody(opts.IO, opts.Body, opts.BodySet, opts.BodyFile)
	if err != nil {
		return "", "", err
	}
	title := opts.Title
	if opts.Fill && (title == "" || !bodyGiven) {
		fillTitle, fillBody, err := commitSummary(opts.Git, repo, head, base)
		if err != nil {
			return "", "", err
		}
		if title == "" {
			title = fillTitle
		}
		if !bodyGiven {
			body, bodyGiven = fillBody, true
		}
	}
	if title == "" || !bodyGiven {
		if !opts.IO.CanPrompt() || opts.Prompter == nil {
			return "", "", cmdutil.FlagErrorf("--title and --body (or --fill) required when not running interactively")
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

// commitSummary derives a title and body from the commits on head that are not on base, as gh's
// --fill does: one commit gives its subject and body; several give the branch name and a list.
func commitSummary(git *gitctx.Resolver, repo gitctx.Repo, head, base string) (string, string, error) {
	if git == nil {
		return "", "", errors.New("--fill needs git")
	}
	remote, err := git.RemoteFor(repo)
	if err != nil {
		return "", "", fmt.Errorf("--fill: %w", err)
	}
	if remote == "" {
		return "", "", fmt.Errorf("--fill needs a git remote for %s", repo.FullName())
	}
	out, err := git.Git("log", "--reverse", "--format=%s%x1f%b%x1e", remote+"/"+base+".."+head)
	if err != nil {
		return "", "", fmt.Errorf("--fill: %w (is %s/%s fetched?)", err, remote, base)
	}
	type commit struct{ subject, body string }
	var commits []commit
	for _, rec := range strings.Split(out, "\x1e") {
		if rec = strings.TrimSpace(rec); rec == "" {
			continue
		}
		subject, body, _ := strings.Cut(rec, "\x1f")
		commits = append(commits, commit{strings.TrimSpace(subject), strings.TrimSpace(body)})
	}
	switch len(commits) {
	case 0:
		return "", "", fmt.Errorf("--fill: no commits on %s that are not on %s/%s", head, remote, base)
	case 1:
		return commits[0].subject, commits[0].body, nil
	}
	lines := make([]string, len(commits))
	for i, c := range commits {
		lines[i] = "- " + c.subject
	}
	return humanize(head), strings.Join(lines, "\n"), nil
}

// humanize turns a branch name into a title: dashes and underscores become spaces.
func humanize(branch string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' {
			return ' '
		}
		return r
	}, branch)
}

// openWeb opens Bitbucket's page for creating a pull request from head. source and t=1 form the URL
// Bitbucket prints after a push; dest is only sent when --base names a branch.
func openWeb(opts *CreateOptions, repo gitctx.Repo, head string) error {
	q := url.Values{"source": {head}, "t": {"1"}}
	if opts.Base != "" {
		q.Set("dest", opts.Base)
	}
	u := fmt.Sprintf("https://bitbucket.org/%s/%s/pull-requests/new?%s", repo.Workspace, repo.Slug, q.Encode())
	if opts.IO.IsStderrTTY() {
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", u)
	}
	return opts.Browser.Browse(u)
}

// warnUnpushed warns when the pull request does not point at the local head commit, which usually
// means commits were not pushed. It stays quiet whenever git cannot tell.
func warnUnpushed(opts *CreateOptions, head string, pr *bitbucket.PullRequest) {
	if opts.Git == nil || pr.Source.Commit == nil || pr.Source.Commit.Hash == "" {
		return
	}
	local, err := opts.Git.Git("rev-parse", "--verify", "--quiet", "refs/heads/"+head)
	if err != nil || local == "" {
		return
	}
	remote := pr.Source.Commit.Hash
	if strings.HasPrefix(local, remote) || strings.HasPrefix(remote, local) {
		return
	}
	fmt.Fprintf(opts.IO.ErrOut, "warning: the pull request uses commit %s, but your local branch %s is at %s; push your latest commits\n",
		shortHash(remote), head, shortHash(local))
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
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
				return &cmdutil.NotFoundError{Msg: fmt.Sprintf("branch %q is not on Bitbucket; push it first (git push -u <remote> %s)", head, head)}
			}
		}
	}
	return err
}
