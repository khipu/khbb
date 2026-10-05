package iostreams

// colorize wraps text in an ANSI SGR sequence when color output is enabled.
func (s *IOStreams) colorize(code, text string) string {
	if !s.colorEnabled || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[m"
}

func (s *IOStreams) Bold(text string) string   { return s.colorize("1", text) }
func (s *IOStreams) Green(text string) string  { return s.colorize("32", text) }
func (s *IOStreams) Red(text string) string    { return s.colorize("31", text) }
func (s *IOStreams) Yellow(text string) string { return s.colorize("33", text) }
func (s *IOStreams) Cyan(text string) string   { return s.colorize("36", text) }
func (s *IOStreams) Gray(text string) string   { return s.colorize("90", text) }
