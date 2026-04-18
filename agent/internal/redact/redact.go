package redact

import (
	"strings"
)

const mask = "*****"

// sensitivePatterns contains substrings (lowercase) that indicate a
// key or flag name carries sensitive data. Keep this list short and
// extend over time as needed.
var sensitivePatterns = []string{
	"password",
	"secret",
	"token",
	"credential",
}

// String replaces all literal occurrences of sensitive in s with *****.
// If sensitive is empty, s is returned unchanged.
func String(s, sensitive string) string {
	if sensitive == "" {
		return s
	}
	return strings.ReplaceAll(s, sensitive, mask)
}

// isSensitive returns true if name contains any sensitive pattern
// (case-insensitive).
func isSensitive(name string) bool {
	lower := strings.ToLower(name)
	for _, p := range sensitivePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// Env takes a slice of "KEY=VALUE" environment variable strings and
// returns a new slice where the values of sensitive keys are replaced
// with *****.
func Env(env []string) []string {
	out := make([]string, len(env))
	for i, entry := range env {
		k, _, ok := strings.Cut(entry, "=")
		if ok && isSensitive(k) {
			out[i] = k + "=" + mask
		} else {
			out[i] = entry
		}
	}
	return out
}

// Args takes a slice of command-line arguments and returns a new slice
// where the value following any flag whose name matches a sensitive
// pattern is replaced with *****. Flags are detected by a leading "-".
//
// Examples:
//
//	["--password", "s3cret"]        → ["--password", "*****"]
//	["--repo", "/data", "--json"]   → ["--repo", "/data", "--json"]
func Args(args []string) []string {
	out := make([]string, len(args))
	redactNext := false
	for i, arg := range args {
		if redactNext {
			out[i] = mask
			redactNext = false
			continue
		}
		out[i] = arg
		if strings.HasPrefix(arg, "-") && isSensitive(arg) {
			redactNext = true
		}
	}
	return out
}
