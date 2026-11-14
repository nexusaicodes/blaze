# 08 — Test fixture and expected plan

One small Go module is the shared test input for Dodge, the plan builder and
the end-to-end test. It is created at test time in a temporary directory; it
is not checked in as a nested module.

The fixture below was built and analysed by a prototype. Statements marked
**(verified)** were observed. The expected plan at the end is derived by hand
from the rules in `05-plan.md`; if the implementation disagrees with it, work
out which one is wrong before changing either.

## Builder (`internal/testutil/fixture.go`)

```go
type Fixture struct {
    Dir     string // repository root (symlinks resolved)
    BaseSHA string // commit on main
    HeadSHA string // commit on feature
}

// NewFixture creates the repository in t.TempDir() and returns it.
func NewFixture(t *testing.T) Fixture
```

Steps:

1. `dir := t.TempDir()`, then `filepath.EvalSymlinks(dir)`.
2. Write the **base files** listed below.
3. `git init --quiet --initial-branch=main`
4. `git add -A` and commit with message `base`.
5. `git checkout --quiet -b feature`
6. Apply the **head changes** listed below.
7. `git add -A` and commit with message `head`.
8. Read both SHAs with `git rev-parse`.

Every git command in the builder is run with these extra arguments so that
the developer's configuration cannot interfere:

```
-c user.name=blaze -c user.email=blaze@example.invalid -c commit.gpgsign=false -c core.hooksPath=/dev/null
```

Tests that need the fixture are skipped with `t.Skip` when `git` or `go` is
not on `PATH`.

File contents are Go string constants in `fixture.go`. Tabs matter; copy them
exactly.

## Base files

`go.mod`

```
module example.com/fixture

go 1.25
```

`README.md`

```
# fixture
```

`old name.txt`

```
x
```

`store/store.go`

```go
package store

import "errors"

// ErrNotFound is returned when a record is missing.
var ErrNotFound = errors.New("not found")

// Record is one stored item.
type Record struct {
	ID   string
	Body string
}

// Store persists records.
type Store interface {
	Save(r Record) error
	Load(id string) (Record, error)
}

// MemStore is an in-memory Store.
type MemStore struct {
	data map[string]Record
}

// NewMemStore builds an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{data: map[string]Record{}}
}

// Save stores r.
func (m *MemStore) Save(r Record) error {
	m.data[r.ID] = r
	return nil
}

// Load fetches a record.
func (m *MemStore) Load(id string) (Record, error) {
	r, ok := m.data[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return r, nil
}
```

`store/store_test.go`

```go
package store

import "testing"

func TestSave(t *testing.T) {
	m := NewMemStore()
	if err := m.Save(Record{ID: "a"}); err != nil {
		t.Fatal(err)
	}
}
```

`store/ext_test.go`

```go
package store_test

import (
	"testing"

	"example.com/fixture/store"
)

func TestLoadMissing(t *testing.T) {
	m := store.NewMemStore()
	if _, err := m.Load("x"); err != store.ErrNotFound {
		t.Fatal(err)
	}
}
```

`service/service.go`

```go
package service

import "example.com/fixture/store"

// Service is the business layer.
type Service struct {
	st store.Store
}

// New wires a Service.
func New(st store.Store) *Service {
	return &Service{st: st}
}

// Put writes a record.
func (s *Service) Put(id, body string) error {
	return s.st.Save(store.Record{ID: id, Body: body})
}

// Get reads a record body.
func (s *Service) Get(id string) (string, error) {
	r, err := s.st.Load(id)
	if err != nil {
		return "", err
	}
	return r.Body, nil
}
```

`cmd/app/main.go`

```go
package main

import (
	"fmt"

	"example.com/fixture/service"
	"example.com/fixture/store"
)

func main() {
	svc := service.New(store.NewMemStore())
	if err := svc.Put("a", "hello"); err != nil {
		panic(err)
	}
	fmt.Println(svc.Get("a"))
}
```

## Head changes

Apply these to the base files. The resulting diff is shown afterwards; the
builder may simply write the full head contents.

1. `store/store.go`
   - In `Record`, add a line `\tVersion int` after `\tBody string`. Do **not**
     re-align the other fields (the fixture is deliberately not gofmt-aligned
     here so that the diff is a single added line).
   - In `Store`, add a line `\tDelete(id string) error` after the `Load` line.
   - In `MemStore.Save`, add a first statement
     `\tr.Version = m.data[r.ID].Version + 1`.
   - Append to the end of the file:

     ```go

     // Delete removes a record.
     func (m *MemStore) Delete(id string) error {
     	delete(m.data, id)
     	return nil
     }
     ```
2. `service/service.go` — append:

   ```go

   // Remove deletes a record.
   func (s *Service) Remove(id string) error {
   	return s.st.Delete(id)
   }
   ```
3. `store/store_test.go` — append:

   ```go

   func TestDelete(t *testing.T) {
   	m := NewMemStore()
   	if err := m.Delete("a"); err != nil {
   		t.Fatal(err)
   	}
   }
   ```
