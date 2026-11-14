package dodge

import "sort"

// newGraph returns an empty Graph for modulePath.
func newGraph(modulePath string) *Graph {
	return &Graph{
		ModulePath: modulePath,
		Symbols:    map[SymbolID]*Symbol{},
		Out:        map[SymbolID][]Ref{},
		In:         map[SymbolID][]Ref{},
		Files:      map[string][]*Symbol{},
	}
}

// addSymbol adds s to g, indexed by ID and by file. It returns false
// without changing g when the ID already exists.
func (g *Graph) addSymbol(s *Symbol) bool {
	if _, exists := g.Symbols[s.ID]; exists {
		return false
	}
	g.Symbols[s.ID] = s
	g.Files[s.File] = append(g.Files[s.File], s)
	return true
}

// addRef appends r to Out[r.From] and In[r.To], unless an identical ref
// (same From, To, Kind, Member, File and Line) is already present.
func (g *Graph) addRef(r Ref) {
	for _, existing := range g.Out[r.From] {
		if existing == r {
			return
		}
	}
	g.Out[r.From] = append(g.Out[r.From], r)
	g.In[r.To] = append(g.In[r.To], r)
}

// finish sorts Files by StartLine and Out/In by (File, Line, To, From), so
// the graph's output is deterministic regardless of map iteration order.
func (g *Graph) finish() {
	for file, syms := range g.Files {
		sort.SliceStable(syms, func(i, j int) bool { return syms[i].StartLine < syms[j].StartLine })
		g.Files[file] = syms
	}

	sortRefs := func(refs []Ref) {
		sort.SliceStable(refs, func(i, j int) bool {
			a, b := refs[i], refs[j]
			if a.File != b.File {
				return a.File < b.File
			}
			if a.Line != b.Line {
				return a.Line < b.Line
			}
			if a.To != b.To {
				return a.To < b.To
			}
			return a.From < b.From
		})
	}
	for k, refs := range g.Out {
		sortRefs(refs)
		g.Out[k] = refs
	}
	for k, refs := range g.In {
		sortRefs(refs)
		g.In[k] = refs
	}
}

// SymbolsInFile returns the symbols declared in path, sorted by StartLine.
func (g *Graph) SymbolsInFile(path string) []*Symbol {
	return g.Files[path]
}

// SymbolAt returns the symbol in path whose span contains line
// (StartLine <= line <= EndLine), or nil if none does.
func (g *Graph) SymbolAt(path string, line int) *Symbol {
	for _, s := range g.Files[path] {
		if s.StartLine <= line && line <= s.EndLine {
			return s
		}
	}
	return nil
}
