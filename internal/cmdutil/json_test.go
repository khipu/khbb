package cmdutil_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
)

type sample struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
}

var samples = []sample{{1, "Add widgets", "OPEN"}, {2, "Fix gears", "MERGED"}}

func runJSON(t *testing.T, data any, args ...string) (string, error) {
	t.Helper()
	ios, _, out, _ := iostreams.Test()
	var exporter cmdutil.Exporter
	cmd := &cobra.Command{
		Use: "sample",
		RunE: func(*cobra.Command, []string) error {
			if exporter == nil {
				return errors.New("exporter not set")
			}
			return exporter.Write(ios, data)
		},
	}
	cmdutil.AddJSONFlags(cmd, &exporter, []string{"id", "title", "state"})
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	return out.String(), err
}

func TestJSON_FiltersFieldsOnList(t *testing.T) {
	out, err := runJSON(t, samples, "--json", "id,state")
	if err != nil {
		t.Fatal(err)
	}
	if want := `[{"id":1,"state":"OPEN"},{"id":2,"state":"MERGED"}]` + "\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

func TestJSON_SingleObject(t *testing.T) {
	out, err := runJSON(t, &samples[0], "--json", "title,id")
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"id":1,"title":"Add widgets"}` + "\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

func TestJSON_EmptyListIsArray(t *testing.T) {
	for _, data := range []any{[]sample{}, []sample(nil)} {
		out, err := runJSON(t, data, "--json", "id")
		if err != nil {
			t.Fatal(err)
		}
		if out != "[]\n" {
			t.Errorf("out = %q, want []", out)
		}
	}
}

func TestJSON_JQ(t *testing.T) {
	out, err := runJSON(t, samples, "--json", "title", "--jq", ".[].title")
	if err != nil {
		t.Fatal(err)
	}
	if out != "Add widgets\nFix gears\n" {
		t.Errorf("out = %q", out)
	}
}

func TestJSON_Template(t *testing.T) {
	out, err := runJSON(t, samples, "--json", "id,title", "--template", `{{range .}}#{{.id}} {{.title}}{{"\n"}}{{end}}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "#1 Add widgets\n#2 Fix gears\n" {
		t.Errorf("out = %q", out)
	}
}

func TestJSON_UnknownFieldListsAvailable(t *testing.T) {
	_, err := runJSON(t, samples, "--json", "author")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) {
		t.Fatalf("expected FlagError, got %v", err)
	}
	if !strings.Contains(err.Error(), `unknown JSON field: "author"`) || !strings.Contains(err.Error(), "  state") {
		t.Errorf("message = %q", err.Error())
	}
}

func TestJSON_NoFieldsListsAvailable(t *testing.T) {
	_, err := runJSON(t, samples, "--json")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "specify one or more comma-separated fields") || !strings.Contains(err.Error(), "  title") {
		t.Errorf("err = %v", err)
	}
}

func TestJSON_FilterFlagsRequireJSON(t *testing.T) {
	_, err := runJSON(t, samples, "--jq", ".")
	if err == nil || !strings.Contains(err.Error(), "without --json") {
		t.Errorf("err = %v", err)
	}
}

func TestJSON_NotRequestedLeavesExporterNil(t *testing.T) {
	_, err := runJSON(t, samples)
	if err == nil || err.Error() != "exporter not set" {
		t.Errorf("err = %v", err)
	}
}

func TestJSON_NilData(t *testing.T) {
	out, err := runJSON(t, nil, "--json", "id")
	if err != nil {
		t.Fatal(err)
	}
	if out != "null\n" {
		t.Errorf("out = %q, want %q", out, "null\n")
	}
}

func TestJSON_AddJSONFlagsSkipsTakenShorthand(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	var title string
	var exporter cmdutil.Exporter
	cmd := &cobra.Command{
		Use: "sample",
		RunE: func(*cobra.Command, []string) error {
			if exporter == nil {
				return errors.New("exporter not set")
			}
			return exporter.Write(ios, samples)
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "the title")
	cmdutil.AddJSONFlags(cmd, &exporter, []string{"id", "title", "state"})
	cmd.SetArgs([]string{"--json", "id", "--template", `{{range .}}#{{.id}}{{end}}`, "-t", "Hello"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if title != "Hello" {
		t.Errorf("title = %q, want %q (-t must stay bound to --title)", title, "Hello")
	}
	if out.String() != "#1#2" {
		t.Errorf("out = %q, want %q (--template should still work long-only)", out.String(), "#1#2")
	}
}

func TestJSON_InvalidFiltersAreRejectedBeforeRunning(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--json", "id", "--jq", ".["}, "invalid --jq expression"},
		{[]string{"--json", "id", "--template", "{{"}, "invalid --template"},
	} {
		_, err := runJSON(t, samples, tc.args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v", tc.args, err)
		}
	}
}
