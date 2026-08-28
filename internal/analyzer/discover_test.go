package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverAndAnalyzeEndToEnd(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// up migration with a destructive op and no down migration
	write("0001_create_users.up.sql", "CREATE TABLE users (id int);\nDROP TABLE legacy;\n")
	// a stray non-migration SQL file that should be ignored
	write("notes.sql", "not a migration, no leading version")
	// a migration with a proper down
	write("0002_add_email.up.sql", "ALTER TABLE users ADD COLUMN email text;\n")
	write("0002_add_email.down.sql", "ALTER TABLE users DROP COLUMN email;\n")

	opts := DefaultOptions()
	res, err := Analyze(dir, opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Migrations) != 3 {
		t.Fatalf("expected 3 migrations (0001.up, 0002.up, 0002.down), got %d: %+v", len(res.Migrations), res.Migrations)
	}

	// destructive-drop should fire for 0001
	if !hasRule(res.Findings, "destructive-drop") {
		t.Error("expected destructive-drop finding")
	}
	// missing-down should fire for 0001 (no 0001.down.sql)
	if !hasRule(res.Findings, "missing-down") {
		t.Error("expected missing-down finding for 0001")
	}
	// 0002.down has DROP COLUMN -> should be flagged destructive-alter
	if !hasRule(res.Findings, "destructive-alter") {
		t.Error("expected destructive-alter for DROP COLUMN in down migration")
	}
	// no version collision
	if hasRule(res.Findings, "version-collision") {
		t.Error("did not expect version-collision")
	}

	// missing-down default is warning, not error
	for _, f := range res.Findings {
		if f.Rule == "missing-down" && f.Severity != "warning" {
			t.Errorf("expected missing-down to be warning by default, got %s", f.Severity)
		}
	}

	if res.ErrorCount() == 0 {
		t.Error("expected at least one error-severity finding (destructive-drop/alter)")
	}
}

func TestMissingDownErrorWhenRequired(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "0001_init.up.sql"), []byte("CREATE TABLE t(id int);"), 0o644)
	opts := Options{RequireDown: true, BaselinePrefix: "baseline"}
	res, err := Analyze(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range res.Findings {
		if f.Rule == "missing-down" {
			found = true
			if f.Severity != "error" {
				t.Errorf("expected error severity with RequireDown, got %s", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected missing-down finding")
	}
}

func TestVersionCollision(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "0001_a.up.sql"), []byte("SELECT 1;"), 0o644)
	os.WriteFile(filepath.Join(dir, "0001_b.up.sql"), []byte("SELECT 2;"), 0o644)
	res, err := Analyze(dir, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !hasRule(res.Findings, "version-collision") {
		t.Error("expected version-collision for duplicate version 0001")
	}
}

func TestVersionOrder(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "0002_second.up.sql"), []byte("SELECT 1;"), 0o644)
	os.WriteFile(filepath.Join(dir, "0001_first.up.sql"), []byte("SELECT 2;"), 0o644)
	// Discover sorts by version, so order in dir is not the trigger; instead
	// force a numeric gap-check scenario via collision-free but out-of-order
	// is not detectable from sorted list — verify no false positive.
	res, err := Analyze(dir, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(res.Findings, "version-order") {
		t.Error("did not expect version-order; numeric versions are sorted deterministically")
	}
}

func TestEmptyDir(t *testing.T) {
	dir := t.TempDir()
	res, err := Analyze(dir, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Migrations) != 0 || len(res.Findings) != 0 {
		t.Errorf("expected empty result, got %d migrations, %d findings", len(res.Migrations), len(res.Findings))
	}
}
