# 02 — Architecture

## Pipeline

One run is a straight pipeline. Every stage finishes before the server starts.

```
CLI args
   │
   ▼
target.Resolve ──────────► Target{BaseSHA, HeadSHA, PR metadata}      (03)
   │
   ├─► gitx: worktree at HeadSHA ─► dodge.Build ─► Graph (head)       (04)
   │
   ├─► gitx: diff BaseSHA HeadSHA ─► diff.Parse ─► []FileDiff         (03)
   │
   ▼
plan.Build(Target, []FileDiff, Graph, base-file reader) ─► Plan       (05)
   │
   ├─► `blaze plan`   : write Plan as JSON to stdout, exit
   └─► `blaze review` : state.Open ─► server.Serve(Plan, State)       (06)
                                          │
                                          ▼
                                     browser UI                        (07)
```

## Repository layout

```
cmd/blaze/main.go            CLI entry point, flag parsing, dependency wiring
internal/config/config.go    Config struct and Validate
internal/logger/logger.go    zerolog global: Init, InitNop, Get
internal/gitx/               git CLI wrapper
    gitx.go                  Runner type and all git commands
internal/target/             what is being reviewed
    target.go                Target type, Resolve
    local.go                 two-ref targets
    github.go                PR targets via gh
internal/diff/               unified diff parsing
    model.go                 FileDiff, Hunk, Line
    parse.go                 Parse
internal/dodge/              symbol graph engine
    model.go                 SymbolID, Symbol, Ref, Graph
    provider.go              IGraphProvider, New(backend)
    graph.go                 Graph construction helpers and queries
    gosyntax.go              syntax-only symbol extraction (shared by base and head)
    gotokens.go              token-stream normalisation: Signature, BodyHash
    goload.go                go/packages loading and file selection
    gorefs.go                reference and implements extraction
internal/plan/               review plan
    model.go                 Plan, Thread, Item, Slice, Blast, Risk
    build.go                 Build: orchestrates the steps below
    attribute.go             diff lines → items
    classify.go              roles
    thread.go                threads
    order.go                 ordering inside and across threads
    blast.go                 blast radius
    risk.go                  risk score
internal/state/              review marks and notes
    store.go                 IStateStore, New(backend)
    file.go                  JSON file backend
    memory.go                in-memory backend (tests, --no-state)
internal/server/             local HTTP server
    server.go                Serve, routing
    handlers.go              API handlers
    security.go              host check, token check, headers
web/                         browser UI
    embed.go                 //go:embed static
    static/                  index.html, style.css, js/*.js
internal/testutil/           fixture repository builder for tests
    fixture.go
docs/                        these documents
```

Package dependency direction (no cycles):

```
cmd/blaze → config, logger, gitx, target, diff, dodge, plan, state, server
server    → plan, state, gitx, web
plan      → diff, dodge, target
target    → gitx
dodge     → logger (standard library, x/tools only)
diff      → (standard library only)
state     → (standard library only)
```

## Dependencies

Module path: `github.com/sakshamsaxena/blaze`.

| Module | Version | Used for |
|---|---|---|
| Go toolchain | `go 1.26.0` in `go.mod` | `x/tools` v0.52.0 requires it **(verified: `go get` raised the directive to 1.26.0)** |
| `golang.org/x/tools` | v0.52.0 | `go/packages` |
| `github.com/spf13/cobra` | v1.10.2 | CLI |
| `github.com/rs/zerolog` | v1.35.0 | logging |
| `github.com/stretchr/testify` | latest v1 | `require` in tests |

No other dependency may be added without a written reason in the commit
message. In particular: no go-git, no web framework, no router, no frontend
package manager.

External programs required at run time: `git` (2.30 or newer), `go` (for
`go list`, invoked by `go/packages`), and `gh` only when the target is a PR.

## CLI

```
blaze review [<pr>] [flags]    build the plan, serve the UI, open the browser
blaze plan   [<pr>] [flags]    build the plan, print JSON to stdout, exit 0
blaze version                  print version
```

`<pr>` is optional. It is a PR number (`482`) or a PR URL. When present the
target is a GitHub PR and `--base`/`--head` must not be set.

Flags common to `review` and `plan`:

| Flag | Default | Meaning |
|---|---|---|
| `--repo <dir>` | `.` | Any directory inside the repository |
| `--base <ref>` | see `03` | Base ref for a local target |
| `--head <ref>` | `HEAD` | Head ref for a local target |
| `--remote <name>` | `origin` | Remote used to fetch PR refs |
| `--module-dir <rel>` | `.` | Directory of `go.mod`, relative to the repo root |
| `--log-level <lvl>` | `warn` | `debug`, `info`, `warn`, `error` |

