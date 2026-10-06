package shared

import (
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
	"github.com/khipu/khbb/internal/cmdutil"
)

// ParseVariables turns --var and --secret-var values (KEY=VALUE) into pipeline variables, secured
// for --secret-var. An error about a --secret-var never repeats its text, which may be a secret.
func ParseVariables(vars, secrets []string) ([]bitbucket.PipelineVariable, error) {
	out := make([]bitbucket.PipelineVariable, 0, len(vars)+len(secrets))
	for _, v := range vars {
		key, value, ok := strings.Cut(v, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, cmdutil.FlagErrorf("invalid --var %q: expected KEY=VALUE", v)
		}
		out = append(out, bitbucket.PipelineVariable{Key: key, Value: value})
	}
	for i, v := range secrets {
		key, value, ok := strings.Cut(v, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, cmdutil.FlagErrorf("invalid --secret-var number %d: expected KEY=VALUE", i+1)
		}
		out = append(out, bitbucket.PipelineVariable{Key: key, Value: value, Secured: true})
	}
	return out, nil
}
