// Package auth groups the `khbb auth` commands.
package auth

import (
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/pkg/cmd/auth/login"
	"github.com/khipu/khbb/pkg/cmd/auth/logout"
	"github.com/khipu/khbb/pkg/cmd/auth/status"
)

// NewCmdAuth returns `khbb auth`.
func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth <command>",
		Short: "Authenticate khbb with Bitbucket Cloud",
	}
	cmd.AddCommand(
		login.NewCmdLogin(f, nil),
		logout.NewCmdLogout(f, nil),
		status.NewCmdStatus(f, nil),
	)
	return cmd
}
