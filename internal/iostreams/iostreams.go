// Package iostreams wraps the standard streams together with terminal capabilities.
package iostreams

import (
	"bytes"
	"io"
	"os"

	"github.com/cli/go-gh/v2/pkg/term"
)

// IOStreams bundles stdin, stdout and stderr with what khbb knows about the terminal.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer

	stdinTTY     bool
	stdoutTTY    bool
	stderrTTY    bool
	colorEnabled bool
	neverPrompt  bool
	width        int
}

// System returns IOStreams bound to the process's standard streams.
func System() *IOStreams {
	t := term.FromEnv()
	width := 80
	if w, _, err := t.Size(); err == nil && w > 0 {
		width = w
	}
	return &IOStreams{
		In:           os.Stdin,
		Out:          t.Out(),
		ErrOut:       t.ErrOut(),
		stdinTTY:     term.IsTerminal(os.Stdin),
		stdoutTTY:    t.IsTerminalOutput(),
		stderrTTY:    term.IsTerminal(os.Stderr),
		colorEnabled: t.IsColorEnabled(),
		width:        width,
	}
}

// Test returns non-TTY IOStreams backed by buffers, plus the stdin, stdout and stderr buffers.
func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in, out, errOut := &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}
	return &IOStreams{In: in, Out: out, ErrOut: errOut, width: 80}, in, out, errOut
}

func (s *IOStreams) IsStdinTTY() bool  { return s.stdinTTY }
func (s *IOStreams) IsStdoutTTY() bool { return s.stdoutTTY }
func (s *IOStreams) IsStderrTTY() bool { return s.stderrTTY }

func (s *IOStreams) SetStdinTTY(v bool)  { s.stdinTTY = v }
func (s *IOStreams) SetStdoutTTY(v bool) { s.stdoutTTY = v }
func (s *IOStreams) SetStderrTTY(v bool) { s.stderrTTY = v }

func (s *IOStreams) ColorEnabled() bool     { return s.colorEnabled }
func (s *IOStreams) SetColorEnabled(v bool) { s.colorEnabled = v }

// SetNeverPrompt disables interactive prompts even on a terminal (KHBB_PROMPT_DISABLED).
func (s *IOStreams) SetNeverPrompt(v bool) { s.neverPrompt = v }

// CanPrompt reports whether khbb may ask the user questions.
func (s *IOStreams) CanPrompt() bool {
	return s.stdinTTY && s.stdoutTTY && !s.neverPrompt
}

// TerminalWidth returns the terminal width in columns (80 when unknown).
func (s *IOStreams) TerminalWidth() int { return s.width }
