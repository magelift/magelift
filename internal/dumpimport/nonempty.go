package dumpimport

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// NonEmpty reports whether the target schema has ≥1 BASE TABLE (D-04).
func NonEmpty(ctx context.Context, opts Options) (bool, error) {
	opts = opts.withDefaults()
	runner, err := resolveRunner(opts)
	if err != nil {
		return false, err
	}
	return nonEmptyWith(ctx, runner, opts.Database)
}

func nonEmptyWith(ctx context.Context, runner mysqlRunner, database string) (bool, error) {
	// information_schema is reachable without selecting the target DB; filter by schema name.
	sql := fmt.Sprintf(
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = '%s' AND table_type = 'BASE TABLE'`,
		escapeSQLString(database),
	)
	out, err := runner.Query(ctx, "", sql)
	if err != nil {
		return false, err
	}
	// Strip mysql password warnings that may leak onto stdout in some client builds.
	out = lastNonEmptyLine(out)
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return false, fmt.Errorf("dumpimport: parse table count %q: %w", out, err)
	}
	return n >= 1, nil
}

// schemaReplace drops and recreates the target database so re-imports converge (D-04).
func schemaReplace(ctx context.Context, runner mysqlRunner, database string) error {
	db := escapeSQLIdent(database)
	script := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; CREATE DATABASE `%s`;", db, db)
	stderr, err := runner.ExecSQL(ctx, "", strings.NewReader(script))
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("dumpimport: schema-replace failed: %s", msg)
	}
	return nil
}

func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func escapeSQLIdent(s string) string {
	return strings.ReplaceAll(s, "`", "``")
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// mysql client warnings often start with "mysql:" — skip non-numeric tails when possible.
		if _, err := strconv.Atoi(line); err == nil {
			return line
		}
		if i == len(lines)-1 {
			return line
		}
	}
	return strings.TrimSpace(s)
}
