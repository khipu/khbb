// Package shared holds what the `khbb pipeline` commands have in common: status normalization, JSON
// shapes, lookup, display and the watch loop.
package shared

import (
	"strings"

	"github.com/khipu/khbb/internal/bitbucket"
)

// Statuses reported for pipelines and steps (spec §7.2, plus expired and, for steps, skipped).
const (
	StatusPending    = "pending"
	StatusRunning    = "running"
	StatusPaused     = "paused"
	StatusSuccessful = "successful"
	StatusFailed     = "failed"
	StatusError      = "error"
	StatusStopped    = "stopped"
	StatusExpired    = "expired"
	StatusSkipped    = "skipped"
)

// ListStatuses maps each --status value of `pipeline list` to Bitbucket's own status filter values;
// several values are ORed.
var ListStatuses = map[string][]string{
	StatusPending:    {"PENDING", "PARSING"},
	StatusRunning:    {"BUILDING"},
	StatusPaused:     {"PAUSED", "HALTED"},
	StatusSuccessful: {"PASSED"},
	StatusFailed:     {"FAILED"},
	StatusError:      {"ERROR"},
	StatusStopped:    {"STOPPED"},
}

// Status normalizes Bitbucket's state, stage and result into one status. Unknown values pass
// through lowercased.
func Status(s bitbucket.PipelineState) string {
	paused := s.Stage != nil && s.Stage.Name == "PAUSED"
	switch s.Name {
	case "PARSING", "PENDING", "READY":
		if paused {
			return StatusPaused
		}
		return StatusPending
	case "IN_PROGRESS":
		if paused {
			return StatusPaused
		}
		return StatusRunning
	case "COMPLETED":
		if s.Result == nil {
			return "unknown"
		}
		switch s.Result.Name {
		case "SUCCESSFUL":
			return StatusSuccessful
		case "FAILED":
			return StatusFailed
		case "ERROR":
			return StatusError
		case "STOPPED":
			return StatusStopped
		case "EXPIRED":
			return StatusExpired
		case "NOT_RUN":
			return StatusSkipped
		}
		return strings.ToLower(s.Result.Name)
	}
	return strings.ToLower(s.Name)
}

// Finished reports whether status is final.
func Finished(status string) bool {
	switch status {
	case StatusSuccessful, StatusFailed, StatusError, StatusStopped, StatusExpired, StatusSkipped:
		return true
	}
	return false
}

// Unsuccessful reports whether a final status counts as a failure for exit codes.
func Unsuccessful(status string) bool {
	switch status {
	case StatusFailed, StatusError, StatusStopped, StatusExpired:
		return true
	}
	return false
}
