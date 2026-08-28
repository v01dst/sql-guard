// Package analyzer implements static safety analysis for SQL migration files.
//
// It is intentionally dependency-free and parses SQL with a lightweight,
// comment-aware tokenizer rather than a full SQL grammar. This keeps the tool
// fast, portable, and suitable for CI use, at the cost of being heuristic for
// a few checks (documented on each rule).
package analyzer

import "sort"

// Severity ranks the importance of a finding.
type Severity int

const (
	Info Severity = iota
	Warning
	Error
)

func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warning:
		return "warning"
	case Error:
		return "error"
	}
	return "unknown"
}

// Finding is a single issue detected in a migration file.
type Finding struct {
	File     string   `json:"file"`
	Version  string   `json:"version"`
	Severity string   `json:"severity"`
	Rule     string   `json:"rule"`
	Message  string   `json:"message"`
	Line     int      `json:"line"`
	Hints    []string `json:"hints,omitempty"`
}

// Migration is one migration file discovered in a directory.
type Migration struct {
	File       string `json:"file"`
	Version    string `json:"version"`
	IsDown     bool   `json:"is_down"`
	IsBaseline bool   `json:"is_baseline"`
}

// Options controls analyzer behavior.
type Options struct {
	// RequireDown controls whether a missing down migration is an error
	// rather than a warning.
	RequireDown bool
	// BaselinePrefix is a filename prefix (e.g. "baseline") treated as a
	// non-versioned, non-reversible baseline migration.
	BaselinePrefix string
}

// Analysis is the full result of analyzing a set of migration files.
type Analysis struct {
	Migrations []Migration `json:"migrations"`
	Findings   []Finding   `json:"findings"`
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{RequireDown: false, BaselinePrefix: "baseline"}
}

// ErrorCount returns the number of findings at Error severity.
func (a *Analysis) ErrorCount() int {
	n := 0
	for _, f := range a.Findings {
		if f.Severity == Error.String() {
			n++
		}
	}
	return n
}

// sortFindings orders findings deterministically: by file, then line, then
// rule. Stable output is important for CI diffing.
func sortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].File != fs[j].File {
			return fs[i].File < fs[j].File
		}
		if fs[i].Line != fs[j].Line {
			return fs[i].Line < fs[j].Line
		}
		if fs[i].Rule != fs[j].Rule {
			return fs[i].Rule < fs[j].Rule
		}
		return fs[i].Message < fs[j].Message
	})
}

// sortMigrations orders migrations by version then filename for stable output.
func sortMigrations(ms []Migration) {
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].Version != ms[j].Version {
			return ms[i].Version < ms[j].Version
		}
		return ms[i].File < ms[j].File
	})
}
