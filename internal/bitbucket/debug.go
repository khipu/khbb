package bitbucket

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"
)

func (c *Client) debugRequest(req *http.Request) {
	if c.opts.Debug == nil {
		return
	}
	fmt.Fprintf(c.opts.Debug, "> %s %s\n", req.Method, req.URL)
	for _, k := range slices.Sorted(maps.Keys(req.Header)) {
		v := strings.Join(req.Header.Values(k), ", ")
		if strings.EqualFold(k, "Authorization") {
			v = "[REDACTED]"
		}
		fmt.Fprintf(c.opts.Debug, "> %s: %s\n", k, v)
	}
}

func (c *Client) debugResponse(resp *http.Response, elapsed time.Duration) {
	if c.opts.Debug == nil {
		return
	}
	fmt.Fprintf(c.opts.Debug, "< %s (%s)\n", resp.Status, elapsed.Round(time.Millisecond))
}
