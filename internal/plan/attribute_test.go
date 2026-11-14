package plan

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sakshamsaxena/blaze/internal/diff"
	"github.com/sakshamsaxena/blaze/internal/dodge"
	"github.com/sakshamsaxena/blaze/internal/logger"
)

func TestMain(m *testing.M) {
	logger.InitNop()
	os.Exit(m.Run())
}

// finishItem runs the same per-item finalization attribute() does, so
// tests that call attributeGoFile directly see fully-formed items.
func finishItem(item *Item, key string, col symbolCollection) {
	item.Added, item.Deleted = countAddDel(item.Slices)
	item.ContentHash = contentHash(key, item.Slices)
	switch item.Kind {
	case ItemSymbol:
		finalizeSymbolItem(item, key, col)
	case ItemPreamble:
		finalizePreambleItem(item, key, col)
	}
}

// line builds a diff.Line. kind is "+", "-" or " "; oldNo/newNo follow the
// usual convention of 0 on the side that does not apply.
func line(kind diff.LineKind, oldNo, newNo int, text string) diff.Line {
	return diff.Line{Kind: kind, OldNo: oldNo, NewNo: newNo, Text: text}
}

func TestAttribute(t *testing.T) {
	t.Run("two-type hunk yields two slices with the right owners", func(t *testing.T) {
		// Mirrors the store.go example of 05-plan.md: one hunk touching
		// two types, each contributing one added line, with an untouched
		// blank line between them that must not create a preamble item.
		record := &dodge.Symbol{ID: "pkg.Record", Kind: dodge.KindType, Name: "Record", PkgName: "pkg", File: "f.go", StartLine: 1, EndLine: 5}
		store := &dodge.Symbol{ID: "pkg.Store", Kind: dodge.KindType, Name: "Store", PkgName: "pkg", File: "f.go", StartLine: 7, EndLine: 12}
		col := symbolCollection{
			headByID:   map[dodge.SymbolID]*dodge.Symbol{record.ID: record, store.ID: store},
			baseByID:   map[dodge.SymbolID]*dodge.Symbol{record.ID: record, store.ID: store},
			headByFile: map[string][]*dodge.Symbol{"f.go": {record, store}},
			baseByFile: map[string][]*dodge.Symbol{"f.go": {record, store}},
		}

		fd := diff.FileDiff{
			OldPath: "f.go", NewPath: "f.go", Status: diff.StatusModified,
			Hunks: []diff.Hunk{{Lines: []diff.Line{
				line(diff.LineContext, 1, 1, "type Record struct {"),
				line(diff.LineContext, 2, 2, "\tID   string"),
				line(diff.LineContext, 3, 3, "\tBody string"),
				line(diff.LineAdd, 0, 4, "\tVersion int"),
				line(diff.LineContext, 4, 5, "}"),
				line(diff.LineContext, 5, 6, ""),
				line(diff.LineContext, 6, 7, "// Store persists records."),
				line(diff.LineContext, 7, 8, "type Store interface {"),
				line(diff.LineContext, 8, 9, "\tSave(r Record) error"),
				line(diff.LineContext, 9, 10, "\tLoad(id string) (Record, error)"),
				line(diff.LineAdd, 0, 11, "\tDelete(id string) error"),
				line(diff.LineContext, 10, 12, "}"),
			}}},
		}

		items := map[string]*Item{}
		attributeGoFile(items, fd, col)

		require.Len(t, items, 2, "only the two symbol items, no preamble item for the dropped blank-line run")

		ri := items["pkg.Record"]
		require.NotNil(t, ri)
		require.Len(t, ri.Slices, 1)
		require.Len(t, ri.Slices[0].Lines, 5)

		si := items["pkg.Store"]
		require.NotNil(t, si)
		require.Len(t, si.Slices, 1)
		require.Len(t, si.Slices[0].Lines, 6)

		finishItem(ri, "pkg.Record", col)
		finishItem(si, "pkg.Store", col)
		require.Equal(t, 1, ri.Added)
		require.Equal(t, 0, ri.Deleted)
		require.Equal(t, 1, si.Added)
		require.Equal(t, 0, si.Deleted)
	})

	t.Run("a deleted function between two untouched ones", func(t *testing.T) {
		a := &dodge.Symbol{ID: "pkg.A", Kind: dodge.KindFunc, Name: "A", PkgName: "pkg", File: "f.go", StartLine: 1, EndLine: 2}
		b := &dodge.Symbol{ID: "pkg.B", Kind: dodge.KindFunc, Name: "B", PkgName: "pkg", File: "f.go", StartLine: 3, EndLine: 4}
		middle := &dodge.Symbol{ID: "pkg.Middle", Kind: dodge.KindFunc, Name: "Middle", PkgName: "pkg", File: "f.go", StartLine: 4, EndLine: 6}
		col := symbolCollection{
			headByID:   map[dodge.SymbolID]*dodge.Symbol{a.ID: a, b.ID: b},
			baseByID:   map[dodge.SymbolID]*dodge.Symbol{a.ID: a, middle.ID: middle, b.ID: b},
			headByFile: map[string][]*dodge.Symbol{"f.go": {a, b}},
			baseByFile: map[string][]*dodge.Symbol{"f.go": {a, middle, b}},
		}

		fd := diff.FileDiff{
			OldPath: "f.go", NewPath: "f.go", Status: diff.StatusModified,
			Hunks: []diff.Hunk{{Lines: []diff.Line{
				line(diff.LineContext, 2, 2, "} // end of A"),
				line(diff.LineDel, 4, 0, "func Middle() {"),
				line(diff.LineDel, 5, 0, "\tx()"),
				line(diff.LineDel, 6, 0, "}"),
				line(diff.LineContext, 8, 3, "func B() {"),
			}}},
		}

		items := map[string]*Item{}
		attributeGoFile(items, fd, col)

		require.Len(t, items, 1, "the untouched A and B context lines form runs with no change, so they create no item")
		mi := items["pkg.Middle"]
		require.NotNil(t, mi)
		require.Len(t, mi.Slices, 1)
		require.Len(t, mi.Slices[0].Lines, 3)

		finishItem(mi, "pkg.Middle", col)
		require.Equal(t, ChangeDeleted, mi.Status)
		require.Same(t, middle, mi.Symbol)
		require.Equal(t, 0, mi.Added)
		require.Equal(t, 3, mi.Deleted)
	})

	t.Run("lines outside any symbol go to the preamble item", func(t *testing.T) {
		col := symbolCollection{
			headByID:   map[dodge.SymbolID]*dodge.Symbol{},
			baseByID:   map[dodge.SymbolID]*dodge.Symbol{},
			headByFile: map[string][]*dodge.Symbol{"f.go": nil},
			baseByFile: map[string][]*dodge.Symbol{"f.go": nil},
		}

		fd := diff.FileDiff{
			OldPath: "f.go", NewPath: "f.go", Status: diff.StatusModified,
			Hunks: []diff.Hunk{{Lines: []diff.Line{
				line(diff.LineContext, 1, 1, "package foo"),
				line(diff.LineContext, 2, 2, ""),
				line(diff.LineAdd, 0, 3, "import \"fmt\""),
			}}},
		}

		items := map[string]*Item{}
		attributeGoFile(items, fd, col)

		require.Len(t, items, 1)
		pi := items["file:f.go#preamble"]
		require.NotNil(t, pi)
		require.Equal(t, ItemPreamble, pi.Kind)
		require.Len(t, pi.Slices, 1)
		require.Len(t, pi.Slices[0].Lines, 3)

		finishItem(pi, "file:f.go#preamble", col)
		require.Equal(t, "f.go (imports and header)", pi.Title)
		require.Equal(t, ChangeModified, pi.Status)
	})

	t.Run("a function moved between two files becomes one item with two slices", func(t *testing.T) {
		baseSym := &dodge.Symbol{ID: "pkg.Moved", Kind: dodge.KindFunc, Name: "Moved", PkgName: "pkg", File: "old.go", StartLine: 1, EndLine: 3}
		headSym := &dodge.Symbol{ID: "pkg.Moved", Kind: dodge.KindFunc, Name: "Moved", PkgName: "pkg", File: "new.go", StartLine: 5, EndLine: 7}
		col := symbolCollection{
			headByID:   map[dodge.SymbolID]*dodge.Symbol{headSym.ID: headSym},
			baseByID:   map[dodge.SymbolID]*dodge.Symbol{baseSym.ID: baseSym},
			headByFile: map[string][]*dodge.Symbol{"old.go": nil, "new.go": {headSym}},
			baseByFile: map[string][]*dodge.Symbol{"old.go": {baseSym}, "new.go": nil},
		}

		oldFd := diff.FileDiff{
			OldPath: "old.go", NewPath: "old.go", Status: diff.StatusModified,
			Hunks: []diff.Hunk{{Lines: []diff.Line{
				line(diff.LineDel, 1, 0, "func Moved() {"),
				line(diff.LineDel, 2, 0, "\treturn"),
				line(diff.LineDel, 3, 0, "}"),
			}}},
		}
		newFd := diff.FileDiff{
			OldPath: "new.go", NewPath: "new.go", Status: diff.StatusModified,
			Hunks: []diff.Hunk{{Lines: []diff.Line{
				line(diff.LineAdd, 0, 5, "func Moved() {"),
				line(diff.LineAdd, 0, 6, "\treturn"),
				line(diff.LineAdd, 0, 7, "}"),
			}}},
		}

		items := map[string]*Item{}
		attributeGoFile(items, oldFd, col)
		attributeGoFile(items, newFd, col)

		require.Len(t, items, 1)
		mi := items["pkg.Moved"]
		require.NotNil(t, mi)
		require.Len(t, mi.Slices, 2)

		finishItem(mi, "pkg.Moved", col)
		require.Equal(t, ChangeModified, mi.Status)
		require.Same(t, headSym, mi.Symbol)
		require.Equal(t, "new.go", mi.Path)
		require.Equal(t, 3, mi.Added)
		require.Equal(t, 3, mi.Deleted)
	})
}

func TestContentHashStable(t *testing.T) {
	slices := []Slice{{
		Path: "f.go", OldStart: 1, NewStart: 1,
		Lines: []diff.Line{
			line(diff.LineContext, 1, 1, "package foo"),
			line(diff.LineAdd, 0, 2, "import \"fmt\""),
			line(diff.LineDel, 2, 0, "import \"os\""),
		},
	}}

	h1 := contentHash("pkg.Key", slices)
	h2 := contentHash("pkg.Key", slices)
	require.Equal(t, h1, h2, "the same key and slices always hash the same")

	changed := []Slice{{
		Path: "f.go", OldStart: 1, NewStart: 1,
		Lines: []diff.Line{
			line(diff.LineContext, 1, 1, "package foo"),
			line(diff.LineAdd, 0, 2, "import \"fmt2\""), // one changed line
			line(diff.LineDel, 2, 0, "import \"os\""),
		},
	}}
	h3 := contentHash("pkg.Key", changed)
	require.NotEqual(t, h1, h3, "a changed line gives a different hash")

	h4 := contentHash("pkg.Other", slices)
	require.NotEqual(t, h1, h4, "a different key gives a different hash")
}
