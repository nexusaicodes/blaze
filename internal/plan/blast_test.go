package plan

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sakshamsaxena/blaze/internal/dodge"
)

// blastSym builds a resolved, non-deleted symbol for the blast tests.
func blastSym(id, pkg, name, file string, kind dodge.SymbolKind, isTest bool) *dodge.Symbol {
	return &dodge.Symbol{
		ID: dodge.SymbolID(id), Kind: kind, Pkg: pkg, PkgName: pkg, Name: name,
		File: file, IsTest: isTest, Exported: true, Resolved: true,
	}
}

// blastGraph returns an empty Graph indexing syms by ID, ready for its
// caller to fill in In and Out.
func blastGraph(syms ...*dodge.Symbol) *dodge.Graph {
	g := &dodge.Graph{
		Symbols: map[dodge.SymbolID]*dodge.Symbol{},
		In:      map[dodge.SymbolID][]dodge.Ref{},
		Out:     map[dodge.SymbolID][]dodge.Ref{},
	}
	for _, s := range syms {
		g.Symbols[s.ID] = s
	}
	return g
}

// noCache is a blastCache over a worktree that has no files, so every
// snippet lookup misses and returns "".
func noCache() *blastCache {
	return newBlastCache("")
}

func TestBlast(t *testing.T) {
	t.Run("direct caller", func(t *testing.T) {
		s := blastSym("p.Save", "p", "Save", "p/f.go", dodge.KindMethod, false)
		caller := blastSym("q.Caller", "q", "Caller", "q/g.go", dodge.KindFunc, false)
		g := blastGraph(s, caller)
		g.In[s.ID] = []dodge.Ref{{From: caller.ID, To: s.ID, Kind: dodge.RefCall, File: "q/g.go", Line: 5}}

		b := blastFor(g, s, map[dodge.SymbolID]*Item{}, noCache())

		require.Equal(t, []Caller{{
			SymbolID: "q.Caller", Name: "Caller", Pkg: "q", File: "q/g.go", Line: 5,
			Sites: 1, Kind: dodge.RefCall, Via: "", InPR: false, IsTest: false, Snippet: "",
		}}, b.Callers)
		require.Equal(t, 1, b.OutsideCount)
		require.Equal(t, 0, b.InPRCount)
		require.Equal(t, 0, b.TestCount)
		require.Equal(t, 1, b.OtherPackages, "caller's package q differs from s's package p")
		require.False(t, b.Truncated)
	})

	t.Run("through interface", func(t *testing.T) {
		iface := blastSym("p.Store", "p", "Store", "p/store.go", dodge.KindType, false)
		impl := blastSym("p.Impl.Save", "p", "Impl.Save", "p/impl.go", dodge.KindMethod, false)
		caller := blastSym("q.Put", "q", "Put", "q/g.go", dodge.KindFunc, false)
		g := blastGraph(iface, impl, caller)
		g.Out[impl.ID] = []dodge.Ref{{From: impl.ID, To: iface.ID, Kind: dodge.RefImplements, Member: "Save", File: "p/impl.go", Line: 1}}
		g.In[iface.ID] = []dodge.Ref{{From: caller.ID, To: iface.ID, Kind: dodge.RefCall, Member: "Save", File: "q/g.go", Line: 9}}

		b := blastFor(g, impl, map[dodge.SymbolID]*Item{}, noCache())

		require.Len(t, b.Callers, 1)
		c := b.Callers[0]
		require.Equal(t, "q.Put", c.SymbolID)
		require.Equal(t, dodge.RefCall, c.Kind)
		require.Equal(t, "Store", c.Via, "reached through the Store interface")
		require.Equal(t, 1, b.OutsideCount)

		t.Run("an implements ref alone is not a caller", func(t *testing.T) {
			concrete := blastSym("p.Impl", "p", "Impl", "p/impl.go", dodge.KindType, false)
			g2 := blastGraph(iface, concrete)
			g2.In[iface.ID] = []dodge.Ref{{From: concrete.ID, To: iface.ID, Kind: dodge.RefImplements, File: "p/impl.go", Line: 1}}
			b2 := blastFor(g2, iface, map[dodge.SymbolID]*Item{}, noCache())
			require.Empty(t, b2.Callers)
		})
	})

	t.Run("counts across outside, in-PR and test callers", func(t *testing.T) {
		s := blastSym("p.Target", "p", "Target", "p/f.go", dodge.KindFunc, false)
		out1 := blastSym("a.Out1", "a", "Out1", "a/f.go", dodge.KindFunc, false)
		out2 := blastSym("b.Out2", "b", "Out2", "b/f.go", dodge.KindFunc, false)
		inPR := blastSym("p.InPR", "p", "InPR", "p/g.go", dodge.KindFunc, false)
		testUnchanged := blastSym("p.TestUnchanged", "p", "TestUnchanged", "p/f_test.go", dodge.KindFunc, true)
		testChanged := blastSym("p.TestChanged", "p", "TestChanged", "p/f_test.go", dodge.KindFunc, true)
		g := blastGraph(s, out1, out2, inPR, testUnchanged, testChanged)
		g.In[s.ID] = []dodge.Ref{
			{From: out1.ID, To: s.ID, Kind: dodge.RefCall, File: "a/f.go", Line: 1},
			{From: out2.ID, To: s.ID, Kind: dodge.RefCall, File: "b/f.go", Line: 1},
			{From: inPR.ID, To: s.ID, Kind: dodge.RefCall, File: "p/g.go", Line: 1},
			{From: testUnchanged.ID, To: s.ID, Kind: dodge.RefCall, File: "p/f_test.go", Line: 1},
			{From: testChanged.ID, To: s.ID, Kind: dodge.RefCall, File: "p/f_test.go", Line: 2},
		}
		itemOf := map[dodge.SymbolID]*Item{
			inPR.ID:        {ID: "i_inpr"},
			testChanged.ID: {ID: "i_testchanged"},
		}

		b := blastFor(g, s, itemOf, noCache())

		require.Equal(t, 2, b.OutsideCount)
		require.Equal(t, 1, b.InPRCount)
		require.Equal(t, 2, b.TestCount)
		require.Equal(t, 1, b.ChangedTests)
		require.Equal(t, 2, b.OtherPackages, "outside callers come from packages a and b, both distinct from p")
	})

	t.Run("sort order: outside, then in-PR, then test; each by (pkg, file, line)", func(t *testing.T) {
		s := blastSym("p.Target", "p", "Target", "p/f.go", dodge.KindFunc, false)
		aY10 := blastSym("a.Y10", "a", "Y10", "a/y.go", dodge.KindFunc, false)
		aZ1 := blastSym("a.Z1", "a", "Z1", "a/z.go", dodge.KindFunc, false)
		bY5 := blastSym("b.Y5", "b", "Y5", "b/y.go", dodge.KindFunc, false)
		inPR := blastSym("p.InPR", "p", "InPR", "p/g.go", dodge.KindFunc, false)
		test := blastSym("p.Test", "p", "Test", "p/f_test.go", dodge.KindFunc, true)
		g := blastGraph(s, aY10, aZ1, bY5, inPR, test)
		g.In[s.ID] = []dodge.Ref{
			{From: bY5.ID, To: s.ID, Kind: dodge.RefCall, File: "b/y.go", Line: 5},
			{From: aZ1.ID, To: s.ID, Kind: dodge.RefCall, File: "a/z.go", Line: 1},
			{From: test.ID, To: s.ID, Kind: dodge.RefCall, File: "p/f_test.go", Line: 1},
			{From: aY10.ID, To: s.ID, Kind: dodge.RefCall, File: "a/y.go", Line: 10},
			{From: inPR.ID, To: s.ID, Kind: dodge.RefCall, File: "p/g.go", Line: 1},
		}
		itemOf := map[dodge.SymbolID]*Item{inPR.ID: {ID: "i_inpr"}}

		b := blastFor(g, s, itemOf, noCache())

		var got []string
		for _, c := range b.Callers {
			got = append(got, c.SymbolID)
		}
		require.Equal(t, []string{"a.Y10", "a.Z1", "b.Y5", "p.InPR", "p.Test"}, got)
	})

	t.Run("truncation at 200", func(t *testing.T) {
		s := blastSym("p.Target", "p", "Target", "p/f.go", dodge.KindFunc, false)
		g := blastGraph(s)
		const n = 250
		var refs []dodge.Ref
		for i := 0; i < n; i++ {
			id := dodge.SymbolID(fmt.Sprintf("a.Out%03d", i))
			g.Symbols[id] = blastSym(string(id), "a", fmt.Sprintf("Out%03d", i), "a/f.go", dodge.KindFunc, false)
			refs = append(refs, dodge.Ref{From: id, To: s.ID, Kind: dodge.RefCall, File: "a/f.go", Line: i + 1})
		}
		g.In[s.ID] = refs

		b := blastFor(g, s, map[dodge.SymbolID]*Item{}, noCache())

		require.Len(t, b.Callers, MaxCallers)
		require.True(t, b.Truncated)
		require.Equal(t, n, b.OutsideCount, "counts are taken before truncation")
	})

	t.Run("transitive count stops at depth 3", func(t *testing.T) {
		s := blastSym("p.S", "p", "S", "p/f.go", dodge.KindFunc, false)
		a := blastSym("p.A", "p", "A", "p/f.go", dodge.KindFunc, false)
		b := blastSym("p.B", "p", "B", "p/f.go", dodge.KindFunc, false)
		c := blastSym("p.C", "p", "C", "p/f.go", dodge.KindFunc, false)
		d := blastSym("p.D", "p", "D", "p/f.go", dodge.KindFunc, false)
		g := blastGraph(s, a, b, c, d)
		g.In[s.ID] = []dodge.Ref{{From: a.ID, To: s.ID, Kind: dodge.RefCall, File: "p/f.go", Line: 1}}
		g.In[a.ID] = []dodge.Ref{{From: b.ID, To: a.ID, Kind: dodge.RefCall, File: "p/f.go", Line: 2}}
		g.In[b.ID] = []dodge.Ref{{From: c.ID, To: b.ID, Kind: dodge.RefCall, File: "p/f.go", Line: 3}}
		g.In[c.ID] = []dodge.Ref{{From: d.ID, To: c.ID, Kind: dodge.RefCall, File: "p/f.go", Line: 4}}

		count, capped := transitiveCount(g, s)

		require.Equal(t, 3, count, "a, b and c are within 3 hops; d is a 4th hop and is excluded")
		require.False(t, capped)
	})

	t.Run("transitive count does not expand through a test symbol", func(t *testing.T) {
		s := blastSym("p.S", "p", "S", "p/f.go", dodge.KindFunc, false)
		test := blastSym("p.T", "p", "T", "p/f_test.go", dodge.KindFunc, true)
		beyond := blastSym("p.X", "p", "X", "p/f.go", dodge.KindFunc, false)
		g := blastGraph(s, test, beyond)
		g.In[s.ID] = []dodge.Ref{{From: test.ID, To: s.ID, Kind: dodge.RefCall, File: "p/f_test.go", Line: 1}}
		g.In[test.ID] = []dodge.Ref{{From: beyond.ID, To: test.ID, Kind: dodge.RefCall, File: "p/f.go", Line: 1}}

		count, capped := transitiveCount(g, s)

		require.Equal(t, 0, count, "the test caller is reached but not counted, and is not expanded through")
		require.False(t, capped)
	})

	t.Run("transitive count stops and reports capped at the visited cap", func(t *testing.T) {
		s := blastSym("p.S", "p", "S", "p/f.go", dodge.KindFunc, false)
		g := blastGraph(s)
		const n = 2500
		var refs []dodge.Ref
		for i := 0; i < n; i++ {
			id := dodge.SymbolID(fmt.Sprintf("a.N%04d", i))
			g.Symbols[id] = blastSym(string(id), "a", fmt.Sprintf("N%04d", i), "a/f.go", dodge.KindFunc, false)
			refs = append(refs, dodge.Ref{From: id, To: s.ID, Kind: dodge.RefCall, File: "a/f.go", Line: i + 1})
		}
		g.In[s.ID] = refs

		count, capped := transitiveCount(g, s)

		require.True(t, capped)
		require.Equal(t, TransitiveCap-1, count, "the search stops the instant the visited set (s plus the symbols counted so far) reaches the cap")
	})
}
