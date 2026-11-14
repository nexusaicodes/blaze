# 06 — Local server and review state

`blaze review` serves the plan to a browser on the same machine. The server
is small, uses only `net/http`, and is strict about who may talk to it,
because it exposes source code and the PR under review may be untrusted.

## `internal/state`

Review marks and notes. They are the only thing Blaze persists.

```go
type Mark struct {
    ReviewedAt time.Time `json:"reviewedAt"`
}

type State struct {
    Version  int               `json:"version"`  // 1
    Reviewed map[string]Mark   `json:"reviewed"` // key: Item.ContentHash
    Notes    map[string]string `json:"notes"`    // key: Item.Key
}

type IStateStore interface {
    // Open loads the state for one review target, creating it if absent.
    Open(repoKey, targetKey string) error
    // Snapshot returns a deep copy of the current state.
    Snapshot() State
    // SetReviewed adds or removes a mark and persists.
    SetReviewed(contentHash string, reviewed bool) error
    // SetNote sets a note and persists. An empty note deletes the entry.
    SetNote(itemKey, note string) error
}

func New(backend string) (IStateStore, error) // "file", "memory"
```

Why two different keys: a mark is keyed by **content hash**, so it disappears
when the item's diff changes after a new push (the reviewer must look again).
A note is keyed by **item key**, so it stays attached to the symbol.

### File backend (`file.go`)

- Path: `<os.UserConfigDir()>/blaze/state/<repoKey>/<targetKey>.json`.
  `repoKey` is the first 12 hex characters of `sha256(absolute repo root)`
  (computed in `main.go`); `targetKey` is `Target.StateKey`.
- The struct holds an unexported `baseDir string`; `New("file")` sets it from
  `os.UserConfigDir()`. Tests construct the struct directly with a temp
  directory.
- `Open`: `os.MkdirAll(dir, 0o700)`; read the file if it exists; a missing
  file gives an empty state; a file that fails to decode is renamed to
  `<name>.corrupt` and an empty state is used.
- Every mutation: lock a `sync.Mutex`, change the maps, write the whole state
  to `<name>.tmp` with mode `0o600`, then `os.Rename` over the real file.
- JSON is indented with two spaces.

### Memory backend (`memory.go`)

Same behaviour without the file. Used by `--no-state` and by tests.

## `internal/server`

```go
type Options struct {
    Plan     *plan.Plan
    State    state.IStateStore
    Git      *gitx.Runner
    Port     int
    OpenURL  bool
    Version  string
    Stderr   io.Writer
}

// Serve blocks until ctx is cancelled or the listener fails.
func Serve(ctx context.Context, opts Options) error
```

### Startup

1. `token`: 16 bytes from `crypto/rand`, hex-encoded (32 characters).
2. `ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", opts.Port))`.
   Never listen on any other address.
3. `port := ln.Addr().(*net.TCPAddr).Port`; `url := "http://127.0.0.1:<port>/"`.
4. Print `blaze: <url>  (Ctrl-C to stop)` to `opts.Stderr`.
5. If `opts.OpenURL`: start `open <url>` (darwin), `xdg-open <url>` (linux) or
   `rundll32 url.dll,FileProtocolHandler <url>` (windows) with `exec.Command`
   and do not wait for it. A failure is logged at warn level only.
6. `srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}`;
   `srv.Serve(ln)` in a goroutine.
7. On `ctx.Done()`: `srv.Shutdown` with a 2-second timeout; return `nil`.

The plan is encoded to JSON once at startup and the bytes are reused for
every `GET /api/plan`.

### Routes

Registered on an `http.ServeMux` using method patterns.

| Pattern | Handler |
|---|---|
| `GET /{$}` | `index.html` with the token inserted |
| `GET /static/` | embedded files |
| `GET /api/plan` | the plan |
| `GET /api/state` | the state |
| `PUT /api/items/{id}/review` | mark or unmark an item |
| `PUT /api/items/{id}/note` | set an item's note |
| `GET /api/source` | lines of a file at base or head |

Everything else: 404.

### Security (`security.go`)

One middleware wraps the whole mux and applies, in this order:

1. **Host check.** `r.Host` must equal `127.0.0.1:<port>` or
   `localhost:<port>`. Otherwise respond 403. This defeats DNS rebinding.
2. **Response headers**, on every response:

   ```
   Content-Security-Policy: default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'
   X-Content-Type-Options: nosniff
   Referrer-Policy: no-referrer
   Cache-Control: no-store
   ```

