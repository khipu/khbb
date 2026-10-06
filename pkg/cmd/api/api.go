// Package api implements `khbb api`.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/jq"
	"github.com/cli/go-gh/v2/pkg/jsonpretty"
	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/spf13/cobra"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/gitctx"
	"github.com/khipu/khbb/internal/iostreams"
)

// APIOptions holds the inputs and dependencies of `khbb api`.
type APIOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*bitbucket.Client, error)
	BaseRepo   func() (gitctx.Repo, error)
	Branch     func() (string, error)

	Path        string
	Method      string
	MethodSet   bool
	RawFields   []string
	TypedFields []string
	Headers     []string
	Input       string
	Paginate    bool
	Include     bool
	Silent      bool
	JQ          string
	Template    string
}

// NewCmdAPI returns `khbb api`.
func NewCmdAPI(f *cmdutil.Factory, runF func(*APIOptions) error) *cobra.Command {
	opts := &APIOptions{IO: f.IOStreams, HTTPClient: f.HTTPClient, BaseRepo: f.BaseRepo, Branch: f.Branch}
	cmd := &cobra.Command{
		Use:   "api <path>",
		Short: "Make an authenticated Bitbucket API request",
		Long: `Make an authenticated HTTP request to the Bitbucket Cloud REST API and print the response.

The path is relative to https://api.bitbucket.org/2.0/. The placeholders {workspace},
{repo} and {branch} are replaced with values from --repo, KHBB_REPO or the current git
repository.

The default method is GET, or POST when fields or --input are given. With GET, fields
are sent as query parameters; otherwise they form a JSON object body.

  -f key=value   adds a string field
  -F key=value   adds a typed field: true, false, null and numbers are converted;
                 @file reads the value from a file, @- from standard input`,
		Example: `  $ khbb api user
  $ khbb api 'repositories/{workspace}/{repo}/pullrequests' --paginate --jq '.[].title'
  $ khbb api 'repositories/{workspace}/{repo}/pullrequests' -X GET -f q='state="OPEN"'
  $ khbb api -X POST 'repositories/{workspace}/{repo}/pullrequests/42/comments' --input comment.json`,
		Args: cmdutil.ExactArgs(1, "<path>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Path = args[0]
			opts.MethodSet = cmd.Flags().Changed("method")
			if err := validateFlags(opts); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return apiRun(cmd.Context(), opts)
		},
	}
	cmdutil.EnableRepoOverride(cmd, f)
	cmdutil.AddDryRunFlag(cmd, f)
	fl := cmd.Flags()
	fl.StringVarP(&opts.Method, "method", "X", "GET", "The HTTP `method` for the request")
	fl.StringArrayVarP(&opts.RawFields, "raw-field", "f", nil, "Add a string parameter in `key=value` format")
	fl.StringArrayVarP(&opts.TypedFields, "field", "F", nil, "Add a typed parameter in `key=value` format")
	fl.StringArrayVarP(&opts.Headers, "header", "H", nil, "Add a HTTP request header in `key:value` format")
	fl.StringVar(&opts.Input, "input", "", "The `file` to use as body for the request (use \"-\" for standard input)")
	fl.BoolVar(&opts.Paginate, "paginate", false, "Follow `next` links and print all values as one JSON array")
	fl.BoolVarP(&opts.Include, "include", "i", false, "Include the HTTP response status line and headers in the output")
	fl.BoolVar(&opts.Silent, "silent", false, "Do not print the response body")
	fl.StringVarP(&opts.JQ, "jq", "q", "", "Filter the response using a jq `expression`")
	fl.StringVarP(&opts.Template, "template", "t", "", "Format the response using a Go `template`")
	return cmd
}

