# 04 — Dodge: the symbol graph engine

Dodge turns a Go module into a graph: **symbols** (top-level declarations) and
**refs** between them. The plan builder uses it for ordering and for blast
radius.

The core idea, which keeps everything else simple:

> A symbol is a span of source text. Any `types.Object` is mapped to a symbol
> by looking up which span contains the object's declaration position.

This one rule handles functions, methods, struct fields, interface methods,
generic instantiations and test-package variants without special cases
**(verified on the fixture: fields resolved to their struct, interface methods
to their interface, and objects from test variants to the same symbols)**.

## Model (`model.go`)

```go
type SymbolID string

type SymbolKind string

const (
    KindFunc   SymbolKind = "func"
    KindMethod SymbolKind = "method"
    KindType   SymbolKind = "type"
    KindVar    SymbolKind = "var"
    KindConst  SymbolKind = "const"
)

type Symbol struct {
    ID          SymbolID   `json:"id"`
    Kind        SymbolKind `json:"kind"`
    Name        string     `json:"name"`        // "Save", "MemStore.Save", "Record"
    Pkg         string     `json:"pkg"`         // import path, e.g. "example.com/fixture/store"
    PkgName     string     `json:"pkgName"`     // package clause name, e.g. "store"
    File        string     `json:"file"`        // repo-relative, forward slashes
    StartLine   int        `json:"startLine"`   // first line, including the doc comment
    EndLine     int        `json:"endLine"`     // last line, inclusive
    DeclLine    int        `json:"declLine"`    // line of the name identifier
    Exported    bool       `json:"exported"`
    IsTest      bool       `json:"isTest"`      // file name ends in _test.go
    IsInterface bool       `json:"isInterface"` // type whose declared type is an interface literal
    Generated   bool       `json:"generated"`   // file carries a "Code generated … DO NOT EDIT." header
    Resolved    bool       `json:"resolved"`    // true when produced by Build with type information
    Signature   string     `json:"-"`
    BodyHash    string     `json:"-"`
}

type RefKind string

const (
    RefCall       RefKind = "call"
    RefUse        RefKind = "ref"
    RefImplements RefKind = "implements"
)

type Ref struct {
    From   SymbolID `json:"from"`
    To     SymbolID `json:"to"`
    Kind   RefKind  `json:"kind"`
    Member string   `json:"member,omitempty"` // method or field name when the target is a member of To
    File   string   `json:"file"`             // where the reference occurs
    Line   int      `json:"line"`
}

type Graph struct {
    ModulePath string
    Symbols    map[SymbolID]*Symbol
    Out        map[SymbolID][]Ref    // refs whose From is the key
    In         map[SymbolID][]Ref    // refs whose To is the key
    Files      map[string][]*Symbol  // file → symbols sorted by StartLine
    Warnings   []string
}
```

## Provider (`provider.go`)

```go
type BuildInput struct {
    RepoRoot  string // absolute path of the head worktree
    ModuleDir string // go.mod directory relative to RepoRoot; "." for the root
}

type ParseInput struct {
    Path       string // repo-relative path of the file
    Src        []byte
    ModulePath string
    ModuleDir  string
}

type IGraphProvider interface {
    // Build loads the module with full type information.
    Build(ctx context.Context, in BuildInput) (*Graph, error)
    // ParseFile extracts symbols from one file using syntax only.
    // The symbols have Resolved == false and no refs.
    ParseFile(in ParseInput) ([]*Symbol, error)
}

func New(backend string) (IGraphProvider, error) // "go" → *goProvider; else error
```

`ParseFile` exists because the base side of the diff, and head files that the
loader skipped, must still be split into symbols. Both paths call the same
extraction code so that IDs, signatures and hashes are comparable.

## Symbol extraction (`gosyntax.go`)

```go
func extractSymbols(fset *token.FileSet, f *ast.File, src []byte, relPath, pkgPath string) []*Symbol
```

