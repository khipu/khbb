package bitbucket_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/httpmock"
)

func TestWhoAmI_ReturnsGrantedScopes(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/user", httpmock.WithHeader(httpmock.JSONResponse(200, userJSON), "X-Oauth-Scopes", "read:user:bitbucket, read:pullrequest:bitbucket"))
	u, scopes, err := c.WhoAmI(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Nickname != "ada" || !slices.Equal(scopes, []string{"read:user:bitbucket", "read:pullrequest:bitbucket"}) {
		t.Errorf("user %+v scopes %v", u, scopes)
	}
}

func TestWhoAmI_WithoutScopeHeader(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, userJSON))
	_, scopes, err := c.WhoAmI(context.Background())
	if err != nil || scopes != nil {
		t.Errorf("scopes %v err %v", scopes, err)
	}
}

func TestWhoAmI_Unauthorized(t *testing.T) {
	c, reg := newTestClient(t, bitbucket.Options{})
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(401, `{}`))
	_, _, err := c.WhoAmI(context.Background())
	var httpErr *bitbucket.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 401 {
		t.Errorf("err = %v", err)
	}
}

func TestMissingScopes(t *testing.T) {
	if got := bitbucket.MissingScopes([]string{"read:user:bitbucket"}); !slices.Equal(got, bitbucket.RequiredScopes[1:]) {
		t.Errorf("MissingScopes = %v", got)
	}
	if got := bitbucket.MissingScopes(bitbucket.RequiredScopes); len(got) != 0 {
		t.Errorf("MissingScopes(all) = %v", got)
	}
}
