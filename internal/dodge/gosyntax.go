package dodge

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strconv"
	"strings"
)

// ParseFile parses in.Src with syntax only and extracts its symbols. The
// symbols have Resolved == false and no refs.
func (p *goProvider) ParseFile(in ParseInput) ([]*Symbol, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, in.Path, in.Src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", in.Path, err)
	}
	pkgPath := packagePath(in.Path, in.ModulePath, in.ModuleDir, f.Name.Name)
	return extractSymbols(fset, f, in.Src, in.Path, pkgPath), nil
}

// packagePath computes the import path of the file at relPath, the way
// packages.Package.PkgPath would, without loading the package.
func packagePath(relPath, modulePath, moduleDir, pkgName string) string {
	dir := relDir(relPath, moduleDir)
	pkgPath := modulePath
	if dir != "." {
		pkgPath = modulePath + "/" + dir
	}
	if strings.HasSuffix(pkgName, "_test") {
		pkgPath += "_test"
	}
	return pkgPath
}

// relDir returns the directory of relPath, relative to moduleDir ("."
// when the file is in moduleDir).
func relDir(relPath, moduleDir string) string {
	dir := path.Dir(relPath)
	if moduleDir == "." || moduleDir == "" {
		return dir
	}
	if dir == moduleDir {
		return "."
	}
	return strings.TrimPrefix(dir, moduleDir+"/")
}

// span is a byte range in one file mapped to the symbol that owns it. It
// backs the position index built in Build (see goload.go).
type span struct {
	start, end int
	id         SymbolID
}

// extractSymbols extracts one Symbol per top-level declaration in f, per
// the rules in docs/04-dodge.md. ParseFile and Build both call it so that
// IDs, signatures and hashes are comparable on both sides of a diff.
func extractSymbols(fset *token.FileSet, f *ast.File, src []byte, relPath, pkgPath string) []*Symbol {
	syms, _, _ := extractAll(fset, f, src, relPath, pkgPath)
	return syms
}

// extractAll is extractSymbols plus each symbol's declaration span (byte
// offsets, Doc-inclusive) and name identifier, used by Build to populate
// the position index and look up each symbol's types.Object.
func extractAll(fset *token.FileSet, f *ast.File, src []byte, relPath, pkgPath string) (syms []*Symbol, spans []span, nameIdents []*ast.Ident) {
	toks := scanTokens(src)
	generated := ast.IsGenerated(f)
	isTest := strings.HasSuffix(relPath, "_test.go")
	pkgName := f.Name.Name
	ordinals := map[string]int{}

	add := func(sym *Symbol, declStart, declEnd token.Pos, nameIdent *ast.Ident) {
		syms = append(syms, sym)
		spans = append(spans, span{
			start: offset(fset, declStart),
			end:   offset(fset, declEnd),
			id:    sym.ID,
		})
		nameIdents = append(nameIdents, nameIdent)
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym, declStart, declEnd := funcSymbol(fset, d, toks, relPath, pkgPath, pkgName, isTest, generated, ordinals)
			add(sym, declStart, declEnd, d.Name)
		case *ast.GenDecl:
			switch d.Tok {
			case token.TYPE:
				for _, s := range d.Specs {
					ts := s.(*ast.TypeSpec)
					sym, declStart, declEnd := typeSymbol(fset, d, ts, toks, relPath, pkgPath, pkgName, isTest, generated, ordinals)
					add(sym, declStart, declEnd, ts.Name)
				}
			case token.VAR:
				for _, s := range d.Specs {
					vs := s.(*ast.ValueSpec)
					sym, declStart, declEnd := valueSymbol(fset, d, vs, KindVar, toks, relPath, pkgPath, pkgName, isTest, generated, ordinals)
					add(sym, declStart, declEnd, vs.Names[0])
				}
			case token.CONST:
				for _, s := range d.Specs {
					vs := s.(*ast.ValueSpec)
					sym, declStart, declEnd := valueSymbol(fset, d, vs, KindConst, toks, relPath, pkgPath, pkgName, isTest, generated, ordinals)
					add(sym, declStart, declEnd, vs.Names[0])
				}
			}
		}
	}
	return syms, spans, nameIdents
}

// funcSymbol builds the Symbol for a *ast.FuncDecl and returns its
// Doc-inclusive declaration span.
func funcSymbol(fset *token.FileSet, d *ast.FuncDecl, toks []tok, relPath, pkgPath, pkgName string, isTest, generated bool, ordinals map[string]int) (sym *Symbol, declStart, declEnd token.Pos) {
	name := d.Name.Name
	kind := KindFunc
	exported := ast.IsExported(name)
	if d.Recv != nil {
		recv := recvTypeName(d.Recv)
		exported = ast.IsExported(recv) && ast.IsExported(name)
		name = recv + "." + name
		kind = KindMethod
	}

	declStart = d.Pos()
	if d.Doc != nil {
		declStart = d.Doc.Pos()
	}
	declEnd = d.End()

	sigEnd := d.End()
	if d.Body != nil {
		sigEnd = d.Body.Lbrace
	}

	sym = &Symbol{
		ID:        symbolID(pkgPath, name, relPath, ordinals),
		Kind:      kind,
		Name:      name,
		Pkg:       pkgPath,
		PkgName:   pkgName,
		File:      relPath,
		StartLine: fset.Position(declStart).Line,
		EndLine:   fset.Position(declEnd - 1).Line,
		DeclLine:  fset.Position(d.Name.Pos()).Line,
		Exported:  exported,
		IsTest:    isTest,
		Generated: generated,
		Signature: sliceTokenText(toks, offset(fset, d.Pos()), offset(fset, sigEnd)),
		BodyHash:  hashTokenText(toks, offset(fset, d.Pos()), offset(fset, d.End())),
	}
	return sym, declStart, declEnd
}

