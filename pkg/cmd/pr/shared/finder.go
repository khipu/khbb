package shared

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
)

// CheckRepoSelector rejects --repo without a pull request argument: the current branch belongs to the local repository.
func CheckRepoSelector(cmd *cobra.Command, args []string) error {
	if len(args) == 0 && cmd.Flags().Changed("repo") {
		return cmdutil.FlagErrorf("argument required when using the --repo flag")
	}
	return nil
}

var prURLRE = regexp.MustCompile(`^https?://(?:www\.)?bitbucket\.org/([^/]+)/([^/]+)/pull-requests/(\d+)(?:[/?#].*)?$`)

// ParseSelector reads a pull request argument: "" (the current branch), "42", "#42" or a
// pull request URL. A URL also names the repository.
func ParseSelector(s string) (int, *gitctx.Repo, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil, nil
	}
	if m := prURLRE.FindStringSubmatch(s); m != nil {
		repo, err := gitctx.ParseRepo(m[1] + "/" + m[2])
		if err != nil {
			return 0, nil, cmdutil.FlagErrorf("invalid pull request URL %q", s)
		}
		id, _ := strconv.Atoi(m[3])
		if id <= 0 {
			return 0, nil, cmdutil.FlagErrorf("invalid pull request URL %q", s)
		}
		return id, &repo, nil
	}
	id, err := strconv.Atoi(strings.TrimPrefix(s, "#"))
	if err != nil || id <= 0 {
		return 0, nil, cmdutil.FlagErrorf("invalid pull request %q: expected a number, #number or a pull request URL", s)
	}
	return id, nil, nil
}

// Finder locates the pull request a `pr` command acts on.
type Finder struct {
	Client   *bitbucket.Client
	BaseRepo func() (gitctx.Repo, error)
	Branch   func() (string, error)
}

// Find returns the pull request named by selector and its repository. An empty selector means
// the only open pull request whose source is the current branch.
func (f *Finder) Find(ctx context.Context, selector string) (*bitbucket.PullRequest, gitctx.Repo, error) {
	id, urlRepo, err := ParseSelector(selector)
	if err != nil {
		return nil, gitctx.Repo{}, err
	}
	var repo gitctx.Repo
	if urlRepo != nil {
		repo = *urlRepo
	} else if repo, err = f.BaseRepo(); err != nil {
		return nil, gitctx.Repo{}, err
	}
	if id == 0 {
		if id, err = f.idForCurrentBranch(ctx, repo); err != nil {
			return nil, repo, err
		}
	}
	pr, err := f.Client.GetPullRequest(ctx, repo.Workspace, repo.Slug, id)
	return pr, repo, err
}

func (f *Finder) idForCurrentBranch(ctx context.Context, repo gitctx.Repo) (int, error) {
	branch, err := f.Branch()
	if err != nil {
		return 0, err
	}
	prs, err := f.Client.ListPullRequests(ctx, repo.Workspace, repo.Slug, bitbucket.PRListOptions{
		States: []string{"OPEN"},
		Query:  "source.branch.name = " + bitbucket.QuoteBBQL(branch),
	}, 10)
	if err != nil {
		return 0, err
	}
	switch len(prs) {
	case 0:
		return 0, &cmdutil.NotFoundError{Msg: fmt.Sprintf("no open pull request found for branch %q in %s", branch, repo.FullName())}
	case 1:
		return prs[0].ID, nil
	}
	ids := make([]string, len(prs))
	for i, pr := range prs {
		ids[i] = "#" + strconv.Itoa(pr.ID)
	}
	return 0, cmdutil.FlagErrorf("branch %q has several open pull requests (%s); specify one", branch, strings.Join(ids, ", "))
}
