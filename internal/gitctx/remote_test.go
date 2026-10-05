package gitctx

import "testing"

func TestParseRemoteURL(t *testing.T) {
	cases := []struct {
		in        string
		host      string
		repo      Repo
		ssh       bool
		expectErr bool
	}{
		{in: "git@bitbucket.org:acme/widgets.git", host: "bitbucket.org", repo: Repo{"acme", "widgets"}, ssh: true},
		{in: "ssh://git@bitbucket.org/acme/widgets.git", host: "bitbucket.org", repo: Repo{"acme", "widgets"}, ssh: true},
		{in: "ssh://git@bitbucket.org:22/acme/widgets.git", host: "bitbucket.org", repo: Repo{"acme", "widgets"}, ssh: true},
		{in: "https://dev@bitbucket.org/acme/widgets.git", host: "bitbucket.org", repo: Repo{"acme", "widgets"}},
		{in: "https://bitbucket.org/acme/widgets", host: "bitbucket.org", repo: Repo{"acme", "widgets"}},
		{in: "https://bitbucket.org/acme/widgets/", host: "bitbucket.org", repo: Repo{"acme", "widgets"}},
		{in: "git@Bitbucket.org:acme/widgets.git", host: "bitbucket.org", repo: Repo{"acme", "widgets"}, ssh: true},
		{in: "git@bb-work:acme/widgets.git", host: "bb-work", repo: Repo{"acme", "widgets"}, ssh: true},
		{in: "git@github.com:acme/widgets.git", host: "github.com", repo: Repo{"acme", "widgets"}, ssh: true},
		{in: "/srv/git/widgets.git", expectErr: true},
		{in: "https://bitbucket.org/acme", expectErr: true},
		{in: `C:\repos\widgets`, expectErr: true},
	}
	for _, tc := range cases {
		host, repo, ssh, err := parseRemoteURL(tc.in)
		if tc.expectErr {
			if err == nil {
				t.Errorf("parseRemoteURL(%q): expected error, got %s %v", tc.in, host, repo)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseRemoteURL(%q): %v", tc.in, err)
			continue
		}
		if host != tc.host || repo != tc.repo || ssh != tc.ssh {
			t.Errorf("parseRemoteURL(%q) = %q %v ssh=%v, want %q %v ssh=%v", tc.in, host, repo, ssh, tc.host, tc.repo, tc.ssh)
		}
	}
}

func TestParseRepo(t *testing.T) {
	r, err := ParseRepo("acme/widgets")
	if err != nil || r.FullName() != "acme/widgets" {
		t.Errorf("ParseRepo = %v, %v", r, err)
	}
	for _, bad := range []string{"", "acme", "acme/", "/widgets", "a/b/c"} {
		if _, err := ParseRepo(bad); err == nil {
			t.Errorf("ParseRepo(%q): expected error", bad)
		}
	}
}