`ParseFile` parses with
`parser.ParseFile(fset, in.Path, in.Src, parser.ParseComments|parser.SkipObjectResolution)`.
A parse error is returned to the caller.

### Package path

The import path is computed, never guessed from the directory name alone:

```
relDir  = directory of Path, relative to ModuleDir   ("." when the file is in ModuleDir)
pkgPath = ModulePath                    if relDir == "."
        = ModulePath + "/" + relDir     otherwise
if the package clause name ends in "_test": pkgPath += "_test"
```

This equals `packages.Package.PkgPath` for every package of the fixture
**(verified, including `store_test` and `cmd/app`)**. `Build` uses
`Package.PkgPath` directly and logs a warning if it differs from this rule.

### One symbol per declaration

Walk `f.Decls`:

| Declaration | Symbols | Kind | Name |
|---|---|---|---|
| `*ast.FuncDecl`, `Recv == nil` | one | `func` | `d.Name.Name` |
| `*ast.FuncDecl`, `Recv != nil` | one | `method` | `<Recv>.<d.Name.Name>` |
| `*ast.GenDecl`, `Tok == token.TYPE` | one per `*ast.TypeSpec` | `type` | `spec.Name.Name` |
| `*ast.GenDecl`, `Tok == token.VAR` | one per `*ast.ValueSpec` | `var` | first name |
| `*ast.GenDecl`, `Tok == token.CONST` | one per `*ast.ValueSpec` | `const` | first name |
| `*ast.GenDecl`, `Tok == token.IMPORT` | none | | |

`<Recv>` is found by unwrapping `d.Recv.List[0].Type`: remove
`*ast.ParenExpr`, `*ast.StarExpr`, `*ast.IndexExpr` (`.X`) and
`*ast.IndexListExpr` (`.X`) repeatedly until an `*ast.Ident` remains; use its
name. If no identifier remains, use `?`.

A `ValueSpec` with several names (`var a, b int`) is one symbol named after
the first name.

### Spans

| Declaration | Start position | End position |
|---|---|---|
| `FuncDecl` | `d.Doc.Pos()` if `d.Doc != nil`, else `d.Pos()` | `d.End()` |
| `GenDecl` without parentheses (`!d.Lparen.IsValid()`) | `d.Doc.Pos()` if present, else `d.Pos()` | `d.End()` |
| Spec inside a parenthesised `GenDecl` | `spec.Doc.Pos()` if present, else `spec.Pos()` | `spec.End()` |

- `StartLine = fset.Position(start).Line`
- `EndLine = fset.Position(end - 1).Line` **(verified: `End()-1` is on the
  closing-brace line)**
- `DeclLine =` line of the name identifier
- The byte span `[offset(start), offset(end))` is kept in an unexported index
  (see "Position index") and is not part of `Symbol`.

### IDs

```
ID = pkgPath + "." + Name
```

Examples **(verified)**: `example.com/fixture/store.Record`,
`example.com/fixture/store.MemStore.Save`,
`example.com/fixture/store_test.TestLoadMissing`.

Two names can legally repeat inside a package: `init` and `_`. For those:

```
ID = pkgPath + "." + Name + "@" + <file base name> + "#" + <n>
```

where `n` counts from 1 among same-named declarations in that file, in source
order. The ordinal is used instead of a line number so that the ID does not
change when lines shift between base and head.

If any other ID is produced twice during `Build` (only possible in code that
does not compile), keep the first symbol, drop the second, and append a
warning.

### Other fields

- `Exported`: `ast.IsExported(name)`. For a method, both the receiver type
  name and the method name must be exported.
- `IsTest`: `strings.HasSuffix(relPath, "_test.go")`.
- `IsInterface`: `spec.Type` is `*ast.InterfaceType`.
- `Generated`: `ast.IsGenerated(f)`.

## Normalised text (`gotokens.go`)

`Signature` and `BodyHash` let the plan builder tell a real change from a
comment or formatting change. They are computed from the **token stream**, not
from `go/printer` output. (`go/printer` on a bare node keeps struct field
comments and the original line breaks **(verified)**, so it is not a usable
normaliser.)

