package dodge

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const extractSymbolsSrc = `package sample

// Add adds a and b.
func Add(a, b int) int {
	return a + b
}

func sub(a, b int) int {
	return a - b
}

// Thing is a type.
type Thing struct {
	X int
}

// Save stores a thing.
func (t *Thing) Save() error {
	return nil
}

// Set is a generic set.
type Set[T comparable] struct {
	m map[T]bool
}

func (s *Set[T]) Add(v T) {
	s.m[v] = true
}

var (
	// A is a.
	A = 1
	B, C = 2, 3
)

const (
	X1 = 1
	X2 = 2
)

func init() {
}

func init() {
}
`

// TestExtractSymbols checks kinds, names, IDs, doc-comment-inclusive spans,
// a parenthesised const block, a multi-name var, two init functions and a
// generic receiver, from literal source, syntax only.
func TestExtractSymbols(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "sample.go", extractSymbolsSrc, parser.ParseComments|parser.SkipObjectResolution)
	require.NoError(t, err)

	syms := extractSymbols(fset, f, []byte(extractSymbolsSrc), "sample.go", "example.com/sample")
	require.Len(t, syms, 12)

	byName := map[string]*Symbol{}
	for _, s := range syms {
		if s.Kind != KindFunc || s.Name != "init" {
			byName[s.Name] = s
		}
	}

	add := byName["Add"]
	require.NotNil(t, add)
	require.Equal(t, KindFunc, add.Kind)
	require.Equal(t, SymbolID("example.com/sample.Add"), add.ID)
	require.True(t, add.Exported)
	require.Equal(t, 3, add.StartLine, "doc comment included in span")
	require.Equal(t, 6, add.EndLine)
	require.Equal(t, 4, add.DeclLine)

	sub := byName["sub"]
	require.NotNil(t, sub)
	require.Equal(t, KindFunc, sub.Kind)
	require.False(t, sub.Exported)
	require.Equal(t, 8, sub.StartLine, "no doc comment: span starts at the func")
	require.Equal(t, 10, sub.EndLine)
	require.Equal(t, 8, sub.DeclLine)

	thing := byName["Thing"]
	require.NotNil(t, thing)
	require.Equal(t, KindType, thing.Kind)
	require.False(t, thing.IsInterface)
	require.Equal(t, SymbolID("example.com/sample.Thing"), thing.ID)
	require.Equal(t, 12, thing.StartLine)
	require.Equal(t, 15, thing.EndLine)
	require.Equal(t, 13, thing.DeclLine)

	save := byName["Thing.Save"]
	require.NotNil(t, save)
	require.Equal(t, KindMethod, save.Kind)
	require.True(t, save.Exported)
	require.Equal(t, SymbolID("example.com/sample.Thing.Save"), save.ID)
	require.Equal(t, 17, save.StartLine)
	require.Equal(t, 20, save.EndLine)
	require.Equal(t, 18, save.DeclLine)

	set := byName["Set"]
	require.NotNil(t, set)
	require.Equal(t, KindType, set.Kind)
	require.Equal(t, SymbolID("example.com/sample.Set"), set.ID)

	setAdd := byName["Set.Add"]
	require.NotNil(t, setAdd, "generic receiver (*Set[T]) resolves to Set")
	require.Equal(t, KindMethod, setAdd.Kind)
	require.Equal(t, SymbolID("example.com/sample.Set.Add"), setAdd.ID)
	require.Equal(t, 27, setAdd.StartLine)
	require.Equal(t, 29, setAdd.EndLine)

	a := byName["A"]
	require.NotNil(t, a)
	require.Equal(t, KindVar, a.Kind)
	require.Equal(t, SymbolID("example.com/sample.A"), a.ID)
	require.Equal(t, 32, a.StartLine, "spec's own doc comment, inside a parenthesised block")
	require.Equal(t, 33, a.EndLine)
	require.Equal(t, 33, a.DeclLine)

	b := byName["B"]
	require.NotNil(t, b, "multi-name var takes the first name")
	require.Equal(t, KindVar, b.Kind)
	require.Equal(t, 34, b.StartLine)
	require.Equal(t, 34, b.EndLine)
	_, hasC := byName["C"]
	require.False(t, hasC, "second name of a multi-name var has no symbol of its own")

	x1 := byName["X1"]
	require.NotNil(t, x1)
	require.Equal(t, KindConst, x1.Kind)
	require.Equal(t, SymbolID("example.com/sample.X1"), x1.ID)
	require.Equal(t, 38, x1.StartLine)
	require.Equal(t, 38, x1.EndLine)

	x2 := byName["X2"]
	require.NotNil(t, x2)
	require.Equal(t, KindConst, x2.Kind)
	require.Equal(t, 39, x2.StartLine)

	var inits []SymbolID
	for _, s := range syms {
		if s.Kind == KindFunc && s.Name == "init" {
			inits = append(inits, s.ID)
		}
	}
	require.Len(t, inits, 2, "two init functions get distinct symbols")
	for _, id := range inits {
		require.True(t, strings.HasPrefix(string(id), "example.com/sample.init@sample.go#"),
			"init id %q has the ordinal-suffixed form", id)
	}
	require.ElementsMatch(t, []SymbolID{
		"example.com/sample.init@sample.go#1",
		"example.com/sample.init@sample.go#2",
	}, inits)
}
