package plan

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sakshamsaxena/blaze/internal/dodge"
)

// riskItem builds a symbol item eligible for scoring (role logic, status
// modified, Blast set) with the given Blast; cases override other fields
// directly on the result.
func riskItem(blast *Blast) *Item {
	return &Item{
		Kind:   ItemSymbol,
		Role:   RoleLogic,
		Status: ChangeModified,
		Symbol: &dodge.Symbol{Kind: dodge.KindFunc},
		Blast:  blast,
	}
}

func TestRisk(t *testing.T) {
	t.Run("ineligible items get Risk none", func(t *testing.T) {
		file := &Item{Kind: ItemFile, Role: RoleOther, Blast: &Blast{}}
		require.Equal(t, Risk{Level: "none", Reasons: []string{}}, risk(file))

		docsFile := &Item{Kind: ItemFile, Role: RoleContract, Blast: &Blast{}}
		require.Equal(t, Risk{Level: "none", Reasons: []string{}}, risk(docsFile), "only ItemSymbol items are eligible, regardless of role")

		test := &Item{Kind: ItemSymbol, Role: RoleTest, Symbol: &dodge.Symbol{Kind: dodge.KindFunc}, Blast: &Blast{OutsideCount: 50}}
		require.Equal(t, Risk{Level: "none", Reasons: []string{}}, risk(test), "role test is not contract, logic or wiring")

		noBlast := riskItem(nil)
		require.Equal(t, Risk{Level: "none", Reasons: []string{}}, risk(noBlast), "Blast nil means not scored")
	})

	t.Run("A outside callers, only when status is modified", func(t *testing.T) {
		cases := []struct {
			n      int
			points int
		}{{1, 10}, {2, 10}, {3, 20}, {9, 20}, {10, 30}, {50, 30}}
		for _, c := range cases {
			it := riskItem(&Blast{OutsideCount: c.n, TestCount: 1, ChangedTests: 1})
			r := risk(it)
			require.Equal(t, c.points, r.Score, "n=%d", c.n)
			require.Equal(t, []string{fmt.Sprintf("%d callers outside this PR", c.n)}, r.Reasons)
		}

		added := riskItem(&Blast{OutsideCount: 50, TestCount: 1, ChangedTests: 1})
		added.Status = ChangeAdded
		require.Equal(t, 0, risk(added).Score, "factor A does not apply when status is not modified")
	})

	t.Run("B other packages, only when status is modified", func(t *testing.T) {
		cases := []struct {
			p      int
			points int
		}{{1, 10}, {3, 10}, {4, 20}, {10, 20}}
		for _, c := range cases {
			it := riskItem(&Blast{OtherPackages: c.p, TestCount: 1, ChangedTests: 1})
			r := risk(it)
			require.Equal(t, c.points, r.Score, "p=%d", c.p)
			require.Equal(t, []string{fmt.Sprintf("used from %d other packages", c.p)}, r.Reasons)
		}

		added := riskItem(&Blast{OtherPackages: 10, TestCount: 1, ChangedTests: 1})
		added.Status = ChangeAdded
		require.Equal(t, 0, risk(added).Score, "factor B does not apply when status is not modified")
	})

	t.Run("C exported, only when status is modified", func(t *testing.T) {
		it := riskItem(&Blast{TestCount: 1, ChangedTests: 1})
		it.Symbol.Exported = true
		r := risk(it)
		require.Equal(t, 10, r.Score)
		require.Equal(t, []string{"exported"}, r.Reasons)

		unexported := riskItem(&Blast{TestCount: 1, ChangedTests: 1})
		require.Equal(t, 0, risk(unexported).Score)

		added := riskItem(&Blast{TestCount: 1, ChangedTests: 1})
		added.Status = ChangeAdded
		added.Symbol.Exported = true
		require.Equal(t, 0, risk(added).Score, "factor C does not apply when status is not modified")
	})

	t.Run("D signature or declaration changed", func(t *testing.T) {
		fn := riskItem(&Blast{TestCount: 1, ChangedTests: 1})
		fn.SignatureChanged = true
		r := risk(fn)
		require.Equal(t, 10, r.Score)
		require.Equal(t, []string{"signature changed"}, r.Reasons)

		typ := riskItem(&Blast{})
		typ.Role = RoleContract
		typ.Symbol.Kind = dodge.KindType
		typ.SignatureChanged = true
		r = risk(typ)
		require.Equal(t, 10, r.Score)
		require.Equal(t, []string{"declaration changed"}, r.Reasons)
	})

	t.Run("E tests", func(t *testing.T) {
		noTests := riskItem(&Blast{TestCount: 0})
		r := risk(noTests)
		require.Equal(t, 15, r.Score)
		require.Equal(t, []string{"no test references this"}, r.Reasons)

		unchanged := riskItem(&Blast{TestCount: 2, ChangedTests: 0})
		r = risk(unchanged)
		require.Equal(t, 5, r.Score)
		require.Equal(t, []string{"tests exist but none changed in this PR"}, r.Reasons)

		changed := riskItem(&Blast{TestCount: 2, ChangedTests: 1})
		require.Equal(t, 0, risk(changed).Score, "a changed test exists, so factor E contributes nothing")

		typ := riskItem(&Blast{TestCount: 0})
		typ.Role = RoleContract
		typ.Symbol.Kind = dodge.KindType
		require.Equal(t, 0, risk(typ).Score, "factor E only applies to kind func or method")
	})

	t.Run("F size of the change", func(t *testing.T) {
		cases := []struct {
			lines  int
			points int
		}{{10, 0}, {49, 0}, {50, 10}, {149, 10}, {150, 15}, {500, 15}}
		for _, c := range cases {
			it := riskItem(&Blast{TestCount: 1, ChangedTests: 1})
			it.Added = c.lines
			r := risk(it)
			require.Equal(t, c.points, r.Score, "lines=%d", c.lines)
			if c.points > 0 {
				require.Equal(t, []string{fmt.Sprintf("large change (%d lines)", c.lines)}, r.Reasons)
			} else {
				require.Empty(t, r.Reasons)
			}
		}

		split := riskItem(&Blast{TestCount: 1, ChangedTests: 1})
		split.Added, split.Deleted = 30, 25
		require.Equal(t, 10, risk(split).Score, "Added and Deleted both count toward the line total")
	})

	t.Run("score is capped at 100", func(t *testing.T) {
		it := riskItem(&Blast{OutsideCount: 50, OtherPackages: 10, TestCount: 0})
		it.Symbol.Exported = true
		it.SignatureChanged = true
		it.Added = 500

		r := risk(it)

		require.Equal(t, 100, r.Score, "A(30)+B(20)+C(10)+D(10)+E(15)+F(15) sums to exactly 100")
		require.Equal(t, "high", r.Level)
		require.Len(t, r.Reasons, 6)
	})

	t.Run("Level thresholds", func(t *testing.T) {
		require.Equal(t, "low", riskLevel(0))
		require.Equal(t, "low", riskLevel(24))
		require.Equal(t, "medium", riskLevel(25))
		require.Equal(t, "medium", riskLevel(49))
		require.Equal(t, "high", riskLevel(50))
		require.Equal(t, "high", riskLevel(100))
	})

	t.Run("threadRisk takes the highest-scoring item, prefixed by its title", func(t *testing.T) {
		low := &Item{ID: "low", Title: "pkg.Low", Risk: Risk{Score: 10, Level: "low", Reasons: []string{"no test references this"}}}
		high := &Item{ID: "high", Title: "pkg.High", Risk: Risk{Score: 50, Level: "high", Reasons: []string{"exported", "signature changed"}}}

		got := threadRisk([]*Item{low, high})

		require.Equal(t, Risk{
			Score:   50,
			Level:   "high",
			Reasons: []string{"pkg.High: exported", "pkg.High: signature changed"},
		}, got)
	})

	t.Run("threadRisk ties go to the item first in review order", func(t *testing.T) {
		first := &Item{ID: "first", Title: "pkg.First", Risk: Risk{Score: 50, Level: "high", Reasons: []string{"exported"}}}
		second := &Item{ID: "second", Title: "pkg.Second", Risk: Risk{Score: 50, Level: "high", Reasons: []string{"exported"}}}

		got := threadRisk([]*Item{first, second})

		require.Equal(t, []string{"pkg.First: exported"}, got.Reasons)
	})

	t.Run("threadRisk is none when no item is scored", func(t *testing.T) {
		a := &Item{ID: "a", Title: "pkg.A", Risk: Risk{Level: "none", Reasons: []string{}}}
		b := &Item{ID: "b", Title: "pkg.B", Risk: Risk{Level: "none", Reasons: []string{}}}

		require.Equal(t, Risk{Level: "none", Reasons: []string{}}, threadRisk([]*Item{a, b}))
	})
}