```go
// tokenText returns the tokens of src within [start, end), joined by one space.
func tokenText(src []byte, start, end int) string
```

Implementation:

1. `fs := token.NewFileSet(); tf := fs.AddFile("", fs.Base(), len(src))`.
2. `var s scanner.Scanner; s.Init(tf, src, nil, 0)`. Mode `0` skips comments.
3. Loop `pos, tok, lit := s.Scan()` until `tok == token.EOF`.
4. Let `off = tf.Offset(pos)`. Skip tokens with `off < start` or `off >= end`.
5. Skip a `token.SEMICOLON` whose `lit == "\n"`. Those are inserted
   automatically at line ends and depend on layout. Keep explicit semicolons.
6. The text of a token is `lit` when `lit != ""`, else `tok.String()`.
7. Join with single spaces.

To avoid rescanning, scan each file once into a slice of `(offset, text)` and
slice that per symbol.

| Symbol kind | `Signature` is `tokenText` over | `BodyHash` is hex SHA-256 of `tokenText` over |
|---|---|---|
| func, method | `d.Pos()` to `d.Body.Lbrace` (to `d.End()` when `Body == nil`) | `d.Pos()` to `d.End()` |
| type | `spec.Pos()` to `spec.End()` | same span |
| var, const | `spec.Pos()` to `spec.Type.End()`, or to the end of the last name when `spec.Type == nil` | `spec.Pos()` to `spec.End()` |

Doc comments are outside these spans, and comments inside are skipped by the
scanner. So: same `BodyHash` on both sides means only comments, blank lines or
intra-line spacing changed. Reflowing code across lines can change the hash
(an automatic semicolon appears or disappears); that is treated as a real
change, which is the safe direction.

## Loading (`goload.go`)

```go
cfg := &packages.Config{
    Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
        packages.NeedImports | packages.NeedTypes | packages.NeedTypesSizes |
        packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule,
    Dir:     filepath.Join(in.RepoRoot, in.ModuleDir),
    Tests:   true,
    Context: ctx,
}
pkgs, err := packages.Load(cfg, "./...")
```

`NeedDeps` is deliberately absent. Packages of the module are type-checked
from source; everything else comes from export data, which is much faster
**(verified: this mode yields `Syntax` and `TypesInfo` for every module
package)**. Dependencies must be in the module cache; `go list` downloads
missing ones, which is the only network access Dodge can cause.

Handling of the result:

1. `err != nil` → return `fmt.Errorf("load packages: %w", err)`.
2. `len(pkgs) == 0` → return error `no Go packages found in <dir>`.
3. `ModulePath`: the `Module.Path` of the first package whose `Module != nil`.
4. For each package with `len(p.Errors) > 0`: append
   `"<PkgPath>: <first error>"` to `Graph.Warnings` (at most 10 in total) and
   continue. Partial type information is still useful.
5. Skip packages where `p.TypesInfo == nil`.

### Test variants

With `Tests: true`, one directory yields several packages **(verified)**:

| `ID` | `PkgPath` | Contains |
|---|---|---|
| `example.com/fixture/store` | `…/store` | non-test files |
| `example.com/fixture/store [example.com/fixture/store.test]` | `…/store` | non-test files + in-package `_test.go` files |
| `example.com/fixture/store_test [example.com/fixture/store.test]` | `…/store_test` | external test files |
| `example.com/fixture/store.test` | `…/store.test` | a generated `main` in the Go build cache |

So the same source file appears in more than one package, with different
`types.Object` values. Rules:

- **Order.** Sort packages: those whose `ID` does not contain `" ["` first,
  then the rest; inside each group by `ID`. Plain packages form one consistent
  type universe, so processing them first gives the best `implements` results.
- **Each file once.** Keep `seen map[string]bool` keyed by absolute file name.
  Process a file the first time it appears; skip it afterwards. Non-test files
  are therefore taken from plain packages and `_test.go` files from variants.
