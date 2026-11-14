# 05 — The review plan

`plan.Build` turns a diff and a graph into the plan the UI renders. It runs
seven steps in a fixed order. Each step is one file in `internal/plan`.

```
1. attribute   diff lines → items                        attribute.go
2. classify    item → role                               classify.go
3. link        item ↔ item edges (Uses / UsedBy)         thread.go
4. thread      items → threads                           thread.go
5. blast       callers of each changed symbol            blast.go
6. risk        score per item and per thread             risk.go
7. order       items inside threads, then threads        order.go
```

## Input

```go
type Input struct {
    Target    target.Target
    Files     []diff.FileDiff
    Graph     *dodge.Graph         // head
    Provider  dodge.IGraphProvider // for syntax-only parsing
    Git       *gitx.Runner         // to read base files
    Worktree  string               // head checkout, to read source lines
    ModuleDir string
}

func Build(ctx context.Context, in Input) (*Plan, error)
```

## Model (`model.go`)

```go
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

type ItemKind string

const (
    ItemSymbol   ItemKind = "symbol"
    ItemFile     ItemKind = "file"
    ItemPreamble ItemKind = "preamble" // lines of a Go file outside every symbol
)

type ChangeStatus string

const (
    ChangeAdded    ChangeStatus = "added"
    ChangeModified ChangeStatus = "modified"
    ChangeDeleted  ChangeStatus = "deleted"
    ChangeRenamed  ChangeStatus = "renamed" // file items only
)

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

type Slice struct {
    Path     string      `json:"path"`
    OldStart int         `json:"oldStart"` // first non-zero OldNo in Lines, else 0
    NewStart int         `json:"newStart"` // first non-zero NewNo in Lines, else 0
    Lines    []diff.Line `json:"lines"`
}

type Link struct {
    ItemID string        `json:"itemId"`
    Kind   dodge.RefKind `json:"kind"`
    Member string        `json:"member,omitempty"`
}

type Caller struct {
    SymbolID string        `json:"symbolId"`
    Name     string        `json:"name"`
    Pkg      string        `json:"pkg"`
    File     string        `json:"file"`
    Line     int           `json:"line"`    // first reference site
    Sites    int           `json:"sites"`   // number of reference sites in this caller
    Kind     dodge.RefKind `json:"kind"`    // "call" if any site is a call, else "ref"
    Via      string        `json:"via"`     // interface name when reached through an interface, else ""
    InPR     bool          `json:"inPr"`    // the caller is itself a changed item
    ItemID   string        `json:"itemId"`  // set when InPR
    IsTest   bool          `json:"isTest"`
    Snippet  string        `json:"snippet"` // source line at File:Line, trimmed, max 160 runes
}

type Blast struct {
    Callers         []Caller `json:"callers"`         // at most 200, see ordering below
    Truncated       bool     `json:"truncated"`
    OutsideCount    int      `json:"outsideCount"`    // non-test callers not in the PR
    InPRCount       int      `json:"inPrCount"`       // non-test callers in the PR
    TestCount       int      `json:"testCount"`       // test callers
    ChangedTests    int      `json:"changedTests"`    // test callers that are changed items
    OtherPackages   int      `json:"otherPackages"`   // distinct packages of outside callers, excluding the symbol's own
    TransitiveCount int      `json:"transitiveCount"` // non-test symbols within 3 hops
    TransitiveCapped bool    `json:"transitiveCapped"`
}

type Risk struct {
    Score   int      `json:"score"`   // 0–100
    Level   string   `json:"level"`   // "none", "low", "medium", "high"
    Reasons []string `json:"reasons"`
}

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
}

type Thread struct {
    ID       string     `json:"id"`
    Kind     ThreadKind `json:"kind"`
    Title    string     `json:"title"`
    Subtitle string     `json:"subtitle"`
    Risk     Risk       `json:"risk"`
    ItemIDs  []string   `json:"itemIds"` // in review order
}

type FileSummary struct {
    Path    string          `json:"path"`
    OldPath string          `json:"oldPath,omitempty"`
    Status  diff.FileStatus `json:"status"`
    Binary  bool            `json:"binary"`
    Added   int             `json:"added"`
    Deleted int             `json:"deleted"`
    ItemIDs []string        `json:"itemIds"`
}

type Stats struct {
    Files      int `json:"files"`
    Items      int `json:"items"`
    Threads    int `json:"threads"`
    Added      int `json:"added"`
    Deleted    int `json:"deleted"`
    Unresolved int `json:"unresolved"` // symbol items without type information
}

type Plan struct {
    Version  int              `json:"version"` // 1
    Target   target.Target    `json:"target"`
    Stats    Stats            `json:"stats"`
    Threads  []Thread         `json:"threads"` // in review order
    Items    map[string]*Item `json:"items"`   // by Item.ID
    Files    []FileSummary    `json:"files"`   // sorted by Path
    Warnings []string         `json:"warnings"`
}
```