4. `README.md` becomes `# fixture\n\nNow with delete.\n`.
5. Rename `old name.txt` to `new name.txt` (`git mv`), content unchanged.
6. Add `bin.dat` containing the two bytes `0x00 0x01`.

`go build ./...` and `go test ./...` pass at both commits **(verified)**.

## Observed git output (verified)

`git diff --name-status -z -M <base> <head>`, with `|` standing for NUL:

```
M|README.md|A|bin.dat|R100|old name.txt|new name.txt|M|service/service.go|M|store/store.go|M|store/store_test.go|
```

Hunk headers of the patch, in order:

```
diff --git a/README.md b/README.md
@@ -1 +1,3 @@
diff --git a/bin.dat b/bin.dat
Binary files /dev/null and b/bin.dat differ
diff --git a/old name.txt b/new name.txt        (similarity index 100%, no hunks)
diff --git a/service/service.go b/service/service.go
@@ -25,3 +25,8 @@ func (s *Service) Get(id string) (string, error) {
diff --git a/store/store.go b/store/store.go
@@ -9,12 +9,14 @@ var ErrNotFound = errors.New("not found")
@@ -29,6 +31,7 @@ func NewMemStore() *MemStore {
@@ -41,3 +44,9 @@ func (m *MemStore) Load(id string) (Record, error) {
diff --git a/store/store_test.go b/store/store_test.go
@@ -8,3 +8,10 @@ func TestSave(t *testing.T) {
```

## Expected graph at head

Write `P` for `example.com/fixture`.

### Symbols (17)

| ID | Kind | File | Start–End |
|---|---|---|---|
| `P/cmd/app.main` | func | `cmd/app/main.go` | 10–16 |
| `P/service.Service` | type | `service/service.go` | 5–8 |
| `P/service.New` | func | `service/service.go` | 10–13 |
| `P/service.Service.Put` | method | `service/service.go` | 15–18 |
| `P/service.Service.Get` | method | `service/service.go` | 20–27 |
| `P/service.Service.Remove` | method | `service/service.go` | 29–32 |
| `P/store.ErrNotFound` | var | `store/store.go` | 5–6 |
| `P/store.Record` | type | `store/store.go` | 8–13 |
| `P/store.Store` | type (interface) | `store/store.go` | 15–20 |
| `P/store.MemStore` | type | `store/store.go` | 22–25 |
| `P/store.NewMemStore` | func | `store/store.go` | 27–30 |
| `P/store.MemStore.Save` | method | `store/store.go` | 32–37 |
| `P/store.MemStore.Load` | method | `store/store.go` | 39–46 |
| `P/store.MemStore.Delete` | method | `store/store.go` | 48–52 |
| `P/store.TestSave` | func (test) | `store/store_test.go` | 5–10 |
| `P/store.TestDelete` | func (test) | `store/store_test.go` | 12–17 |
| `P/store_test.TestLoadMissing` | func (test) | `store/ext_test.go` | 9–14 |

The ID set and the file of each symbol were observed at the base commit
**(verified)**; the three head-only symbols (`Remove`, `Delete`,
`TestDelete`) and all line ranges follow from the file contents above. Assert
the ID set exactly; assert line ranges for the changed symbols.

### Refs to assert

Call refs **(verified at base, unchanged at head)**:

```
cmd/app.main            → service.New             call
cmd/app.main            → store.NewMemStore       call
cmd/app.main            → service.Service.Put     call  member "Put"
cmd/app.main            → service.Service.Get     call  member "Get"
service.Service.Put     → store.Store             call  member "Save"
service.Service.Get     → store.Store             call  member "Load"
store.TestSave          → store.NewMemStore       call
store.TestSave          → store.MemStore.Save     call  member "Save"
store_test.TestLoadMissing → store.NewMemStore    call
store_test.TestLoadMissing → store.MemStore.Load  call  member "Load"
store_test.TestLoadMissing → store.ErrNotFound    ref
```

New at head (by the same rules):

```
service.Service.Remove  → store.Store             call  member "Delete"
store.TestDelete        → store.NewMemStore       call
store.TestDelete        → store.MemStore.Delete   call  member "Delete"
store.MemStore.Save     → store.Record            ref   member "Version"
```

Implements:

```
store.MemStore          → store.Store   implements
store.MemStore.Save     → store.Store   implements  member "Save"
store.MemStore.Load     → store.Store   implements  member "Load"
store.MemStore.Delete   → store.Store   implements  member "Delete"
```

## Expected plan

### Items (9)

| Title | Kind | Status | Role | Reason | + | − |
|---|---|---|---|---|---|---|
| `store.Record` | symbol | modified | contract | type declaration | 1 | 0 |
| `store.Store` | symbol | modified | contract | type declaration | 1 | 0 |
| `store.MemStore.Save` | symbol | modified | logic | implementation | 1 | 0 |
| `store.MemStore.Delete` | symbol | added | logic | implementation | 6 | 0 |
| `service.Service.Remove` | symbol | added | logic | implementation | 5 | 0 |
| `store.TestDelete` | symbol | added | test | test code | 7 | 0 |
| `README.md` | file | modified | docs | documentation | 2 | 0 |
| `bin.dat` | file | added | other | other file | 0 | 0 |
| `new name.txt` | file | renamed | mechanical | renamed without changes | 0 | 0 |

