package shared

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
)

// ReviewerResolver turns --reviewer values into workspace members.
type ReviewerResolver struct {
	Client    *bitbucket.Client
	Workspace string

	members []bitbucket.User // every member, loaded once for display-name lookups
	loaded  bool
}

// Resolve returns the one workspace member named by who: @me, a {uuid}, an account ID, a nickname
// or a display name. Bitbucket cannot filter members by display name, so a name that matches no
// account ID or nickname is looked up in the full member list.
func (r *ReviewerResolver) Resolve(ctx context.Context, who string) (bitbucket.User, error) {
	who = strings.TrimSpace(who)
	if who == "" {
		return bitbucket.User{}, cmdutil.FlagErrorf("empty reviewer")
	}
	if who == "@me" {
		me, err := r.Client.CurrentUser(ctx)
		if err != nil {
			return bitbucket.User{}, err
		}
		return *me, nil
	}
	isUUID := strings.HasPrefix(who, "{") && strings.HasSuffix(who, "}")
	query := "user.uuid = " + bitbucket.QuoteBBQL(who)
	if !isUUID {
		q := bitbucket.QuoteBBQL(who)
		query = "(user.account_id = " + q + " OR user.nickname = " + q + ")"
	}
	found, err := r.Client.FindWorkspaceMembers(ctx, r.Workspace, query)
	if err != nil {
		return bitbucket.User{}, err
	}
	if len(found) == 0 && !isUUID {
		if found, err = r.byDisplayName(ctx, who); err != nil {
			return bitbucket.User{}, err
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return bitbucket.User{}, &cmdutil.NotFoundError{Msg: fmt.Sprintf("no member of workspace %s matches reviewer %q", r.Workspace, who)}
	}
	names := make([]string, len(found))
	for i, u := range found {
		names[i] = fmt.Sprintf("%s (%s, %s)", u.Nickname, u.DisplayName, u.UUID)
	}
	return bitbucket.User{}, cmdutil.FlagErrorf("reviewer %q matches several members of workspace %s: %s; use a nickname or {uuid}",
		who, r.Workspace, strings.Join(names, "; "))
}

// ResolveAll resolves every value and returns the members' UUIDs in order.
func (r *ReviewerResolver) ResolveAll(ctx context.Context, values []string) ([]string, error) {
	uuids := make([]string, 0, len(values))
	for _, v := range values {
		u, err := r.Resolve(ctx, v)
		if err != nil {
			return nil, err
		}
		uuids = append(uuids, u.UUID)
	}
	return uuids, nil
}

func (r *ReviewerResolver) byDisplayName(ctx context.Context, name string) ([]bitbucket.User, error) {
	if !r.loaded {
		all, err := r.Client.FindWorkspaceMembers(ctx, r.Workspace, "")
		if err != nil {
			return nil, err
		}
		r.members, r.loaded = all, true
	}
	var found []bitbucket.User
	for _, u := range r.members {
		if strings.EqualFold(u.DisplayName, name) {
			found = append(found, u)
		}
	}
	return found, nil
}

// UniqueUUIDs returns uuids in order without duplicates and without exclude, comparing case-insensitively.
// The result is never nil.
func UniqueUUIDs(uuids []string, exclude ...string) []string {
	out := []string{}
	seen := func(list []string, u string) bool {
		return slices.ContainsFunc(list, func(v string) bool { return strings.EqualFold(v, u) })
	}
	for _, u := range uuids {
		if seen(exclude, u) || seen(out, u) {
			continue
		}
		out = append(out, u)
	}
	return out
}