`Plan.Warnings` starts as a copy of `Graph.Warnings`.

### IDs and hashes

- `Item.Key`: the symbol ID for symbol items; `file:<path>` for file items;
  `file:<path>#preamble` for preamble items.
- `Item.ID`: `"i"` + first 12 hex characters of `sha256(Key)`.
- `Thread.ID`: `"t"` + first 12 hex characters of `sha256(Key of the thread's anchor item)`.
  The anchor is defined in step 4. For fixed threads use `sha256("thread:" + Kind)`.
- `Item.ContentHash`: hex SHA-256 of `Key + "\n"` followed by, for every slice
  in order and every line in order, `<c><Text>\n` where `<c>` is `' '`, `'+'`
  or `'-'`. Review marks are stored against this hash, so a mark survives a
  new push when the item's diff is unchanged.

## Step 1 — attribute

Goal: every changed line belongs to exactly one item.

### Which files are Go files

A `FileDiff` is handled as Go when all hold: not `Binary`; `Path()` ends in
`.go`; `Path()` is inside `ModuleDir`. Everything else becomes a file item.

### Collect symbols for both sides

For every Go `FileDiff`, before attributing any line:

- **Head symbols** (when `NewPath != ""`): `Graph.SymbolsInFile(NewPath)`. If
  that is empty, read the file from `Worktree`, call `Provider.ParseFile`, and
  append the warning
  `"<path>: not loaded by the Go toolchain (build constraints?); shown without blast radius"`.
- **Base symbols** (when `OldPath != ""` and status is not `added`):
  `Git.Show(BaseSHA, OldPath)` then `Provider.ParseFile`.
- A parse error on either side: treat that side as having no symbols and
  append a warning `"<path>: <error>"`.

Build two maps over all files: `baseByID` and `headByID`
(`SymbolID → *dodge.Symbol`), and per file a line lookup
(`StartLine <= n <= EndLine`).

### Owner of a diff line

| Line kind | Owner key |
|---|---|
| `add` | ID of the head symbol containing `NewNo` in `NewPath` |
| `del` | ID of the base symbol containing `OldNo` in `OldPath` |
| `context` | ID of the head symbol containing `NewNo` in `NewPath` |

When no symbol contains the line, the owner is `file:<Path()>#preamble`.

**Blank-line rule.** A blank line between two declarations belongs to no
symbol. Without a correction, adding a function would create a preamble item
holding one empty line. So, after assigning owners in a hunk: every `add` or
`del` line whose owner is the preamble and whose `strings.TrimSpace(Text)` is
empty takes the owner of the nearest **following** line in the same hunk whose
owner is a symbol; if there is none, the nearest **preceding** such line; if
there is none either, it stays with the preamble.

### Slices

Inside each hunk, cut the lines into maximal runs of consecutive lines with
the same owner. Drop runs that contain no `add` or `del` line. Each remaining
run is a `Slice` appended to the item with that owner key.

Example from the fixture, hunk `@@ -9,12 +9,14 @@` of `store/store.go`, which
touches two types:

```
 type Record struct {          owner store.Record
 	ID   string              owner store.Record
 	Body string              owner store.Record
+	Version int              owner store.Record     ← run 1: slice for store.Record
 }                             owner store.Record
                               owner preamble         (dropped: no change)
 // Store persists records.    owner store.Store
 type Store interface {        owner store.Store
 	Save(r Record) error     owner store.Store
 	Load(id string) (…)      owner store.Store
+	Delete(id string) error  owner store.Store      ← run 2: slice for store.Store
 }                             owner store.Store
```

One item may collect slices from several hunks, and from two files when a
function moved between files of the same package (deleted lines come from the
old file, added lines from the new one, and the ID is the same).

### Item fields after attribution

For a symbol item with key `id`:

| Field | Value |
|---|---|
| `Status` | `modified` if `id` is in both maps; `added` if only in `headByID`; `deleted` if only in `baseByID` |
| `Symbol` | head symbol, or base symbol when deleted |
| `Path` | `Symbol.File` |
| `Title` | `Symbol.PkgName + "." + Symbol.Name` |
| `SignatureChanged` | `modified` and `base.Signature != head.Signature` |
| format-only (unexported bool) | `modified` and `base.BodyHash == head.BodyHash` |
| `Added`, `Deleted` | counts of `add` / `del` lines over all slices |

