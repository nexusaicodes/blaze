package plan

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sakshamsaxena/blaze/internal/dodge"
)

// ids returns the Key of every item in order, for compact assertions.
func ids(items []*Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Key
	}
	return out
}

func TestOrder(t *testing.T) {
	t.Run("dependency order: definitions before uses", func(t *testing.T) {
		a := mkItem("a", RoleLogic, "p", "p/f.go", 30, ChangeModified)
		b := mkItem("b", RoleLogic, "p", "p/f.go", 20, ChangeModified)
		c := mkItem("c", RoleLogic, "p", "p/f.go", 10, ChangeModified)
		addUse(a, b, dodge.RefCall, "")
		addUse(b, c, dodge.RefCall, "")

		got := orderCodeGroup([]*Item{a, b, c})
		require.Equal(t, []string{"c", "b", "a"}, ids(got))
	})

	t.Run("a cycle of two tolerates the back edge", func(t *testing.T) {
		a := mkItem("a", RoleLogic, "p", "p/f.go", 1, ChangeModified)
		b := mkItem("b", RoleLogic, "p", "p/f.go", 2, ChangeModified)
		addUse(a, b, dodge.RefCall, "")
		addUse(b, a, dodge.RefCall, "")

		got := orderCodeGroup([]*Item{a, b})
		require.Equal(t, []string{"b", "a"}, ids(got), "visiting a first, then its dependency b; the back edge b->a is skipped")
	})

	t.Run("the interface bridge orders an implementation before its caller", func(t *testing.T) {
		caller := mkItem("caller", RoleLogic, "svc", "svc/f.go", 1, ChangeModified)
		iface := mkItem("iface", RoleContract, "store", "store/f.go", 1, ChangeModified)
		impl := mkItem("impl", RoleLogic, "store", "store/f.go", 10, ChangeModified)

		addUse(caller, iface, dodge.RefCall, "Save")
		addUse(impl, iface, dodge.RefImplements, "Save")

		got := orderCodeGroup([]*Item{caller, impl})
		require.Equal(t, []string{"impl", "caller"}, ids(got), "impl implements the interface member caller uses, so it is ordered first")
	})

	t.Run("tests are ordered last", func(t *testing.T) {
		a := mkItem("a", RoleLogic, "p", "p/f.go", 1, ChangeModified)
		test := mkItem("test", RoleTest, "p", "p/f_test.go", 1, ChangeAdded)
		test.Symbol.IsTest = true
		addUse(test, a, dodge.RefCall, "")

		got := orderCodeGroup([]*Item{a, test})
		require.Equal(t, []string{"a", "test"}, ids(got))
	})
}
