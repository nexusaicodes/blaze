# 09 — Milestones

Ten milestones. Each is one commit (or a short series) that builds, passes
`make lint` and `make test`, and leaves the tree in a working state. Do them
in order; each depends only on earlier ones.

Rules for every milestone:

- Read the listed documents first. Implement what they say; when something is
  not specified, choose the simplest behaviour and note it in the commit
  message. Do not add features, flags or dependencies that are not listed.
- Write the tests named in the documents in the same commit as the code.
- `go vet ./...` and `go test -race ./...` must pass before committing.
- Update `CLAUDE.md` and the root `README.md` when commands or flags change.

## M0 — Scaffold

Read: `02-architecture.md`.

Create:

- `go.mod` with module `github.com/sakshamsaxena/blaze`, `go 1.26.0`, and the
  dependencies in the table of `02-architecture.md`.
- `Makefile` with the four targets.
- `.gitignore`: `/blaze`, `*.test`, `.DS_Store`.
- `internal/logger/logger.go`: `Init(level string) error`, `InitNop()`,
  `Get() *zerolog.Logger`, backed by `atomic.Pointer[zerolog.Logger]`. Output
  to stderr with `zerolog.ConsoleWriter`.
- `internal/config/config.go`: `Config` and `Validate`.
- `cmd/blaze/main.go`: cobra root command with `review`, `plan`, `version`.
  `review` and `plan` parse flags into `Config`, validate, and for now return
  the error `not implemented`.
- Root `README.md`: what Blaze is (three sentences), build and run commands.

Tests: `config.Validate` table (each rejection in `02-architecture.md`).

Done when: `make build && ./blaze version` prints a version;
`./blaze plan --base x 12` fails with the "PR together with --base" error.

## M1 — Git, targets, diff

Read: `03-git-and-diff.md`, the builder section of `08-fixture.md`.

Create `internal/gitx`, `internal/target` (local targets only),
`internal/diff`, `internal/testutil/fixture.go`.

Tests:

- `diff`: the nine parser cases.
- `gitx` (on the fixture): `RepoRoot`, `ResolveCommit`, `MergeBase`,
  `NameStatus` equals the observed string in `08-fixture.md`, `Show` returns
  base `store/store.go`, `Exists` false for a missing path, `EnsureWorktree`
  creates a checkout whose `HEAD` is the head SHA and reuses it on a second
  call. Point the worktree cache at a temp directory in tests (give `Runner`
  an unexported `cacheDir` field that defaults to `os.UserCacheDir()`).
- `target`: local resolution on the fixture gives `BaseSHA == fx.BaseSHA`,
  `HeadSHA == fx.HeadSHA`; `--base feature --head feature` gives the "nothing
  to review" error.
- `diff.Parse` on the fixture's real output: six files with the statuses and
  hunk counts listed in `08-fixture.md`.

Done when: all of the above pass.

## M2 — Dodge

Read: `04-dodge.md`, the graph section of `08-fixture.md`.

Create `internal/dodge` (all files).

Tests: the six listed in `04-dodge.md`.

Done when: `TestBuildSymbols`, `TestBuildRefs` and `TestImplements` match
`08-fixture.md` exactly.

## M3 — Plan: attribution and classification

Read: `05-plan.md` (model, steps 1–2).

Create `internal/plan/model.go`, `attribute.go`, `classify.go`, and a
`build.go` that runs steps 1–2 and puts every item into a single placeholder
thread.

Tests: `TestAttribute`, `TestClassify`, `TestContentHashStable`.

Done when: on the fixture, the item table of `08-fixture.md` (titles, kinds,
statuses, roles, line counts) is reproduced.

## M4 — Plan: links, threads, order

Read: `05-plan.md` (steps 3, 4, 7).

Create `thread.go`, `order.go`; replace the placeholder thread.

Tests: `TestThreads`, `TestOrder`.

Done when: on the fixture, the four threads and the item order of thread 1
match `08-fixture.md` (ignoring risk, which is still zero; thread order among
code threads is then decided by the tie-break).

## M5 — Plan: blast and risk

Read: `05-plan.md` (steps 5, 6, finishing).

Create `blast.go`, `risk.go`; finish `build.go`.

Tests: `TestBlast`, `TestRisk`.

Done when: on the fixture, the blast and risk table matches.

## M6 — `blaze plan` end to end

Read: wiring section of `02-architecture.md`, golden test in `08-fixture.md`.

Wire `run` in `cmd/blaze/main.go` for mode `plan`. Print progress lines to
stderr: `resolving target …`, `analysing packages …`, `plan ready: N threads, M items`.

Tests: `TestFixturePlan` (golden, including the determinism check).

Done when:

- `./blaze plan --repo <fixture> --base main --head feature | jq '.threads[0].title'`
  prints `"store.Store + 5 more"`.
- Running `./blaze plan --base <some older commit>` in the Blaze repository
  itself produces a plan without errors. Read it and check that the order
  makes sense. Record anything odd in the commit message; do not tune yet.

## M7 — Server and state

Read: `06-server.md`.

Create `internal/state`, `internal/server`, `web/embed.go`, and a placeholder
`web/static/index.html` that contains the token meta tag and the text
`Blaze`.

Wire mode `review` in `main.go`.

Tests: the state and server tests listed in `06-server.md`.

Done when: `./blaze review --no-open --base … --head …` prints a URL;
`curl` with the right `Host` and token returns the plan; without the token it
returns 403.

## M8 — Web UI

Read: `07-web-ui.md`.

Create everything under `web/static/`.

Manual checklist, run against the fixture
(`./blaze review --repo <fixture> --base main --head feature`):

1. Sidebar shows four threads in the expected order; the last one starts
   collapsed.
2. First item selected is `store.Record`; its diff shows one added line.
3. `j` walks the items in the expected order.
4. `store.Store` shows four outside callers under `service` and one in-PR
   caller; clicking `Service.Put` expands eleven lines of context; clicking
   `Service.Remove` selects that item.
5. `store.MemStore.Save` shows `Service.Put` with `via Store`.
6. `r` marks an item, moves on, and updates the three progress counters.
7. Restarting the server keeps the marks. `--no-state` does not.
8. A note survives a restart.
9. `bin.dat` shows `Binary file`; `new name.txt` shows `No content changes`.
10. `Show whole declaration` on `MemStore.Save` shows lines 32–37 with line
    34 highlighted.
11. The Files tab lists six files; clicking one selects its first item.
12. Reloading with `#/item/<id>` in the URL selects that item.
13. Browser console: no errors, no CSP violations.
14. Dark mode renders legibly.
15. Search the JavaScript for `innerHTML`, `outerHTML`, `insertAdjacentHTML`:
    no matches.

Done when: every point passes.

## M9 — GitHub PR targets

Read: the GitHub target section of `03-git-and-diff.md`.

Create `internal/target/github.go`; wire the `<pr>` argument.

Tests: with `runGH` stubbed to return canned JSON whose OIDs are the fixture's
SHAs, `Resolve` returns kind `github`, the title, the number, the merge base
and `StateKey == "pr-<n>"`. A second case: `gh` missing → the documented
error.

Done when: `./blaze review <n>` works on a real PR of a real Go repository
without touching that repository's working tree (`git status` unchanged).

## M10 — Validation

Read: `10-validation.md`.

No code unless validation shows a defect. Deliverables are the results table
and a decision, as described there.

## Order of risk

If time is short, the milestones that prove or disprove the idea are M2, M4
and M6. M6's output can be judged by reading JSON. Do not polish the UI (M8)
before the plan for real PRs looks right.
