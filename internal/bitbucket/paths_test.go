package bitbucket_test

import (
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
)

func TestRepoPath(t *testing.T) {
	cases := map[string]string{
		bitbucket.RepoPath("acme", "widgets", "pullrequests", "12"):          "repositories/acme/widgets/pullrequests/12",
		bitbucket.RepoPath("acme", "widgets", "pipelines", "{abc}", "steps"): "repositories/acme/widgets/pipelines/%7Babc%7D/steps",
		bitbucket.RepoPath("acme", "widgets"):                                "repositories/acme/widgets",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("RepoPath = %q, want %q", got, want)
		}
	}
}

func TestQuoteBBQL(t *testing.T) {
	if got, want := bitbucket.QuoteBBQL(`fix/"quoted"\x`), `"fix/\"quoted\"\\x"`; got != want {
		t.Errorf("QuoteBBQL = %s, want %s", got, want)
	}
}
