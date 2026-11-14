package plan

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sakshamsaxena/blaze/internal/dodge"
)

// TransitiveDepth is the hop limit of transitiveCount's breadth-first
// search.
const TransitiveDepth = 3

// TransitiveCap is the size the visited set of transitiveCount may reach
// before the search stops early.
const TransitiveCap = 2000

// MaxCallers is the number of Callers kept in a Blast after sorting; the
// rest are dropped and Blast.Truncated is set.
const MaxCallers = 200

// SnippetMaxRunes is the length a Caller.Snippet is cut to.
const SnippetMaxRunes = 160

// viaRef pairs one incoming Ref with the name of the interface it was
// reached through, or "" for a direct reference.
type viaRef struct {
	Ref dodge.Ref
	Via string
}

// incoming returns the refs that depend on s, including calls made through
// interfaces that s implements.
func incoming(g *dodge.Graph, s *dodge.Symbol) []viaRef {
	var out []viaRef
	for _, r := range g.In[s.ID] {
		if r.Kind != dodge.RefImplements {
			out = append(out, viaRef{Ref: r})
		}
	}
	if s.Kind != dodge.KindMethod {
		return out
	}

	for _, impl := range g.Out[s.ID] {
		if impl.Kind != dodge.RefImplements || impl.Member == "" {
			continue
		}
		iface := g.Symbols[impl.To]
		if iface == nil {
			continue
		}
		for _, r := range g.In[impl.To] {
			if r.Kind == dodge.RefImplements || r.Member != impl.Member {
				continue
			}
			out = append(out, viaRef{Ref: r, Via: iface.Name})
		}
	}
	return out
}

// blastCache caches a worktree file's lines, keyed by repo-relative path,
// so every Caller.Snippet reads a file at most once.
type blastCache struct {
	worktree string
	lines    map[string][]string
}

// newBlastCache returns a blastCache that reads files under worktree.
func newBlastCache(worktree string) *blastCache {
	return &blastCache{worktree: worktree, lines: map[string][]string{}}
}

// snippet returns line number line of file, trimmed and cut to
// SnippetMaxRunes runes, or "" if the file or the line cannot be read.
func (c *blastCache) snippet(file string, line int) string {
	lines, ok := c.lines[file]
	if !ok {
		data, err := os.ReadFile(filepath.Join(c.worktree, file))
		if err == nil {
			lines = strings.Split(string(data), "\n")
		}
		c.lines[file] = lines
	}
	if line < 1 || line > len(lines) {
		return ""
	}
	return truncateRunes(strings.TrimSpace(lines[line-1]), SnippetMaxRunes)
}

// truncateRunes cuts s to at most n runes.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// buildItemOf returns the map from a changed symbol's ID to its Item,
// covering every symbol item regardless of role or kind: blast marks a
// caller as in-PR through this map, not through step 3's code-item-only
// itemOf.
func buildItemOf(items map[string]*Item) map[dodge.SymbolID]*Item {
	m := map[dodge.SymbolID]*Item{}
	for _, it := range items {
		if it.Kind == ItemSymbol && it.Symbol != nil {
			m[it.Symbol.ID] = it
		}
	}
	return m
}

// computeBlast implements step 5: it fills in Blast for every symbol item
// with Symbol.Resolved == true and Status != ChangeDeleted, leaving it nil
// for every other item.
func computeBlast(items map[string]*Item, g *dodge.Graph, itemOf map[dodge.SymbolID]*Item, cache *blastCache) {
	for _, it := range items {
		if it.Kind != ItemSymbol || !it.Symbol.Resolved || it.Status == ChangeDeleted {
			continue
		}
		it.Blast = blastFor(g, it.Symbol, itemOf, cache)
	}
}

// fromGroup collects every viaRef for one symbol's incoming refs that share
// the same Ref.From, in the order incoming returned them.
type fromGroup struct {
	from dodge.SymbolID
	refs []viaRef
}

// groupByFrom groups refs by Ref.From, preserving the order each From was
// first seen in.
func groupByFrom(refs []viaRef) []fromGroup {
	idx := map[dodge.SymbolID]int{}
	var groups []fromGroup
	for _, vr := range refs {
		from := vr.Ref.From
		if i, ok := idx[from]; ok {
			groups[i].refs = append(groups[i].refs, vr)
			continue
		}
		idx[from] = len(groups)
		groups = append(groups, fromGroup{from: from, refs: []viaRef{vr}})
	}
	return groups
}

