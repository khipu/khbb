// Package skills holds the agent skills that ship inside the khbb binary.
package skills

import _ "embed" // for go:embed

// Khbb is the khbb agent skill (skills/khbb/SKILL.md), written out by `khbb skill install`.
//
//go:embed khbb/SKILL.md
var Khbb string
