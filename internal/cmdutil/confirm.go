package cmdutil

import "github.com/khipu/khbb/internal/iostreams"

// ConfirmDestructive guards irreversible actions. With yes it proceeds; on a terminal it asks
// question (default No); otherwise it refuses with ErrConfirmationRequired. Callers pass
// yes || dryRun, since a dry run sends nothing.
func ConfirmDestructive(ios *iostreams.IOStreams, p Prompter, yes bool, question string) error {
	if yes {
		return nil
	}
	if !ios.CanPrompt() || p == nil {
		return ErrConfirmationRequired
	}
	ok, err := p.Confirm(question, false)
	if err != nil {
		return err
	}
	if !ok {
		return ErrCancel
	}
	return nil
}
