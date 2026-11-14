package dodge

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTokenText checks the token-stream normalisation that Signature and
// BodyHash are built on: comments and layout do not affect it, a changed
// literal does, and a signature's line breaks do not either.
func TestTokenText(t *testing.T) {
	commented := []byte("func F(x int) int {\n\treturn x // comment\n}")
	respaced := []byte("func F(x int) int {\n\n\treturn    x\n}")
	changed := []byte("func F(x int) int {\n\treturn 2\n}")

	hash := func(src []byte) string {
		return hashTokenText(scanTokens(src), 0, len(src))
	}

	require.Equal(t, hash(commented), hash(respaced),
		"a comment-only edit gives an equal BodyHash")
	require.NotEqual(t, hash(commented), hash(changed),
		"a changed literal gives a different BodyHash")

	oneLine := []byte("func F(a, b int) int ")
	twoLine := []byte("func F(a,\n\tb int) int ")
	require.Equal(t, tokenText(oneLine, 0, len(oneLine)), tokenText(twoLine, 0, len(twoLine)),
		"a signature spread over two lines equals the one-line form")
}
