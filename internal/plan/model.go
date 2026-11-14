// Package plan turns a diff and a symbol graph into the review plan the UI
// renders: items grouped into threads, in the order an experienced
// reviewer would read them.
package plan

import (
	"github.com/sakshamsaxena/blaze/internal/diff"
	"github.com/sakshamsaxena/blaze/internal/dodge"
	"github.com/sakshamsaxena/blaze/internal/target"
)

// Role is why an item is reviewed the way it is.
type Role string

const (
	RoleContract   Role = "contract"
	RoleLogic      Role = "logic"
	RoleWiring     Role = "wiring"
	RoleTest       Role = "test"
	RoleConfig     Role = "config"
	RoleDocs       Role = "docs"
	RoleGenerated  Role = "generated"
	RoleMechanical Role = "mechanical"
	RoleOther      Role = "other"
)

// ItemKind is what an Item represents.
type ItemKind string

const (
	ItemSymbol   ItemKind = "symbol"
	ItemFile     ItemKind = "file"
	ItemPreamble ItemKind = "preamble" // lines of a Go file outside every symbol
)

// ChangeStatus is how an item changed between base and head.
type ChangeStatus string

const (
	ChangeAdded    ChangeStatus = "added"
	ChangeModified ChangeStatus = "modified"
	ChangeDeleted  ChangeStatus = "deleted"
	ChangeRenamed  ChangeStatus = "renamed" // file items only
)

// ThreadKind is the category a Thread was built from.
type ThreadKind string

const (
	ThreadCode       ThreadKind = "code"
	ThreadShared     ThreadKind = "shared"
	ThreadContracts  ThreadKind = "contracts"
	ThreadTests      ThreadKind = "tests"
	ThreadConfig     ThreadKind = "config"
	ThreadDocs       ThreadKind = "docs"
	ThreadOther      ThreadKind = "other"
	ThreadMechanical ThreadKind = "mechanical"
	ThreadGenerated  ThreadKind = "generated"
)

// Slice is one contiguous run of diff lines, all owned by the same item.
type Slice struct {
	Path     string      `json:"path"`
	OldStart int         `json:"oldStart"` // first non-zero OldNo in Lines, else 0
	NewStart int         `json:"newStart"` // first non-zero NewNo in Lines, else 0
	Lines    []diff.Line `json:"lines"`
}

// Link is one edge between two items.
type Link struct {
	ItemID string        `json:"itemId"`
	Kind   dodge.RefKind `json:"kind"`
	Member string        `json:"member,omitempty"`
}

// Caller is one item that depends on a symbol, grouped by its From symbol.
type Caller struct {
	SymbolID string        `json:"symbolId"`
	Name     string        `json:"name"`
	Pkg      string        `json:"pkg"`
	File     string        `json:"file"`
	Line     int           `json:"line"`   // first reference site
	Sites    int           `json:"sites"`  // number of reference sites in this caller
	Kind     dodge.RefKind `json:"kind"`   // "call" if any site is a call, else "ref"
	Via      string        `json:"via"`    // interface name when reached through an interface, else ""
	InPR     bool          `json:"inPr"`   // the caller is itself a changed item
	ItemID   string        `json:"itemId"` // set when InPR
	IsTest   bool          `json:"isTest"`
	Snippet  string        `json:"snippet"` // source line at File:Line, trimmed, max 160 runes
}

// Blast is the blast radius of one changed symbol.
type Blast struct {
	Callers          []Caller `json:"callers"` // at most 200, see ordering below
	Truncated        bool     `json:"truncated"`
	OutsideCount     int      `json:"outsideCount"`    // non-test callers not in the PR
	InPRCount        int      `json:"inPrCount"`       // non-test callers in the PR
	TestCount        int      `json:"testCount"`       // test callers
	ChangedTests     int      `json:"changedTests"`    // test callers that are changed items
	OtherPackages    int      `json:"otherPackages"`   // distinct packages of outside callers, excluding the symbol's own
	TransitiveCount  int      `json:"transitiveCount"` // non-test symbols within 3 hops
	TransitiveCapped bool     `json:"transitiveCapped"`
}

// Risk is a score for one item or thread.
type Risk struct {
	Score   int      `json:"score"` // 0-100
	Level   string   `json:"level"` // "none", "low", "medium", "high"
	Reasons []string `json:"reasons"`
}

// Item is one changed symbol, file or preamble, and everything the UI
// shows about it.
type Item struct {
	ID               string        `json:"id"`
	Key              string        `json:"key"`
	ThreadID         string        `json:"threadId"`
	Kind             ItemKind      `json:"kind"`
	Symbol           *dodge.Symbol `json:"symbol"` // head symbol; base symbol when deleted; nil for file and preamble items
	Title            string        `json:"title"`
	Path             string        `json:"path"`
	OldPath          string        `json:"oldPath,omitempty"`
	Role             Role          `json:"role"`
	RoleReason       string        `json:"roleReason"`
	Status           ChangeStatus  `json:"status"`
	SignatureChanged bool          `json:"signatureChanged"`
	Binary           bool          `json:"binary"`
	Added            int           `json:"added"`
	Deleted          int           `json:"deleted"`
	Slices           []Slice       `json:"slices"`
	ContentHash      string        `json:"contentHash"`
	Uses             []Link        `json:"uses"`
	UsedBy           []Link        `json:"usedBy"`
	Blast            *Blast        `json:"blast"` // nil when not computed
	Risk             Risk          `json:"risk"`

	// formatOnly is true for a modified symbol item whose base and head
	// BodyHash are equal: the change is comments or formatting only.
	// classify reads it to assign RoleMechanical; it is not serialized.
	formatOnly bool

	// fileGenerated is true for a preamble item when any symbol of its
	// file is generated. classify reads it to assign RoleGenerated; it is
	// not serialized.
	fileGenerated bool
}

// Thread is a group of items reviewed together, in review order.
type Thread struct {
	ID       string     `json:"id"`
	Kind     ThreadKind `json:"kind"`
	Title    string     `json:"title"`
	Subtitle string     `json:"subtitle"`
	Risk     Risk       `json:"risk"`
	ItemIDs  []string   `json:"itemIds"` // in review order
}

// FileSummary is one changed file and the items that touch it.
type FileSummary struct {
	Path    string          `json:"path"`
	OldPath string          `json:"oldPath,omitempty"`
	Status  diff.FileStatus `json:"status"`
	Binary  bool            `json:"binary"`
	Added   int             `json:"added"`
	Deleted int             `json:"deleted"`
	ItemIDs []string        `json:"itemIds"`
}

// Stats are totals over a Plan.
type Stats struct {
	Files      int `json:"files"`
	Items      int `json:"items"`
	Threads    int `json:"threads"`
	Added      int `json:"added"`
	Deleted    int `json:"deleted"`
	Unresolved int `json:"unresolved"` // symbol items without type information
}

// Plan is the full review plan built by Build.
type Plan struct {
	Version  int              `json:"version"` // 1
	Target   target.Target    `json:"target"`
	Stats    Stats            `json:"stats"`
	Threads  []Thread         `json:"threads"` // in review order
	Items    map[string]*Item `json:"items"`   // by Item.ID
	Files    []FileSummary    `json:"files"`   // sorted by Path
	Warnings []string         `json:"warnings"`
}
