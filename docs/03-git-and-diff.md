# 03 — Git, targets and diff parsing

## `internal/gitx`

```go
type Runner struct {
    dir      string // working directory for every command
    cacheDir string // root for worktrees; defaults to os.UserCacheDir(), overridden in tests
}

func New(dir string) *Runner
func (r *Runner) WithDir(dir string) *Runner
```

One private helper runs everything:

```go
// run executes `git <args...>` in r.dir and returns stdout.
// On a non-zero exit it returns an error containing the arguments and
// trimmed stderr.
func (r *Runner) run(ctx context.Context, args ...string) ([]byte, error)
```

`run` always prepends these arguments so user configuration cannot change the
output format:

```
-c core.quotepath=false -c color.ui=false
```

Public methods and the exact arguments they pass:

| Method | git arguments | Returns |
|---|---|---|
| `RepoRoot(ctx)` | `rev-parse --show-toplevel` | absolute path, trimmed |
| `ResolveCommit(ctx, ref)` | `rev-parse --verify --quiet <ref>^{commit}` | 40-char SHA; error if empty |
| `MergeBase(ctx, a, b)` | `merge-base <a> <b>` | SHA |
| `DefaultBase(ctx, remote)` | see below | ref name |
| `HasCommit(ctx, sha)` | `cat-file -e <sha>^{commit}` | bool (exit code) |
| `Fetch(ctx, remote, refspec)` | `fetch --quiet --no-tags <remote> <refspec>` | error |
| `NameStatus(ctx, base, head)` | `diff --name-status -z -M <base> <head>` | raw bytes |
| `Diff(ctx, base, head)` | `diff --no-color --no-ext-diff -M -U3 --src-prefix=a/ --dst-prefix=b/ <base> <head>` | raw bytes |
| `Show(ctx, sha, path)` | `show <sha>:<path>` | file bytes |
| `Exists(ctx, sha, path)` | `cat-file -e <sha>:<path>` | bool (exit code) |
| `EnsureWorktree(ctx, sha)` | see below | absolute path |

`NameStatus` and `Diff` must be called with the same two SHAs so that their
file order matches **(verified: both list files in the same order)**.

### `DefaultBase`

Try in order and return the first that resolves with `ResolveCommit`:

1. Output of `symbolic-ref --quiet --short refs/remotes/<remote>/HEAD`
   (for example `origin/main`).
2. `<remote>/main`
3. `<remote>/master`
4. `main`
5. `master`

If none resolves: error `cannot determine base branch; pass --base`.

### `EnsureWorktree`

Analysis needs the head sources on disk without touching the user's checkout.

- Path: `<os.UserCacheDir()>/blaze/worktrees/<repoKey>/<sha[:12]>` where
  `repoKey` is the first 12 hex characters of `sha256(absolute repo root)`.
- If the path exists and `git -C <path> rev-parse HEAD` equals `sha`, reuse it.
- If the path exists otherwise: run `worktree remove --force <path>`, ignore
  its error, `os.RemoveAll(path)`, then `worktree prune`.
- Create: `worktree add --detach --quiet <path> <sha>`.
- Worktrees are kept between runs. They are cheap and make a second run
  faster. (A cleanup command is deferred.)

## `internal/target`

```go
type Kind string

const (
    KindLocal  Kind = "local"
    KindGitHub Kind = "github"
)

type Target struct {
    Kind     Kind   `json:"kind"`
    RepoRoot string `json:"-"`
    Title    string `json:"title"`
    Body     string `json:"body"`
    URL      string `json:"url"`     // "" for local
    Number   int    `json:"number"`  // 0 for local
    Author   string `json:"author"`  // "" for local
    BaseRef  string `json:"baseRef"` // name as given or as reported by gh
    HeadRef  string `json:"headRef"`
    BaseSHA  string `json:"baseSha"` // the MERGE BASE, not the base branch tip
    HeadSHA  string `json:"headSha"`
    StateKey string `json:"-"`
}

func Resolve(ctx context.Context, git *gitx.Runner, cfg config.Config) (Target, error)
```

`Resolve` calls `resolveGitHub` when `cfg.PR != ""`, else `resolveLocal`.

### Local target (`local.go`)

1. `baseRef := cfg.Base`; if empty, `git.DefaultBase(ctx, cfg.Remote)`.
2. `headSHA := git.ResolveCommit(ctx, cfg.Head)`.
3. `baseTip := git.ResolveCommit(ctx, baseRef)`.
4. `BaseSHA := git.MergeBase(ctx, baseTip, headSHA)`.
5. If `BaseSHA == headSHA`: error `nothing to review: <head> has no commits beyond <base>`.
6. `Title = "<headRef> → <baseRef>"`.
7. `StateKey = "local-" + sanitize(baseRef) + "--" + sanitize(cfg.Head)`.

`sanitize` replaces every character outside `[A-Za-z0-9._-]` with `_`.

### GitHub target (`github.go`)

1. Run, in the repo root:

   ```
   gh pr view <cfg.PR> --json number,title,body,url,author,baseRefName,baseRefOid,headRefName,headRefOid
   ```

   Decode into:

   ```go
   type ghPR struct {
       Number      int    `json:"number"`
       Title       string `json:"title"`
       Body        string `json:"body"`
       URL         string `json:"url"`
       Author      struct{ Login string `json:"login"` } `json:"author"`
       BaseRefName string `json:"baseRefName"`
       BaseRefOid  string `json:"baseRefOid"`
       HeadRefName string `json:"headRefName"`
       HeadRefOid  string `json:"headRefOid"`
   }
   ```

   If `gh` is not on `PATH`: error `the gh CLI is required to review a PR by number; install it or use --base/--head`.
