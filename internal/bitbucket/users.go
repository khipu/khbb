package bitbucket

import (
	"context"
	"net/http"
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