- **Only files of the repository.** The file name of syntax tree `f` is
  `p.Fset.Position(f.Package).Filename`. Resolve symlinks on both the worktree
  root and the file name with `filepath.EvalSymlinks` before calling
  `filepath.Rel` (on macOS, temp directories live behind `/var → /private/var`).
  Skip the file if `Rel` fails or the result starts with `..`. This drops the
  generated test `main` and cgo output.

For each processed file: read its bytes with `os.ReadFile`, call
`extractSymbols` with `p.PkgPath`, set `Resolved = true`, add the symbols to
the graph, and record `(p, f, relPath)` in a work list for the refs pass.
Record for each symbol its `types.Object` from `p.TypesInfo.Defs[nameIdent]`
in an unexported map `objs map[SymbolID]types.Object`.

### Position index

```go
type span struct {
    start, end int // byte offsets, end exclusive
    id         SymbolID
}

type posIndex struct {
    root  string            // symlink-resolved worktree root
    files map[string][]span // repo-relative path → spans sorted by start
}

// resolve returns the symbol whose span contains pos, or "" if none.
func (ix *posIndex) resolve(fset *token.FileSet, pos token.Pos) SymbolID
```

`resolve`: return `""` when `!pos.IsValid()`; take `p := fset.Position(pos)`;
make `p.Filename` relative to `root` (with the same symlink handling); binary
search the spans of that file for `start <= p.Offset < end`.

Positions of objects from other modules and the standard library point
outside the root, so they resolve to `""` and are ignored. Universe objects
(`len`, `error`, `nil`) have no position.

## Refs (`gorefs.go`)

For each `(p, f, relPath)` in the work list:

**Pass 1 — find call positions.** `ast.Inspect(f, …)`; for each
`*ast.CallExpr`:

```go
fun := ast.Unparen(call.Fun)
switch x := fun.(type) {          // strip generic instantiation
case *ast.IndexExpr:     fun = x.X
case *ast.IndexListExpr: fun = x.X
}
switch x := fun.(type) {
case *ast.Ident:        callIdents[x] = true
case *ast.SelectorExpr: callIdents[x.Sel] = true
}
```

**Pass 2 — emit refs.** `ast.Inspect(f, …)`; for each `*ast.Ident` `id`:

1. `obj := p.TypesInfo.Uses[id]`; skip if `nil`. (Selector names are in
   `Uses` too **(verified)**.)
2. Skip `*types.PkgName`.
3. `to := ix.resolve(fset, obj.Pos())`; skip if `""`.
4. `from := ix.resolve(fset, id.Pos())`; skip if `""` (import block, package
   clause).
5. Skip if `from == to`.
6. `Member`:
   - `*types.Func` whose `Type().(*types.Signature).Recv() != nil` → `obj.Name()`
   - `*types.Var` with `IsField()` → `obj.Name()`
   - otherwise `""`
7. `Kind`: `RefCall` when `callIdents[id]` and `obj` is a `*types.Func` or a
   `*types.Var`; otherwise `RefUse`. (A conversion `T(x)` has a `*types.TypeName`
   and stays `RefUse`.)
8. `File = relPath`, `Line = fset.Position(id.Pos()).Line`.

Deduplicate on the full tuple `(From, To, Kind, Member, File, Line)`.

Observed on the fixture **(verified)**, for
`func (s *Service) Put(id, body string) error { return s.st.Save(store.Record{ID: id, Body: body}) }`:

```
service.Service.Put → service.Service   ref   member=""      (receiver type)
service.Service.Put → service.Service   ref   member="st"    (field)
service.Service.Put → store.Store       call  member="Save"  (interface method)
service.Service.Put → store.Record      ref   member=""      (composite literal type)
service.Service.Put → store.Record      ref   member="ID"
service.Service.Put → store.Record      ref   member="Body"
```

The call through the interface points at the **interface type** `store.Store`
with `Member = "Save"`. It does not point at `MemStore.Save`. The link from
the interface to its implementations is added by the next pass.

