package bitbucket

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type page[T any] struct {
	Values []T    `json:"values"`
	Next   string `json:"next"`
}

// List follows Bitbucket's `next` links and collects up to limit values (limit <= 0 means all).
func List[T any](ctx context.Context, c *Client, path string, limit int) ([]T, error) {
	next := withPagelen(path, limit)
	all := []T{}
	for next != "" {
		var p page[T]
		if err := c.Do(ctx, http.MethodGet, next, nil, &p); err != nil {
			return nil, err
		}
		all = append(all, p.Values...)
		if limit > 0 && len(all) >= limit {
			return all[:limit], nil
		}
		next = p.Next
	}
	return all, nil
}

func withPagelen(path string, limit int) string {
	if strings.Contains(path, "pagelen=") {
		return path
	}
	n := 50
	if limit > 0 && limit < n {
		n = limit
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%spagelen=%d", path, sep, n)
}
