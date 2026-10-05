// Package factory builds the Factory used by the khbb binary.
package factory

import (
	"errors"
	"os"

	"github.com/cli/go-gh/v2/pkg/prompter"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
)

// New returns the production Factory.
func New(version, commit, date string) *cmdutil.Factory {
	ios := iostreams.System()
	if os.Getenv("KHBB_PROMPT_DISABLED") != "" {
		ios.SetNeverPrompt(true)
	}
	f := &cmdutil.Factory{
		AppVersion:  version,
		BuildCommit: commit,
		BuildDate:   date,
		IOStreams:   ios,
		// Prompts render on stderr so that stdout carries only data.
		Prompter: prompter.New(os.Stdin, os.Stderr, os.Stderr),
		Git:      gitctx.NewResolver(""),
	}
	f.Config = cachedConfig()
	f.HTTPClient = func() (*bitbucket.Client, error) { return newClient(f) }
	return f
}

func cachedConfig() func() (*config.Config, error) {
	var cfg *config.Config
	return func() (*config.Config, error) {
		if cfg != nil {
			return cfg, nil
		}
		c, err := config.Load()
		if err != nil {
			return nil, err
		}
		cfg = c
		return cfg, nil
	}
}

func newClient(f *cmdutil.Factory) (*bitbucket.Client, error) {
	cfg, err := f.Config()
	if err != nil {
		return nil, err
	}
	creds, err := config.ResolveCredentials(cfg)
	if errors.Is(err, config.ErrNoCredentials) {
		return nil, &cmdutil.AuthError{Msg: "not logged in to bitbucket.org"}
	}
	if err != nil {
		return nil, err
	}
	opts := bitbucket.Options{
		Email:     creds.Email,
		Token:     creds.Token,
		UserAgent: "khbb/" + f.AppVersion,
		DryRun:    f.DryRun,
		DryRunOut: f.IOStreams.Out,
	}
	if os.Getenv("KHBB_DEBUG") != "" {
		opts.Debug = f.IOStreams.ErrOut
	}
	return bitbucket.New(opts), nil
}
