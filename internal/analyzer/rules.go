package analyzer

import "strconv"

// analyzeFile runs all per-file rules over one migration's SQL and appends
// findings for the given migration.
func analyzeFile(m Migration, src string, opts Options, out *[]Finding) {
	toks := tokenize(src)
	add := func(sev Severity, rule, msg string, line int, hints ...string) {
		*out = append(*out, Finding{
			File:     m.File,
			Version:  m.Version,
			Severity: sev.String(),
			Rule:     rule,
			Message:  msg,
			Line:     line,
			Hints:    hints,
		})
	}

	// ---- destructive / dangerous statements ----
	checkDestructive(toks, m, add)

	// ---- DELETE / UPDATE without WHERE ----
	checkMissingWhere(toks, m, add)

	// ---- transaction-wrapping heuristic ----
	checkTransaction(toks, m, add)
}

// checkDestructive flags statements that can destroy data or schema.
func checkDestructive(toks []token, m Migration, add func(Severity, string, string, int, ...string)) {
	for i := 0; i < len(toks); i++ {
		t := toks[i].text

		switch t {
		case "DROP":
			if i+1 < len(toks) {
				next := toks[i+1].text
				sev := Error
				msg := "DROP " + next + " is destructive and irreversible"
				hint := []string{"prefer a drop-and-recreate with a down migration, or gate behind an explicit maintenance window"}
				if next == "INDEX" || next == "CONSTRAINT" {
					sev = Warning
					msg = "DROP " + next + " may affect query performance or integrity"
					hint = []string{"recreate the index/constraint in the same migration if needed"}
				}
				add(sev, "destructive-drop", msg, toks[i].line, hint...)
			}
		case "TRUNCATE":
			add(Error, "destructive-truncate", "TRUNCATE removes all rows without logging each row", toks[i].line,
				"prefer DELETE with a WHERE clause and a down migration")
		case "ALTER":
			// flag ALTER statements that contain a DROP COLUMN (or DROP with
			// column) anywhere within the statement.
			if statementContains(toks, i, "DROP") && statementContains(toks, i, "COLUMN") {
				add(Error, "destructive-alter", "ALTER statement drops a column, destroying its data", toks[i].line,
					"archive or back up the column's data before dropping")
			}
		case "RENAME":
			if statementContains(toks, i, "TO") {
				add(Warning, "rename", "RENAME may break references to the old name", toks[i].line,
					"update all referencing code and ORM mappings in the same change")
			}
		}
	}
}

// checkMissingWhere flags unqualified DELETE and UPDATE statements. It is
// heuristic: a WHERE appearing later in the statement still counts as safe,
// which can miss multi-statement files; documented limitation.
func checkMissingWhere(toks []token, m Migration, add func(Severity, string, string, int, ...string)) {
	for i := 0; i < len(toks); i++ {
		t := toks[i].text
		if t != "DELETE" && t != "UPDATE" {
			continue
		}
		// find end of statement
		end := i
		for end < len(toks) && toks[end].text != ";" {
			end++
		}
		hasWhere := false
		hasLimit := false
		for j := i; j < end; j++ {
			if toks[j].text == "WHERE" {
				hasWhere = true
			}
			if toks[j].text == "LIMIT" {
				hasLimit = true
			}
		}
		if !hasWhere {
			sev := Error
			if t == "DELETE" && hasLimit {
				sev = Warning
			}
			add(sev, "missing-where", t+" statement has no WHERE clause and will affect every row", toks[i].line,
				"add a WHERE clause, or an explicit guard, unless a full-table operation is intentional")
		}
	}
}

// checkTransaction flags migrations with multiple statements that wrap only a
// subset in BEGIN/COMMIT, or that mix BEGIN and COMMIT inconsistently.
func checkTransaction(toks []token, m Migration, add func(Severity, string, string, int, ...string)) {
	begin := 0
	commit := 0
	for _, t := range toks {
		switch t.text {
		case "BEGIN", "START":
			begin++
		case "COMMIT":
			commit++
		}
	}
	// Only meaningful if statements exist beyond the transaction keywords.
	if begin != commit {
		line := 1
		if len(toks) > 0 {
			line = toks[0].line
		}
		add(Warning, "unbalanced-transaction", "unbalanced BEGIN/COMMIT: "+strconv.Itoa(begin)+" begin vs "+strconv.Itoa(commit)+" commit", line,
			"wrap the whole migration in an explicit transaction to make it atomic")
	}
}

// statementContains reports whether the statement starting at idx (up to the
// next ';') contains a token equal to want.
func statementContains(toks []token, idx int, want string) bool {
	for j := idx; j < len(toks); j++ {
		if j > idx && toks[j].text == ";" {
			break
		}
		if toks[j].text == want {
			return true
		}
	}
	return false
}
