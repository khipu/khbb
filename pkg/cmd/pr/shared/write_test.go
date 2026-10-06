package shared_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/iostreams"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
)

func prWithBranches(state string) *bitbucket.PullRequest {
	pr := &bitbucket.PullRequest{ID: 42, State: state}
	pr.Source.Branch.Name = "feature/widgets"
	pr.Destination.Branch.Name = "main"
	return pr
}

func TestRequireOpen(t *testing.T) {
	if err := shared.RequireOpen(prWithBranches("OPEN"), "merged"); err != nil {
		t.Errorf("open: %v", err)
	}
	err := shared.RequireOpen(prWithBranches("MERGED"), "approved")
	var conflict *cmdutil.ConflictError
	if !errors.As(err, &conflict) || err.Error() != "pull request #42 is merged; only open pull requests can be approved" {
		t.Errorf("err = %v", err)
	}
}

func TestDescribe(t *testing.T) {
	if got := shared.Describe(prWithBranches("OPEN")); got != "#42 (feature/widgets → main)" {
		t.Errorf("Describe = %q", got)
	}
}

func TestReadBody(t *testing.T) {
	ios, stdin, _, _ := iostreams.Test()
	stdin.WriteString("from stdin\n")
	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte("from file"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		body     string
		bodySet  bool
		file     string
		want     string
		provided bool
	}{
		{"flag", "inline", true, "", "inline", true},
		{"empty flag is a body", "", true, "", "", true},
		{"stdin", "", false, "-", "from stdin\n", true},
		{"file", "", false, path, "from file", true},
		{"nothing", "", false, "", "", false},
	}
	for _, tc := range cases {
		got, provided, err := shared.ReadBody(ios, tc.body, tc.bodySet, tc.file)
		if err != nil || got != tc.want || provided != tc.provided {
			t.Errorf("%s: got %q %v %v", tc.name, got, provided, err)
		}
	}
	var flagErr *cmdutil.FlagError
	if _, _, err := shared.ReadBody(ios, "x", true, path); !errors.As(err, &flagErr) {
		t.Errorf("both flags: err = %v", err)
	}
	if _, _, err := shared.ReadBody(ios, "", false, filepath.Join(t.TempDir(), "missing.md")); err == nil || !strings.Contains(err.Error(), "--body-file") {
		t.Errorf("missing file: err = %v", err)
	}
}

func TestPrintSuccess(t *testing.T) {
	ios, _, out, errOut := iostreams.Test()
	shared.PrintSuccess(ios, "Merged pull request %s", "#42")
	ios.SetStderrTTY(true)
	shared.PrintSuccess(ios, "Declined pull request %s", "#43")
	if want := "Merged pull request #42\n✓ Declined pull request #43\n"; errOut.String() != want || out.Len() != 0 {
		t.Errorf("stderr %q stdout %q", errOut.String(), out.String())
	}
}
