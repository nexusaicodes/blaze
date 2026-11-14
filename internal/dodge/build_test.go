package dodge_test

import (
	"context"
	"testing"

	"github.com/sakshamsaxena/blaze/internal/dodge"
	"github.com/sakshamsaxena/blaze/internal/testutil"
	"github.com/stretchr/testify/require"
)

// buildFixtureGraph builds the symbol graph of the 08-fixture.md fixture at
// its head commit.
func buildFixtureGraph(t *testing.T) *dodge.Graph {
	t.Helper()
	fx := testutil.NewFixture(t)
	gp, err := dodge.New("go")
	require.NoError(t, err)
	g, err := gp.Build(context.Background(), dodge.BuildInput{RepoRoot: fx.Dir, ModuleDir: "."})
	require.NoError(t, err)
	return g
}

// TestBuildSymbols asserts the exact symbol ID set of the fixture at head,
// and the kind, file and line range of each, against 08-fixture.md.
func TestBuildSymbols(t *testing.T) {
	g := buildFixtureGraph(t)
	const p = "example.com/fixture"

	want := []struct {
		id    string
		kind  dodge.SymbolKind
		file  string
		start int
		end   int
	}{
		{p + "/cmd/app.main", dodge.KindFunc, "cmd/app/main.go", 10, 16},
		{p + "/service.Service", dodge.KindType, "service/service.go", 5, 8},
		{p + "/service.New", dodge.KindFunc, "service/service.go", 10, 13},
		{p + "/service.Service.Put", dodge.KindMethod, "service/service.go", 15, 18},
		{p + "/service.Service.Get", dodge.KindMethod, "service/service.go", 20, 27},
		{p + "/service.Service.Remove", dodge.KindMethod, "service/service.go", 29, 32},
		{p + "/store.ErrNotFound", dodge.KindVar, "store/store.go", 5, 6},
		{p + "/store.Record", dodge.KindType, "store/store.go", 8, 13},
		{p + "/store.Store", dodge.KindType, "store/store.go", 15, 20},
		{p + "/store.MemStore", dodge.KindType, "store/store.go", 22, 25},
		{p + "/store.NewMemStore", dodge.KindFunc, "store/store.go", 27, 30},
		{p + "/store.MemStore.Save", dodge.KindMethod, "store/store.go", 32, 37},
		{p + "/store.MemStore.Load", dodge.KindMethod, "store/store.go", 39, 46},
		{p + "/store.MemStore.Delete", dodge.KindMethod, "store/store.go", 48, 52},
		{p + "/store.TestSave", dodge.KindFunc, "store/store_test.go", 5, 10},
		{p + "/store.TestDelete", dodge.KindFunc, "store/store_test.go", 12, 17},
		{p + "/store_test.TestLoadMissing", dodge.KindFunc, "store/ext_test.go", 9, 14},
	}

	gotIDs := make([]string, 0, len(g.Symbols))
	for id := range g.Symbols {
		gotIDs = append(gotIDs, string(id))
	}
	wantIDs := make([]string, 0, len(want))
	for _, w := range want {
		wantIDs = append(wantIDs, w.id)
	}
	require.ElementsMatch(t, wantIDs, gotIDs, "exact symbol ID set")

	for _, w := range want {
		sym, ok := g.Symbols[dodge.SymbolID(w.id)]
		require.True(t, ok, "missing symbol %s", w.id)
		require.Equal(t, w.kind, sym.Kind, "%s kind", w.id)
		require.Equal(t, w.file, sym.File, "%s file", w.id)
		require.Equal(t, w.start, sym.StartLine, "%s startLine", w.id)
		require.Equal(t, w.end, sym.EndLine, "%s endLine", w.id)
		require.True(t, sym.Resolved, "%s resolved", w.id)
	}

	require.True(t, g.Symbols[dodge.SymbolID(p+"/store.Store")].IsInterface)
	require.True(t, g.Symbols[dodge.SymbolID(p+"/store.TestSave")].IsTest)
}

