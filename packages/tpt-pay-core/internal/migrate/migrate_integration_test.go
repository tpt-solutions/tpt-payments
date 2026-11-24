package migrate_test

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/migrate"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/migrations"
)

// emptySchemaPool returns a pool bound to a new empty schema, so Up starts from nothing.
func emptySchemaPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database integration test")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(admin.Close)

	schema := "mig_" + uuid.New().String()[:8] + uuid.New().String()[:8]
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })

	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func TestIntegration_UpAppliesEverythingOnceAndIsIdempotent(t *testing.T) {
	pool := emptySchemaPool(t)
	ctx := context.Background()

	first, err := migrate.Up(ctx, pool, migrations.FS)
	require.NoError(t, err)
	assert.Len(t, first, embeddedCount(t))

	second, err := migrate.Up(ctx, pool, migrations.FS)
	require.NoError(t, err)
	assert.Empty(t, second, "a second run applies nothing")
}

// Two runs at once (two replicas, or a Job and a pod) must not race: together they apply the set
// exactly once.
func TestIntegration_ConcurrentRunsApplyEachMigrationOnce(t *testing.T) {
	pool := emptySchemaPool(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	results := make([][]string, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = migrate.Up(ctx, pool, migrations.FS)
		}(i)
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Equal(t, embeddedCount(t), len(results[0])+len(results[1]))
}

func TestIntegration_RefusesAnAtlasDatabase(t *testing.T) {
	pool := emptySchemaPool(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `CREATE TABLE atlas_schema_revisions (version TEXT PRIMARY KEY)`)
	require.NoError(t, err)

	_, err = migrate.Up(ctx, pool, migrations.FS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "created by Atlas")
}

// embeddedCount is the number of migration files in the embedded set.
func embeddedCount(t *testing.T) int {
	t.Helper()
	entries, err := migrations.FS.ReadDir(".")
	require.NoError(t, err)
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return n // the embed pattern matches only *.sql, so every entry is a migration
}