For a file item: one slice per hunk containing all its lines; `Title = Path()`;
`Status` from the file status (`added`, `deleted`, `renamed` for renamed and
copied, else `modified`); `OldPath` set when renamed; `Binary` copied.

For a preamble item: `Title = Path() + " (imports and header)"`,
`Status = modified`.

## Step 2 — classify

`classify(item) (Role, reason string)`. Rules are tried in order; the first
match wins. `base` means the last path element; `seg(x)` means some path
element equals `x`; `ext` is the lower-cased extension.

| # | Applies to | Condition | Role | Reason text |
|---|---|---|---|---|
| 1 | symbol, preamble | `Symbol.Generated` (for preamble: any symbol of the file is generated) | generated | `generated file` |
| 2 | file | `seg("vendor")` or `seg("node_modules")` | generated | `vendored dependency` |
| 3 | file | `base` ∈ {`go.sum`, `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `Cargo.lock`, `poetry.lock`, `Gemfile.lock`, `composer.lock`} | generated | `lockfile` |
| 4 | file | `base` matches `*.pb.go`, `*_gen.go`, `*.gen.go`, `zz_generated*`, `*.min.js`, `*.snap` | generated | `generated file` |
| 5 | preamble | always | mechanical | `imports and file header` |
| 6 | symbol | format-only | mechanical | `comments or formatting only` |
| 7 | file | no hunks, not binary, status renamed | mechanical | `renamed without changes` |
| 8 | symbol | `Symbol.IsTest` | test | `test code` |
| 9 | file | `seg("testdata")`, or `base` matches `*_test.*`, `*.test.*`, `*.spec.*` | test | `test file or test data` |
| 10 | symbol | kind `type` | contract | `type declaration` |
| 11 | symbol | kind func/method and `SignatureChanged` | contract | `signature changed` |
| 12 | symbol | status `deleted` and `Exported` | contract | `exported symbol removed` |
| 13 | symbol | kind var/const and `Exported` | contract | `exported value` |
| 14 | symbol | `PkgName == "main"`, or `seg("cmd")`, or name is `init` or `main` | wiring | `entry point or wiring` |
| 15 | symbol | otherwise | logic | `implementation` |
| 16 | file | `ext` ∈ {`.proto`, `.sql`, `.graphql`, `.gql`, `.thrift`, `.avsc`}, or `seg("migrations")`, or `seg("migration")`, or `base` starts with `openapi` or `swagger` | contract | `schema or API definition` |
| 17 | file | `ext` ∈ {`.md`, `.mdx`, `.rst`, `.txt`, `.adoc`}, or first path element is `docs`, or `base` starts with `LICENSE`, `NOTICE` or `CHANGELOG` | docs | `documentation` |
| 18 | file | `base == "go.mod"`, or `ext` ∈ {`.yaml`, `.yml`, `.json`, `.toml`, `.ini`, `.tf`, `.hcl`}, or `base` starts with `Dockerfile`, or `base` ∈ {`Makefile`, `.gitignore`, `.editorconfig`}, or path starts with `.github/` | config | `configuration` |
| 19 | file | otherwise | other | `other file` |

Glob matching uses `path.Match` on `base`.

## Step 3 — link

**Code items** are symbol items whose role is contract, logic, wiring or test.
Let `itemOf` map symbol ID → code item.

For each code item `a` and each `r` in `Graph.Out[a.Symbol.ID]` with
`itemOf[r.To] == b` and `b != a`:

- add `Link{ItemID: b.ID, Kind: r.Kind, Member: r.Member}` to `a.Uses`
- add `Link{ItemID: a.ID, Kind: r.Kind, Member: r.Member}` to `b.UsedBy`

Deduplicate each list on `(ItemID, Kind, Member)` and sort by
`(ItemID, Kind, Member)`.

Deleted and unresolved symbols have no outgoing refs in the head graph, so
they get no links here. Step 4 places them.

## Step 4 — thread

### 4a. Hubs

A **hub** is a helper that many unrelated changes happen to use. Its edges
would glue unrelated threads together.

A non-test code item `h` is a hub when all hold:

- `h.Symbol.Kind` is func or method
- `h.Status == modified` and `!h.SignatureChanged`
- the number of distinct non-test code items in `h.UsedBy` is `>= HubMinUsers`

`const HubMinUsers = 5`.

Each hub becomes its own thread of kind `shared`. Edges to and from hubs are
ignored in 4b. (If the signature changed, callers changed *because* of it, so
it is not treated as a hub.)

### 4b. Components

Nodes: non-test, non-hub code items. Edges: every `Uses` link between two
nodes, undirected. Compute connected components with union-find. Each
component is one thread of kind `code`.

### 4c. Tests

For each test code item `t`, count its `Uses` links per thread from 4a/4b
(a link to an item counts for that item's thread). Assign `t` to the thread
with the highest count. Ties: the thread whose anchor key sorts first.
A test item with no such links goes to the fixed `tests` thread.

Tests never merge threads; they only join one.

### 4d. Orphans

A node from 4b that ends up alone in its component **and** has no links at all
(typically a deleted or unresolved symbol) is an orphan. Attach it to the
`code` thread that contains another item with the same `Path`; if several, the
one whose item is nearest by `Symbol.StartLine`. If there is none, leave it as
its own single-item thread.

### 4e. Fixed threads

Items that are not code items are grouped by role into fixed threads:

| Role | Thread kind | Title |
|---|---|---|
| contract (file items) | `contracts` | `Schemas and API definitions` |
| test (file items, and test symbols from 4c without links) | `tests` | `Tests and test data` |
| config | `config` | `Configuration` |
| docs | `docs` | `Documentation` |
| other | `other` | `Other files` |
| mechanical | `mechanical` | `Mechanical changes` |
| generated | `generated` | `Generated files` |

A fixed thread exists only when it has at least one item.

### 4f. Anchor, title, subtitle

For `code` and `shared` threads:

- **Anchor**: the non-test item with the most **distinct items** in `UsedBy`
  that belong to the same thread (several links from one item count once).
  Ties: role order contract < logic < wiring, then `Key`.
- **Title**: `anchor.Title` when the thread has one item; otherwise
  `anchor.Title + " + N more"` where `N = len(items) - 1`.
- **Subtitle**: the distinct `Symbol.PkgName` values of the thread's items,
  sorted, joined with `", "`; at most 4, then `", …"`.

For fixed threads the subtitle is `"N files"` or `"N items"`.

## Step 5 — blast

Computed for every symbol item with `Symbol.Resolved == true` and
`Status != deleted`. Otherwise `Blast` stays `nil`.

### Incoming refs of a symbol

```go
// incoming returns the refs that depend on s, including calls made through
// interfaces that s implements.
func incoming(g *dodge.Graph, s *dodge.Symbol) []viaRef // viaRef{Ref dodge.Ref; Via string}
```

1. **Direct**: every `r` in `g.In[s.ID]` with `r.Kind != RefImplements`.
   `Via = ""`.
2. **Through interfaces** (only when `s.Kind == KindMethod`): for every
   `impl` in `g.Out[s.ID]` with `impl.Kind == RefImplements` and
   `impl.Member != ""`, let `I = impl.To`. Add every `r` in `g.In[I]` with
   `r.Kind != RefImplements` and `r.Member == impl.Member`.
   `Via = g.Symbols[I].Name`.

On the fixture, `incoming(MemStore.Save)` is:

```
store.TestSave      → MemStore.Save  call          via ""
service.Service.Put → store.Store    call "Save"   via "Store"
```

### Callers

Group `incoming` by `Ref.From`. One `Caller` per group:

- `Line`: the smallest `(File, Line)` site; `Sites`: group size
- `Kind`: `call` if any ref in the group is a call, else `ref`
- `Via`: the first non-empty `Via` in the group, else `""`
- `InPR`, `ItemID`: from `itemOf` (any item kind, including mechanical)
- `IsTest`: `Symbol.IsTest` of the caller
- `Snippet`: line `Line` of `File` read from `Worktree`, `strings.TrimSpace`,
  cut to 160 runes. Cache file contents per path.

Counts:

| Field | Definition |
|---|---|
| `OutsideCount` | callers with `!IsTest && !InPR` |
| `InPRCount` | callers with `!IsTest && InPR` |
| `TestCount` | callers with `IsTest` |
| `ChangedTests` | callers with `IsTest && InPR` |
| `OtherPackages` | distinct `Pkg` among outside callers, excluding `s.Pkg` |

Sort callers: outside non-test first, then in-PR non-test, then tests; inside
each group by `(Pkg, File, Line)`. Keep the first 200; set `Truncated` when
more existed. Counts are taken before truncation.

### Transitive count

Breadth-first search over `incoming`, starting at `s`, depth limit
`TransitiveDepth = 3`. Count distinct non-test symbols reached, excluding `s`.
Do not expand through test symbols. Stop when the visited set reaches
`TransitiveCap = 2000` and set `TransitiveCapped`.

On the fixture: `MemStore.Save` → `Service.Put` → `main.main`, so
`TransitiveCount = 2`.

## Step 6 — risk

Scored only for symbol items whose role is contract, logic or wiring and whose
`Blast != nil`. Every other item gets `Risk{Score: 0, Level: "none"}`.

Add the points of every factor that applies. Each factor appends its reason.

| # | Factor | Applies when | Points | Reason text |
|---|---|---|---|---|
| A | Outside callers | status `modified`; `n = OutsideCount` | 1–2 → 10; 3–9 → 20; ≥ 10 → 30 | `N callers outside this PR` |
| B | Other packages | status `modified`; `p = OtherPackages` | 1–3 → 10; ≥ 4 → 20 | `used from P other packages` |
| C | Exported | status `modified` and `Symbol.Exported` | 10 | `exported` |
| D | Signature | `SignatureChanged` | 10 | `signature changed` (kind type: `declaration changed`) |
| E | Tests | kind func or method | `TestCount == 0` → 15; else `ChangedTests == 0` → 5 | `no test references this` / `tests exist but none changed in this PR` |
| F | Size | `lines = Added + Deleted` | ≥ 150 → 15; ≥ 50 → 10 | `large change (N lines)` |

`Score = min(100, sum)`. `Level`: `high` when `Score >= 50`, `medium` when
`>= 25`, else `low`.

Thread risk: the `Risk` of its highest-scoring item, with each reason prefixed
by that item's title and `": "`. Ties: the item that comes first in review
order. A thread with no scored item has `Risk{0, "none", nil}`.

The weights are starting values. `10-validation.md` describes how to tune
them. Keep them as named constants at the top of `risk.go`.

## Step 7 — order

### Items inside a `code` or `shared` thread

Principle: **definitions before uses**. If `a` uses `b`, review `b` first.

1. Split the thread's items into `tests` (role test) and `nodes` (the rest).
2. Dependency edges between nodes: `a → b` for every link in `a.Uses` whose
   target is a node of this thread.
3. **Interface bridge.** If `a` uses interface item `I` with `Member = m`
   (kind `call` or `ref`), and node `c` uses `I` with kind `implements` and
   the same `Member`, add `a → c`. This puts an implementation before the
   code that calls it through the interface.
4. Sort key of a node: `(roleRank, Path, Symbol.StartLine, Key)` with
   `roleRank` contract 0, logic 1, wiring 2.
5. Depth-first post-order:

   ```
   order = []
   state = {}                       // absent, "visiting", "done"
   visit(n):
       if state[n] is set: return   // "visiting" means a cycle; skip the back edge
       state[n] = "visiting"
       for d in deps(n) sorted by sort key: visit(d)
       state[n] = "done"
       order.append(n)
   for n in nodes sorted by sort key: visit(n)
   ```

6. Append `tests` sorted by `(Path, Symbol.StartLine, Key)`.

Post-order guarantees dependencies come first, tolerates cycles, and keeps a
chain (a method, then its caller) together.

### Items inside a fixed thread

Sort by `(Path, first slice NewStart or OldStart, Key)`.

### Threads

1. The `contracts` thread, if present.
2. `code` and `shared` threads by `Risk.Score` descending. Ties: smallest
   `(Path, StartLine)` of the first item.
3. Fixed threads in this order: `tests`, `config`, `docs`, `other`,
   `mechanical`, `generated`.

## Finishing

- `Files`: one `FileSummary` per `FileDiff`, sorted by `Path`, with the IDs of
  the items that have a slice in that file (for pure renames and binary files:
  the file item).
- `Stats`: totals; `Unresolved` counts symbol items with `!Symbol.Resolved`.
- Replace every `nil` slice with an empty slice before returning, so the JSON
  contains `[]` and never `null` for lists.

## Tests (`internal/plan`)

Unit tests build small graphs and diffs by hand (no git, no packages):

1. `TestAttribute`: the two-type hunk above yields two slices with the right
   owners; a deleted function between two untouched ones; lines outside any
   symbol go to the preamble item; a function moved between two files becomes
   one item with two slices.
2. `TestClassify`: one case per row of the table.
3. `TestThreads`: two unrelated pairs → two threads; a hub with five users is
   split out; the same hub with `SignatureChanged` is not; a test that
   references two threads joins the one it references more; an orphan deleted
   symbol attaches to the thread of its file.
4. `TestOrder`: dependency order; cycle of two; interface bridge; tests last.
5. `TestBlast`: direct, through-interface, counts, sort order, truncation at
   200, transitive depth and cap.
6. `TestRisk`: one case per factor row and the cap at 100.
7. `TestContentHashStable`: unchanged slices give the same hash; one changed
   line gives a different one.

The end-to-end expectation for the fixture is in `08-fixture.md`.
