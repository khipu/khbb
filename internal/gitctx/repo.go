// Package gitctx works out which Bitbucket repository and branch a command targets.
package gitctx

import (
	"fmt"
	"strings"
)

// Repo identifies a Bitbucket repository.
type Repo struct {
	Workspace string
	Slug      string
}

// FullName returns WORKSPACE/REPO.
func (r Repo) FullName() string { return r.Workspace + "/" + r.Slug }

// ParseRepo parses WORKSPACE/REPO.
func ParseRepo(s string) (Repo, error) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Repo{}, fmt.Errorf("invalid repository %q: expected WORKSPACE/REPO", s)
	}
	return Repo{Workspace: parts[0], Slug: parts[1]}, nil
}
