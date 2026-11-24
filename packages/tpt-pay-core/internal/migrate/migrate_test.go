package migrate

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/migrations"
)

func TestMigrationFiles_EmbeddedSetIsValid(t *testing.T) {
	files, err := migrationFiles(migrations.FS)
	require.NoError(t, err)
	require.NotEmpty(t, files)
	// The deferred payout migration lives in migrations_deferred/ and must not be embedded.
	assert.NotContains(t, files, "20260101000008_create_payout_schedule_enhanced.sql")
}

func TestMigrationFiles_RejectsBadNames(t *testing.T) {
	_, err := migrationFiles(fstest.MapFS{"create_things.sql": {Data: []byte("")}})
	require.Error(t, err)
}

func TestMigrationFiles_RejectsDuplicateVersions(t *testing.T) {
	_, err := migrationFiles(fstest.MapFS{
		"20260101000001_a.sql": {Data: []byte("")},
		"20260101000001_b.sql": {Data: []byte("")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate version")
}

func TestMigrationFiles_RejectsAnEmptySet(t *testing.T) {
	_, err := migrationFiles(fstest.MapFS{})
	require.Error(t, err)
}

func TestMigrationFiles_SortsIntoApplyOrder(t *testing.T) {
	files, err := migrationFiles(fstest.MapFS{
		"20260101000010_later.sql": {Data: []byte("")},
		"20260101000002_early.sql": {Data: []byte("")},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"20260101000002_early.sql", "20260101000010_later.sql"}, files)
}

func TestPendingMigrations_SkipsAppliedKeepingOrder(t *testing.T) {
	files := []string{"20260101000001_a.sql", "20260101000002_b.sql", "20260101000003_c.sql"}
	applied := map[string]bool{"20260101000001_a": true, "20260101000003_c": true}
	assert.Equal(t, []string{"20260101000002_b.sql"}, pendingMigrations(files, applied))
	assert.Empty(t, pendingMigrations(files, map[string]bool{
		"20260101000001_a": true, "20260101000002_b": true, "20260101000003_c": true,
	}))
}
