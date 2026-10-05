// Package gitctx works out which Bitbucket repository and branch a command targets.
package gitctx

import (
	"fmt"
	"regexp"
	"strings"
)

// Repo identifies a Bitbucket repository.
type Repo struct {
	Workspace string
	Slug      string
}

// FullName returns WORKSPACE/REPO.
func (r Repo) FullName() string { return r.Workspace + "/" + r.Slug }

var repoPartRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ParseRepo parses WORKSPACE/REPO. Each part may contain letters, digits, '.', '_' and '-'.
func ParseRepo(s string) (Repo, error) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 2 || !validRepoPart(parts[0]) || !validRepoPart(parts[1]) {
		return Repo{}, fmt.Errorf("invalid repository %q: expected WORKSPACE/REPO", s)
	}
	return Repo{Workspace: parts[0], Slug: parts[1]}, nil
}

func validRepoPart(p string) bool {
	return p != "." && p != ".." && repoPartRE.MatchString(p)
}