func validateFlags(opts *APIOptions) error {
	switch {
	case opts.Paginate && opts.MethodSet && !strings.EqualFold(opts.Method, http.MethodGet):
		return cmdutil.FlagErrorf("--paginate only works with GET requests")
	case opts.Paginate && opts.Include:
		return cmdutil.FlagErrorf("--include cannot be combined with --paginate")
	case opts.Paginate && opts.Input != "":
		return cmdutil.FlagErrorf("--input cannot be combined with --paginate")
	case opts.Input != "" && (len(opts.RawFields) > 0 || len(opts.TypedFields) > 0):
		return cmdutil.FlagErrorf("--input cannot be combined with -f/-F")
	case opts.JQ != "" && opts.Template != "":
		return cmdutil.FlagErrorf("cannot use --jq and --template together")
	}
	if err := cmdutil.ValidateJQ(opts.JQ); err != nil {
		return err
	}
	if err := cmdutil.ValidateTemplate(opts.Template); err != nil {
		return err
	}
	return validateFields(opts)
}

// validateFields checks key=value syntax without reading files or stdin.
func validateFields(opts *APIOptions) error {
	for _, f := range append(slices.Clone(opts.RawFields), opts.TypedFields...) {
		if k, _, ok := strings.Cut(f, "="); !ok || k == "" {
			return cmdutil.FlagErrorf("field %q must be in key=value format", f)
		}
	}
	return nil
}

func apiRun(ctx context.Context, opts *APIOptions) error {
	path, err := fillPlaceholders(opts.Path, opts.BaseRepo, opts.Branch)
	if err != nil {
		return err
	}
	params, err := parseFields(opts.RawFields, opts.TypedFields, opts.IO.In)
	if err != nil {
		return err
	}
	method := strings.ToUpper(opts.Method)
	switch {
	case opts.Paginate:
		// validateFlags rejects --paginate with --input or a non-GET -X; fields go on the query string.
		method = http.MethodGet
	case !opts.MethodSet && (len(params) > 0 || opts.Input != ""):
		method = http.MethodPost
	}
	var body []byte
	switch {
	case opts.Input != "":
		body, err = readInput(opts.Input, opts.IO.In)
	case len(params) > 0 && method == http.MethodGet:
		path = addQuery(path, params)
	case len(params) > 0:
		body, err = json.Marshal(params)
	}
	if err != nil {
		return err
	}
	header, err := parseHeaders(opts.Headers)
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}
	if opts.Paginate {
		return paginate(ctx, client, path, header, opts)
	}

	resp, err := client.Request(ctx, method, path, header, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if opts.Include {
		writeHeaders(opts.IO.Out, resp)
	}
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !ok {
		writeErrorBody(opts, resp.Header.Get("Content-Type"), data)
		return bitbucket.ParseHTTPError(resp, data)
	}
	return writeBody(opts, resp.Header.Get("Content-Type"), data, true)
}

func paginate(ctx context.Context, client *bitbucket.Client, path string, header http.Header, opts *APIOptions) error {
	all := []json.RawMessage{}
	pageNumber := 0
	for next := path; next != ""; {
		pageNumber++
		resp, err := client.Request(ctx, http.MethodGet, next, header, nil)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			writeErrorBody(opts, resp.Header.Get("Content-Type"), data)
			return bitbucket.ParseHTTPError(resp, data)
		}
		var page struct {
			Values []json.RawMessage `json:"values"`
			Next   string            `json:"next"`
		}
		if err := json.Unmarshal(data, &page); err != nil || page.Values == nil {
			// Not a paginated collection.
			if pageNumber == 1 {
				// First page: pass it through.
				return writeBody(opts, resp.Header.Get("Content-Type"), data, true)
			}
			// Later page: error.
			return fmt.Errorf("page %d of %s is not a paginated collection", pageNumber, path)
		}
		all = append(all, page.Values...)
		next = page.Next
	}
	merged, err := json.Marshal(all)
	if err != nil {
		return err
	}
	return writeBody(opts, "application/json", append(merged, '\n'), true)
}

var placeholderRE = regexp.MustCompile(`\{(workspace|repo|branch)\}`)

func fillPlaceholders(path string, baseRepo func() (gitctx.Repo, error), branch func() (string, error)) (string, error) {
	var (
		repo     *gitctx.Repo
		firstErr error
	)
	result := placeholderRE.ReplaceAllStringFunc(path, func(m string) string {
		if firstErr != nil {
			return m
		}
		switch m {
		case "{workspace}", "{repo}":
			if repo == nil {
				r, err := baseRepo()
				if err != nil {
					firstErr = err
					return m
				}
				repo = &r
			}
			if m == "{workspace}" {
				return repo.Workspace
			}
			return repo.Slug
		default: // {branch}
			b, err := branch()
			if err != nil {
				firstErr = err
				return m
			}
			return b
		}
	})
	return result, firstErr
}

