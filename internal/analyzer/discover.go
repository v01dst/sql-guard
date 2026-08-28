package analyzer

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// versionRe matches the leading version/timestamp of a migration filename.
// It captures a leading run of digits (optionally with a trailing Flyway-style
// separator like "V1__" handled separately) and stops at the first '_' or '-'
// or '.' that signals the descriptive suffix.
var leadingIntRe = regexp.MustCompile(`^\d+`)

// flywayRe matches Flyway-style "V<version>__<description>" names.
var flywayRe = regexp.MustCompile(`^[Vv](\d[\d_]*)__`)

// knownExtensions are file extensions considered SQL migration files.
var knownExtensions = map[string]bool{
	".sql": true,
}

// Discover scans dir (non-recursive by default) for SQL migration files.
func Discover(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var migs []Migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !knownExtensions[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		m := classifyFile(name)
		if m.Version == "" && !m.IsBaseline {
			continue // not a recognizable migration
		}
		m.File = filepath.Join(dir, name)
		migs = append(migs, m)
	}
	sortMigrations(migs)
	return migs, nil
}

// classifyFile determines version, and whether the file is a down or baseline
// migration, purely from its name.
func classifyFile(name string) Migration {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	lower := strings.ToLower(base)

	m := Migration{File: name}

	// down migrations: common suffixes .down or names like *_down / *-down
	if strings.HasSuffix(lower, ".down") || strings.HasSuffix(lower, "-down") || strings.HasSuffix(lower, "_down") {
		m.IsDown = true
		base = base[:len(base)-len(lower)+len(strings.TrimSuffix(lower, ".down"))]
		lower = strings.ToLower(base)
	}

	if strings.HasPrefix(lower, "baseline") {
		m.IsBaseline = true
		return m
	}

	if mm := flywayRe.FindStringSubmatch(base); mm != nil {
		m.Version = mm[1]
		return m
	}
	if mm := leadingIntRe.FindStringSubmatch(base); mm != nil {
		m.Version = mm[0]
	}
	return m
}

// Analyze reads and analyzes all migrations in dir.
func Analyze(dir string, opts Options) (*Analysis, error) {
	migs, err := Discover(dir)
	if err != nil {
		return nil, err
	}

	a := &Analysis{Migrations: migs}

	// per-file structural checks
	for _, m := range migs {
		data, err := os.ReadFile(m.File)
		if err != nil {
			a.Findings = append(a.Findings, Finding{
				File:     m.File,
				Version:  m.Version,
				Severity: Error.String(),
				Rule:     "read-error",
				Message:  "could not read migration file: " + err.Error(),
				Line:     0,
			})
			continue
		}
		analyzeFile(m, string(data), opts, &a.Findings)
	}

	// cross-file checks
	checkDownMigrations(migs, opts, &a.Findings)
	checkVersionCollisions(migs, &a.Findings)
	checkVersionOrder(migs, &a.Findings)

	sortFindings(a.Findings)
	return a, nil
}

// checkDownMigrations flags up migrations lacking a matching down counterpart.
func checkDownMigrations(migs []Migration, opts Options, out *[]Finding) {
	upByVersion := map[string]bool{}
	downByVersion := map[string]bool{}
	for _, m := range migs {
		if m.IsBaseline {
			continue
		}
		if m.Version == "" {
			continue
		}
		if m.IsDown {
			downByVersion[m.Version] = true
		} else {
			upByVersion[m.Version] = true
		}
	}
	for _, m := range migs {
		if m.IsDown || m.IsBaseline || m.Version == "" {
			continue
		}
		if !downByVersion[m.Version] {
			sev := Warning
			if opts.RequireDown {
				sev = Error
			}
			*out = append(*out, Finding{
				File:     m.File,
				Version:  m.Version,
				Severity: sev.String(),
				Rule:     "missing-down",
				Message:  "up migration has no matching down (rollback) migration",
				Line:     1,
				Hints:    []string{"add a " + m.Version + ".down.sql to make this migration reversible"},
			})
		}
	}
	// down migrations with no up counterpart
	for _, m := range migs {
		if !m.IsDown || m.Version == "" || m.IsBaseline {
			continue
		}
		if !upByVersion[m.Version] {
			*out = append(*out, Finding{
				File:     m.File,
				Version:  m.Version,
				Severity: Warning.String(),
				Rule:     "orphan-down",
				Message:  "down migration has no matching up migration",
				Line:     1,
			})
		}
	}
}

// checkVersionCollisions flags duplicate version numbers among migrations of
// the same direction. A version appearing once as `.up` and once as `.down`
// is the *correct* up/down pair, not a collision.
func checkVersionCollisions(migs []Migration, out *[]Finding) {
	counts := map[string]int{}
	for _, m := range migs {
		if m.Version != "" && !m.IsBaseline {
			counts[versionKey(m)]++
		}
	}
	seen := map[string]bool{}
	for _, m := range migs {
		if m.Version == "" || m.IsBaseline || counts[versionKey(m)] <= 1 {
			continue
		}
		key := versionKey(m)
		if seen[key] {
			continue
		}
		seen[key] = true
		*out = append(*out, Finding{
			File:     m.File,
			Version:  m.Version,
			Severity: Error.String(),
			Rule:     "version-collision",
			Message:  "version " + m.Version + " appears in multiple files",
			Line:     1,
			Hints:    []string{"each migration version must be unique"},
		})
	}
}

// checkVersionOrder flags non-monotonic version sequences for numeric versions.
func checkVersionOrder(migs []Migration, out *[]Finding) {
	var nums []int
	for _, m := range migs {
		if m.Version == "" || m.IsBaseline || m.IsDown {
			continue
		}
		if n, ok := numericVersion(m.Version); ok {
			nums = append(nums, n)
		}
	}
	if len(nums) < 2 {
		return
	}
	sorted := make([]int, len(nums))
	copy(sorted, nums)
	sort.Ints(sorted)
	for i := range nums {
		if nums[i] != sorted[i] {
			*out = append(*out, Finding{
				Severity: Warning.String(),
				Rule:     "version-order",
				Message:  "numeric migration versions are not applied in ascending order",
				Line:     1,
				Hints:    []string{"reorder or renumber migrations so versions increase monotonically"},
			})
			return
		}
	}
}

// versionKey returns a collision key combining version and direction so an
// up/down pair is not mistaken for a duplicate.
func versionKey(m Migration) string {
	if m.IsDown {
		return m.Version + "|down"
	}
	return m.Version + "|up"
}

// numericVersion returns the pure-integer prefix as an int if the version is
// numeric (1, 0001, 20240101). Timestamps exceed int32 but fit int64.
func numericVersion(v string) (int, bool) {
	if v == "" {
		return 0, false
	}
	trim := strings.TrimLeft(v, "0")
	if trim == "" {
		trim = "0"
	}
	// ensure all digits
	for _, c := range trim {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	if len(trim) > 18 {
		return 0, false // beyond int64 range; treat as non-numeric
	}
	n := 0
	for _, c := range trim {
		n = n*10 + int(c-'0')
	}
	return n, true
}