### Implements pass

Inputs: every `type` symbol with an entry in `objs` that is a
`*types.TypeName`.

Skip a type name when any of these holds:

- `tn.IsAlias()`
- its type is a `*types.Named` with `TypeParams().Len() > 0` (generic types)

Split the rest into:

- **interfaces**: `types.IsInterface(tn.Type())`, with
  `iface := tn.Type().Underlying().(*types.Interface)`,
  `iface.NumMethods() > 0` and `iface.IsMethodSet()`
- **concrete types**: everything else

For each concrete `T` and each interface `I`:

```go
if types.Implements(T.Type(), iface) || types.Implements(types.NewPointer(T.Type()), iface) {
    // type-level ref
    add(Ref{From: T.id, To: I.id, Kind: RefImplements, File: T.File, Line: T.DeclLine})
    // method-level refs
    for i := 0; i < iface.NumMethods(); i++ {
        m := iface.Method(i).Name()
        mid := SymbolID(T.Pkg + "." + T.Name + "." + m)
        if msym, ok := g.Symbols[mid]; ok {
            add(Ref{From: mid, To: I.id, Kind: RefImplements, Member: m, File: msym.File, Line: msym.DeclLine})
        }
    }
}
```

`iface.NumMethods()` covers embedded interfaces. Methods that `T` gets by
embedding another type have no symbol under `T` and are skipped.

On the fixture this yields **(type-level verified)**:

```
store.MemStore       → store.Store   implements
store.MemStore.Save  → store.Store   implements  member="Save"
store.MemStore.Load  → store.Store   implements  member="Load"
```

Cost is `concrete types × interfaces`. That is acceptable for the MVP; log the
duration at debug level.

## Graph helpers (`graph.go`)

```go
func newGraph(modulePath string) *Graph
func (g *Graph) addSymbol(s *Symbol) bool          // false when the ID already exists
func (g *Graph) addRef(r Ref)                      // appends to Out and In; deduplicates
func (g *Graph) finish()                           // sorts Files by StartLine, Out/In by (File, Line, To, From)

func (g *Graph) SymbolsInFile(path string) []*Symbol
func (g *Graph) SymbolAt(path string, line int) *Symbol // StartLine <= line <= EndLine; nil if none
```

Everything stored in the graph is sorted in `finish` so that output is
deterministic regardless of map iteration order.

## Known limits

State these in the UI help text; do not try to fix them in the MVP.

| Limit | Effect |
|---|---|
| Files excluded by build constraints for the current OS/arch | Not loaded. Plan falls back to syntax-only symbols without blast data |
| Calls through function values, reflection, `go:linkname` | Not seen |
| Generic types | No `implements` refs |
| Methods promoted through embedding | No method-level `implements` refs |
| Types declared in `_test.go` files | `implements` against non-test interfaces may be missed (different type universe) |
| Code that does not type-check | Partial refs; a warning is shown |
| cgo | Go side only |

## Tests (`internal/dodge`)

All use the fixture from `08-fixture.md` at its **head** commit.

1. `TestExtractSymbols` (syntax only, literal source strings): kinds, names,
   IDs, spans with and without doc comments, parenthesised `const` block,
   multi-name `var`, two `init` functions, generic receiver `(*Set[T])`.
2. `TestTokenText`: comment-only and spacing-only edits give equal
   `BodyHash`; a changed literal gives a different one; a signature spread
   over two lines equals the one-line form.
3. `TestBuildSymbols`: the exact symbol ID set listed in `08-fixture.md`.
4. `TestBuildRefs`: the refs listed above for `Service.Put`; the test-variant
   refs `store.TestSave → store.MemStore.Save (call, "Save")` and
   `store_test.TestLoadMissing → store.MemStore.Load (call, "Load")`.
5. `TestImplements`: the three `implements` refs above.
6. `TestBuildDeterministic`: two builds produce deeply equal graphs.
