package bitbucket

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// userEntry is the shape of workspace members and default reviewers: a user wrapped in an object.
type userEntry struct {
	User User `json:"user"`
}

func toUsers(entries []userEntry) []User {
	out := make([]User, len(entries))
	for i, e := range entries {
		out[i] = e.User
	}
	return out
}

// RepositoryMainBranch returns the name of a repository's main branch.
func (c *Client) RepositoryMainBranch(ctx context.Context, workspace, slug string) (string, error) {
	var repo struct {
		MainBranch *struct {
			Name string `json:"name"`
		} `json:"mainbranch"`
	}
	if err := c.Do(ctx, http.MethodGet, RepoPath(workspace, slug), nil, &repo); err != nil {
		return "", err
	}
	if repo.MainBranch == nil || repo.MainBranch.Name == "" {
		return "", fmt.Errorf("repository %s/%s has no main branch", workspace, slug)
	}
	return repo.MainBranch.Name, nil
}

// EffectiveDefaultReviewers returns the default reviewers of a repository, including its project's.
func (c *Client) EffectiveDefaultReviewers(ctx context.Context, workspace, slug string) ([]User, error) {
	entries, err := List[userEntry](ctx, c, RepoPath(workspace, slug, "effective-default-reviewers"), 0)
	if err != nil {
		return nil, err
	}
	return toUsers(entries), nil
}

// FindWorkspaceMembers returns the members of a workspace matching a BBQL query ("" means every
// member). Bitbucket filters on user.uuid, user.account_id and user.nickname, not on display_name.
func (c *Client) FindWorkspaceMembers(ctx context.Context, workspace, query string) ([]User, error) {
	path := "workspaces/" + url.PathEscape(workspace) + "/members"
	if query != "" {
		path += "?" + url.Values{"q": {query}}.Encode()
	}
	entries, err := List[userEntry](ctx, c, path, 0)
	if err != nil {
		return nil, err
	}
	return toUsers(entries), nil
}
