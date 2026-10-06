package bitbucket

import (
	"context"
	"net/http"
	"strconv"
)

// PRCreate describes a new pull request. Source and Destination are branch names; Reviewers are
// account UUIDs.
type PRCreate struct {
	Title             string
	Description       string
	Source            string
	Destination       string
	Draft             bool
	CloseSourceBranch bool
	Reviewers         []string
}

// PRUpdate lists the fields of a pull request to change; nil fields keep their value.
type PRUpdate struct {
	Title       *string
	Description *string
	Destination *string
	Draft       *bool
	// Reviewers replaces the reviewer list when non-nil; an empty, non-nil slice removes everyone.
	Reviewers []string
}

// PRCommentInput describes a new comment. Path alone makes a file comment, Path with Line a comment
// on that line of the new version, and ParentID a reply.
type PRCommentInput struct {
	Body     string
	Path     string
	Line     int
	ParentID int
}

func prPath(workspace, slug string, id int, segments ...string) string {
	return RepoPath(workspace, slug, append([]string{"pullrequests", strconv.Itoa(id)}, segments...)...)
}

func branchRef(name string) map[string]any {
	return map[string]any{"branch": map[string]any{"name": name}}
}

func userRefs(uuids []string) []map[string]string {
	refs := make([]map[string]string, len(uuids))
	for i, u := range uuids {
		refs[i] = map[string]string{"uuid": u}
	}
	return refs
}

// CreatePullRequest opens a pull request. Bitbucket answers a second request for the same source
// and destination by retitling the open pull request, so callers check for one first.
func (c *Client) CreatePullRequest(ctx context.Context, workspace, slug string, in PRCreate) (*PullRequest, error) {
	body := map[string]any{
		"title":               in.Title,
		"description":         in.Description,
		"source":              branchRef(in.Source),
		"destination":         branchRef(in.Destination),
		"draft":               in.Draft,
		"close_source_branch": in.CloseSourceBranch,
		"reviewers":           userRefs(in.Reviewers),
	}
	var pr PullRequest
	if err := c.Do(ctx, http.MethodPost, RepoPath(workspace, slug, "pullrequests"), body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// UpdatePullRequest changes the given fields. Bitbucket's PUT is partial: fields left out keep their value.
func (c *Client) UpdatePullRequest(ctx context.Context, workspace, slug string, id int, in PRUpdate) (*PullRequest, error) {
	body := map[string]any{}
	if in.Title != nil {
		body["title"] = *in.Title
	}
	if in.Description != nil {
		body["description"] = *in.Description
	}
	if in.Destination != nil {
		body["destination"] = branchRef(*in.Destination)
	}
	if in.Draft != nil {
		body["draft"] = *in.Draft
	}
	if in.Reviewers != nil {
		body["reviewers"] = userRefs(in.Reviewers)
	}
	var pr PullRequest
	if err := c.Do(ctx, http.MethodPut, prPath(workspace, slug, id), body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// CreatePRComment adds a comment to a pull request.
func (c *Client) CreatePRComment(ctx context.Context, workspace, slug string, id int, in PRCommentInput) (*Comment, error) {
	body := map[string]any{"content": map[string]any{"raw": in.Body}}
	if in.Path != "" {
		inline := map[string]any{"path": in.Path}
		if in.Line > 0 {
			inline["to"] = in.Line
		}
		body["inline"] = inline
	}
	if in.ParentID > 0 {
		body["parent"] = map[string]any{"id": in.ParentID}
	}
	var cm Comment
	if err := c.Do(ctx, http.MethodPost, prPath(workspace, slug, id, "comments"), body, &cm); err != nil {
		return nil, err
	}
	return &cm, nil
}

// ApprovePR approves a pull request as the authenticated user.
func (c *Client) ApprovePR(ctx context.Context, workspace, slug string, id int) error {
	return c.Do(ctx, http.MethodPost, prPath(workspace, slug, id, "approve"), nil, nil)
}

// UnapprovePR withdraws the authenticated user's approval.
func (c *Client) UnapprovePR(ctx context.Context, workspace, slug string, id int) error {
	return c.Do(ctx, http.MethodDelete, prPath(workspace, slug, id, "approve"), nil, nil)
}

// RequestChangesPR requests changes on a pull request as the authenticated user.
func (c *Client) RequestChangesPR(ctx context.Context, workspace, slug string, id int) error {
	return c.Do(ctx, http.MethodPost, prPath(workspace, slug, id, "request-changes"), nil, nil)
}

// DeclinePR declines a pull request; a non-empty message becomes its reason. Declined pull requests
// cannot be reopened.
func (c *Client) DeclinePR(ctx context.Context, workspace, slug string, id int, message string) (*PullRequest, error) {
	var in any
	if message != "" {
		in = map[string]string{"message": message}
	}
	var pr PullRequest
	if err := c.Do(ctx, http.MethodPost, prPath(workspace, slug, id, "decline"), in, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}
