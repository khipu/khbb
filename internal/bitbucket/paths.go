package bitbucket

import (
	"net/url"
	"strings"
)

// RepoPath builds "repositories/{workspace}/{slug}/{segments...}" with every element path-escaped.
func RepoPath(workspace, slug string, segments ...string) string {
	parts := []string{"repositories", url.PathEscape(workspace), url.PathEscape(slug)}
	for _, s := range segments {
		parts = append(parts, url.PathEscape(s))
	}
	return strings.Join(parts, "/")
}

// QuoteBBQL returns s as a double-quoted BBQL string literal.
func QuoteBBQL(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
