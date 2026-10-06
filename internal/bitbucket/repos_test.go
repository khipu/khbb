package bitbucket_test

import (
	"context"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

func TestRepositoryMainBranch(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/repositories/acme/widgets", httpmock.JSONResponse(200, `{"mainbranch":{"name":"main","type":"branch"}}`))
	reg.Register("GET", "/2.0/repositories/acme/widgets", httpmock.JSONResponse(200, `{"mainbranch":null}`))

	if name, err := c.RepositoryMainBranch(context.Background(), "acme", "widgets"); err != nil || name != "main" {
		t.Errorf("name %q err %v", name, err)
	}
	if _, err := c.RepositoryMainBranch(context.Background(), "acme", "widgets"); err == nil || !strings.Contains(err.Error(), "no main branch") {
		t.Errorf("err = %v", err)
	}
}

func TestEffectiveDefaultReviewers(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/repositories/acme/widgets/effective-default-reviewers", httpmock.JSONResponse(200,
		`{"values":[{"type":"default_reviewer","reviewer_type":"repository","user":`+userJSON+`}]}`))

	users, err := c.EffectiveDefaultReviewers(context.Background(), "acme", "widgets")
	if err != nil || len(users) != 1 || users[0].Nickname != "ada" {
		t.Errorf("users %+v err %v", users, err)
	}
}

func TestFindWorkspaceMembers(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/workspaces/acme/members", httpmock.JSONResponse(200, `{"values":[{"type":"workspace_membership","user":`+userJSON+`}]}`))
	reg.Register("GET", "/2.0/workspaces/acme/members", httpmock.JSONResponse(200, `{"values":[]}`))

	users, err := c.FindWorkspaceMembers(context.Background(), "acme", `user.nickname = "ada"`)
	if err != nil || len(users) != 1 || users[0].UUID != "{00000000-0000-0000-0000-000000000001}" {
		t.Fatalf("users %+v err %v", users, err)
	}
	if q := reg.Calls[0].URL.Query().Get("q"); q != `user.nickname = "ada"` {
		t.Errorf("q = %q", q)
	}
	if _, err := c.FindWorkspaceMembers(context.Background(), "acme", ""); err != nil {
		t.Fatal(err)
	}
	if reg.Calls[1].URL.Query().Has("q") {
		t.Errorf("an empty query must not send q: %s", reg.Calls[1].URL.RawQuery)
	}
}