Notes:

- The blank line added before each new function has no symbol. The
  blank-line rule gives it to the function that follows, which is why
  `Delete`, `Remove` and `TestDelete` count 6, 5 and 7 added lines. There is
  **no** preamble item.
- `store.Record` and `store.Store` come from the same hunk and are two items.
- `SignatureChanged` is true for `Record` and `Store` (their declaration
  tokens changed) and false for `MemStore.Save`.

### Links

```
store.Store            uses  store.Record           (ref)
store.MemStore.Save    uses  store.Record           (ref; ref member "ID"; ref member "Version")
store.MemStore.Save    uses  store.Store            (implements "Save")
store.MemStore.Delete  uses  store.Store            (implements "Delete")
service.Service.Remove uses  store.Store            (call "Delete")
store.TestDelete       uses  store.MemStore.Delete  (call "Delete")
```

### Threads (4), in order

| # | Kind | Title | Subtitle | Risk |
|---|---|---|---|---|
| 1 | code | `store.Store + 5 more` | `service, store` | 50 high |
| 2 | docs | `Documentation` | `1 file` | none |
| 3 | other | `Other files` | `1 file` | none |
| 4 | mechanical | `Mechanical changes` | `1 file` | none |

Thread 1:

- All five non-test code items are connected through the links above; no hub
  (the largest in-PR user count is 3).
- `TestDelete` joins through its link to `MemStore.Delete`.
- Anchor: `store.Store` (used by 3 distinct items; `store.Record` by 2).

Item order in thread 1:

```
1. store.Record
2. store.Store
3. store.MemStore.Delete
4. service.Service.Remove
5. store.MemStore.Save
6. store.TestDelete
```

Derivation: sort keys give the visit order `Record, Store, Remove, Save,
Delete` (contracts first; then logic by path, and `service/…` sorts before
`store/…`). Visiting `Remove` first visits its dependencies: `Store` (done)
and, through the interface bridge on member `Delete`, `MemStore.Delete`. So
`Delete` is emitted, then `Remove`, then `Save`. The test comes last.

### Blast and risk

| Item | Outside | In PR | Tests | Changed tests | Other pkgs | Transitive | Score | Level |
|---|---|---|---|---|---|---|---|---|
| `store.Record` | 5 | 2 | 1 | 0 | 1 | — | 50 | high |
| `store.Store` | 4 | 1 | 0 | 0 | 1 | — | 50 | high |
| `store.MemStore.Save` | 1 | 0 | 1 | 0 | 1 | 2 | 35 | medium |
| `store.MemStore.Delete` | 0 | 1 | 1 | 1 | 0 | — | 0 | low |
| `service.Service.Remove` | 0 | 0 | 0 | 0 | 0 | 0 | 15 | low |

"—" means: do not assert this value in the golden test.

Derivations:

- **`store.Record`** — outside callers: `MemStore`, `NewMemStore`,
  `MemStore.Load` (package `store`), `Service.Put`, `Service.Get` (package
  `service`). In PR: `Store`, `MemStore.Save`. Test: `TestSave`.
  Factors: A (5 → 20) + B (1 → 10) + C (10) + D (10) = 50. E does not apply
  to types.
- **`store.Store`** — outside: `Service`, `New`, `Service.Put`,
  `Service.Get`. In PR: `Service.Remove`. `implements` refs are not callers.
  Factors: A (4 → 20) + B (10) + C (10) + D (10) = 50.
- **`store.MemStore.Save`** — direct: `TestSave` (test). Through `Store`,
  member `Save`: `Service.Put` (outside). Transitive: `Service.Put`,
  `main.main`.
  Factors: A (1 → 10) + B (10) + C (10) + E (tests exist, none changed → 5)
  = 35.
- **`store.MemStore.Delete`** — direct: `TestDelete` (test, changed).
  Through `Store`, member `Delete`: `Service.Remove` (in PR). Status is
  `added`, so A, B and C do not apply. E: a changed test exists → 0.
- **`service.Service.Remove`** — no callers. E: no test references → 15.

Thread 1 risk: score 50, level high, reasons taken from `store.Record` (the
first of the two 50-point items in review order), each prefixed with
`store.Record: `.

### Stats

```
files 6 · items 9 · threads 4 · added 23 · deleted 0 · unresolved 0
```

## Golden test

`cmd/blaze` (or `internal/plan`) has `TestFixturePlan`:

1. `fx := testutil.NewFixture(t)`.
2. Run the same pipeline as `blaze plan --repo fx.Dir --base main --head feature`,
   in-process.
3. Assert, by item **title** (IDs are hashes): the item table, the thread
   table, the item order of thread 1, the blast counts and scores above, and
   the stats.
4. Run the pipeline twice and require identical JSON bytes.
