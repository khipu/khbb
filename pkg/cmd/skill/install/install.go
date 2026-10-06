// Package install implements `khbb skill install`.
package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/skills"
)

// InstallOptions holds the dependencies and flags of `khbb skill install`.
type InstallOptions struct {
	IO      *iostreams.IOStreams
	HomeDir func() (string, error)
	Content string

	Dir   string
	Force bool
}

// NewCmdInstall returns `khbb skill install`.
func NewCmdInstall(f *cmdutil.Factory, runF func(*InstallOptions) error) *cobra.Command {
	opts := &InstallOptions{IO: f.IOStreams, HomeDir: os.UserHomeDir, Content: skills.Khbb}
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the khbb skill for AI coding agents",
		Long: `Write the khbb agent skill, a SKILL.md file that teaches AI coding agents such as Claude Code
to use khbb: JSON output, exit statuses, and which commands need a human's approval. The skill
matches this version of khbb.

The skill is written to ~/.claude/skills/khbb/SKILL.md, or into the directory given with --dir,
such as a repository's .claude/skills/khbb. An existing SKILL.md with other content is only
replaced with --force: run "khbb skill install --force" after upgrading khbb.`,
		Example: `  $ khbb skill install
  $ khbb skill install --force
  $ khbb skill install --dir .claude/skills/khbb`,
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return installRun(opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", "", "Write SKILL.md into this `directory` (default: ~/.claude/skills/khbb)")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Replace an existing SKILL.md that differs")
	return cmd
}

func installRun(opts *InstallOptions) error {
	dir := opts.Dir
	if dir == "" {
		home, err := opts.HomeDir()
		if err != nil {
			return fmt.Errorf("could not find your home directory (%w); use --dir", err)
		}
		dir = filepath.Join(home, ".claude", "skills", "khbb")
	}
	path := filepath.Join(dir, "SKILL.md")

	existing, err := os.ReadFile(path)
	exists := err == nil
	switch {
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("reading %s: %w", path, err)
	case exists && string(existing) == opts.Content:
		fmt.Fprintf(opts.IO.ErrOut, "The khbb skill at %s is already up to date\n", path)
		return nil
	case exists && !opts.Force:
		return &cmdutil.ConflictError{Msg: fmt.Sprintf("%s already exists and differs from this version's skill; rerun with --force to replace it", path)}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(opts.Content), 0o644); err != nil {
		return err
	}
	verb := "Installed"
	if exists {
		verb = "Updated"
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s the khbb skill at %s\n", verb, path)
	return nil
}
