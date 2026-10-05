// Package logout implements `khbb auth logout`.
package logout

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/config"
	"github.com/khipu/khbb/internal/iostreams"
)

// LogoutOptions holds the dependencies of `khbb auth logout`.
type LogoutOptions struct {
	IO          *iostreams.IOStreams
	Config      func() (*config.Config, error)
	DeleteToken func(email string) error
}

// NewCmdLogout returns `khbb auth logout`.
func NewCmdLogout(f *cmdutil.Factory, runF func(*LogoutOptions) error) *cobra.Command {
	opts := &LogoutOptions{IO: f.IOStreams, Config: f.Config, DeleteToken: config.DeleteToken}
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored Bitbucket credentials",
		Args:  cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return logoutRun(opts)
		},
	}
}

func logoutRun(opts *LogoutOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	if cfg.Email == "" && cfg.InsecureToken == "" {
		return errors.New("not logged in to bitbucket.org")
	}
	if cfg.Email != "" {
		// With insecure storage the keyring may be unavailable altogether; the file token is what matters.
		if err := opts.DeleteToken(cfg.Email); err != nil && cfg.InsecureToken == "" {
			return fmt.Errorf("removing the token from the system keyring: %w", err)
		}
	}
	name := cfg.Username
	if name == "" {
		name = cfg.Email
	}
	cfg.Email, cfg.Username, cfg.InsecureToken = "", "", ""
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "Logged out of bitbucket.org (%s)\n", name)
	if os.Getenv("KHBB_TOKEN") != "" {
		fmt.Fprintln(opts.IO.ErrOut, "warning: KHBB_TOKEN is still set and will keep authenticating khbb")
	}
	return nil
}
