package dodge

import (
	"go/ast"
	"go/types"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/tools/go/packages"
)

// buildRefs extracts call and use refs from one file into g, resolving
// positions to symbols through ix.
func buildRefs(g *Graph, ix *posIndex, pk *packages.Package, f *ast.File, relPath string) {
	callIdents := map[*ast.Ident]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fun := ast.Unparen(call.Fun)
		switch x := fun.(type) {
		case *ast.IndexExpr:
			fun = x.X
		case *ast.IndexListExpr:
			fun = x.X
		}
		switch x := fun.(type) {
		case *ast.Ident:
			callIdents[x] = true
		case *ast.SelectorExpr:
			callIdents[x.Sel] = true
		}
		return true
	})

	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}

		obj := pk.TypesInfo.Uses[id]
		if obj == nil {
			return true
		}
		if _, ok := obj.(*types.PkgName); ok {
			return true
		}

		to := ix.resolve(pk.Fset, obj.Pos())
		if to == "" {
			return true
		}
		from := ix.resolve(pk.Fset, id.Pos())
		if from == "" {
			return true
		}
		if from == to {
			return true
		}

		member := ""
		switch o := obj.(type) {
		case *types.Func:
			if sig, ok := o.Type().(*types.Signature); ok && sig.Recv() != nil {
				member = o.Name()
			}
		case *types.Var:
			if o.IsField() {
				member = o.Name()
			}
		}

		kind := RefUse
		if callIdents[id] {
			switch obj.(type) {
			case *types.Func, *types.Var:
				kind = RefCall
			}
		}

		g.addRef(Ref{
			From:   from,
			To:     to,
			Kind:   kind,
			Member: member,
			File:   relPath,
			Line:   pk.Fset.Position(id.Pos()).Line,
		})
		return true
	})
}

// buildImplements adds "implements" refs, at the type level and the
// method level, between each concrete type and each interface it
// implements. objs maps a type symbol's ID to its *types.TypeName. Cost is
// concrete types × interfaces; the duration is logged at debug level.
func buildImplements(g *Graph, objs map[SymbolID]types.Object, log zerolog.Logger) {
	start := time.Now()

	var interfaces []*Symbol
	var concretes []*Symbol

	for id, sym := range g.Symbols {
		if sym.Kind != KindType {
			continue
		}
		obj, ok := objs[id]
		if !ok {
			continue
		}
		tn, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}
		if tn.IsAlias() {
			continue
		}
		if named, ok := tn.Type().(*types.Named); ok && named.TypeParams().Len() > 0 {
			continue
		}

		if types.IsInterface(tn.Type()) {
			if iface, ok := tn.Type().Underlying().(*types.Interface); ok &&
				iface.NumMethods() > 0 && iface.IsMethodSet() {
				interfaces = append(interfaces, sym)
			}
			continue
		}
		concretes = append(concretes, sym)
	}

	for _, t := range concretes {
		tn := objs[t.ID].(*types.TypeName)
		for _, ifaceSym := range interfaces {
			iface := objs[ifaceSym.ID].(*types.TypeName).Type().Underlying().(*types.Interface)

			if !types.Implements(tn.Type(), iface) && !types.Implements(types.NewPointer(tn.Type()), iface) {
				continue
			}

			g.addRef(Ref{From: t.ID, To: ifaceSym.ID, Kind: RefImplements, File: t.File, Line: t.DeclLine})

			for i := 0; i < iface.NumMethods(); i++ {
				m := iface.Method(i).Name()
				mid := SymbolID(t.Pkg + "." + t.Name + "." + m)
				if msym, ok := g.Symbols[mid]; ok {
					g.addRef(Ref{From: mid, To: ifaceSym.ID, Kind: RefImplements, Member: m, File: msym.File, Line: msym.DeclLine})
				}
			}
		}
	}

	log.Debug().
		Dur("duration", time.Since(start)).
		Int("concreteTypes", len(concretes)).
		Int("interfaces", len(interfaces)).
		Msg("implements pass")
}
