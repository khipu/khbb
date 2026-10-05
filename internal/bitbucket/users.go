package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// User is a Bitbucket account as returned by the API.
type User struct {
	DisplayName string `json:"display_name"`
	Nickname    string `json:"nickname"`
	UUID        string `json:"uuid"`
	AccountID   string `json:"account_id"`
}

// CurrentUser returns the account the client authenticates as.
func (c *Client) CurrentUser(ctx context.Context) (*User, error) {
	var u User
	if err := c.Do(ctx, http.MethodGet, "user", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// WhoAmI returns the authenticated account and the scopes granted to its token, read from the
// X-Oauth-Scopes response header (nil when Bitbucket does not send it).
func (c *Client) WhoAmI(ctx context.Context) (*User, []string, error) {
	resp, err := c.Request(ctx, http.MethodGet, "user", nil, nil)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, nil, readHTTPError(resp)
	}
	var u User
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, nil, fmt.Errorf("decoding the current user: %w", err)
	}
	return &u, splitScopes(resp.Header.Get("X-Oauth-Scopes")), nil
}

func splitScopes(header string) []string {
	var scopes []string
	for _, s := range strings.Split(header, ",") {
		if s = strings.TrimSpace(s); s != "" {
			scopes = append(scopes, s)
		}
	}
	return scopes
}

// MissingScopes returns the RequiredScopes that granted lacks, in RequiredScopes order.
func MissingScopes(granted []string) []string {
	var missing []string
	for _, s := range RequiredScopes {
		if !slices.Contains(granted, s) {
			missing = append(missing, s)
		}
	}
	return missing
}
