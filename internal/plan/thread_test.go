package plan

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sakshamsaxena/blaze/internal/dodge"
)

// mkItem builds a symbol item for the threading tests: role, path and
// StartLine are what step 4 keys off; pkg and status fill in the rest.
func mkItem(key string, role Role, pkg, path string, line int, status ChangeStatus) *Item {
	return &Item{
		Key:    key,
		ID:     hashID("i", key),
		Kind:   ItemSymbol,
		Role:   role,
		Status: status,
		Symbol: &dodge.Symbol{ID: dodge.SymbolID(key), Kind: dodge.KindFunc, PkgName: pkg, Name: key, File: path, StartLine: line, EndLine: line},
		Path:   path,
		Title:  pkg + "." + key,
		Uses:   []Link{},
		UsedBy: []Link{},
	}
}

// addUse records a Link{Kind, Member} from a to b on both sides, exactly
// as link (step 3) would.
func addUse(a, b *Item, kind dodge.RefKind, member string) {
	a.Uses = append(a.Uses, Link{ItemID: b.ID, Kind: kind, Member: member})
	b.UsedBy = append(b.UsedBy, Link{ItemID: a.ID, Kind: kind, Member: member})
}

// byID indexes items by ID for buildThreads, which takes a map.
func byID(items ...*Item) map[string]*Item {
	m := make(map[string]*Item, len(items))
	for _, it := range items {
		m[it.ID] = it
	}
	return m
}

// groupContaining returns the group holding item id, or nil.
func groupContaining(groups []*group, id string) *group {
	for _, g := range groups {
		for _, it := range g.items {
			if it.ID == id {
				return g
			}
		}
	}
	return nil
}

