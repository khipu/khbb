// Package list implements `khbb pr list`.
package list

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// ListOptions holds the inputs and dependencies of `khbb pr list`.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Exporter   cmdutil.Exporter

	State    string
	Author   string
	Reviewer string
	Base     string
	Head     string
	Query    string
	Limit    int
}

var stateValues = map[string][]string{
	"open":       {"OPEN"},
	"merged":     {"MERGED"},
	"declined":   {"DECLINED"},
	"superseded": {"SUPERSEDED"},
	"all":        {"OPEN", "MERGED", "DECLINED", "SUPERSEDED"},
}

// NewCmdList returns `khbb pr list`.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo}
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List pull requests in a repository",
		Example: `  $ khbb pr list
  $ khbb pr list --state merged --author @me --limit 10
  $ khbb pr list --reviewer @me --json id,title,url
  $ khbb pr list --query 'title ~ "hotfix"'`,
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.State = strings.ToLower(opts.State)
			if _, ok := stateValues[opts.State]; !ok {
				return cmdutil.FlagErrorf("invalid --state %q: use open, merged, declined, superseded or all", opts.State)
			}
			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid --limit %d: must be at least 1", opts.Limit)
			}
			if runF != nil {
				return runF(opts)
			}
			return listRun(cmd.Context(), opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.State, "state", "s", "open", "Filter by `state`: open, merged, declined, superseded or all")
	fl.StringVarP(&opts.Author, "author", "A", "", "Filter by author: @me, a {uuid}, an account ID or a nickname")
	fl.StringVar(&opts.Reviewer, "reviewer", "", "Filter by reviewer: @me, a {uuid}, an account ID or a nickname")
	fl.StringVarP(&opts.Base, "base", "B", "", "Filter by destination `branch`")
	fl.StringVarP(&opts.Head, "head", "H", "", "Filter by source `branch`")
	fl.StringVar(&opts.Query, "query", "", "Extra BBQL `expression`, combined with the other filters using AND")
	fl.IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of pull requests to fetch")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.PullRequestFields)
	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	query, err := buildQuery(ctx, client, opts)
	if err != nil {
		return err
	}
	raws, err := client.ListPullRequests(ctx, repo.Workspace, repo.Slug, bitbucket.PRListOptions{
		States:           stateValues[opts.State],
		Query:            query,
		WithParticipants: needsParticipants(opts.Exporter),
	}, opts.Limit)
	if err != nil {
		return err
	}
	prs := make([]shared.PullRequest, len(raws))
	for i := range raws {
		prs[i] = shared.NewPullRequest(&raws[i])
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, prs)
	}
	if len(prs) == 0 {
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "No pull requests match your filters in %s\n", repo.FullName())
		}
		return nil
	}
	return printTable(opts.IO, prs)
}

func needsParticipants(e cmdutil.Exporter) bool {
	if e == nil {
		return false
	}
	return slices.Contains(e.Fields(), "reviewers") || slices.Contains(e.Fields(), "participants")
}

func buildQuery(ctx context.Context, client *bitbucket.Client, opts *ListOptions) (string, error) {
	var clauses []string
	for _, filter := range []struct{ field, who string }{{"author", opts.Author}, {"reviewers", opts.Reviewer}} {
		if filter.who == "" {
			continue
		}
		clause, err := shared.UserClause(ctx, client, filter.field, filter.who)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, clause)
	}
	if opts.Base != "" {
		clauses = append(clauses, "destination.branch.name = "+bitbucket.QuoteBBQL(opts.Base))
	}
	if opts.Head != "" {
		clauses = append(clauses, "source.branch.name = "+bitbucket.QuoteBBQL(opts.Head))
	}
	if opts.Query != "" {
		clauses = append(clauses, "("+opts.Query+")")
	}
	return strings.Join(clauses, " AND "), nil
}

func printTable(ios *iostreams.IOStreams, prs []shared.PullRequest) error {
	tty := ios.IsStdoutTTY()
	tp := tableprinter.New(ios.Out, tty, ios.TerminalWidth())
	tp.AddHeader([]string{"ID", "TITLE", "BRANCH", "AUTHOR", "UPDATED"})
	for _, pr := range prs {
		if tty {
			tp.AddField("#"+strconv.Itoa(pr.ID), tableprinter.WithColor(shared.StateColor(ios, pr)))
			tp.AddField(pr.Title)
			tp.AddField(pr.SourceBranch + " → " + pr.DestinationBranch)
			tp.AddField(shared.AuthorName(pr))
			tp.AddField(pr.UpdatedOn.Format("2006-01-02"))
		} else {
			tp.AddField(strconv.Itoa(pr.ID))
			tp.AddField(pr.Title)
			tp.AddField(pr.SourceBranch)
			tp.AddField(pr.DestinationBranch)
			tp.AddField(pr.State)
			tp.AddField(shared.AuthorName(pr))
			tp.AddField(pr.UpdatedOn.Format(time.RFC3339))
		}
		tp.EndRow()
	}
	return tp.Render()
}
