package tracing_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/tracing"
)

func TestNew_NotExportingWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")

	tp, err := tracing.New(context.Background(), "tpt-pay-core", "test")
	require.NoError(t, err)
	assert.False(t, tp.Exporting())
	assert.NoError(t, tp.Shutdown(context.Background()))
}

func TestNew_ExportingWhenEndpointIsSet(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1") // nothing listens here; nothing is sent in this test

	tp, err := tracing.New(context.Background(), "tpt-pay-core", "test")
	require.NoError(t, err)
	assert.True(t, tp.Exporting())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = tp.Shutdown(ctx) // the flush to the dead endpoint may fail; only the setup is under test
}
