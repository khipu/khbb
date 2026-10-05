package bitbucket

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// GetText fetches a non-JSON resource (diffs, patches, logs). It sends Accept: */* because some
// of these endpoints answer 406 to Accept: application/json.
func (c *Client) GetText(ctx context.Context, path string) (string, error) {
	resp, err := c.Request(ctx, http.MethodGet, path, http.Header{"Accept": []string{"*/*"}}, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", readHTTPError(resp)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return string(b), nil
}