// typeSymbol builds the Symbol for one *ast.TypeSpec inside d and returns
// its Doc-inclusive declaration span.
func typeSymbol(fset *token.FileSet, d *ast.GenDecl, ts *ast.TypeSpec, toks []tok, relPath, pkgPath, pkgName string, isTest, generated bool, ordinals map[string]int) (sym *Symbol, declStart, declEnd token.Pos) {
	name := ts.Name.Name
	declStart, declEnd = genDeclSpan(d, ts.Doc, ts.Pos(), ts.End())
	_, isIface := ts.Type.(*ast.InterfaceType)

	sym = &Symbol{
		ID:          symbolID(pkgPath, name, relPath, ordinals),
		Kind:        KindType,
		Name:        name,
		Pkg:         pkgPath,
		PkgName:     pkgName,
		File:        relPath,
		StartLine:   fset.Position(declStart).Line,
		EndLine:     fset.Position(declEnd - 1).Line,
		DeclLine:    fset.Position(ts.Name.Pos()).Line,
		Exported:    ast.IsExported(name),
		IsTest:      isTest,
		IsInterface: isIface,
		Generated:   generated,
		Signature:   sliceTokenText(toks, offset(fset, ts.Pos()), offset(fset, ts.End())),
		BodyHash:    hashTokenText(toks, offset(fset, ts.Pos()), offset(fset, ts.End())),
	}
	return sym, declStart, declEnd
}

// valueSymbol builds the Symbol for one *ast.ValueSpec (var or const)
// inside d and returns its Doc-inclusive declaration span.
func valueSymbol(fset *token.FileSet, d *ast.GenDecl, vs *ast.ValueSpec, kind SymbolKind, toks []tok, relPath, pkgPath, pkgName string, isTest, generated bool, ordinals map[string]int) (sym *Symbol, declStart, declEnd token.Pos) {
	name := vs.Names[0].Name
	declStart, declEnd = genDeclSpan(d, vs.Doc, vs.Pos(), vs.End())

	sigEnd := vs.Names[len(vs.Names)-1].End()
	if vs.Type != nil {
		sigEnd = vs.Type.End()
	}

	sym = &Symbol{
		ID:        symbolID(pkgPath, name, relPath, ordinals),
		Kind:      kind,
		Name:      name,
		Pkg:       pkgPath,
		PkgName:   pkgName,
		File:      relPath,
		StartLine: fset.Position(declStart).Line,
		EndLine:   fset.Position(declEnd - 1).Line,
		DeclLine:  fset.Position(vs.Names[0].Pos()).Line,
		Exported:  ast.IsExported(name),
		IsTest:    isTest,
		Generated: generated,
		Signature: sliceTokenText(toks, offset(fset, vs.Pos()), offset(fset, sigEnd)),
		BodyHash:  hashTokenText(toks, offset(fset, vs.Pos()), offset(fset, vs.End())),
	}
	return sym, declStart, declEnd
}

// genDeclSpan returns the Doc-inclusive declaration span of one spec
// inside d: the whole of d when d is not parenthesised, else just the
// spec.
func genDeclSpan(d *ast.GenDecl, specDoc *ast.CommentGroup, specStart, specEnd token.Pos) (start, end token.Pos) {
	if !d.Lparen.IsValid() {
		start = d.Pos()
		if d.Doc != nil {
			start = d.Doc.Pos()
		}
		return start, d.End()
	}
	start = specStart
	if specDoc != nil {
		start = specDoc.Pos()
	}
	return start, specEnd
}

// recvTypeName returns the method receiver's type identifier, unwrapping
// parentheses, pointers and generic instantiation. It returns "?" when no
// identifier remains.
func recvTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return "?"
	}
	t := recv.List[0].Type
loop:
	for {
		switch x := t.(type) {
		case *ast.ParenExpr:
			t = x.X
		case *ast.StarExpr:
			t = x.X
		case *ast.IndexExpr:
			t = x.X
		case *ast.IndexListExpr:
			t = x.X
		default:
			break loop
		}
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return "?"
}

// symbolID returns the SymbolID for name in pkgPath. "init" and "_" can
// legally repeat inside a package, so they get an ordinal suffix counting
// same-named declarations in relPath, in source order.
func symbolID(pkgPath, name, relPath string, ordinals map[string]int) SymbolID {
	if name == "init" || name == "_" {
		key := relPath + "\x00" + name
		ordinals[key]++
		return SymbolID(pkgPath + "." + name + "@" + path.Base(relPath) + "#" + strconv.Itoa(ordinals[key]))
	}
	return SymbolID(pkgPath + "." + name)
}

// offset returns the byte offset of pos in fset.
func offset(fset *token.FileSet, pos token.Pos) int {
	return fset.Position(pos).Offset
}
