package plan

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sakshamsaxena/blaze/internal/dodge"
)

// HubMinUsers is the number of distinct non-test code items a modified,
// signature-stable function or method must be used by to be split out into
// its own shared thread (step 4a).
const HubMinUsers = 5

// group is one thread's items, as steps 3 and 4 settle them. Step 7 (order)
// turns it into a Thread with the items in review order.
type group struct {
	id       string
	kind     ThreadKind
	title    string
	subtitle string
	items    []*Item
}

// isNonTestCode reports whether it is a symbol item whose role makes it a
// node candidate for linking and threading: contract, logic or wiring.
func isNonTestCode(it *Item) bool {
	return it.Kind == ItemSymbol && (it.Role == RoleContract || it.Role == RoleLogic || it.Role == RoleWiring)
}

// isCodeItem reports whether it is a "code item" per step 3: a symbol item
// whose role is contract, logic, wiring or test.
func isCodeItem(it *Item) bool {
	return isNonTestCode(it) || (it.Kind == ItemSymbol && it.Role == RoleTest)
}

// roleRank orders contract before logic before wiring, used for the anchor
// tie-break and the item sort key inside a thread.
func roleRank(r Role) int {
	switch r {
	case RoleContract:
		return 0
	case RoleLogic:
		return 1
	case RoleWiring:
		return 2
	default:
		return 3
	}
}

// link implements step 3: for every code item that refers to another code
// item in the head graph, it records the edge on both sides as a Link,
// deduplicated and sorted by (ItemID, Kind, Member).
func link(items map[string]*Item, g *dodge.Graph) {
	itemOf := map[dodge.SymbolID]*Item{}
	for _, it := range items {
		if isCodeItem(it) {
			itemOf[it.Symbol.ID] = it
		}
	}

	for _, a := range items {
		if !isCodeItem(a) {
			continue
		}
		for _, r := range g.Out[a.Symbol.ID] {
			b, ok := itemOf[r.To]
			if !ok || b == a {
				continue
			}
			a.Uses = append(a.Uses, Link{ItemID: b.ID, Kind: r.Kind, Member: r.Member})
			b.UsedBy = append(b.UsedBy, Link{ItemID: a.ID, Kind: r.Kind, Member: r.Member})
		}
	}

	for _, it := range items {
		it.Uses = dedupeSortLinks(it.Uses)
		it.UsedBy = dedupeSortLinks(it.UsedBy)
	}
}

// dedupeSortLinks deduplicates links on (ItemID, Kind, Member) and sorts
// the result by the same key.
func dedupeSortLinks(links []Link) []Link {
	seen := map[Link]bool{}
	out := make([]Link, 0, len(links))
	for _, l := range links {
		if seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ItemID != b.ItemID {
			return a.ItemID < b.ItemID
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Member < b.Member
	})
	return out
}

// unionFind is a disjoint-set structure over item IDs, used to compute the
// connected components of step 4b.
type unionFind struct {
	parent map[string]string
}

func newUnionFind(ids []string) *unionFind {
	p := make(map[string]string, len(ids))
	for _, id := range ids {
		p[id] = id
	}
	return &unionFind{parent: p}
}

