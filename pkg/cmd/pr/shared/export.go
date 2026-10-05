// Package shared holds what the `khbb pr` commands have in common: JSON shapes, pull request
// lookup and display helpers.
package shared

import (
	"strings"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
)

// User is the stable JSON shape of a Bitbucket account.
type User struct {
	DisplayName string `json:"displayName"`
	Nickname    string `json:"nickname"`
	UUID        string `json:"uuid"`
	AccountID   string `json:"accountId"`
}

// Reviewer is a requested reviewer and their decision: approved, changes_requested or pending.
type Reviewer struct {
	User  User   `json:"user"`
	State string `json:"state"`
}

// Participant is anyone involved in a pull request. Role is reviewer or participant.
type Participant struct {
	User     User   `json:"user"`
	Role     string `json:"role"`
	Approved bool   `json:"approved"`
	State    string `json:"state"`
}

// PullRequest is the stable JSON shape of a pull request (spec §8.1).
type PullRequest struct {
	ID                int           `json:"id"`
	Title             string        `json:"title"`
	Body              string        `json:"body"`
	State             string        `json:"state"`
	Draft             bool          `json:"draft"`
	Author            *User         `json:"author"`
	SourceBranch      string        `json:"sourceBranch"`
	SourceCommit      string        `json:"sourceCommit"`
	SourceRepo        string        `json:"sourceRepo"`
	DestinationBranch string        `json:"destinationBranch"`
	DestinationCommit string        `json:"destinationCommit"`
	MergeCommit       string        `json:"mergeCommit"`
	Reviewers         []Reviewer    `json:"reviewers"`
	Participants      []Participant `json:"participants"`
	CommentCount      int           `json:"commentCount"`
	TaskCount         int           `json:"taskCount"`
	CloseSourceBranch bool          `json:"closeSourceBranch"`
	URL               string        `json:"url"`
	CreatedOn         time.Time     `json:"createdOn"`
	UpdatedOn         time.Time     `json:"updatedOn"`
	ClosedBy          *User         `json:"closedBy"`
}

// PullRequestFields lists PullRequest's JSON fields in spec order.
var PullRequestFields = []string{
	"id", "title", "body", "state", "draft", "author",
	"sourceBranch", "sourceCommit", "sourceRepo", "destinationBranch", "destinationCommit", "mergeCommit",
	"reviewers", "participants", "commentCount", "taskCount", "closeSourceBranch",
	"url", "createdOn", "updatedOn", "closedBy",
}

// Comment is the stable JSON shape of a pull request comment.
type Comment struct {
	ID        int       `json:"id"`
	Author    *User     `json:"author"`
	Body      string    `json:"body"`
	Path      string    `json:"path"`
	Line      *int      `json:"line"`
	ParentID  *int      `json:"parentId"`
	Deleted   bool      `json:"deleted"`
	URL       string    `json:"url"`
	CreatedOn time.Time `json:"createdOn"`
	UpdatedOn time.Time `json:"updatedOn"`
}

// CommentFields lists Comment's JSON fields.
var CommentFields = []string{"id", "author", "body", "path", "line", "parentId", "deleted", "url", "createdOn", "updatedOn"}

// Check is the stable JSON shape of a commit status. State is successful, failed, inprogress or stopped.
type Check struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	State       string    `json:"state"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	UpdatedOn   time.Time `json:"updatedOn"`
}

// CheckFields lists Check's JSON fields.
var CheckFields = []string{"key", "name", "state", "description", "url", "updatedOn"}

// NewUser maps an API account; nil stays nil.
func NewUser(u *bitbucket.User) *User {
	if u == nil {
		return nil
	}
	return &User{DisplayName: u.DisplayName, Nickname: u.Nickname, UUID: u.UUID, AccountID: u.AccountID}
}

// NewPullRequest maps an API pull request. Reviewers and Participants are never nil.
func NewPullRequest(pr *bitbucket.PullRequest) PullRequest {
	out := PullRequest{
		ID:                pr.ID,
		Title:             pr.Title,
		Body:              pr.Description,
		State:             pr.State,
		Draft:             pr.Draft,
		Author:            NewUser(pr.Author),
		SourceBranch:      pr.Source.Branch.Name,
		DestinationBranch: pr.Destination.Branch.Name,
		Reviewers:         []Reviewer{},
		Participants:      []Participant{},
		CommentCount:      pr.CommentCount,
		TaskCount:         pr.TaskCount,
		CloseSourceBranch: pr.CloseSourceBranch,
		URL:               pr.Links.HTML.Href,
		CreatedOn:         pr.CreatedOn.UTC(),
		UpdatedOn:         pr.UpdatedOn.UTC(),
		ClosedBy:          NewUser(pr.ClosedBy),
	}
	if pr.Source.Commit != nil {
		out.SourceCommit = pr.Source.Commit.Hash
	}
	if pr.Source.Repository != nil {
		out.SourceRepo = pr.Source.Repository.FullName
	}
	if pr.Destination.Commit != nil {
		out.DestinationCommit = pr.Destination.Commit.Hash
	}
	if pr.MergeCommit != nil {
		out.MergeCommit = pr.MergeCommit.Hash
	}
	states := map[string]string{}
	for _, p := range pr.Participants {
		state := participantState(p)
		states[p.User.UUID] = state
		out.Participants = append(out.Participants, Participant{
			User: *NewUser(&p.User), Role: strings.ToLower(p.Role), Approved: p.Approved, State: state,
		})
	}
	for _, r := range pr.Reviewers {
		state := states[r.UUID]
		if state == "" {
			state = "pending"
		}
		out.Reviewers = append(out.Reviewers, Reviewer{User: *NewUser(&r), State: state})
	}
	return out
}

func participantState(p bitbucket.Participant) string {
	switch {
	case p.State != nil && *p.State != "":
		return *p.State
	case p.Approved:
		return "approved"
	}
	return "pending"
}

// NewComment maps an API comment. Line is the new-side line, else the old-side line.
func NewComment(c *bitbucket.Comment) Comment {
	out := Comment{
		ID:        c.ID,
		Author:    NewUser(c.User),
		Body:      c.Content.Raw,
		Deleted:   c.Deleted,
		URL:       c.Links.HTML.Href,
		CreatedOn: c.CreatedOn.UTC(),
		UpdatedOn: c.UpdatedOn.UTC(),
	}
	if c.Inline != nil {
		out.Path = c.Inline.Path
		switch {
		case c.Inline.To != nil:
			out.Line = c.Inline.To
		case c.Inline.From != nil:
			out.Line = c.Inline.From
		}
	}
	if c.Parent != nil {
		id := c.Parent.ID
		out.ParentID = &id
	}
	return out
}

// NewCheck maps an API commit status, lower-casing its state.
func NewCheck(s *bitbucket.CommitStatus) Check {
	return Check{
		Key:         s.Key,
		Name:        s.Name,
		State:       strings.ToLower(s.State),
		Description: s.Description,
		URL:         s.URL,
		UpdatedOn:   s.UpdatedOn.UTC(),
	}
}