func parseFields(raw, typed []string, stdin io.Reader) (map[string]any, error) {
	params := map[string]any{}
	for _, f := range raw {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, cmdutil.FlagErrorf("field %q must be in key=value format", f)
		}
		params[k] = v
	}
	for _, f := range typed {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, cmdutil.FlagErrorf("field %q must be in key=value format", f)
		}
		val, err := typedValue(v, stdin)
		if err != nil {
			return nil, err
		}
		params[k] = val
	}
	return params, nil
}

func typedValue(v string, stdin io.Reader) (any, error) {
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	var n json.Number
	if err := json.Unmarshal([]byte(v), &n); err == nil {
		return n, nil
	}
	if strings.HasPrefix(v, "@") {
		b, err := readInput(v[1:], stdin)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	}
	return v, nil
}

func readInput(name string, stdin io.Reader) ([]byte, error) {
	if name == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(name)
}

func addQuery(path string, params map[string]any) string {
	q := url.Values{}
	for k, v := range params {
		if v == nil {
			q.Set(k, "")
			continue
		}
		q.Set(k, fmt.Sprint(v))
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + q.Encode()
}

func parseHeaders(values []string) (http.Header, error) {
	h := http.Header{}
	for _, s := range values {
		k, v, ok := strings.Cut(s, ":")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, cmdutil.FlagErrorf("header %q must be in key:value format", s)
		}
		h.Add(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	return h, nil
}

func writeHeaders(w io.Writer, resp *http.Response) {
	proto := resp.Proto
	if proto == "" {
		proto = "HTTP/1.1"
	}
	fmt.Fprintf(w, "%s %s\n", proto, resp.Status)
	for _, k := range slices.Sorted(maps.Keys(resp.Header)) {
		fmt.Fprintf(w, "%s: %s\n", k, strings.Join(resp.Header.Values(k), ", "))
	}
	fmt.Fprintln(w)
}

// writeBody prints a response body; filter applies --jq/--template (never to error bodies).
func writeBody(opts *APIOptions, contentType string, data []byte, filter bool) error {
	if opts.Silent || len(data) == 0 {
		return nil
	}
	switch {
	case filter && opts.JQ != "":
		return jq.EvaluateFormatted(bytes.NewReader(data), opts.IO.Out, opts.JQ, "  ", opts.IO.ColorEnabled())
	case filter && opts.Template != "":
		t := template.New(opts.IO.Out, opts.IO.TerminalWidth(), opts.IO.ColorEnabled())
		if err := t.Parse(opts.Template); err != nil {
			return err
		}
		if err := t.Execute(bytes.NewReader(data)); err != nil {
			return err
		}
		return t.Flush()
	case strings.Contains(contentType, "json") && opts.IO.IsStdoutTTY():
		return jsonpretty.Format(opts.IO.Out, bytes.NewReader(data), "  ", opts.IO.ColorEnabled())
	default:
		_, err := opts.IO.Out.Write(data)
		return err
	}
}

// writeErrorBody prints a JSON error body unfiltered, as gh does. Any other body is dropped: some
// Bitbucket errors are web pages that embed the caller's profile and a short-lived web token.
func writeErrorBody(opts *APIOptions, contentType string, data []byte) {
	if opts.Silent || len(data) == 0 {
		return
	}
	if strings.Contains(contentType, "json") || (contentType == "" && json.Valid(data)) {
		_ = writeBody(opts, contentType, data, false)
		return
	}
	kind, _, _ := strings.Cut(contentType, ";")
	if kind = strings.TrimSpace(kind); kind == "" {
		kind = "non-JSON"
	}
	fmt.Fprintf(opts.IO.ErrOut, "note: the %s error body (%d bytes) was not printed\n", kind, len(data))
}
