package shared

import (
	"context"
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
)

// UserClause turns a user filter into a BBQL clause on field ("author" or "reviewers").
// who is @me, a {uuid}, an account ID or a nickname. Atlassian account IDs exist without a ':',
// so anything that is not @me or a {uuid} matches either field to avoid guessing wrong.
func UserClause(ctx context.Context, client *bitbucket.Client, field, who string) (string, error) {
	who = strings.TrimSpace(who)
	switch {
	case who == "@me":
		me, err := client.CurrentUser(ctx)
		if err != nil {
			return "", err
		}
		return field + ".uuid = " + bitbucket.QuoteBBQL(me.UUID), nil
	case strings.HasPrefix(who, "{") && strings.HasSuffix(who, "}"):
		return field + ".uuid = " + bitbucket.QuoteBBQL(who), nil
	}
	q := bitbucket.QuoteBBQL(who)
	return "(" + field + ".account_id = " + q + " OR " + field + ".nickname = " + q + ")", nil
}
