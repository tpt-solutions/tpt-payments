// Package migrate applies the embedded SQL migrations to a database.
//
// Each migration file runs in its own transaction together with the row that records it in
// schema_migrations, so a failed file leaves nothing half-applied. A PostgreSQL advisory lock
// serialises runs, so several replicas or a Helm Job can call Up at once without racing.
//
// A database that was built with Atlas (the compose stack's migration service) has an
// atlas_schema_revisions table and its tables already exist. Up refuses such a database rather
// than re-running CREATE statements over it.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// lockKey identifies the advisory lock that serialises migration runs.
const lockKey int64 = 7154203911

// fileName is the shape every migration file must have: a 14-digit timestamp, an underscore, and a
// lower-case name. The timestamp is the version, and versions sort in apply order.
var fileName = regexp.MustCompile(`^[0-9]{14}_[a-z0-9_]+\.sql$`)

const ensureTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT        PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`

// Up applies every migration in fsys that is not yet recorded, in filename order, and returns the
// versions it applied. Re-running it is a no-op.
func Up(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) ([]string, error) {
	files, err := migrationFiles(fsys)
	if err != nil {
		return nil, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		return nil, fmt.Errorf("migrate: take lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", lockKey)
	}()

	if err := refuseAtlasDatabase(ctx, conn); err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, ensureTable); err != nil {
		return nil, fmt.Errorf("migrate: create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return nil, err
	}

	var ran []string
	for _, name := range pendingMigrations(files, applied) {
		sql, err := fs.ReadFile(fsys, name)
		if err != nil {
			return ran, fmt.Errorf("migrate: read %s: %w", name, err)
		}
		version := strings.TrimSuffix(name, ".sql")
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version)
			return err
		})
		if err != nil {
			return ran, fmt.Errorf("migrate: apply %s: %w", name, err)
		}
		ran = append(ran, version)
	}
	return ran, nil
}

// migrationFiles lists the migration files in fsys in apply order and rejects any that do not
// follow the naming rule or that share a version.
func migrationFiles(fsys fs.FS) ([]string, error) {
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("migrate: list migrations: %w", err)
	}
	if len(names) == 0 {
		return nil, errors.New("migrate: no migrations found")
	}
	sort.Strings(names)
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if !fileName.MatchString(name) {
			return nil, fmt.Errorf("migrate: %q does not match <14-digit timestamp>_<name>.sql", name)
		}
		// The timestamp is the version: two files with one timestamp cannot be ordered.
		stamp := name[:14]
		if seen[stamp] {
			return nil, fmt.Errorf("migrate: duplicate version %s", stamp)
		}
		seen[stamp] = true
	}
	return names, nil
}

// pendingMigrations returns the files whose versions are not in applied, keeping their order.
// files must already be sorted, as migrationFiles returns them.
func pendingMigrations(files []string, applied map[string]bool) []string {
	var pending []string
	for _, name := range files {
		if !applied[strings.TrimSuffix(name, ".sql")] {
			pending = append(pending, name)
		}
	}
	return pending
}

func appliedVersions(ctx context.Context, conn *pgxpool.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrate: read applied versions: %w", err)
	}
	defer rows.Close()
	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("migrate: scan version: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// refuseAtlasDatabase stops Up on a database whose schema was created by Atlas and that has no
// schema_migrations table. Such a database needs its own migration path, not a second set of
// CREATE statements.
func refuseAtlasDatabase(ctx context.Context, conn *pgxpool.Conn) error {
	var atlas, ours bool
	if err := conn.QueryRow(ctx, `
		SELECT to_regclass('atlas_schema_revisions') IS NOT NULL,
		       to_regclass('schema_migrations') IS NOT NULL
	`).Scan(&atlas, &ours); err != nil {
		return fmt.Errorf("migrate: inspect database: %w", err)
	}
	if atlas && !ours {
		return errors.New("migrate: this database was created by Atlas (atlas_schema_revisions exists) " +
			"and has no schema_migrations table; keep migrating it with Atlas, or start from an empty database")
	}
	return nil
}
