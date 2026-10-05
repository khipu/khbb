package bitbucket

import (
	"net/url"
	"strings"
)

// RequiredScopes lists the API token scopes khbb v1 needs.
var RequiredScopes = []string{
	"read:user:bitbucket",
	"read:workspace:bitbucket",
	"read:repository:bitbucket",
	"read:pullrequest:bitbucket",
	"write:pullrequest:bitbucket",
	"read:pipeline:bitbucket",
	"write:pipeline:bitbucket",
}

// ScopeFor returns the token scope a request most likely needs, or "" when unknown.
// It only feeds hints on 403 responses; Bitbucket's own "required" list takes precedence.
func ScopeFor(method, rawURL string) string {
	p := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		p = u.Path
	}
	p = strings.TrimPrefix(strings.TrimPrefix(p, "/"), "2.0/")
	write := isMutating(method)
	access := func(resource string) string {
		if write {
			return "write:" + resource + ":bitbucket"
		}
		return "read:" + resource + ":bitbucket"
	}
	repo := strings.HasPrefix(p, "repositories/")
	switch {
	case p == "user" || strings.HasPrefix(p, "user/"):
		return access("user")
	case strings.HasPrefix(p, "workspaces/") && strings.Contains(p, "/members"):
		return "read:workspace:bitbucket"
	case repo && (strings.Contains(p, "/pullrequests") || strings.Contains(p, "default-reviewers")):
		return access("pullrequest")
	case repo && strings.Contains(p, "/pipelines"):
		return access("pipeline")
	case repo && !write:
		return "read:repository:bitbucket"
	case repo && strings.Count(strings.TrimSuffix(p, "/"), "/") <= 2:
		return "admin:repository:bitbucket"
	case repo:
		return "write:repository:bitbucket"
	}
	return ""
}
