package analyzer

import "strings"

// token is a single SQL token with its 1-based line and column offsets.
type token struct {
	text string // uppercased for keywords, original case for identifiers
	line int
	col  int
	// upper holds the uppercased text; for keywords we store upper in text.
	raw string // original text as written
}

// tokenize breaks SQL into tokens, handling single-quoted strings, double-
// quoted identifiers, line comments (-- and #), block comments (/* */), and
// dollar-quoted strings ($$ ... $$) found in PL/pgSQL. String contents are
// collapsed to a placeholder so a string that happens to contain a keyword
// (e.g. a comment text "drop table") never triggers a false positive.
func tokenize(src string) []token {
	var out []token
	line, col := 1, 1
	i := 0
	n := len(src)

	advance := func(c byte) {
		if c == '\n' {
			line++
			col = 1
		} else {
			col++
		}
		i++
	}

	for i < n {
		c := src[i]

		// whitespace
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			advance(c)
			continue
		}

		// line comments
		if c == '-' && i+1 < n && src[i+1] == '-' {
			for i < n && src[i] != '\n' {
				advance(src[i])
			}
			continue
		}
		if c == '#' {
			for i < n && src[i] != '\n' {
				advance(src[i])
			}
			continue
		}

		// block comment
		if c == '/' && i+1 < n && src[i+1] == '*' {
			advance(src[i])
			advance(src[i])
			for i < n && !(src[i] == '*' && i+1 < n && src[i+1] == '/') {
				advance(src[i])
			}
			if i < n {
				advance(src[i]) // '*'
			}
			if i < n {
				advance(src[i]) // '/'
			}
			continue
		}

		// dollar-quoted string $$ ... $tag$ ... $tag$
		if c == '$' {
			if seq, ok := dollarTag(src, i); ok {
				startLine, startCol := line, col
				for k := 0; k < len(seq); k++ {
					advance(src[i])
				}
				// consume until closing tag
				for i < n {
					if strings.HasPrefix(src[i:], seq) {
						for k := 0; k < len(seq); k++ {
							advance(src[i])
						}
						break
					}
					advance(src[i])
				}
				out = append(out, token{text: "$$", raw: "${dollar-quoted}", line: startLine, col: startCol})
				continue
			}
			advance(c)
			continue
		}

		// single-quoted string
		if c == '\'' {
			startLine, startCol := line, col
			advance(c)
			for i < n {
				if src[i] == '\'' {
					if i+1 < n && src[i+1] == '\'' { // escaped quote ''
						advance(src[i])
						advance(src[i])
						continue
					}
					advance(src[i])
					break
				}
				advance(src[i])
			}
			out = append(out, token{text: "''", raw: "${str}", line: startLine, col: startCol})
			continue
		}

		// double-quoted identifier
		if c == '"' {
			startLine, startCol := line, col
			advance(c)
			for i < n && src[i] != '"' {
				advance(src[i])
			}
			if i < n {
				advance(src[i])
			}
			out = append(out, token{text: "IDENT", raw: "${ident}", line: startLine, col: startCol})
			continue
		}

		// punctuation
		if strings.ContainsRune("(),;=<>", rune(c)) {
			out = append(out, token{text: string(c), raw: string(c), line: line, col: col})
			advance(c)
			continue
		}

		// word / number / dotted identifier
		start := i
		startLine, startCol := line, col
		for i < n && isWordByte(src[i]) {
			advance(src[i])
		}
		word := src[start:i]
		up := strings.ToUpper(word)
		out = append(out, token{text: up, raw: word, line: startLine, col: startCol})
	}

	return out
}

// dollarTag returns the dollar-quote opening tag at src[i] if present.
func dollarTag(src string, i int) (string, bool) {
	// match $...$ where ... is empty or [A-Za-z_][A-Za-z0-9_]* or digits
	if i >= len(src) || src[i] != '$' {
		return "", false
	}
	j := i + 1
	for j < len(src) && (isWordByte(src[j]) && src[j] != '$') {
		j++
	}
	// tag content is src[i+1:j] if next char is $
	if j < len(src) && src[j] == '$' {
		return src[i : j+1], true
	}
	return "", false
}

func isWordByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '_' || c == '.' || c == '-':
		return true
	}
	return false
}