// blastFor computes the Blast of one symbol s.
func blastFor(g *dodge.Graph, s *dodge.Symbol, itemOf map[dodge.SymbolID]*Item, cache *blastCache) *Blast {
	groups := groupByFrom(incoming(g, s))

	callers := make([]Caller, 0, len(groups))
	otherPkgs := map[string]bool{}
	var outsideCount, inPRCount, testCount, changedTests int

	for _, grp := range groups {
		c := buildCaller(g, grp, itemOf, cache)
		callers = append(callers, c)

		switch {
		case c.IsTest:
			testCount++
			if c.InPR {
				changedTests++
			}
		case c.InPR:
			inPRCount++
		default:
			outsideCount++
			if c.Pkg != s.Pkg {
				otherPkgs[c.Pkg] = true
			}
		}
	}

	sortCallers(callers)
	truncated := false
	if len(callers) > MaxCallers {
		callers = callers[:MaxCallers]
		truncated = true
	}

	transitive, capped := transitiveCount(g, s)

	return &Blast{
		Callers:          nonNil(callers),
		Truncated:        truncated,
		OutsideCount:     outsideCount,
		InPRCount:        inPRCount,
		TestCount:        testCount,
		ChangedTests:     changedTests,
		OtherPackages:    len(otherPkgs),
		TransitiveCount:  transitive,
		TransitiveCapped: capped,
	}
}

// buildCaller builds the Caller for one from-group: the caller symbol is
// g.Symbols[grp.from], Line/File come from the smallest (File, Line) site
// in the group, Kind is "call" if any site is a call, and Via is the first
// non-empty Via in the group.
func buildCaller(g *dodge.Graph, grp fromGroup, itemOf map[dodge.SymbolID]*Item, cache *blastCache) Caller {
	sym := g.Symbols[grp.from]

	best := grp.refs[0].Ref
	kind := dodge.RefUse
	via := ""
	for _, vr := range grp.refs {
		r := vr.Ref
		if r.File < best.File || (r.File == best.File && r.Line < best.Line) {
			best = r
		}
		if r.Kind == dodge.RefCall {
			kind = dodge.RefCall
		}
		if via == "" && vr.Via != "" {
			via = vr.Via
		}
	}

	c := Caller{
		SymbolID: string(grp.from),
		Name:     sym.Name,
		Pkg:      sym.Pkg,
		File:     best.File,
		Line:     best.Line,
		Sites:    len(grp.refs),
		Kind:     kind,
		Via:      via,
		IsTest:   sym.IsTest,
		Snippet:  cache.snippet(best.File, best.Line),
	}
	if it, ok := itemOf[grp.from]; ok {
		c.InPR = true
		c.ItemID = it.ID
	}
	return c
}

// callerBucket orders a Caller into the three groups the sort below uses:
// 0 outside non-test, 1 in-PR non-test, 2 test.
func callerBucket(c Caller) int {
	switch {
	case c.IsTest:
		return 2
	case c.InPR:
		return 1
	default:
		return 0
	}
}

// sortCallers sorts callers outside non-test first, then in-PR non-test,
// then tests; inside each group by (Pkg, File, Line).
func sortCallers(callers []Caller) {
	sort.Slice(callers, func(i, j int) bool {
		a, b := callers[i], callers[j]
		if ba, bb := callerBucket(a), callerBucket(b); ba != bb {
			return ba < bb
		}
		if a.Pkg != b.Pkg {
			return a.Pkg < b.Pkg
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
}

// bfsNode is one entry of transitiveCount's queue.
type bfsNode struct {
	id    dodge.SymbolID
	depth int
}

// transitiveCount runs a breadth-first search over incoming starting at s,
// depth limit TransitiveDepth, and returns the number of distinct non-test
// symbols reached (excluding s), plus whether the visited set reached
// TransitiveCap before the search finished. The search does not expand
// through test symbols.
func transitiveCount(g *dodge.Graph, s *dodge.Symbol) (int, bool) {
	visited := map[dodge.SymbolID]bool{s.ID: true}
	queue := []bfsNode{{id: s.ID, depth: 0}}
	count := 0

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= TransitiveDepth {
			continue
		}
		sym := g.Symbols[cur.id]
		if sym == nil {
			continue
		}

		for _, vr := range incoming(g, sym) {
			from := vr.Ref.From
			if visited[from] {
				continue
			}
			visited[from] = true

			fromSym := g.Symbols[from]
			if fromSym != nil && !fromSym.IsTest {
				count++
				queue = append(queue, bfsNode{id: from, depth: cur.depth + 1})
			}

			if len(visited) >= TransitiveCap {
				return count, true
			}
		}
	}
	return count, false
}
