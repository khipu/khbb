package shared

import (
	"context"
	"fmt"
	"time"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
	prshared "github.com/khipu/khbb/pkg/cmd/pr/shared"
)

// ReportStarted reports a pipeline that was just started: a confirmation on stderr, then its URL
// on stdout — or its JSON with --json.
func ReportStarted(ios *iostreams.IOStreams, exporter cmdutil.Exporter, p Pipeline, what string) error {
	prshared.PrintSuccess(ios, "Started pipeline #%d (%s)", p.Number, what)
	if exporter != nil {
		return exporter.Write(ios, p)
	}
	_, err := fmt.Fprintln(ios.Out, p.URL)
	return err
}

// Started maps a pipeline that was just started. Bitbucket fills in the selector only after parsing
// bitbucket-pipelines.yml, so the requested selector stands in until then.
func Started(raw *bitbucket.Pipeline, requested *bitbucket.PipelineSelector, repo gitctx.Repo) Pipeline {
	if raw.Target.Selector == nil {
		raw.Target.Selector = requested
	}
	return NewPipeline(raw, repo)
}

// WatchStarted follows a pipeline that was just started until it finishes, and turns an
// unsuccessful end into exit status 1 (`run --watch`, `rerun --watch`).
func WatchStarted(ctx context.Context, ios *iostreams.IOStreams, client *bitbucket.Client, repo gitctx.Repo, number int, sleep func(time.Duration)) error {
	final, _, err := Watch(ctx, WatchOptions{IO: ios, Client: client, Repo: repo, Number: number, Interval: 5 * time.Second, Sleep: sleep})
	if err != nil {
		return err
	}
	return ExitStatus(final)
}
