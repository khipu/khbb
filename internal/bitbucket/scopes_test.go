package bitbucket_test

import (
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
)

func TestScopeFor(t *testing.T) {
	cases := []struct{ method, url, want string }{
		{"GET", "user", "read:user:bitbucket"},
		{"GET", "workspaces/acme/members", "read:workspace:bitbucket"},
		{"GET", "repositories/acme/widgets/pullrequests/1", "read:pullrequest:bitbucket"},
		{"POST", "https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests/1/merge", "write:pullrequest:bitbucket"},
		{"GET", "repositories/acme/widgets/effective-default-reviewers", "read:pullrequest:bitbucket"},
		{"GET", "https://api.bitbucket.org/2.0/repositories/acme/widgets/pipelines/", "read:pipeline:bitbucket"},
		{"POST", "repositories/acme/widgets/pipelines/", "write:pipeline:bitbucket"},
		{"GET", "repositories/acme/widgets", "read:repository:bitbucket"},
		{"PUT", "repositories/acme/widgets", "admin:repository:bitbucket"},
		{"POST", "repositories/acme/widgets/src", "write:repository:bitbucket"},
		{"GET", "snippets/acme", ""},
	}
	for _, tc := range cases {
		if got := bitbucket.ScopeFor(tc.method, tc.url); got != tc.want {
			t.Errorf("ScopeFor(%s, %s) = %q, want %q", tc.method, tc.url, got, tc.want)
		}
	}
}

func TestRequiredScopes(t *testing.T) {
	want := []string{
		"read:user:bitbucket",
		"read:workspace:bitbucket",
		"read:repository:bitbucket",
		"read:pullrequest:bitbucket",
		"write:pullrequest:bitbucket",
		"read:pipeline:bitbucket",
		"write:pipeline:bitbucket",
	}
	if len(bitbucket.RequiredScopes) != len(want) {
		t.Fatalf("RequiredScopes = %v", bitbucket.RequiredScopes)
	}
	for i := range want {
		if bitbucket.RequiredScopes[i] != want[i] {
			t.Errorf("RequiredScopes[%d] = %q, want %q", i, bitbucket.RequiredScopes[i], want[i])
		}
	}
}
