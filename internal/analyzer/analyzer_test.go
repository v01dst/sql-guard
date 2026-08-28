package analyzer

import "testing"

func TestTokenizeSkipsStrings(t *testing.T) {
	src := "DROP TABLE 'not a keyword' ; -- comment DROP\nUPDATE x SET y=1"
	toks := tokenize(src)
	for _, tk := range toks {
		if tk.text == "DROP" {
			// there should be exactly one DROP (the real statement), the
			// comment and string must not contribute extra keywords
		}
	}
	// count real DROP keywords
	drops := 0
	for _, tk := range toks {
		if tk.text == "DROP" {
			drops++
		}
	}
	if drops != 1 {
		t.Fatalf("expected exactly 1 DROP keyword, got %d", drops)
	}
	// string content collapsed
	for _, tk := range toks {
		if tk.raw == "${str}" {
			return
		}
	}
	t.Fatal("expected a collapsed string token")
}

func TestTokenizeDollarQuoted(t *testing.T) {
	src := "CREATE FUNCTION f() RETURNS void AS $$ SELECT 'DROP'; $$ LANGUAGE sql;"
	toks := tokenize(src)
	drops := 0
	for _, tk := range toks {
		if tk.text == "DROP" {
			drops++
		}
	}
	// the DROP inside the dollar-quoted body must be suppressed
	if drops != 0 {
		t.Fatalf("expected 0 DROP keywords (inside dollar-quoted body), got %d", drops)
	}
}

func TestTokenizeBlockComment(t *testing.T) {
	src := "/* DROP TABLE users; */ SELECT 1"
	toks := tokenize(src)
	for _, tk := range toks {
		if tk.text == "DROP" {
			t.Fatal("DROP inside block comment should be ignored")
		}
	}
}

func TestTokenizeLineNumbers(t *testing.T) {
	src := "SELECT 1;\nDROP TABLE t;"
	toks := tokenize(src)
	var dropLine int
	for _, tk := range toks {
		if tk.text == "DROP" {
			dropLine = tk.line
		}
	}
	if dropLine != 2 {
		t.Fatalf("expected DROP on line 2, got %d", dropLine)
	}
}

func TestClassifyFile(t *testing.T) {
	cases := []struct {
		name         string
		wantVersion  string
		wantDown     bool
		wantBaseline bool
	}{
		{"0001_create_users.up.sql", "0001", false, false},
		{"0001_create_users.down.sql", "0001", true, false},
		{"20240101000000_init.sql", "20240101000000", false, false},
		{"V1__init.sql", "1", false, false},
		{"baseline.sql", "", false, true},
		{"3_add_index.down.sql", "3", true, false},
	}
	for _, c := range cases {
		m := classifyFile(c.name)
		if m.Version != c.wantVersion || m.IsDown != c.wantDown || m.IsBaseline != c.wantBaseline {
			t.Errorf("%s: got version=%q down=%v baseline=%v, want version=%q down=%v baseline=%v",
				c.name, m.Version, m.IsDown, m.IsBaseline, c.wantVersion, c.wantDown, c.wantBaseline)
		}
	}
}

func TestNumericVersion(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"0001", 1, true},
		{"1", 1, true},
		{"20240101000000", 20240101000000, true},
		{"12", 12, true},
		{"1.2", 0, false},
		{"abc", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := numericVersion(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("numericVersion(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestDestructiveDetection(t *testing.T) {
	opts := DefaultOptions()
	m := Migration{File: "0001.up.sql", Version: "0001"}

	var out []Finding
	analyzeFile(m, "DROP TABLE users;", opts, &out)
	if !hasRule(out, "destructive-drop") {
		t.Fatal("expected destructive-drop finding")
	}

	out = nil
	analyzeFile(m, "DROP INDEX idx_users_email;", opts, &out)
	if !hasRule(out, "destructive-drop") {
		t.Fatal("expected destructive-drop finding for index")
	}
	if out[0].Severity != "warning" {
		t.Errorf("expected DROP INDEX to be a warning, got %s", out[0].Severity)
	}

	out = nil
	analyzeFile(m, "TRUNCATE users;", opts, &out)
	if !hasRule(out, "destructive-truncate") {
		t.Fatal("expected destructive-truncate finding")
	}
}

func TestMissingWhere(t *testing.T) {
	opts := DefaultOptions()
	m := Migration{File: "0001.up.sql", Version: "0001"}

	var out []Finding
	analyzeFile(m, "DELETE FROM users;", opts, &out)
	if !hasRule(out, "missing-where") {
		t.Fatal("expected missing-where for DELETE without WHERE")
	}

	out = nil
	analyzeFile(m, "DELETE FROM users WHERE id = 1;", opts, &out)
	if hasRule(out, "missing-where") {
		t.Fatal("did not expect missing-where when WHERE present")
	}

	out = nil
	analyzeFile(m, "UPDATE accounts SET balance = 0;", opts, &out)
	if !hasRule(out, "missing-where") {
		t.Fatal("expected missing-where for UPDATE without WHERE")
	}
}

func TestBalancedTransactionNoWarning(t *testing.T) {
	opts := DefaultOptions()
	m := Migration{File: "0001.up.sql", Version: "0001"}
	var out []Finding
	analyzeFile(m, "BEGIN; ALTER TABLE t ADD COLUMN c int; COMMIT;", opts, &out)
	if hasRule(out, "unbalanced-transaction") {
		t.Fatal("did not expect unbalanced-transaction for balanced BEGIN/COMMIT")
	}
	// but an unbalanced one should warn
	out = nil
	analyzeFile(m, "BEGIN; ALTER TABLE t ADD COLUMN c int;", opts, &out)
	if !hasRule(out, "unbalanced-transaction") {
		t.Fatal("expected unbalanced-transaction for missing COMMIT")
	}
}

func hasRule(fs []Finding, rule string) bool {
	for _, f := range fs {
		if f.Rule == rule {
			return true
		}
	}
	return false
}
