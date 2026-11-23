package apikey

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRawKey_PrefixesFollowTypeAndMode(t *testing.T) {
	cases := []struct {
		typ    KeyType
		mode   Mode
		prefix string
	}{
		{TypeSecret, ModeTest, "sk_test_"},
		{TypeSecret, ModeLive, "sk_live_"},
		{TypePublishable, ModeTest, "pk_test_"},
		{TypePublishable, ModeLive, "pk_live_"},
	}
	for _, c := range cases {
		key, err := buildRawKey(c.typ, c.mode)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(key, c.prefix), "key %q should start with %q", key, c.prefix)
		assert.True(t, looksLikeKey(key), "generated key must pass the pre-check")
	}
}

func TestBuildRawKey_IsUniqueAndHighEntropy(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		key, err := buildRawKey(TypeSecret, ModeTest)
		require.NoError(t, err)
		assert.False(t, seen[key], "duplicate key generated")
		seen[key] = true
		// 24 random bytes encode to 32 base64url characters after the prefix.
		assert.Len(t, strings.TrimPrefix(key, "sk_test_"), 32)
	}
}

func TestDigest_IsStableAndNotTheKey(t *testing.T) {
	raw := "sk_test_abcdefghijklmnop"
	d := Digest(raw)
	assert.Equal(t, d, Digest(raw), "same key, same digest")
	assert.NotEqual(t, d, Digest(raw+"x"), "different key, different digest")
	assert.Len(t, d, 64, "hex SHA-256")
	assert.NotContains(t, d, raw, "the digest must not contain the key")
}

func TestLooksLikeKey_RejectsNonKeys(t *testing.T) {
	assert.False(t, looksLikeKey(""))
	assert.False(t, looksLikeKey("sk_short"), "too short to be a key")
	assert.False(t, looksLikeKey("Bearer sk_test_abcdefghijklmnop"), "must start with the key prefix")
	assert.False(t, looksLikeKey("rk_test_abcdefghijklmnop"), "unknown prefix")
	assert.False(t, looksLikeKey("sk_test_"+strings.Repeat("a", 250)), "too long")
	assert.True(t, looksLikeKey("sk_test_abcdefghijklmnop"))
}
