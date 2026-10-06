// Package login implements `khbb auth login`.
package login

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/iostreams"
)

const tokenURL = "https://id.atlassian.com/manage-profile/security/api-tokens"

// LoginOptions holds the inputs and dependencies of `khbb auth login`.
type LoginOptions struct {
	IO          *iostreams.IOStreams
	Config      func() (*config.Config, error)
	Prompter    cmdutil.Prompter
	NewClient   func(email, token string) *bitbucket.Client
	StoreToken  func(email, token string) error
	DeleteToken func(email string) error

	Email           string
	WithToken       bool
	InsecureStorage bool
}

// NewCmdLogin returns `khbb auth login`.
func NewCmdLogin(f *cmdutil.Factory, runF func(*LoginOptions) error) *cobra.Command {
	opts := &LoginOptions{
		IO:       f.IOStreams,
		Config:   f.Config,
		Prompter: f.Prompter,
		NewClient: func(email, token string) *bitbucket.Client {
			return bitbucket.New(bitbucket.Options{Email: email, Token: token, UserAgent: "khbb/" + f.AppVersion, Notices: f.IOStreams.ErrOut})
		},
		StoreToken:  config.StoreToken,
		DeleteToken: config.DeleteToken,
	}
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to Bitbucket Cloud with an Atlassian API token",
		Long: fmt.Sprintf(`Authenticate with Bitbucket Cloud using an Atlassian API token.

Create a token at %s
("Create API token with scopes", app: Bitbucket) with these scopes:
  %s

The token is stored in the system keyring. To use credentials from the
environment instead, set KHBB_EMAIL and KHBB_TOKEN.`, tokenURL, strings.Join(bitbucket.RequiredScopes, "\n  ")),
		Example: `  # Interactive
  $ khbb auth login

  # Non-interactive (scripts, agents)
  $ echo "$TOKEN" | khbb auth login --email you@example.com --with-token`,
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return loginRun(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Email, "email", "", "Atlassian account `email`")
	cmd.Flags().BoolVar(&opts.WithToken, "with-token", false, "Read the API token from standard input")
	cmd.Flags().BoolVar(&opts.InsecureStorage, "insecure-storage", false, "Store the token in plain text in the config file instead of the system keyring")
	return cmd
}

func loginRun(ctx context.Context, opts *LoginOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	if os.Getenv("KHBB_TOKEN") != "" {
		fmt.Fprintln(opts.IO.ErrOut, "warning: KHBB_TOKEN is set and takes precedence over stored credentials")
	}

	if !opts.WithToken && opts.IO.CanPrompt() {
		printTokenInstructions(opts.IO.ErrOut)
	}

	email, err := readEmail(opts, cfg.Email)
	if err != nil {
		return err
	}
	token, err := readToken(opts)
	if err != nil {
		return err
	}

	user, err := opts.NewClient(email, token).CurrentUser(ctx)
	if err != nil {
		var httpErr *bitbucket.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized {
			return &cmdutil.AuthError{Msg: "authentication failed: check the email and the API token"}
		}
		return err
	}

	previous := cfg.Email
	if opts.InsecureStorage {
		cfg.InsecureToken = token
	} else {
		if err := opts.StoreToken(email, token); err != nil {
			return fmt.Errorf("could not store the token in the system keyring: %w\n"+
				"set KHBB_EMAIL and KHBB_TOKEN instead, or rerun with --insecure-storage", err)
		}
		cfg.InsecureToken = ""
	}
	cfg.Email = email
	cfg.Username = user.Nickname
	if err := cfg.Save(); err != nil {
		return err
	}

	if previous != "" && previous != email {
		if err := opts.DeleteToken(previous); err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "warning: could not remove the previous token for %s from the system keyring: %v\n", previous, err)
		}
	}
	if opts.InsecureStorage {
		// A keyring copy would take precedence over the new file token; the keyring may be unavailable here, so this is best effort.
		_ = opts.DeleteToken(email)
	}

	fmt.Fprintf(opts.IO.ErrOut, "Logged in to bitbucket.org as %s\n", user.Nickname)
	if opts.InsecureStorage {
		fmt.Fprintf(opts.IO.ErrOut, "warning: the token is stored in plain text in %s\n", cfg.Path())
	}
	return nil
}

func readEmail(opts *LoginOptions, current string) (string, error) {
	email := strings.TrimSpace(opts.Email)
	if email != "" {
		return email, nil
	}
	if !opts.IO.CanPrompt() {
		return "", cmdutil.FlagErrorf("--email is required when not running interactively")
	}
	answer, err := opts.Prompter.Input("Atlassian account email:", current)
	if err != nil {
		return "", err
	}
	if email = strings.TrimSpace(answer); email == "" {
		return "", cmdutil.FlagErrorf("email cannot be empty")
	}
	return email, nil
}

func readToken(opts *LoginOptions) (string, error) {
	var token string
	switch {
	case opts.WithToken:
		b, err := io.ReadAll(opts.IO.In)
		if err != nil {
			return "", fmt.Errorf("reading the token from standard input: %w", err)
		}
		token = string(b)
	case opts.IO.CanPrompt():
		answer, err := opts.Prompter.Password("API token:")
		if err != nil {
			return "", err
		}
		token = answer
	default:
		return "", cmdutil.FlagErrorf("--with-token is required when not running interactively")
	}
	if token = strings.TrimSpace(token); token == "" {
		return "", cmdutil.FlagErrorf("the API token cannot be empty")
	}
	return token, nil
}

func printTokenInstructions(w io.Writer) {
	fmt.Fprintf(w, "Create an API token at %s\nwith these scopes:\n", tokenURL)
	for _, s := range bitbucket.RequiredScopes {
		fmt.Fprintf(w, "  - %s\n", s)
	}
}
