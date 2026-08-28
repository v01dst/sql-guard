// Command sql-guard statically analyzes SQL migration files for dangerous and
// non-reversible schema changes, and plugs into CI via JSON or GitHub
// annotations output.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/v01dst/sql-guard/internal/analyzer"
)

const version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("sql-guard", flag.ContinueOnError)
	dir := fs.String("dir", ".", "directory containing migration files")
	format := fs.String("format", "human", "output format: human, json, github")
	requireDown := fs.Bool("require-down", false, "treat missing down migrations as errors")
	showVersion := fs.Bool("version", false, "print version and exit")
	noColor := fs.Bool("no-color", false, "disable ANSI color output")

	fs.SetOutput(os.Stderr)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println("sql-guard " + version)
		return 0
	}
	if fs.NArg() > 0 {
		*dir = fs.Arg(0)
	}

	opts := analyzer.Options{RequireDown: *requireDown, BaselinePrefix: "baseline"}
	res, err := analyzer.Analyze(*dir, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sql-guard: %v\n", err)
		return 2
	}

	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintf(os.Stderr, "sql-guard: %v\n", err)
			return 2
		}
	case "github":
		printGitHub(res)
	default:
		printHuman(res, *noColor)
	}

	if res.ErrorCount() > 0 {
		return 1
	}
	return 0
}

const (
	cReset = "\x1b[0m"
	cRed   = "\x1b[31m"
	cYel   = "\x1b[33m"
	cDim   = "\x1b[2m"
	cBold  = "\x1b[1m"
	cGreen = "\x1b[32m"
)

func colorize(s, code string, enabled bool) string {
	if !enabled {
		return s
	}
	return code + s + cReset
}

func printHuman(res *analyzer.Analysis, noColor bool) {
	if len(res.Migrations) == 0 {
		fmt.Println("No migration files found.")
		return
	}

	fmt.Printf("Scanned %d migration(s)\n\n", len(res.Migrations))

	sevColor := func(s string) string {
		switch s {
		case "error":
			return cRed
		case "warning":
			return cYel
		default:
			return cDim
		}
	}

	for i, f := range res.Findings {
		if i > 0 {
			fmt.Println()
		}
		sev := colorize(f.Severity, sevColor(f.Severity), !noColor)
		file := colorize(f.File, cBold, !noColor)
		fmt.Printf("%s  %s:%d  [%s]\n", sev, file, f.Line, f.Rule)
		fmt.Printf("    %s\n", f.Message)
		if len(f.Hints) > 0 {
			hints := make([]string, 0, len(f.Hints))
			for _, h := range f.Hints {
				hints = append(hints, "→ "+h)
			}
			fmt.Printf("    %s\n", colorize(strings.Join(hints, "\n    "), cDim, !noColor))
		}
	}

	errs := res.ErrorCount()
	warns := 0
	for _, f := range res.Findings {
		if f.Severity == "warning" {
			warns++
		}
	}

	fmt.Println()
	switch {
	case errs > 0:
		printColored(fmt.Sprintf("✗ %d error(s), %d warning(s)\n", errs, warns), cRed, noColor)
	case warns > 0:
		printColored(fmt.Sprintf("△ %d warning(s)\n", warns), cYel, noColor)
	default:
		printColored("✓ clean — no issues found\n", cGreen, noColor)
	}
}

// reorderArgs rewrites args so flags may appear after the positional
// directory argument (Go's flag package stops at the first non-flag token).
func reorderArgs(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' {
			flags = append(flags, a)
			// consume the flag's value if it's a separate token
			if !strings.Contains(a, "=") && needsValue(a) && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}

// needsValue reports whether a flag token expects a following value token.
func needsValue(flag string) bool {
	switch strings.TrimLeft(flag, "-") {
	case "dir", "format":
		return true
	}
	return false
}

func printColored(s, code string, noColor bool) {
	if noColor {
		fmt.Print(s)
		return
	}
	fmt.Print(code + s + cReset)
}

// printGitHub emits GitHub Actions workflow command annotations so findings
// surface inline on pull requests.
func printGitHub(res *analyzer.Analysis) {
	for _, f := range res.Findings {
		msg := escapeGitHub(f.Message)
		fmt.Printf("::%s file=%s,line=%d,title=%s::%s\n",
			ghLevel(f.Severity), escapeProperty(f.File), f.Line, f.Rule, msg)
	}
	if len(res.Findings) == 0 {
		fmt.Println("::notice::sql-guard: no issues found")
	}
}

func ghLevel(s string) string {
	switch s {
	case "error":
		return "error"
	case "warning":
		return "warning"
	default:
		return "notice"
	}
}

func escapeProperty(s string) string {
	r := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
	return r.Replace(s)
}

func escapeGitHub(s string) string {
	r := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	return r.Replace(s)
}