2. If `!git.HasCommit(HeadRefOid)`: `git.Fetch(ctx, cfg.Remote, "refs/pull/<number>/head")`.
3. If `!git.HasCommit(BaseRefOid)`: `git.Fetch(ctx, cfg.Remote, BaseRefName)`.
4. After fetching, both commits must exist, else error naming the missing SHA.
5. `BaseSHA = git.MergeBase(ctx, BaseRefOid, HeadRefOid)`; `HeadSHA = HeadRefOid`.
6. `StateKey = "pr-<number>"`.

`gh` is run through an unexported package variable
`var runGH = func(ctx context.Context, dir string, args ...string) ([]byte, error)`
so tests can replace it with a stub that returns canned JSON.

## `internal/diff`

### Model (`model.go`)

```go
type FileStatus string

const (
    StatusAdded    FileStatus = "added"
    StatusModified FileStatus = "modified"
    StatusDeleted  FileStatus = "deleted"
    StatusRenamed  FileStatus = "renamed"
    StatusCopied   FileStatus = "copied"
    StatusTypeChanged FileStatus = "typechanged"
)

type LineKind string

const (
    LineContext LineKind = "context"
    LineAdd     LineKind = "add"
    LineDel     LineKind = "del"
)

type Line struct {
    Kind  LineKind `json:"kind"`
    OldNo int      `json:"oldNo"` // 0 for add
    NewNo int      `json:"newNo"` // 0 for del
    Text  string   `json:"text"`  // without the leading ' ', '+' or '-'; no trailing newline
    NoEOL bool     `json:"noEol,omitempty"`
}

type Hunk struct {
    OldStart int    `json:"oldStart"`
    OldLines int    `json:"oldLines"`
    NewStart int    `json:"newStart"`
    NewLines int    `json:"newLines"`
    Section  string `json:"section"`
    Lines    []Line `json:"lines"`
}

type FileDiff struct {
    OldPath string     `json:"oldPath"` // "" when added
    NewPath string     `json:"newPath"` // "" when deleted
    Status  FileStatus `json:"status"`
    Binary  bool       `json:"binary"`
    Hunks   []Hunk     `json:"hunks"`
    Added   int        `json:"added"`
    Deleted int        `json:"deleted"`
}

// Path returns NewPath, or OldPath for a deleted file.
func (f FileDiff) Path() string
```

Paths are repository-relative with forward slashes, exactly as git prints
them.

### `Parse(nameStatus, patch []byte) ([]FileDiff, error)`

Two inputs are parsed and joined by position. Paths come only from
`nameStatus`, which is unambiguous. Hunks come only from `patch`.

**Step 1 — parse `nameStatus`.** The bytes are NUL-separated fields
**(verified)**:

```
M\0README.md\0A\0bin.dat\0R100\0old name.txt\0new name.txt\0M\0service/service.go\0
```

Read a status field, then:

| Status field starts with | Fields that follow | Result |
|---|---|---|
| `A` | path | added, `NewPath` |
| `M` | path | modified, both paths equal |
| `D` | path | deleted, `OldPath` |
| `T` | path | typechanged, both paths equal |
| `R` | old path, new path | renamed |
| `C` | old path, new path | copied |

Any other status letter: return an error naming it. A trailing empty field
after the final NUL is ignored.

**Step 2 — split `patch` into file sections.** A section starts at each line
that begins with `diff --git ` and runs to the next such line or the end.
Lines are split on `\n` only; a trailing `\r` is part of `Text`.

The number of sections must equal the number of entries from step 1, else
return `fmt.Errorf("diff: %d name-status entries but %d patch sections", …)`.
Section *i* belongs to entry *i*.

**Step 3 — parse one section.** Walk its lines after the `diff --git` line:

- Before the first hunk header, lines are extended headers. Only one matters:
  a line starting with `Binary files ` or equal to `GIT binary patch` sets
  `Binary = true`. All other header lines (`index`, `old mode`, `new mode`,
  `new file mode`, `deleted file mode`, `similarity index`, `rename from`,
  `rename to`, `copy from`, `copy to`, `--- `, `+++ `) are skipped.
- A hunk header matches
  `^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$`.
  A missing count means 1. Group 5 is `Section`.
- Inside a hunk, keep two counters: `oldNo` starts at `OldStart`, `newNo` at
  `NewStart`. For each line, by its first byte:

  | First byte | Line | Counters |
  |---|---|---|
  | space | context, `OldNo=oldNo`, `NewNo=newNo` | both ++ |
  | `+` | add, `NewNo=newNo` | `newNo`++, `Added`++ |
  | `-` | del, `OldNo=oldNo` | `oldNo`++, `Deleted`++ |
  | `\` | sets `NoEOL = true` on the previous line; adds no line | none |

  An empty line inside a hunk (git emits a bare `\n` for an empty context
  line only if a tool stripped trailing whitespace) is treated as context
  with empty text.
- A hunk ends when it has consumed `OldLines` old lines and `NewLines` new
  lines, or at the next hunk header, or at the end of the section.

A section with no hunks is valid: a pure rename **(verified: a 100 % rename
has no `---`/`+++` lines and no hunks)**, a mode change, or a binary file.

### Tests (`parse_test.go`)

Table-driven, each case gives literal `nameStatus` and `patch` bytes:

1. One modified file, one hunk with context, add and del lines; assert every
   `OldNo`/`NewNo`.
2. Hunk header with omitted counts (`@@ -1 +1,3 @@`).
3. Added file (`@@ -0,0 +1,2 @@`) and deleted file.
4. Pure rename with a space in both names, no hunks.
5. Rename with edits.
6. Binary file.
7. `\ No newline at end of file` on the last added line.
8. Two files, second has two hunks; assert section-to-entry pairing.
9. Count mismatch between the two inputs returns an error.
