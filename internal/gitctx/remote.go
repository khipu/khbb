package gitctx

import (
	"fmt"
	"net/url"
	"strings"
)

const bitbucketHost = "bitbucket.org"

// Remote is a git remote and its fetch URL.
type Remote struct {
	Name string
	URL  string
}

// parseRemoteURL extracts the lowercase host and WORKSPACE/REPO from a git remote URL.
// isSSH is true for SSH URLs, whose host may be an alias from ~/.ssh/config.
func parseRemoteURL(raw string) (host string, repo Repo, isSSH bool, err error) {
	raw = strings.TrimSpace(raw)
	var path string
	switch {
	case strings.Contains(raw, "://"):
		u, perr := url.Parse(raw)
		if perr != nil {
			return "", Repo{}, false, perr
		}
		host, path = u.Hostname(), u.Path
		isSSH = u.Scheme == "ssh" || u.Scheme == "git+ssh"
	case strings.Index(raw, ":") > 1: // scp-like user@host:path; index 1 would be a Windows drive letter
		i := strings.Index(raw, ":")
		hostPart := raw[:i]
		if at := strings.LastIndex(hostPart, "@"); at >= 0 {
			hostPart = hostPart[at+1:]
		}
		host, path, isSSH = hostPart, raw[i+1:], true
	default:
		return "", Repo{}, false, fmt.Errorf("unsupported remote URL %q", raw)
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	repo, err = ParseRepo(path)
	if err != nil {
		return "", Repo{}, false, err
	}
	return strings.ToLower(host), repo, isSSH, nil
}
