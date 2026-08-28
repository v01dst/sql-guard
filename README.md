<div align="center">

<img src="https://raw.githubusercontent.com/v01dst/sql-guard/main/docs/images/demo.png" alt="sql-guard terminal output" width="600" />

# sql-guard

**Stop shipping destructive database migrations.**

A fast, dependency-free command-line tool that statically analyzes your SQL migration files and flags schema changes that can destroy data or break deployments — before they reach production.

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![license](https://img.shields.io/badge/license-MIT-22c55e?style=for-the-badge)](./LICENSE)
[![Discord](https://img.shields.io/badge/Discord-9p.1-5865F2?style=for-the-badge&logo=discord&logoColor=white)](https://discord.com)
[![PRs welcome](https://img.shields.io/badge/PRs-welcome-22c55e?style=for-the-badge)](https://github.com/v01dst/sql-guard)

</div>

---

## Why it exists

Most incidents in a database-backed service trace back to a migration that somebody merged without realizing what it actually did. A well-meaning `ALTER TABLE ... DROP COLUMN`, a `DELETE` with no `WHERE`, or a migration with no rollback path can take a production database down for hours.

Code linters and formatters cover almost every language you write — but your **SQL migrations** are usually a blind spot. sql-guard closes that gap. It runs in milliseconds, needs no database connection, and plugs directly into your CI.

## What it does

sql-guard scans a directory of SQL migration files and reports:

| Rule | Severity | What it catches |
|------|----------|-----------------|
| `destructive-drop` | error | `DROP TABLE`, `DROP COLUMN`, and other irreversible drops |
| `destructive-alter` | error | `ALTER` statements that destroy column data |
| `destructive-truncate` | error | `TRUNCATE`, which wipes rows without per-row logging |
| `missing-where` | error | `DELETE` / `UPDATE` with no `WHERE` clause |
| `rename` | warning | `RENAME ... TO`, which can break references |
| `missing-down` | warning | `up` migrations with no matching `down` (rollback) |
| `orphan-down` | warning | `down` migrations with no matching `up` |
| `version-collision` | error | the same version number used by multiple files |
| `version-order` | warning | numeric versions not increasing monotonically |
| `unbalanced-transaction` | warning | `BEGIN`/`COMMIT` pairs that don't balance |

Every rule understands SQL comments, string literals, and even PL/pgSQL dollar-quoted bodies — so a keyword inside a string or a comment never triggers a false positive.

## Install

### Prebuilt binary

```sh
# macOS / Linux (x86_64 and arm64)
curl -sSL https://github.com/v01dst/sql-guard/releases/latest/download/sql-guard_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m).tar.gz | tar xz
sudo mv sql-guard /usr/local/bin/
```

### From source (requires Go 1.21+)

```sh
go install github.com/v01dst/sql-guard@latest
# or clone and build
git clone https://github.com/v01dst/sql-guard && cd sql-guard && go build -o sql-guard .
```

## Usage

```sh
# analyze the migrations/ directory
sql-guard migrations/

# treat missing down migrations as hard errors
sql-guard migrations/ --require-down

# machine-readable JSON (for scripts)
sql-guard migrations/ --format json

# GitHub Actions workflow annotations (inline on PRs)
sql-guard migrations/ --format github

# disable color in logs / CI
sql-guard migrations/ --no-color
```

Exit code is `0` when the directory is clean (or only warnings), and `1` when any error-severity finding exists — so it drops straight into a CI `run:` step.

## Migration file naming

sql-guard auto-detects versions and direction from file names. All of these styles are supported:

```
0001_create_users.up.sql
0001_create_users.down.sql
20240101000000_init.sql
V1__init.sql
3_add_index.down.sql
baseline.sql          # treated as a non-reversible baseline
```

- Files ending in `.up.sql` / `.down.sql` (or `_up` / `_down` / `-up` / `-down`) are paired by version.
- Any file with a leading numeric version is a migration.
- `baseline*` files are treated as non-versioned, non-reversible baselines.

## GitHub Actions

```yaml
name: lint-migrations
on: [pull_request]
jobs:
  sql-guard:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.21"
      - run: go install github.com/v01dst/sql-guard@latest
      - run: sql-guard migrations/ --format github --require-down
```

Findings will appear inline on the pull request as annotations, and the check will fail if anything destructive or non-reversible is merged.

## Configuration

sql-guard is intentionally zero-configuration. Flags are enough:

| Flag | Default | Description |
|------|---------|-------------|
| `--dir` | `.` | directory to scan (or pass as a positional argument) |
| `--format` | `human` | `human`, `json`, or `github` |
| `--require-down` | `false` | promote `missing-down` from warning to error |
| `--no-color` | `false` | disable ANSI colors |
| `--version` | — | print version and exit |

<details>
<summary><b>Full JSON schema</b></summary>

```json
{
  "migrations": [
    { "file": "migrations/0001_create_users.up.sql", "version": "0001", "is_down": false, "is_baseline": false }
  ],
  "findings": [
    {
      "file": "migrations/0001_create_users.up.sql",
      "version": "0001",
      "severity": "error",
      "rule": "missing-down",
      "message": "up migration has no matching down (rollback) migration",
      "line": 1,
      "hints": ["add a 0001.down.sql to make this migration reversible"]
    }
  ]
}
```

</details>

## How it works

sql-guard uses a lightweight, comment-aware SQL tokenizer rather than a full SQL grammar. This trade-off keeps the tool fast (thousands of lines per second), portable (zero dependencies — pure Go standard library), and predictable. It is deliberately heuristic and every rule is documented with its limitations in the source.

```
                    ┌─────────────────────────────┐
                    │        Discover files       │
                    │  (version / up / down / base)│
                    └──────────────┬──────────────┘
                                   ▼
                    ┌─────────────────────────────┐
                    │      Tokenize SQL           │
                    │  (strings, comments, $$)    │
                    └──────────────┬──────────────┘
                                   ▼
          ┌────────────────────────┴────────────────────────┐
          ▼                       ▼                          ▼
  destructive rules      missing-WHERE rules         cross-file rules
 (DROP/TRUNCATE/ALTER)   (DELETE/UPDATE)       (down pairs, collisions, order)
          │                       │                          │
          └───────────────────────┴──────────────────────────┘
                                   ▼
                     human / json / github output
```

## Development

```sh
git clone https://github.com/v01dst/sql-guard
cd sql-guard
go build ./...          # build
go test ./...           # run tests
go vet ./...            # static analysis
```

The codebase is intentionally small and dependency-free. `internal/analyzer` holds the tokenizer, rules, and discovery logic; `main.go` is the CLI. Tests cover the tokenizer, every rule, and end-to-end discovery.

## Testing

```sh
$ go test ./...
ok   github.com/v01dst/sql-guard/internal/analyzer   0.139s
```

Tests cover:
- string / comment / dollar-quote suppression (no false positives)
- line-number accuracy
- every destructive rule
- `WHERE` detection
- up/down pairing, collisions, ordering
- empty-directory and baseline handling

## Limitations

- sql-guard analyzes migration files **statically** — it does not connect to a database or detect drift against a live schema.
- The `missing-WHERE` check is statement-scoped via `;` boundaries; multi-statement files are handled per statement.
- Rules target common dialect-neutral SQL. Dialect-specific features (e.g. Postgres `CREATE INDEX CONCURRENTLY`) may be treated conservatively.

For these, pair sql-guard with a live-schema drift tool.

## License

MIT © [v01dst](https://github.com/v01dst)

## Connect

Questions, feedback, or ideas? Find me on Discord: **9p.1**