Flags for `review` only:

| Flag | Default | Meaning |
|---|---|---|
| `--port <n>` | `0` | TCP port on 127.0.0.1; 0 picks a free port |
| `--no-open` | false | Do not open the browser |
| `--no-state` | false | Keep review marks in memory only |

Progress lines go to stderr prefixed `blaze: `. Logs go to stderr through
zerolog. Stdout carries only the JSON of `blaze plan`.

Exit codes: `0` success, `1` any error (message on stderr).

`config.Config` holds one field per flag plus `PR string`.
`Config.Validate()` rejects: `PR` together with `--base` or `--head`; a
`--module-dir` that is absolute or contains `..`; a port outside 0–65535; an
unknown log level.

## Wiring in `cmd/blaze/main.go`

`run(ctx, cfg, mode)`:

1. `cfg.Validate()`; `logger.Init(cfg.LogLevel)`.
2. `git := gitx.New(cfg.Repo)`; `root, err := git.RepoRoot(ctx)`; from here
   every git command runs with `root` as its directory.
3. `tgt, err := target.Resolve(ctx, git, cfg)`.
4. `wt, err := git.EnsureWorktree(ctx, tgt.HeadSHA)` (path of a checkout of head).
5. `gp, err := dodge.New("go")`;
   `graph, err := gp.Build(ctx, dodge.BuildInput{RepoRoot: wt, ModuleDir: cfg.ModuleDir})`.
6. `raw, err := git.Diff(ctx, tgt.BaseSHA, tgt.HeadSHA)`;
   `names, err := git.NameStatus(ctx, tgt.BaseSHA, tgt.HeadSHA)`;
   `files, err := diff.Parse(names, raw)`.
7. `p, err := plan.Build(ctx, plan.Input{Target: tgt, Files: files, Graph: graph, Provider: gp, Git: git, Worktree: wt, ModuleDir: cfg.ModuleDir})`.
8. Mode `plan`: encode `p` with `json.NewEncoder(os.Stdout)`, indent two
   spaces, return.
9. Mode `review`: `st, err := state.New(backend)` where backend is `"file"` or
   `"memory"`; `st.Open(repoKey, tgt.StateKey)`, where `repoKey` is the first 12
   hex characters of `sha256(root)`; `server.Serve(ctx, server.Options{...})`.

`ctx` is cancelled on SIGINT and SIGTERM (`signal.NotifyContext`).

## Coding conventions

These follow the author's established Go style and are mandatory.

**Errors**
- Wrap every error with context: `fmt.Errorf("load packages: %w", err)`.
- No custom error types. Sentinel errors (`var ErrX = errors.New(...)`) only
  where a caller must branch on them.
- Startup errors: print to stderr, `os.Exit(1)`.

**Logging**
- zerolog behind a global atomic pointer: `logger.Init(level)`,
  `logger.InitNop()`, `logger.Get()`.
- Component loggers: `logger.Get().With().Str("component", "dodge").Logger()`.
- Tests call `logger.InitNop()` in `TestMain`.

**Interfaces and factories**
- Interfaces live in the package of their implementations.
- Interfaces with more than one implementation carry an `I` prefix:
  `IGraphProvider`, `IStateStore`.
- Each such subsystem has a factory `New(backend string) (IFoo, error)` that
  switches on the backend string and returns an error for an unknown value.
- Adding an implementation means adding a file and a `case`.

**Naming**
- Packages: lowercase, one word.
- Short local names: `cfg`, `ctx`, `err`, `fd`, `sym`, `g`.

**Tests**
- `testify/require` for assertions.
- Table-driven where there are several input cases.
- Stub implementations of interfaces, no mocking library.
- `go test -race ./...` must pass at every milestone.

**Comments**
- Comments describe what the code does now. No change history in comments.

**Processes**
- External programs run through `exec.CommandContext` with an argument slice.
  Never through a shell. (A shell would also mangle arguments such as
  `<sha>:<path>`; zsh treats `:s` after a variable as a modifier.)

## Makefile targets

| Target | Command |
|---|---|
| `build` | `go build -o blaze ./cmd/blaze` |
| `test` | `go test -race ./...` |
| `lint` | `go vet ./...` |
| `install` | `go install ./cmd/blaze` |
