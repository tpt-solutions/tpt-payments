package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The sample file shipped at the module root must always load, so it can be copied as a starting point.
func TestExampleDocumentLoads(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "policy.example.json"))
	require.NoError(t, err)
	engine, err := Parse(data)
	require.NoError(t, err)
	assert.Len(t, engine.Rules(), 3)
	assert.Equal(t, Block, engine.Evaluate(Input{Amount: 20000000, Currency: "usd"}))
	assert.Equal(t, Flag, engine.Evaluate(Input{Amount: 60000, Currency: "usd"}))
}