// TestBuildRefs asserts the exact refs observed for Service.Put, and the
// presence of every other ref listed in 08-fixture.md.
func TestBuildRefs(t *testing.T) {
	g := buildFixtureGraph(t)
	const p = "example.com/fixture"

	type simpleRef struct {
		To     dodge.SymbolID
		Kind   dodge.RefKind
		Member string
	}
	simplify := func(refs []dodge.Ref) []simpleRef {
		out := make([]simpleRef, len(refs))
		for i, r := range refs {
			out[i] = simpleRef{To: r.To, Kind: r.Kind, Member: r.Member}
		}
		return out
	}

	put := dodge.SymbolID(p + "/service.Service.Put")
	require.ElementsMatch(t, []simpleRef{
		{dodge.SymbolID(p + "/service.Service"), dodge.RefUse, ""},
		{dodge.SymbolID(p + "/service.Service"), dodge.RefUse, "st"},
		{dodge.SymbolID(p + "/store.Store"), dodge.RefCall, "Save"},
		{dodge.SymbolID(p + "/store.Record"), dodge.RefUse, ""},
		{dodge.SymbolID(p + "/store.Record"), dodge.RefUse, "ID"},
		{dodge.SymbolID(p + "/store.Record"), dodge.RefUse, "Body"},
	}, simplify(g.Out[put]), "the exact refs observed on the fixture for Service.Put")

	has := func(from, to, member string, kind dodge.RefKind) bool {
		for _, r := range g.Out[dodge.SymbolID(from)] {
			if r.To == dodge.SymbolID(to) && r.Kind == kind && r.Member == member {
				return true
			}
		}
		return false
	}

	cases := []struct {
		name   string
		from   string
		to     string
		kind   dodge.RefKind
		member string
	}{
		{"main->New", p + "/cmd/app.main", p + "/service.New", dodge.RefCall, ""},
		{"main->NewMemStore", p + "/cmd/app.main", p + "/store.NewMemStore", dodge.RefCall, ""},
		{"main->Put", p + "/cmd/app.main", p + "/service.Service.Put", dodge.RefCall, "Put"},
		{"main->Get", p + "/cmd/app.main", p + "/service.Service.Get", dodge.RefCall, "Get"},
		{"Get->Store.Load", p + "/service.Service.Get", p + "/store.Store", dodge.RefCall, "Load"},
		{"TestSave->NewMemStore", p + "/store.TestSave", p + "/store.NewMemStore", dodge.RefCall, ""},
		{"TestSave->MemStore.Save", p + "/store.TestSave", p + "/store.MemStore.Save", dodge.RefCall, "Save"},
		{"TestLoadMissing->NewMemStore", p + "/store_test.TestLoadMissing", p + "/store.NewMemStore", dodge.RefCall, ""},
		{"TestLoadMissing->MemStore.Load", p + "/store_test.TestLoadMissing", p + "/store.MemStore.Load", dodge.RefCall, "Load"},
		{"TestLoadMissing->ErrNotFound", p + "/store_test.TestLoadMissing", p + "/store.ErrNotFound", dodge.RefUse, ""},
		{"Remove->Store.Delete", p + "/service.Service.Remove", p + "/store.Store", dodge.RefCall, "Delete"},
		{"TestDelete->NewMemStore", p + "/store.TestDelete", p + "/store.NewMemStore", dodge.RefCall, ""},
		{"TestDelete->MemStore.Delete", p + "/store.TestDelete", p + "/store.MemStore.Delete", dodge.RefCall, "Delete"},
		{"MemStore.Save->Record.Version", p + "/store.MemStore.Save", p + "/store.Record", dodge.RefUse, "Version"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.True(t, has(c.from, c.to, c.member, c.kind),
				"missing ref %s -> %s (%s member=%q); got %+v", c.from, c.to, c.kind, c.member, g.Out[dodge.SymbolID(c.from)])
		})
	}
}

// TestImplements asserts the exact set of "implements" refs in the fixture
// graph at head, against 08-fixture.md.
func TestImplements(t *testing.T) {
	g := buildFixtureGraph(t)
	const p = "example.com/fixture"

	type implRef struct {
		From, To dodge.SymbolID
		Member   string
	}

	var got []implRef
	for _, refs := range g.Out {
		for _, r := range refs {
			if r.Kind == dodge.RefImplements {
				got = append(got, implRef{From: r.From, To: r.To, Member: r.Member})
			}
		}
	}

	store := dodge.SymbolID(p + "/store.Store")
	want := []implRef{
		{dodge.SymbolID(p + "/store.MemStore"), store, ""},
		{dodge.SymbolID(p + "/store.MemStore.Save"), store, "Save"},
		{dodge.SymbolID(p + "/store.MemStore.Load"), store, "Load"},
		{dodge.SymbolID(p + "/store.MemStore.Delete"), store, "Delete"},
	}
	require.ElementsMatch(t, want, got)
}

// TestBuildDeterministic checks that two builds of the same fixture
// produce deeply equal graphs.
func TestBuildDeterministic(t *testing.T) {
	fx := testutil.NewFixture(t)
	gp, err := dodge.New("go")
	require.NoError(t, err)

	in := dodge.BuildInput{RepoRoot: fx.Dir, ModuleDir: "."}
	g1, err := gp.Build(context.Background(), in)
	require.NoError(t, err)
	g2, err := gp.Build(context.Background(), in)
	require.NoError(t, err)

	require.Equal(t, g1, g2)
}