func TestThreads(t *testing.T) {
	t.Run("link builds Uses and UsedBy from the graph, deduplicated and sorted", func(t *testing.T) {
		a := mkItem("a", RoleLogic, "p", "p/a.go", 1, ChangeModified)
		b := mkItem("b", RoleLogic, "p", "p/b.go", 1, ChangeModified)
		m := mkItem("m", RoleMechanical, "p", "p/m.go", 1, ChangeModified) // not a code item

		g := &dodge.Graph{Out: map[dodge.SymbolID][]dodge.Ref{
			dodge.SymbolID("a"): {
				{From: "a", To: "b", Kind: dodge.RefCall},
				{From: "a", To: "b", Kind: dodge.RefCall}, // duplicate, must collapse
				{From: "a", To: "b", Kind: dodge.RefUse, Member: "X"},
				{From: "a", To: "m", Kind: dodge.RefCall}, // m is not a code item: no link
			},
		}}

		items := byID(a, b, m)
		link(items, g)

		require.Equal(t, []Link{{ItemID: b.ID, Kind: dodge.RefCall}, {ItemID: b.ID, Kind: dodge.RefUse, Member: "X"}}, a.Uses)
		require.Equal(t, []Link{{ItemID: a.ID, Kind: dodge.RefCall}, {ItemID: a.ID, Kind: dodge.RefUse, Member: "X"}}, b.UsedBy)
		require.Empty(t, b.Uses)
		require.Empty(t, m.Uses)
		require.Empty(t, m.UsedBy)
	})

	t.Run("two unrelated pairs produce two threads", func(t *testing.T) {
		a1 := mkItem("a1", RoleLogic, "pkg1", "pkg1/f.go", 1, ChangeModified)
		b1 := mkItem("b1", RoleLogic, "pkg1", "pkg1/f.go", 10, ChangeModified)
		addUse(a1, b1, dodge.RefCall, "")

		a2 := mkItem("a2", RoleLogic, "pkg2", "pkg2/f.go", 1, ChangeModified)
		b2 := mkItem("b2", RoleLogic, "pkg2", "pkg2/f.go", 10, ChangeModified)
		addUse(a2, b2, dodge.RefCall, "")

		groups := buildThreads(byID(a1, b1, a2, b2))

		require.Len(t, groups, 2)
		g1, g2 := groupContaining(groups, a1.ID), groupContaining(groups, a2.ID)
		require.NotNil(t, g1)
		require.NotNil(t, g2)
		require.NotSame(t, g1, g2)
		require.Equal(t, ThreadCode, g1.kind)
		require.Len(t, g1.items, 2)
		require.Len(t, g2.items, 2)
	})

	t.Run("a hub with five users is split into its own thread", func(t *testing.T) {
		h := mkItem("h", RoleLogic, "p", "p/h.go", 1, ChangeModified)
		var users []*Item
		for i := 0; i < HubMinUsers; i++ {
			u := mkItem(fmt.Sprintf("u%d", i), RoleLogic, "p", fmt.Sprintf("p/u%d.go", i), 1, ChangeModified)
			addUse(u, h, dodge.RefCall, "")
			users = append(users, u)
		}

		all := append([]*Item{h}, users...)
		groups := buildThreads(byID(all...))

		hg := groupContaining(groups, h.ID)
		require.NotNil(t, hg)
		require.Equal(t, ThreadShared, hg.kind)
		require.Len(t, hg.items, 1)

		for _, u := range users {
			ug := groupContaining(groups, u.ID)
			require.NotNil(t, ug)
			require.Equal(t, ThreadCode, ug.kind)
			require.NotSame(t, hg, ug, "edges to a hub are ignored, so a user does not join the hub's thread")
			require.Len(t, ug.items, 1, "users are not linked to each other, so each stays its own thread")
		}
	})

	t.Run("the same hub with a changed signature is not split out", func(t *testing.T) {
		h := mkItem("h", RoleLogic, "p", "p/h.go", 1, ChangeModified)
		h.SignatureChanged = true
		var users []*Item
		for i := 0; i < HubMinUsers; i++ {
			u := mkItem(fmt.Sprintf("u%d", i), RoleLogic, "p", fmt.Sprintf("p/u%d.go", i), 1, ChangeModified)
			addUse(u, h, dodge.RefCall, "")
			users = append(users, u)
		}

		all := append([]*Item{h}, users...)
		groups := buildThreads(byID(all...))

		for _, g := range groups {
			require.NotEqual(t, ThreadShared, g.kind, "a signature change disqualifies the hub rule")
		}
		hg := groupContaining(groups, h.ID)
		require.NotNil(t, hg)
		require.Equal(t, ThreadCode, hg.kind)
		require.Len(t, hg.items, 1+HubMinUsers, "h and every user are now connected into one component")
	})

	t.Run("a test joins the thread it references more", func(t *testing.T) {
		p1 := mkItem("p1", RoleContract, "svc", "svc/f.go", 1, ChangeModified)
		p2 := mkItem("p2", RoleLogic, "svc", "svc/f.go", 10, ChangeModified)
		addUse(p2, p1, dodge.RefUse, "")

		q1 := mkItem("q1", RoleContract, "store", "store/f.go", 1, ChangeModified)
		q2 := mkItem("q2", RoleLogic, "store", "store/f.go", 10, ChangeModified)
		addUse(q2, q1, dodge.RefUse, "")

		test := mkItem("t", RoleTest, "svc", "svc/f_test.go", 1, ChangeAdded)
		test.Symbol.IsTest = true
		addUse(test, p1, dodge.RefCall, "")
		addUse(test, p2, dodge.RefCall, "")
		addUse(test, q1, dodge.RefCall, "")

		groups := buildThreads(byID(p1, p2, q1, q2, test))

		tg := groupContaining(groups, test.ID)
		pg := groupContaining(groups, p1.ID)
		qg := groupContaining(groups, q1.ID)
		require.Same(t, pg, tg, "the test references p1 and p2 (2) more than q1 (1)")
		require.NotSame(t, qg, tg)
	})

	t.Run("an orphan deleted symbol attaches to the thread of its file", func(t *testing.T) {
		x := mkItem("x", RoleLogic, "p", "p/f.go", 1, ChangeModified)
		y := mkItem("y", RoleLogic, "p", "p/f.go", 20, ChangeModified)
		addUse(x, y, dodge.RefCall, "")

		orphan := mkItem("orphan", RoleLogic, "p", "p/f.go", 5, ChangeDeleted)
		// orphan has no Uses and no UsedBy: it is alone in its component
		// and has no links at all.

		groups := buildThreads(byID(x, y, orphan))

		xg := groupContaining(groups, x.ID)
		og := groupContaining(groups, orphan.ID)
		require.Same(t, xg, og, "the orphan attaches to the code thread of its file")
		require.Len(t, xg.items, 3)
	})
}