3. **Token check**, for paths starting with `/api/`: header `X-Blaze-Token`
   must equal the token, compared with `subtle.ConstantTimeCompare`.
   Otherwise respond 403. No CORS headers are ever sent.

The token reaches the page through `index.html`, which contains
`<meta name="blaze-token" content="__BLAZE_TOKEN__">`. The index handler
replaces `__BLAZE_TOKEN__` with the token (`bytes.Replace`, once). The CSP
forbids inline scripts, so the token cannot be put in a `<script>` block.

Request bodies are limited with `http.MaxBytesReader(w, r.Body, 64<<10)`.

### API

All responses are `application/json; charset=utf-8`. Errors use the shape
`{"error": "<message>"}` with the status codes below.

#### `GET /api/plan`

200, the `Plan` from `05-plan.md`.

#### `GET /api/state`

200:

```json
{
  "version": 1,
  "reviewed": { "9f2c…": { "reviewedAt": "2026-11-14T10:22:31Z" } },
  "notes": { "example.com/fixture/store.MemStore.Save": "check overflow" }
}
```

Both maps are always present (`{}` when empty).

#### `PUT /api/items/{id}/review`

Request: `{"reviewed": true}` or `{"reviewed": false}`.

- Unknown `id` → 404.
- Body that is not that JSON object → 400.
- Otherwise call `State.SetReviewed(item.ContentHash, reviewed)` and respond
  200 with the full state (same shape as `GET /api/state`).

#### `PUT /api/items/{id}/note`

Request: `{"note": "text"}`.

- Unknown `id` → 404. Invalid body → 400. Note longer than 10 000 runes → 400.
- Otherwise `State.SetNote(item.Key, note)`; respond 200 with the full state.

#### `GET /api/source?side=head&path=store/store.go&start=30&end=52`

Returns lines of one file at the base or head commit. The UI uses it to show
the code around a caller and the whole body of a symbol.

Validation, in order (first failure → 400 unless noted):

1. `side` is `head` or `base`.
2. `path` is non-empty, not absolute, equals `path.Clean(path)`, contains no
   `..` element, and does not start with `-`.
3. `start >= 1`, `end >= start`, `end - start < 400`.
4. `Git.Exists(sha, path)` is true, where `sha` is `Target.HeadSHA` or
   `Target.BaseSHA` → otherwise 404.

Then `Git.Show(sha, path)`; split on `\n`; respond 200:

```json
{
  "side": "head",
  "path": "store/store.go",
  "start": 30,
  "total": 52,
  "lines": ["}", "", "// Save stores r.", "…"]
}
```

`lines` holds lines `start` through `min(end, total)`. If `start > total` it
is empty. A file containing a NUL byte in its first 8000 bytes is binary →
respond 415.

Git is the only way file content is read here. The server never opens a path
from a request on the file system.

## `web/embed.go`

```go
package web

//go:embed static
var files embed.FS

// Static returns the UI files rooted at static/.
func Static() fs.FS
```

`GET /static/` is served with
`http.StripPrefix("/static/", http.FileServerFS(web.Static()))`.
`index.html` is read once from the same FS at startup.

## Tests

`internal/state`:

1. Round trip: set marks and notes, construct a new store on the same
   directory, `Open`, compare `Snapshot`.
2. Unmark removes the entry; empty note removes the entry.
3. Corrupt file is renamed and replaced by an empty state.
4. `Snapshot` is a deep copy (mutating it does not affect the store).
5. Concurrent `SetReviewed` calls under `-race`.

`internal/server` (using `httptest.NewServer` around the handler, a
hand-built two-item plan, the memory store and a stub for source reads):

1. Wrong `Host` header → 403.
2. `/api/plan` without token → 403; with token → 200 and valid JSON.
3. `PUT review` → state changes; unknown id → 404; bad body → 400.
4. `PUT note` over the limit → 400.
5. `/api/source` rejects `../x`, absolute paths, `end < start` and a range
   over 400 lines.
6. Index response contains the token and no `__BLAZE_TOKEN__` text.
7. Every response carries the four security headers.

To make test 5 possible without a repository, the handler takes source
through a small interface defined in `server`:

```go
type sourceReader interface {
    Exists(ctx context.Context, sha, path string) bool
    Show(ctx context.Context, sha, path string) ([]byte, error)
}
```

`*gitx.Runner` satisfies it.
