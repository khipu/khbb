package cmdutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/jq"
	"github.com/cli/go-gh/v2/pkg/jsonpretty"
	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/iostreams"
)

// Exporter writes command output as JSON, optionally filtered by --jq or --template.
type Exporter interface {
	Fields() []string
	Write(ios *iostreams.IOStreams, data any) error
}

type jsonExporter struct {
	fields   []string
	jq       string
	template string
}

// AddJSONFlags registers --json, --jq and --template on cmd. After flag parsing,
// *exporter is non-nil if and only if --json was given.
func AddJSONFlags(cmd *cobra.Command, exporter *Exporter, fields []string) {
	flags := cmd.Flags()
	flags.StringSlice("json", nil, "Output JSON with the specified `fields`")
	if flags.ShorthandLookup("q") == nil {
		flags.StringP("jq", "q", "", "Filter JSON output using a jq `expression`")
	} else {
		flags.String("jq", "", "Filter JSON output using a jq `expression`")
	}
	if flags.ShorthandLookup("t") == nil {
		flags.StringP("template", "t", "", "Format JSON output using a Go `template`")
	} else {
		flags.String("template", "", "Format JSON output using a Go `template`")
	}

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		jqExpr, _ := flags.GetString("jq")
		tmpl, _ := flags.GetString("template")
		if !flags.Changed("json") {
			if jqExpr != "" || tmpl != "" {
				return FlagErrorf("cannot use --jq or --template without --json")
			}
			return nil
		}
		if jqExpr != "" && tmpl != "" {
			return FlagErrorf("cannot use --jq and --template together")
		}
		requested, _ := flags.GetStringSlice("json")
		if len(requested) == 0 {
			return FlagErrorf("specify one or more comma-separated fields for `--json`:\n%s", fieldList(fields))
		}
		for _, r := range requested {
			if !slices.Contains(fields, r) {
				return FlagErrorf("unknown JSON field: %q\navailable fields:\n%s", r, fieldList(fields))
			}
		}
		*exporter = &jsonExporter{fields: requested, jq: jqExpr, template: tmpl}
		return nil
	}

	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		if c == cmd && strings.Contains(err.Error(), "flag needs an argument") && strings.Contains(err.Error(), "--json") {
			return FlagErrorf("specify one or more comma-separated fields for `--json`:\n%s", fieldList(fields))
		}
		if p := c.Parent(); p != nil {
			return p.FlagErrorFunc()(c, err)
		}
		return &FlagError{Err: err}
	})
}

func fieldList(fields []string) string {
	return "  " + strings.Join(slices.Sorted(slices.Values(fields)), "\n  ")
}

func (e *jsonExporter) Fields() []string { return e.fields }

func (e *jsonExporter) Write(ios *iostreams.IOStreams, data any) error {
	filtered, err := filterFields(data, e.fields)
	if err != nil {
		return err
	}
	buf, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	switch {
	case e.jq != "":
		return jq.EvaluateFormatted(bytes.NewReader(buf), ios.Out, e.jq, "  ", ios.ColorEnabled())
	case e.template != "":
		t := template.New(ios.Out, ios.TerminalWidth(), ios.ColorEnabled())
		if err := t.Parse(e.template); err != nil {
			return err
		}
		if err := t.Execute(bytes.NewReader(buf)); err != nil {
			return err
		}
		return t.Flush()
	case ios.IsStdoutTTY():
		return jsonpretty.Format(ios.Out, bytes.NewReader(buf), "  ", ios.ColorEnabled())
	default:
		_, err := fmt.Fprintf(ios.Out, "%s\n", buf)
		return err
	}
}

// filterFields turns data (a struct, a pointer, or a slice of them) into maps holding only fields.
// Slices always become JSON arrays, never null.
func filterFields(data any, fields []string) (any, error) {
	v := reflect.ValueOf(data)
	if !v.IsValid() {
		return nil, nil
	}
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Slice {
		out := make([]map[string]any, v.Len())
		for i := range v.Len() {
			m, err := pick(v.Index(i).Interface(), fields)
			if err != nil {
				return nil, err
			}
			out[i] = m
		}
		return out, nil
	}
	return pick(v.Interface(), fields)
}

func pick(item any, fields []string) (map[string]any, error) {
	b, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var all map[string]any
	if err := dec.Decode(&all); err != nil {
		return nil, err
	}
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		out[f] = all[f]
	}
	return out, nil
}
