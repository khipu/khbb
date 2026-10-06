package cmdutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/iostreams"
)

// FlagError is a usage error: wrong or missing flags or arguments.
type FlagError struct{ Err error }

func (e *FlagError) Error() string { return e.Err.Error() }
func (e *FlagError) Unwrap() error { return e.Err }

// FlagErrorf returns a *FlagError.
func FlagErrorf(format string, args ...any) error {
	return &FlagError{Err: fmt.Errorf(format, args...)}
}

// ErrCancel means the user declined a prompt.
var ErrCancel = errors.New("cancelled")

// ErrConfirmationRequired means a destructive action ran without a terminal and without --yes.
var ErrConfirmationRequired = errors.New("this action needs confirmation: rerun with --yes")

// AuthError means credentials are missing or were rejected.
type AuthError struct{ Msg string }

func (e *AuthError) Error() string { return e.Msg }

// NotFoundError reports a missing resource detected without an HTTP 404, such as a branch with no open pull request.
type NotFoundError struct{ Msg string }

func (e *NotFoundError) Error() string { return e.Msg }

// ConflictError means the resource is in a state that does not allow the action, such as a pull
// request that is no longer open.
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

// ExitError ends the command with Code without printing anything.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// ErrorInfo is the classified form of an error, as printed to stderr.
type ErrorInfo struct {
	Code    string `json:"code"`
	Status  int    `json:"status,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	Exit    int    `json:"-"`
	Silent  bool   `json:"-"`
}

// Classify maps an error to its code, message, hint and process exit code.
func Classify(err error) ErrorInfo {
	var (
		flagErr  *FlagError
		authErr  *AuthError
		exitErr  *ExitError
		notFound *NotFoundError
		conflict *ConflictError
		httpErr  *bitbucket.HTTPError
		netErr   *bitbucket.NetworkError
	)
	switch {
	case errors.Is(err, bitbucket.ErrDryRun):
		return ErrorInfo{Exit: 0, Silent: true}
	case errors.As(err, &exitErr):
		return ErrorInfo{Exit: exitErr.Code, Silent: true}
	case errors.Is(err, ErrCancel), errors.Is(err, terminal.InterruptErr):
		return ErrorInfo{Code: "cancelled", Message: "cancelled", Exit: 2}
	case errors.Is(err, ErrConfirmationRequired):
		return ErrorInfo{Code: "confirmation_required", Message: err.Error(), Exit: 1}
	case errors.As(err, &flagErr):
		return ErrorInfo{Code: "usage", Message: err.Error(), Hint: "see `--help` for usage", Exit: 1}
	case errors.As(err, &authErr):
		return ErrorInfo{Code: "auth_required", Message: err.Error(), Hint: "run `khbb auth login`", Exit: 4}
	case errors.As(err, &notFound):
		return ErrorInfo{Code: "not_found", Message: err.Error(), Exit: 1}
	case errors.As(err, &conflict):
		return ErrorInfo{Code: "conflict", Message: err.Error(), Exit: 1}
	case errors.As(err, &httpErr):
		return classifyHTTP(httpErr)
	case errors.As(err, &netErr):
		return ErrorInfo{Code: "network", Message: err.Error(), Exit: 1}
	}
	return ErrorInfo{Code: "error", Message: err.Error(), Exit: 1}
}

func classifyHTTP(e *bitbucket.HTTPError) ErrorInfo {
	info := ErrorInfo{Status: e.StatusCode, Message: e.Error(), Exit: 1}
	switch {
	case e.StatusCode == 401:
		info.Code, info.Exit = "auth_required", 4
		info.Hint = "the token is invalid or expired; run `khbb auth login`"
	case e.StatusCode == 403:
		info.Code = "forbidden"
		if len(e.RequiredScopes) > 0 {
			info.Hint = "the token is missing scope(s) " + strings.Join(e.RequiredScopes, ", ") + "; create a token with them and run `khbb auth login`"
		} else if scope := bitbucket.ScopeFor(e.Method, e.URL); scope != "" {
			info.Hint = "this request needs the " + scope + " scope, or repository permissions you may not have"
		}
	case e.StatusCode == 404:
		info.Code = "not_found"
		switch {
		case strings.Contains(e.URL, "/pipelines"):
			// Pipeline 404s name what is missing (a build number, a branch, a tag); a repository hint would mislead.
		case strings.Contains(e.URL, "/pullrequests/"):
			info.Hint = "check the pull request number and that it belongs to this repository"
		case strings.Contains(e.URL, "/repositories/"):
			info.Hint = "check the repository name and your access: private repositories return 404 when you lack access"
		}
	case e.StatusCode == 409:
		info.Code = "conflict"
	case e.StatusCode == 400 || e.StatusCode == 422:
		info.Code = "validation"
	case e.StatusCode == 429:
		info.Code = "rate_limited"
	case e.StatusCode == 555:
		info.Code = "server_error"
		info.Hint = "Bitbucket timed out; retry later"
	case e.StatusCode >= 500:
		info.Code = "server_error"
	default:
		info.Code = "error"
	}
	if info.Hint == "" && e.Detail != "" {
		info.Hint = e.Detail
	}
	if info.Hint == "" && len(e.Fields) > 0 {
		// Bitbucket often repeats its only field error as the message; such a hint adds nothing.
		if hint := formatFields(e.Fields); hint != e.Message {
			info.Hint = hint
		}
	}
	return info
}

// formatFields renders Bitbucket field errors as "field: msg, msg; other: msg", sorted by field.
func formatFields(fields map[string][]string) string {
	keys := slices.Sorted(maps.Keys(fields))
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+strings.Join(fields[k], ", "))
	}
	return strings.Join(parts, "; ")
}

// PrintError reports err on stderr (as one JSON line when asJSON) and returns the exit code.
func PrintError(ios *iostreams.IOStreams, err error, asJSON bool) int {
	info := Classify(err)
	if info.Silent {
		return info.Exit
	}
	if asJSON {
		b, _ := json.Marshal(map[string]ErrorInfo{"error": info})
		fmt.Fprintln(ios.ErrOut, string(b))
		return info.Exit
	}
	fmt.Fprintf(ios.ErrOut, "error: %s\n", info.Message)
	if info.Hint != "" {
		fmt.Fprintf(ios.ErrOut, "hint: %s\n", info.Hint)
	}
	return info.Exit
}
