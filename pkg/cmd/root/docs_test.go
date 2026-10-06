package root_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/root"
	"github.com/khipu/khbb/skills"
)

// The documentation tests keep the agent skill (and the README) in step with the command tree:
// every command is mentioned in the skill, and every khbb command line in their code blocks names
// a real command and only flags it accepts.

func newDocsRoot() *cobra.Command {
	ios, _, _, _ := iostreams.Test()
	cmd := root.NewCmdRoot(&cmdutil.Factory{AppVersion: "test", IOStreams: ios})
	cmd.InitDefaultCompletionCmd()
	return cmd
}

// leafCommands returns every visible command that has no subcommands, such as `khbb pr create`.
func leafCommands(c *cobra.Command) []*cobra.Command {
	if c.Hidden || c.Name() == "help" {
		return nil
	}
	if !c.HasSubCommands() {
		return []*cobra.Command{c}
	}
	var out []*cobra.Command
	for _, sub := range c.Commands() {
		out = append(out, leafCommands(sub)...)
	}
	return out
}

// codeLines returns the lines inside the fenced code blocks of a Markdown document, without a
// leading "$ " prompt.
func codeLines(doc string) []string {
	var lines []string
	inCode := false
	for _, line := range strings.Split(doc, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			lines = append(lines, strings.TrimPrefix(t, "$ "))
		}
	}
	return lines
}

var shellSeparator = regexp.MustCompile(`&&|\|\||;|\|`)

// commandLines returns the khbb invocations in a document's code blocks: each part of a code line,
// split at &&, ||, ; and |, that starts with "khbb ".
func commandLines(doc string) []string {
	var out []string
	for _, line := range codeLines(doc) {
		for _, part := range shellSeparator.Split(line, -1) {
			if part = strings.TrimSpace(part); strings.HasPrefix(part, "khbb ") {
				out = append(out, part)
			}
		}
	}
	return out
}

// checkCommandLine returns what is wrong with a khbb command line, or "" when it names a command
// and only flags that command accepts. Arguments are split on spaces, so quoted values must not
// start with "-".
func checkCommandLine(rootCmd *cobra.Command, line string) string {
	cmd, rest, err := rootCmd.Find(strings.Fields(line)[1:])
	if err != nil {
		return err.Error()
	}
	if cmd == rootCmd || cmd.HasSubCommands() {
		return "names no khbb command"
	}
	for _, arg := range rest {
		if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
			continue
		}
		if !hasFlag(cmd, arg) {
			return fmt.Sprintf("%s has no flag %s", cmd.CommandPath(), arg)
		}
	}
	return ""
}

func hasFlag(cmd *cobra.Command, arg string) bool {
	name, _, _ := strings.Cut(arg, "=")
	if name == "--help" || name == "-h" {
		return true
	}
	for _, set := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags()} {
		if long, ok := strings.CutPrefix(name, "--"); ok {
			if set.Lookup(long) != nil {
				return true
			}
		} else if short := name[1:]; len(short) == 1 && set.ShorthandLookup(short) != nil {
			return true
		}
	}
	return false
}

// checkDocCommands fails the test for every khbb command line in doc that checkCommandLine rejects.
func checkDocCommands(t *testing.T, name, doc string) {
	t.Helper()
	rootCmd := newDocsRoot()
	lines := commandLines(doc)
	if len(lines) == 0 {
		t.Fatalf("%s: no khbb command lines in code blocks", name)
	}
	for _, line := range lines {
		if problem := checkCommandLine(rootCmd, line); problem != "" {
			t.Errorf("%s: %q: %s", name, line, problem)
		}
	}
}

func TestCheckCommandLine(t *testing.T) {
	rootCmd := newDocsRoot()
	cases := map[string]string{
		"khbb pr list --state merged -L 5 --json id,title":    "",
		"khbb pr ls -R acme/widgets":                          "",
		"khbb pr view 42 --json comments --jq '.comments[]'":  "",
		"khbb pipeline logs --failed --tail 200 --step=Build": "",
		"khbb api user -X GET --help":                         "",
		"khbb nope":                                           "names no khbb command",
		"khbb pr nope":                                        "names no khbb command",
		"khbb pr":                                             "names no khbb command",
		"khbb pr list --nope":                                 "khbb pr list has no flag --nope",
		"khbb pr view -Z":                                     "khbb pr view has no flag -Z",
	}
	for line, want := range cases {
		if got := checkCommandLine(rootCmd, line); got != want {
			t.Errorf("%q: got %q, want %q", line, got, want)
		}
	}
}

func TestCommandLines(t *testing.T) {
	doc := "Run `khbb pr list` first.\n\n```bash\n$ git push && khbb pipeline watch --exit-status\nkhbb pr view --json id | jq .\n# khbb in a comment\n```\nkhbb outside a block\n"
	got := commandLines(doc)
	want := []string{"khbb pipeline watch --exit-status", "khbb pr view --json id"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("commandLines = %q, want %q", got, want)
	}
}

func TestSkillMentionsEveryCommand(t *testing.T) {
	for _, c := range leafCommands(newDocsRoot()) {
		mention := regexp.MustCompile("`" + regexp.QuoteMeta(c.CommandPath()) + "[ `]")
		if !mention.MatchString(skills.Khbb) {
			t.Errorf("skills/khbb/SKILL.md does not mention `%s`", c.CommandPath())
		}
	}
}

func TestSkillCommandLinesAreValid(t *testing.T) {
	checkDocCommands(t, "skills/khbb/SKILL.md", skills.Khbb)
}
