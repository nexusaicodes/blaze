package plan

import (
	"sort"

	"github.com/sakshamsaxena/blaze/internal/dodge"
)

// fixedRank orders the fixed thread kinds that are not contracts, after
// the code and shared threads.
var fixedRank = map[ThreadKind]int{
	ThreadTests:      0,
	ThreadConfig:     1,
	ThreadDocs:       2,
	ThreadOther:      3,
	ThreadMechanical: 4,
	ThreadGenerated:  5,
}

// orderedGroup carries a group's final Thread alongside the Path and
// Symbol.StartLine of its first item, which the code/shared tie-break of
// step 7 needs.
type orderedGroup struct {
	thread     Thread
	firstPath  string
	firstStart int
}

// order implements step 7: it orders the items inside each group and then
// orders the groups themselves, returning the final Thread slice in
// review order.
func order(groups []*group) []Thread {
	ordered := make([]orderedGroup, len(groups))
	for i, g := range groups {
		var items []*Item
		if g.kind == ThreadCode || g.kind == ThreadShared {
			items = orderCodeGroup(g.items)
		} else {
			items = orderFixedGroup(g.items)
		}

		ids := make([]string, len(items))
		for j, it := range items {
			ids[j] = it.ID
		}

		og := orderedGroup{thread: Thread{
			ID:       g.id,
			Kind:     g.kind,
			Title:    g.title,
			Subtitle: g.subtitle,
			Risk:     threadRisk(items),
			ItemIDs:  nonNil(ids),
		}}
		if len(items) > 0 {
			og.firstPath = items[0].Path
			if items[0].Symbol != nil {
				og.firstStart = items[0].Symbol.StartLine
			}
		}
		ordered[i] = og
	}

	sort.SliceStable(ordered, func(i, j int) bool { return threadLess(ordered[i], ordered[j]) })

	threads := make([]Thread, len(ordered))
	for i, og := range ordered {
		threads[i] = og.thread
	}
	return threads
}

// threadClass buckets a thread kind into the three groups the final
// ordering sorts by in turn: 0 contracts, 1 code/shared, 2 every other
// fixed kind.
func threadClass(k ThreadKind) int {
	switch k {
	case ThreadContracts:
		return 0
	case ThreadCode, ThreadShared:
		return 1
	default:
		return 2
	}
}

// threadLess orders a's thread before b's per step 7's "Threads" rules:
// the contracts thread first, then code and shared threads by Risk.Score
// descending (ties by the smallest (Path, StartLine) of the first item),
// then the remaining fixed threads in a fixed order.
func threadLess(a, b orderedGroup) bool {
	ca, cb := threadClass(a.thread.Kind), threadClass(b.thread.Kind)
	if ca != cb {
		return ca < cb
	}
	switch ca {
	case 1:
		if a.thread.Risk.Score != b.thread.Risk.Score {
			return a.thread.Risk.Score > b.thread.Risk.Score
		}
		if a.firstPath != b.firstPath {
			return a.firstPath < b.firstPath
		}
		return a.firstStart < b.firstStart
	case 2:
		return fixedRank[a.thread.Kind] < fixedRank[b.thread.Kind]
	default: // contracts: at most one such thread
		return false
	}
}

// orderCodeGroup implements the "Items inside a code or shared thread"
// rules of step 7: a dependency post-order over the thread's non-test
// items (definitions before uses), with its test items appended last.
func orderCodeGroup(items []*Item) []*Item {
	var nodes, tests []*Item
	for _, it := range items {
		if it.Role == RoleTest {
			tests = append(tests, it)
		} else {
			nodes = append(nodes, it)
		}
	}

	nodeSet := map[string]bool{}
	byID := map[string]*Item{}
	for _, n := range nodes {
		nodeSet[n.ID] = true
		byID[n.ID] = n
	}

	deps := map[string][]string{}
	for _, a := range nodes {
		for _, l := range a.Uses {
			if nodeSet[l.ItemID] {
				deps[a.ID] = append(deps[a.ID], l.ItemID)
			}
		}
	}
	// Interface bridge: a using interface item I on member m, and node c
	// implementing I on the same member m, puts an edge a -> c.
	for _, a := range nodes {
		for _, l := range a.Uses {
			if l.Member == "" || (l.Kind != dodge.RefCall && l.Kind != dodge.RefUse) {
				continue
			}
			for _, c := range nodes {
				if c.ID == a.ID {
					continue
				}
				for _, cl := range c.Uses {
					if cl.ItemID == l.ItemID && cl.Kind == dodge.RefImplements && cl.Member == l.Member {
						deps[a.ID] = append(deps[a.ID], c.ID)
					}
				}
			}
		}
	}

	sortedNodes := append([]*Item{}, nodes...)
	sort.Slice(sortedNodes, func(i, j int) bool { return nodeSortLess(sortedNodes[i], sortedNodes[j]) })

	state := map[string]int{} // 0 absent, 1 visiting, 2 done
	var result []*Item
	var visit func(id string)
	visit = func(id string) {
		if state[id] != 0 {
			return
		}
		state[id] = 1
		ds := append([]string{}, deps[id]...)
		sort.Slice(ds, func(i, j int) bool { return nodeSortLess(byID[ds[i]], byID[ds[j]]) })
		for _, d := range ds {
			visit(d)
		}
		state[id] = 2
		result = append(result, byID[id])
	}
	for _, n := range sortedNodes {
		visit(n.ID)
	}

	sort.Slice(tests, func(i, j int) bool { return testSortLess(tests[i], tests[j]) })
	return append(result, tests...)
}

// nodeSortLess is the node sort key of step 7: (roleRank, Path,
// Symbol.StartLine, Key).
func nodeSortLess(a, b *Item) bool {
	if ra, rb := roleRank(a.Role), roleRank(b.Role); ra != rb {
		return ra < rb
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	if a.Symbol.StartLine != b.Symbol.StartLine {
		return a.Symbol.StartLine < b.Symbol.StartLine
	}
	return a.Key < b.Key
}

// testSortLess sorts a thread's test items by (Path, Symbol.StartLine,
// Key).
func testSortLess(a, b *Item) bool {
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	al, bl := startLine(a), startLine(b)
	if al != bl {
		return al < bl
	}
	return a.Key < b.Key
}

func startLine(it *Item) int {
	if it.Symbol == nil {
		return 0
	}
	return it.Symbol.StartLine
}

// orderFixedGroup implements the "Items inside a fixed thread" rule of
// step 7: sort by (Path, first slice NewStart or OldStart, Key).
func orderFixedGroup(items []*Item) []*Item {
	sorted := append([]*Item{}, items...)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		as, bs := firstSliceLine(a), firstSliceLine(b)
		if as != bs {
			return as < bs
		}
		return a.Key < b.Key
	})
	return sorted
}

// firstSliceLine is the first slice's NewStart, or its OldStart when
// NewStart is 0, or 0 when the item has no slices.
func firstSliceLine(it *Item) int {
	if len(it.Slices) == 0 {
		return 0
	}
	s := it.Slices[0]
	if s.NewStart != 0 {
		return s.NewStart
	}
	return s.OldStart
}