func (u *unionFind) find(x string) string {
	for u.parent[x] != x {
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}

// buildThreads implements step 4: it groups items into threads (hubs,
// code components, tests, orphans and the fixed role-based threads) and
// fills in each group's anchor-derived ID, title and subtitle. The items
// inside each group are not yet in review order; step 7 (order) does that.
func buildThreads(items map[string]*Item) []*group {
	hubIDs := findHubs(items)
	nodeIDs, nodeSet := nonHubNodes(items, hubIDs)
	sort.Strings(nodeIDs)

	uf := newUnionFind(nodeIDs)
	for _, id := range nodeIDs {
		for _, l := range items[id].Uses {
			if nodeSet[l.ItemID] {
				uf.union(id, l.ItemID)
			}
		}
	}

	compOf := map[string][]*Item{}
	for _, id := range nodeIDs {
		root := uf.find(id)
		compOf[root] = append(compOf[root], items[id])
	}
	var roots []string
	for root := range compOf {
		roots = append(roots, root)
	}
	sort.Strings(roots)

	var groups []*group
	for _, hid := range hubIDs {
		groups = append(groups, &group{kind: ThreadShared, items: []*Item{items[hid]}})
	}
	for _, root := range roots {
		groups = append(groups, &group{kind: ThreadCode, items: compOf[root]})
	}

	groupIndexOf := map[string]int{}
	for gi, g := range groups {
		for _, it := range g.items {
			groupIndexOf[it.ID] = gi
		}
	}

	assignTests(items, groups, groupIndexOf)
	attachOrphans(groups)
	groups = compactGroups(groups)
	groups = append(groups, buildFixedThreads(items, groups)...)

	for _, g := range groups {
		if g.kind == ThreadCode || g.kind == ThreadShared {
			finishCodeGroup(g)
		}
	}

	return groups
}

// findHubs returns the IDs of the items that qualify as hubs (step 4a),
// sorted for deterministic group ordering.
func findHubs(items map[string]*Item) []string {
	var ids []string
	for _, it := range items {
		if isHub(items, it) {
			ids = append(ids, it.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// isHub reports whether h is a hub: a non-test, modified, signature-stable
// func or method used by at least HubMinUsers distinct non-test code items.
func isHub(items map[string]*Item, h *Item) bool {
	if !isNonTestCode(h) {
		return false
	}
	if h.Symbol.Kind != dodge.KindFunc && h.Symbol.Kind != dodge.KindMethod {
		return false
	}
	if h.Status != ChangeModified || h.SignatureChanged {
		return false
	}
	distinct := map[string]bool{}
	for _, l := range h.UsedBy {
		if u := items[l.ItemID]; u != nil && isNonTestCode(u) {
			distinct[l.ItemID] = true
		}
	}
	return len(distinct) >= HubMinUsers
}

// nonHubNodes returns the IDs (sorted by the caller) of the non-test,
// non-hub code items that are the nodes of step 4b, plus the same set as a
// membership map.
func nonHubNodes(items map[string]*Item, hubIDs []string) ([]string, map[string]bool) {
	hub := map[string]bool{}
	for _, id := range hubIDs {
		hub[id] = true
	}
	var ids []string
	set := map[string]bool{}
	for _, it := range items {
		if isNonTestCode(it) && !hub[it.ID] {
			ids = append(ids, it.ID)
			set[it.ID] = true
		}
	}
	return ids, set
}

// assignTests implements step 4c: each test code item joins the hub or
// code-component group its Uses links reference most, breaking ties by
// that group's current anchor key. A test with no such links is left
// unplaced, for the fixed tests thread (step 4e) to pick up.
func assignTests(items map[string]*Item, groups []*group, groupIndexOf map[string]int) {
	var testIDs []string
	for id, it := range items {
		if it.Kind == ItemSymbol && it.Role == RoleTest {
			testIDs = append(testIDs, id)
		}
	}
	sort.Strings(testIDs)

	for _, tid := range testIDs {
		t := items[tid]
		counts := map[int]int{}
		for _, l := range t.Uses {
			if gi, ok := groupIndexOf[l.ItemID]; ok {
				counts[gi]++
			}
		}
		if len(counts) == 0 {
			continue
		}

		best, bestCount, bestAnchorKey := -1, -1, ""
		for gi, c := range counts {
			ak := computeAnchor(groups[gi].items).Key
			if best == -1 || c > bestCount || (c == bestCount && ak < bestAnchorKey) {
				best, bestCount, bestAnchorKey = gi, c, ak
			}
		}
		groups[best].items = append(groups[best].items, t)
		groupIndexOf[tid] = best
	}
}

// attachOrphans implements step 4d: a code-component group left with a
// single item that has no links at all is attached to the code group
// holding another item at the same Path, nearest by Symbol.StartLine; with
// no such group, it stays as its own single-item thread.
func attachOrphans(groups []*group) {
	var orphanIdx []int
	for gi, g := range groups {
		if g.kind == ThreadCode && len(g.items) == 1 {
			it := g.items[0]
			if len(it.Uses) == 0 && len(it.UsedBy) == 0 {
				orphanIdx = append(orphanIdx, gi)
			}
		}
	}
	sort.Slice(orphanIdx, func(i, j int) bool {
		return groups[orphanIdx[i]].items[0].Key < groups[orphanIdx[j]].items[0].Key
	})

	for _, gi := range orphanIdx {
		o := groups[gi].items[0]
		targetIdx, target := findOrphanTarget(groups, gi, o)
		if target == nil {
			continue
		}
		groups[targetIdx].items = append(groups[targetIdx].items, o)
		groups[gi].items = nil
	}
}

// findOrphanTarget searches every ThreadCode group other than selfIdx for
// the item nearest to o by Symbol.StartLine among those at the same Path,
// returning its group index and the item, or (-1, nil) when none matches.
func findOrphanTarget(groups []*group, selfIdx int, o *Item) (int, *Item) {
	best, bestItem, bestDist := -1, (*Item)(nil), -1
	for idx, g := range groups {
		if idx == selfIdx || g.kind != ThreadCode {
			continue
		}
		for _, it := range g.items {
			if it.Path != o.Path {
				continue
			}
			d := abs(it.Symbol.StartLine - o.Symbol.StartLine)
			if bestItem == nil || d < bestDist || (d == bestDist && it.Key < bestItem.Key) {
				best, bestItem, bestDist = idx, it, d
			}
		}
	}
	return best, bestItem
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// compactGroups drops groups left empty by attachOrphans.
func compactGroups(groups []*group) []*group {
	out := make([]*group, 0, len(groups))
	for _, g := range groups {
		if len(g.items) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// fixedThreadMeta describes the fixed thread kind and title for each role
// of step 4e.
var fixedThreadMeta = map[Role]struct {
	kind  ThreadKind
	title string
}{
	RoleContract:   {ThreadContracts, "Schemas and API definitions"},
	RoleTest:       {ThreadTests, "Tests and test data"},
	RoleConfig:     {ThreadConfig, "Configuration"},
	RoleDocs:       {ThreadDocs, "Documentation"},
	RoleOther:      {ThreadOther, "Other files"},
	RoleMechanical: {ThreadMechanical, "Mechanical changes"},
	RoleGenerated:  {ThreadGenerated, "Generated files"},
}

// fixedRoleOrder lists the roles buildFixedThreads considers, in a fixed
// order so the resulting groups are built deterministically.
var fixedRoleOrder = []Role{RoleContract, RoleTest, RoleConfig, RoleDocs, RoleOther, RoleMechanical, RoleGenerated}

// buildFixedThreads implements step 4e: every item not already placed in
// placedGroups is grouped by role into the matching fixed thread, which
// exists only when it has at least one item.
func buildFixedThreads(items map[string]*Item, placedGroups []*group) []*group {
	placed := map[string]bool{}
	for _, g := range placedGroups {
		for _, it := range g.items {
			placed[it.ID] = true
		}
	}

	buckets := map[Role][]*Item{}
	for _, it := range items {
		if placed[it.ID] {
			continue
		}
		if _, ok := fixedThreadMeta[it.Role]; ok {
			buckets[it.Role] = append(buckets[it.Role], it)
		}
	}

	var out []*group
	for _, r := range fixedRoleOrder {
		bucket := buckets[r]
		if len(bucket) == 0 {
			continue
		}
		meta := fixedThreadMeta[r]
		out = append(out, &group{
			id:       hashID("t", "thread:"+string(meta.kind)),
			kind:     meta.kind,
			title:    meta.title,
			subtitle: fixedSubtitle(bucket),
			items:    bucket,
		})
	}
	return out
}

// fixedSubtitle is "N files" when every item in items is a file item, else
// "N items", pluralized.
func fixedSubtitle(items []*Item) string {
	allFiles := true
	for _, it := range items {
		if it.Kind != ItemFile {
			allFiles = false
			break
		}
	}
	noun := "item"
	if allFiles {
		noun = "file"
	}
	if len(items) == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", len(items), noun)
}

// finishCodeGroup fills in a code or shared group's ID, title and subtitle
// from its final set of items, per step 4f.
func finishCodeGroup(g *group) {
	anchor := computeAnchor(g.items)
	g.id = hashID("t", anchor.Key)
	if len(g.items) == 1 {
		g.title = anchor.Title
	} else {
		g.title = fmt.Sprintf("%s + %d more", anchor.Title, len(g.items)-1)
	}
	g.subtitle = pkgSubtitle(g.items)
}

// computeAnchor returns the non-test item of groupItems with the most
// distinct items in UsedBy that also belong to groupItems, breaking ties
// by role order (contract < logic < wiring) then by Key.
func computeAnchor(groupItems []*Item) *Item {
	member := map[string]bool{}
	for _, it := range groupItems {
		member[it.ID] = true
	}

	var best *Item
	bestCount := -1
	for _, it := range groupItems {
		if it.Role == RoleTest {
			continue
		}
		distinct := map[string]bool{}
		for _, l := range it.UsedBy {
			if member[l.ItemID] {
				distinct[l.ItemID] = true
			}
		}
		c := len(distinct)
		if best == nil || c > bestCount || (c == bestCount && lessAnchor(it, best)) {
			best, bestCount = it, c
		}
	}
	return best
}

// lessAnchor orders anchor candidates by role rank then by Key.
func lessAnchor(a, b *Item) bool {
	ra, rb := roleRank(a.Role), roleRank(b.Role)
	if ra != rb {
		return ra < rb
	}
	return a.Key < b.Key
}

// pkgSubtitle is the distinct Symbol.PkgName values of items, sorted and
// joined with ", ", capped at 4 with a trailing "…".
func pkgSubtitle(items []*Item) string {
	seen := map[string]bool{}
	var pkgs []string
	for _, it := range items {
		if it.Symbol == nil {
			continue
		}
		if !seen[it.Symbol.PkgName] {
			seen[it.Symbol.PkgName] = true
			pkgs = append(pkgs, it.Symbol.PkgName)
		}
	}
	sort.Strings(pkgs)
	if len(pkgs) > 4 {
		return strings.Join(pkgs[:4], ", ") + ", …"
	}
	return strings.Join(pkgs, ", ")
}
