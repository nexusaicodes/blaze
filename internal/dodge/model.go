// Package dodge turns a Go module into a graph of symbols (top-level
// declarations) and refs between them. The plan builder uses it for
// ordering and for blast radius.
package dodge

// SymbolID identifies one Symbol. It is "<pkgPath>.<name>", or
// "<pkgPath>.<name>@<file>#<n>" for the two names that can legally repeat
// inside a package, "init" and "_".
type SymbolID string

// SymbolKind is the kind of declaration a Symbol represents.
type SymbolKind string

const (
	KindFunc   SymbolKind = "func"
	KindMethod SymbolKind = "method"
	KindType   SymbolKind = "type"
	KindVar    SymbolKind = "var"
	KindConst  SymbolKind = "const"
)

// Symbol is a span of source text: one top-level declaration.
type Symbol struct {
	ID          SymbolID   `json:"id"`
	Kind        SymbolKind `json:"kind"`
	Name        string     `json:"name"`      // "Save", "MemStore.Save", "Record"
	Pkg         string     `json:"pkg"`       // import path, e.g. "example.com/fixture/store"
	PkgName     string     `json:"pkgName"`   // package clause name, e.g. "store"
	File        string     `json:"file"`      // repo-relative, forward slashes
	StartLine   int        `json:"startLine"` // first line, including the doc comment
	EndLine     int        `json:"endLine"`   // last line, inclusive
	DeclLine    int        `json:"declLine"`  // line of the name identifier
	Exported    bool       `json:"exported"`
	IsTest      bool       `json:"isTest"`      // file name ends in _test.go
	IsInterface bool       `json:"isInterface"` // type whose declared type is an interface literal
	Generated   bool       `json:"generated"`   // file carries a "Code generated … DO NOT EDIT." header
	Resolved    bool       `json:"resolved"`    // true when produced by Build with type information
	Signature   string     `json:"-"`
	BodyHash    string     `json:"-"`
}

// RefKind is the kind of relationship a Ref represents.
type RefKind string

const (
	RefCall       RefKind = "call"
	RefUse        RefKind = "ref"
	RefImplements RefKind = "implements"
)

// Ref is one reference from one symbol to another.
type Ref struct {
	From   SymbolID `json:"from"`
	To     SymbolID `json:"to"`
	Kind   RefKind  `json:"kind"`
	Member string   `json:"member,omitempty"` // method or field name when the target is a member of To
	File   string   `json:"file"`             // where the reference occurs
	Line   int      `json:"line"`
}

// Graph is the symbol graph of one module at one commit.
type Graph struct {
	ModulePath string
	Symbols    map[SymbolID]*Symbol
	Out        map[SymbolID][]Ref   // refs whose From is the key
	In         map[SymbolID][]Ref   // refs whose To is the key
	Files      map[string][]*Symbol // file → symbols sorted by StartLine
	Warnings   []string
}
