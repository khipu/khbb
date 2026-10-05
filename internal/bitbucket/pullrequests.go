package bitbucket

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// PullRequest is a Bitbucket pull request as returned by the API. List responses omit
// Reviewers and Participants unless PRListOptions.WithParticipants is set.
type PullRequest struct {
	ID                int           `json:"id"`
	Title             string        `json:"title"`
	Description       string        `json:"description"`
	State             string        `json:"state"`
	Draft             bool          `json:"draft"`
	Author            *User         `json:"author"`
	Source            PREndpoint    `json:"source"`
	Destination       PREndpoint    `json:"destination"`
	MergeCommit       *Commit       `json:"merge_commit"`
	Reviewers         []User        `json:"reviewers"`
	Participants      []Participant `json:"participants"`
	CommentCount      int           `json:"comment_count"`
	TaskCount         int           `json:"task_count"`
	CloseSourceBranch bool          `json:"close_source_branch"`
	ClosedBy          *User         `json:"closed_by"`
	CreatedOn         time.Time     `json:"created_on"`
	UpdatedOn         time.Time     `json:"updated_on"`
	Links             struct {
		HTML Link `json:"html"`
	} `json:"links"`
}

// PREndpoint is the source or destination of a pull request.
type PREndpoint struct {
	Branch struct {
		Name string `json:"name"`
	} `json:"branch"`
	Commit     *Commit `json:"commit"`
	Repository *struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// Commit identifies a commit.
type Commit struct {
	Hash string `json:"hash"`
}

// Link is a hyperlink in an API response.
type Link struct {
	Href string `json:"href"`
}

// Participant is a user's involvement in a pull request. State is "approved",
// "changes_requested" or nil.
type Participant struct {
	User     User    `json:"user"`
	Role     string  `json:"role"`
	Approved bool    `json:"approved"`
	State    *string `json:"state"`
}

// Comment is a pull request comment. Inline is set for comments on a file; Parent for replies.
type Comment struct {
	ID      int `json:"id"`
	Content struct {
		Raw string `json:"raw"`
	} `json:"content"`
	User   *User `json:"user"`
	Inline *struct {
		Path string `json:"path"`
		From *int   `json:"from"`
		To   *int   `json:"to"`
	} `json:"inline"`
	Parent *struct {
		ID int `json:"id"`
	} `json:"parent"`
	Deleted   bool      `json:"deleted"`
	CreatedOn time.Time `json:"created_on"`
	UpdatedOn time.Time `json:"updated_on"`
	Links     struct {
		HTML Link `json:"html"`
	} `json:"links"`
}

// CommitStatus is a build status reported on a commit (pipelines and external CI).
// State is SUCCESSFUL, FAILED, INPROGRESS or STOPPED.
type CommitStatus struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	State       string    `json:"state"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	UpdatedOn   time.Time `json:"updated_on"`
}

// DiffStat summarizes the change to one file. Old is nil for added files, New for removed ones.
type DiffStat struct {
	Status       string `json:"status"`
	LinesAdded   int    `json:"lines_added"`
	LinesRemoved int    `json:"lines_removed"`
	Old          *struct {
		Path string `json:"path"`
	} `json:"old"`
	New *struct {
		Path string `json:"path"`
	} `json:"new"`
}

// PRListOptions filters ListPullRequests.
type PRListOptions struct {
	States           []string // OPEN, MERGED, DECLINED, SUPERSEDED; empty means Bitbucket's default (OPEN)
	Query            string   // BBQL expression
	WithParticipants bool     // include reviewers and participants in each item
}

// ListPullRequests lists a repository's pull requests (limit <= 0 means all).
func (c *Client) ListPullRequests(ctx context.Context, workspace, slug string, opts PRListOptions, limit int) ([]PullRequest, error) {
	q := url.Values{}
	for _, s := range opts.States {
		q.Add("state", s)
	}
	if opts.Query != "" {
		q.Set("q", opts.Query)
	}
	if opts.WithParticipants {
		q.Set("fields", "+values.participants,+values.reviewers")
	}
	path := RepoPath(workspace, slug, "pullrequests")
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	return List[PullRequest](ctx, c, path, limit)
}

// GetPullRequest returns one pull request, including reviewers and participants.
func (c *Client) GetPullRequest(ctx context.Context, workspace, slug string, id int) (*PullRequest, error) {
	var pr PullRequest
	if err := c.Do(ctx, http.MethodGet, RepoPath(workspace, slug, "pullrequests", strconv.Itoa(id)), nil, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// ListPRComments returns every comment on a pull request.
func (c *Client) ListPRComments(ctx context.Context, workspace, slug string, id int) ([]Comment, error) {
	return List[Comment](ctx, c, RepoPath(workspace, slug, "pullrequests", strconv.Itoa(id), "comments"), 0)
}

// ListPRStatuses returns the commit statuses reported on a pull request.
func (c *Client) ListPRStatuses(ctx context.Context, workspace, slug string, id int) ([]CommitStatus, error) {
	return List[CommitStatus](ctx, c, RepoPath(workspace, slug, "pullrequests", strconv.Itoa(id), "statuses"), 0)
}

// PRDiffStat returns the per-file summary of a pull request's changes.
func (c *Client) PRDiffStat(ctx context.Context, workspace, slug string, id int) ([]DiffStat, error) {
	return List[DiffStat](ctx, c, RepoPath(workspace, slug, "pullrequests", strconv.Itoa(id), "diffstat"), 0)
}

// PRDiff returns a pull request's unified diff.
func (c *Client) PRDiff(ctx context.Context, workspace, slug string, id int) (string, error) {
	return c.GetText(ctx, RepoPath(workspace, slug, "pullrequests", strconv.Itoa(id), "diff"))
}

// PRPatch returns a pull request's changes as git patches.
func (c *Client) PRPatch(ctx context.Context, workspace, slug string, id int) (string, error) {
	return c.GetText(ctx, RepoPath(workspace, slug, "pullrequests", strconv.Itoa(id), "patch"))
}
