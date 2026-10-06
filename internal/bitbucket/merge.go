package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// PRMerge describes how to merge a pull request. An empty Strategy lets Bitbucket use merge_commit.
type PRMerge struct {
	Strategy          string // merge_commit, squash, fast_forward, …
	Message           string
	CloseSourceBranch bool
}

// MergeStrategies are the merge strategies a pull request's destination branch allows, and its default.
type MergeStrategies struct {
	Allowed []string
	Default string
}

// MergeCheck is one of Bitbucket's merge checks. A FAILED check with Blocking set prevents the merge.
type MergeCheck struct {
	Type     string `json:"type"`
	Status   string `json:"status"` // PASSED or FAILED
	Reason   string `json:"reason"` // such as "clean" or "conflicts"; often null
	Required bool   `json:"required"`
	Blocking bool   `json:"blocking"`
}

// StartMerge asks Bitbucket to merge a pull request asynchronously. It returns the merged pull
// request when Bitbucket finishes at once, or else the URL of the merge task to poll with
// MergeTaskStatus. close_source_branch is always sent: when omitted, Bitbucket falls back to the
// pull request's own setting and may delete the branch.
func (c *Client) StartMerge(ctx context.Context, workspace, slug string, id int, in PRMerge) (*PullRequest, string, error) {
	body := map[string]any{"type": "pullrequest", "close_source_branch": in.CloseSourceBranch}
	if in.Strategy != "" {
		body["merge_strategy"] = in.Strategy
	}
	if in.Message != "" {
		body["message"] = in.Message
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("encoding request body: %w", err)
	}
	resp, err := c.Request(ctx, http.MethodPost, prPath(workspace, slug, id, "merge")+"?async=true", nil, b)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusAccepted:
		_, _ = io.Copy(io.Discard, resp.Body)
		loc := resp.Header.Get("Location")
		if loc == "" {
			return nil, "", fmt.Errorf("the merge of pull request #%d was accepted without a task location", id)
		}
		return nil, loc, nil
	case resp.StatusCode >= 200 && resp.StatusCode <= 299:
		var pr PullRequest
		if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
			return nil, "", fmt.Errorf("decoding the merged pull request: %w", err)
		}
		return &pr, "", nil
	}
	return nil, "", readHTTPError(resp)
}

// MergeTaskStatus polls a merge task. It reports done=false while the task has no result; note
// that Bitbucket answers PENDING forever for an unknown task, so callers need a deadline. A merge
// that fails inside the task comes back as an *HTTPError, like a failed synchronous merge.
func (c *Client) MergeTaskStatus(ctx context.Context, taskURL string) (*PullRequest, bool, error) {
	var st struct {
		TaskStatus  string       `json:"task_status"`
		MergeResult *PullRequest `json:"merge_result"`
	}
	if err := c.Do(ctx, http.MethodGet, taskURL, nil, &st); err != nil {
		return nil, false, err
	}
	if st.MergeResult == nil {
		return nil, false, nil
	}
	return st.MergeResult, true, nil
}

// PRMergeStrategies returns the merge strategies allowed into a pull request's destination branch
// and its default. Bitbucket includes them only when asked through the fields parameter.
func (c *Client) PRMergeStrategies(ctx context.Context, workspace, slug string, id int) (MergeStrategies, error) {
	var pr struct {
		Destination struct {
			Branch struct {
				MergeStrategies      []string `json:"merge_strategies"`
				DefaultMergeStrategy string   `json:"default_merge_strategy"`
			} `json:"branch"`
		} `json:"destination"`
	}
	path := prPath(workspace, slug, id) + "?" + url.Values{"fields": {"destination.branch.*"}}.Encode()
	if err := c.Do(ctx, http.MethodGet, path, nil, &pr); err != nil {
		return MergeStrategies{}, err
	}
	b := pr.Destination.Branch
	return MergeStrategies{Allowed: b.MergeStrategies, Default: b.DefaultMergeStrategy}, nil
}

// PRMergeabilityChecks returns the merge checks of a pull request.
func (c *Client) PRMergeabilityChecks(ctx context.Context, workspace, slug string, id int) ([]MergeCheck, error) {
	var res struct {
		Values []MergeCheck `json:"values"`
	}
	if err := c.Do(ctx, http.MethodGet, prPath(workspace, slug, id, "mergeability", "checks"), nil, &res); err != nil {
		return nil, err
	}
	return res.Values, nil
}
