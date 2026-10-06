package shared

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
)

var pipelineURLRE = regexp.MustCompile(`^https?://(?:www\.)?bitbucket\.org/([^/]+)/([^/]+)/pipelines/results/(\d+)(?:[/?#].*)?$`)

// ParseSelector reads a pipeline argument: "" (the newest pipeline of the current branch), "42",
// "#42" or a pipeline URL, which also names the repository.
func ParseSelector(s string) (int, *gitctx.Repo, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil, nil
	}
	if m := pipelineURLRE.FindStringSubmatch(s); m != nil {
		repo, err := gitctx.ParseRepo(m[1] + "/" + m[2])
		n, _ := strconv.Atoi(m[3])
		if err != nil || n <= 0 {
			return 0, nil, cmdutil.FlagErrorf("invalid pipeline URL %q", s)
		}
		return n, &repo, nil
	}
	n, err := strconv.Atoi(strings.TrimPrefix(s, "#"))
	if err != nil || n <= 0 {
		return 0, nil, cmdutil.FlagErrorf("invalid pipeline %q: expected a build number, #number or a pipeline URL", s)
	}
	return n, nil, nil
}

// Finder locates the pipeline a command acts on.
type Finder struct {
	Client   *bitbucket.Client
	BaseRepo func() (gitctx.Repo, error)
	Branch   func() (string, error)
}

// Find returns the pipeline named by selector and its repository. An empty selector means the
// newest pipeline of the current branch.
func (f *Finder) Find(ctx context.Context, selector string) (*bitbucket.Pipeline, gitctx.Repo, error) {
	n, urlRepo, err := ParseSelector(selector)
	if err != nil {
		return nil, gitctx.Repo{}, err
	}
	var repo gitctx.Repo
	if urlRepo != nil {
		repo = *urlRepo
	} else if repo, err = f.BaseRepo(); err != nil {
		return nil, gitctx.Repo{}, err
	}
	if n > 0 {
		p, err := f.Client.GetPipeline(ctx, repo.Workspace, repo.Slug, n)
		return p, repo, err
	}
	branch, err := f.Branch()
	if err != nil {
		return nil, repo, err
	}
	ps, err := f.Client.ListPipelines(ctx, repo.Workspace, repo.Slug, bitbucket.PipelineListOptions{Branch: branch}, 1)
	if err != nil {
		return nil, repo, err
	}
	if len(ps) == 0 {
		return nil, repo, &cmdutil.NotFoundError{Msg: fmt.Sprintf("no pipelines found for branch %q in %s", branch, repo.FullName())}
	}
	return &ps[0], repo, nil
}
