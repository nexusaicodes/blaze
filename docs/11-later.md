# 11 — Deferred work

Nothing here is part of the MVP. Do not start any of it before
`10-validation.md` passes. Items are listed in the order they are likely to
matter.

## 1. Graph cache

Loading a large module takes time on every run. Cache the `Graph` on disk:

- Key: head SHA + `ModuleDir` + Go version + Blaze version.
- Location: `<UserCacheDir>/blaze/graphs/<repoKey>/<key>.gob`, written with
  `encoding/gob`.
- The `objs` map is not cached; the `implements` refs are already in the
  graph.

Also add `blaze clean` to remove worktrees and cached graphs older than 14
days.

## 2. Reviewing new pushes

Marks already survive a push when an item's content hash is unchanged. Add:

- A record of the head SHA at which each mark was made.
- A sidebar filter `Changed since my last review`.

## 3. Posting comments to GitHub

Let the reviewer comment on a diff line from Blaze.

- Collect drafts locally (per item, with path, side and line).
- Submit them as one review with
  `gh api repos/{owner}/{repo}/pulls/{number}/reviews`, passing a JSON body
  with `event` and a `comments` array.
- Before implementing, read the current GitHub REST documentation for "Create
  a review for a pull request" and confirm the field names for line and side.
  Do not rely on memory for this API.
- Submitting is an outward-facing action: require an explicit confirmation
  step in the UI that lists every comment.

## 4. More languages

Do **not** write a second analyser by hand. `IGraphProvider` has two
implementations in mind:

- `go` — the current one.
- `scip` — reads a SCIP index (the open code-intelligence format produced by
  existing indexers such as `scip-typescript`, `scip-python` and `scip-java`)
  and converts its documents, symbol definitions and occurrences into
  `Symbol` and `Ref`.

What the `scip` backend must provide to keep the rest unchanged:

| Need | Source in a SCIP index |
|---|---|
| Symbol spans | definition occurrences with an enclosing range, or symbol information |
| Refs | non-definition occurrences, attributed to the enclosing definition by position (the same span rule as in `04-dodge.md`) |
| `implements` | relationship entries on symbol information |
| `Signature`, `BodyHash` | a per-language tokenizer, or a whitespace-and-comment-insensitive text hash as a first version |
| Base-side symbols | a second index at the base commit, or a syntax-only splitter per language |

Before starting, run one indexer on one real repository and check which of
these the index actually contains. Call-versus-use distinction and enclosing
ranges vary by indexer.

The role rules in `05-plan.md` (test file patterns, entry points) need a
per-language table at that point.

## 5. A local model, optional and off by default

The deterministic plan stays the product. A model may add **text**, never
ordering or risk.

Useful, bounded tasks:

- A one-sentence description of what a thread does, generated from the
  thread's diff slices and symbol names.
- A check that the PR description matches the diff ("the description
  mentions caching; no thread touches a cache").
- A short explanation of why a given outside caller might be affected, from
  the changed symbol's diff and the caller's source.

Constraints:

- Local inference only: a model served on the same machine (for example
  through an Ollama or llama.cpp server on localhost). No hosted API, ever.
- Interface `ISummarizer` with backends `none` (default) and `local`,
  selected by a flag. With `none`, the UI shows no model-related elements.
- All model output is labelled as generated in the UI and is rendered as
  plain text.
- Diff content is untrusted input to the model. Its output must never trigger
  an action.
- Results are cached by content hash so a thread is summarised once.

Choose the model by testing on the validation PRs, on the hardware you
actually have. Small code models are adequate for summarising a diff with
symbol context; they are not adequate for judging correctness, which is
consistent with what Blaze is for.

## 6. Smaller items

- Syntax highlighting. Server-side tokenisation keeps the frontend free of
  dependencies.
- Symbol rename detection: a deleted and an added symbol in the same package
  with equal `BodyHash` apart from the name.
- A "callers first" ordering toggle (reverse of the default) for reviewers
  who prefer to start at the entry point.
- Multi-module repositories and `go.work`.
- Reviewing uncommitted changes.
- Build-constraint coverage: load the module once per `GOOS`/`GOARCH` pair
  that appears in changed files' constraints.
