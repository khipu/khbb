// Package status implements `khbb auth status`.
package status

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/iostreams"
)

// StatusOptions holds the dependencies of `khbb auth status`.
type StatusOptions struct {
	IO        *iostreams.IOStreams
	Config    func() (*config.Config, error)
	NewClient func(email, token string) *bitbucket.Client
	Resolve   func(*config.Config) (config.Credentials, error)
}

// NewCmdStatus returns `khbb auth status`.
func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{
		IO:     f.IOStreams,
		Config: f.Config,
		NewClient: func(email, token string) *bitbucket.Client {
			return bitbucket.New(bitbucket.Options{Email: email, Token: token, UserAgent: "khbb/" + f.AppVersion})
		},
		Resolve: config.ResolveCredentials,
	}
	return &cobra.Command{
		Use:   "status",
		Short: "Show and verify the current authentication",
		Long:  "Show which account khbb uses and verify the token. Exits with status 4 when not logged in or when the token is rejected.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return statusRun(cmd.Context(), opts)
		},
	}
}

func statusRun(ctx context.Context, opts *StatusOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	creds, err := opts.Resolve(cfg)
	if errors.Is(err, config.ErrNoCredentials) {
		return &cmdutil.AuthError{Msg: "not logged in to bitbucket.org"}
	}
	if err != nil {
		return err
	}
	user, err := opts.NewClient(creds.Email, creds.Token).CurrentUser(ctx)
	if err != nil {
		var httpErr *bitbucket.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized {
			return &cmdutil.AuthError{Msg: fmt.Sprintf("the token for %s (from %s) is invalid or expired", creds.Email, creds.Source)}
		}
		return err
	}
	fmt.Fprintf(opts.IO.Out, "bitbucket.org\n  Logged in as %s (%s)\n  Email: %s\n  Token source: %s\n",
		user.Nickname, user.DisplayName, creds.Email, creds.Source)
	return nil
}
