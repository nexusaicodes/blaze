# 07 — Web UI

A single page. Plain HTML, CSS and JavaScript ES modules, embedded in the
binary. No framework, no bundler, no package manager, no external requests.

## Hard rules

1. **Never use `innerHTML`, `outerHTML`, `insertAdjacentHTML` or
   `document.write`.** Everything shown comes from a PR that may be hostile.
   Build DOM nodes with `document.createElement` and set text with
   `textContent`. The CSP in `06-server.md` is the second line of defence, not
   the first.
2. No inline `<script>` and no inline `style=""` attributes (the CSP blocks
   them). Use classes. Dynamic values that need styling (for example a
   progress bar width) are set through `element.style.setProperty` from
   JavaScript, which the CSP allows.
3. Every `fetch` to `/api/` sends the header `X-Blaze-Token`.
4. The page must work in current Chrome, Firefox and Safari without
   polyfills.

## Files (`web/static/`)

```
index.html
style.css
js/main.js        start-up, routing, top-level render
js/api.js         fetch wrappers
js/store.js       client state and derived data
js/dom.js         h() element helper
js/keys.js        keyboard shortcuts
js/sidebar.js     thread and item list
js/item.js        item header, navigation links, note box
js/diff.js        diff slices
js/blast.js       blast radius panel
js/files.js       flat file list (fallback view)
js/help.js        shortcut and limits overlay
```

### `index.html`

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="blaze-token" content="__BLAZE_TOKEN__">
  <title>Blaze</title>
  <link rel="stylesheet" href="/static/style.css">
</head>
<body>
  <header id="topbar"></header>
  <div id="layout">
    <nav id="sidebar"></nav>
    <main id="main"></main>
  </div>
  <div id="overlay" hidden></div>
  <script type="module" src="/static/js/main.js"></script>
</body>
</html>
```

### `js/dom.js`

```js
// h("div", {class: "row", onclick: fn, dataset: {id}}, child1, "text", …)
export function h(tag, props, ...children)
export function clear(node)   // removes all children
```

`h` rules: `class` sets `className`; keys starting with `on` add event
listeners; `dataset` assigns into `node.dataset`; `hidden`, `disabled`,
`checked` are set as properties; every other key uses `setAttribute`. String
and number children become text nodes. `null`, `undefined` and `false`
children are skipped. Arrays are flattened.

### `js/api.js`

```js
export async function getPlan()                 // GET /api/plan
export async function getState()                // GET /api/state
export async function setReviewed(id, reviewed) // PUT /api/items/{id}/review → state
export async function setNote(id, note)         // PUT /api/items/{id}/note   → state
export async function getSource(side, path, start, end) // GET /api/source
```

The token is read once from
`document.querySelector('meta[name="blaze-token"]').content`. A non-2xx
response throws `new Error(body.error || response.statusText)`.

### `js/store.js`

One module-level object, no reactivity library:

```js
export const store = {
  plan: null,          // from /api/plan
  state: null,         // from /api/state
  order: [],           // item IDs in review order (threads flattened)
  selectedId: null,
  view: "plan",        // "plan" | "files"
  blastOpen: true,
  collapsed: new Set() // thread IDs collapsed in the sidebar
};

export function isReviewed(item)        // store.state.reviewed has item.contentHash
export function threadProgress(thread)  // {done, total}
export function totalProgress()         // {done, total}
export function nextItem(id, delta)     // neighbour in store.order, clamped
export function nextUnreviewed(id)      // next unreviewed item after id, wrapping; null if none
export function subscribe(fn)           // fn is called after every change
export function update(mutator)         // mutator(store); then notify subscribers
```

`order` is built once: for each thread in `plan.threads`, its `itemIds`.

Rendering strategy: on every `update`, `main.js` re-renders the top bar, the
sidebar and the main pane from scratch (`clear` then rebuild). Plans are small
enough for this. Two exceptions keep the UI usable: the note `<textarea>` is
not rebuilt while it has focus, and scroll position of the sidebar is saved
and restored around its re-render.

## Layout

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ #482 Add delete to store        base a1b2c3d → head 9f8e7d6    7/23 reviewed │  topbar
│ [Plan] [Files]                                             [?]  ⚠ 1 warning │
├───────────────────────────┬──────────────────────────────────────────────────┤
│ ▼ store.Store + 5 more    │ store.Store                       contract  HIGH │
│   HIGH · service, store   │ type · modified · store/store.go:15              │
│   ✓ store.Record          │ Why here: type declaration                       │
│   ● store.Store           │ Risk 50: 4 callers outside this PR · used from 1 │
│   ○ store.MemStore.Delete │          other packages · exported · declaration │
│   ○ service.Service.Remove│          changed                                 │
│   ○ store.MemStore.Save   │ Uses: store.Record                               │
│   ○ store.TestDelete      │ Used by: MemStore.Delete (implements) · …        │
│ ▶ Documentation           │──────────────────────────────────────────────────│
│ ▶ Other files             │ store/store.go                                   │
│ ▶ Mechanical changes      │  15  15   // Store persists records.             │
│                           │  16  16   type Store interface {                 │
│                           │  17  17   	Save(r Record) error                 │
│                           │  18  18   	Load(id string) (Record, error)      │
│                           │      19 + 	Delete(id string) error              │
│                           │  19  20   }                                      │
│                           │──────────────────────────────────────────────────│
│                           │ Blast radius   4 outside · 1 in PR · 0 tests     │
│                           │ service                                          │
│                           │   Service          service/service.go:7   ref    │
│                           │   New              service/service.go:11  ref    │
│                           │   Service.Put      service/service.go:17  call   │
│                           │   Service.Get      service/service.go:22  call   │
│                           │──────────────────────────────────────────────────│
│                           │ Note [                                        ]  │
│                           │ [r] Mark reviewed        [k] prev    [j] next    │
└───────────────────────────┴──────────────────────────────────────────────────┘
```

CSS: `#layout` is a two-column grid, `minmax(260px, 340px) 1fr`. Both columns
scroll independently (`overflow: auto; height: calc(100vh - topbar height)`).
Monospace font stack for code:
`ui-monospace, SFMono-Regular, Menlo, Consolas, monospace`. Support light and
dark with `@media (prefers-color-scheme: dark)` and CSS custom properties for
all colours. Code uses `white-space: pre` and `tab-size: 4`; the diff
container scrolls horizontally.

## Components

### Top bar (`main.js`)

- Left: `#<number> <title>` for a PR (the number links to `target.url`,
  `target="_blank" rel="noopener noreferrer"`); just the title for a local
  target.
- Middle: `base <baseSha[:7]> → head <headSha[:7]>`.
- Right: `<done>/<total> reviewed`, a thin progress bar, a `?` button, and
  `⚠ N warnings` when `plan.warnings` is non-empty (click opens the overlay
  listing them).
- Second row: two tabs, `Plan` and `Files`, bound to `store.view`.

### Sidebar (`sidebar.js`)

For each thread, in order:

- Header row: collapse arrow, `thread.title`, a risk badge (`HIGH`, `MED`,
  `LOW`; nothing for `none`), `thread.subtitle`, and `done/total`.
  Clicking the header toggles `collapsed`.
- Item rows (hidden when collapsed): a status glyph (`✓` reviewed, `●`
  selected, `○` otherwise), `item.title`, and a small role tag. Added items
  get class `added`, deleted items `deleted` (for colour).
  Clicking selects the item.

Threads of kind `mechanical` and `generated` start collapsed.

After selection changes, the selected row is scrolled into view with
`scrollIntoView({block: "nearest"})`.

### Item pane (`item.js`)

Top to bottom:

1. **Header**: `item.title`; role tag; risk badge.
2. **Meta line**: for symbols `<kind> · <status> · <path>:<declLine>`; for
   files `<status> · <path>` (and `renamed from <oldPath>`).
   `signature changed` tag when `item.signatureChanged`.
3. **Why here**: `item.roleReason`.
4. **Risk**: `Risk <score>: ` followed by `risk.reasons` joined with ` · `.
   Omitted when the level is `none`.
5. **Uses / Used by**: each link renders the target item's title as a button
   that selects that item; the link kind is shown in parentheses when it is
   `implements`, and `.Member` is appended when present. Omitted when empty.
6. **Diff** (`diff.js`).
7. **Blast** (`blast.js`), when `item.blast` is not null.
8. **Note**: a `<textarea>`; saved with `setNote` 600 ms after the last
   keystroke and on blur. A small `saved` indicator appears after success.
9. **Actions**: `Mark reviewed` / `Unmark` button, previous and next buttons.

If the item's symbol has `resolved: false`, show a notice above the diff:
`No type information for this file. Blast radius is not available.`

### Diff (`diff.js`)

For each slice: a file path header (only when the path differs from the
previous slice's path), then a table with one row per line:

| Column | Content |
|---|---|
| old number | `line.oldNo` or empty |
| new number | `line.newNo` or empty |
| marker | `+`, `-` or space |
| code | `line.text` |

Row classes: `add`, `del`, `context`. A line with `noEol` gets a following
muted row `\ No newline at end of file`.

Between two slices of the same file, render a separator row `⋯`.

A binary file item shows `Binary file` instead of a table. A file item with
no slices and not binary shows `No content changes`.

For symbol items with status `modified` or `added`, a toggle
`Show whole declaration` fetches
`getSource("head", symbol.file, symbol.startLine, symbol.endLine)` and renders
the full text below the slices, with lines that appear as `add` in any slice
highlighted. The result is cached per item ID.

### Blast panel (`blast.js`)

- Summary line: `<outsideCount> outside · <inPrCount> in PR · <testCount> tests`,
  then `· <transitiveCount> within 3 hops` (append `+` when
  `transitiveCapped`).
- Three sections, each omitted when empty: **Outside this PR**, **In this
  PR**, **Tests**. Inside a section, callers are grouped by `pkg` under a
  package sub-header.
- Caller row: `name`, `file:line`, kind (`call` or `ref`), `×N` when
  `sites > 1`, `via <Via>` when set, and the `snippet` in monospace.
- Clicking an in-PR caller selects that item (`caller.itemId`).
- Clicking any other caller expands context below the row:
  `getSource("head", file, max(1, line-5), line+5)`, with the caller's line
  highlighted. Clicking again collapses. Cached per `file:line`.
- When `truncated`, a last row: `Showing the first 200 callers.`
- When `outsideCount === 0` and the item is modified: a muted line
  `No callers outside this PR.`

The panel is shown or hidden by `store.blastOpen`.

### Files view (`files.js`)

The escape hatch: a flat table of `plan.files` (status, path, `+added`,
`-deleted`). Clicking a file switches to the plan view and selects the first
of its `itemIds`.

### Help overlay (`help.js`)

Lists the shortcuts, then a short "What Blaze cannot see" list taken from the
"Known limits" table in `04-dodge.md`, then any plan warnings. Closed with
`Esc` or a click outside.

## Routing

The selected item is mirrored in the URL fragment: `#/item/<id>`. On load and
on `hashchange`, select that item if it exists. With no fragment, select
`nextUnreviewed(null)`, or the first item when everything is reviewed.

## Keyboard (`keys.js`)

One `keydown` listener on `document`. Ignore events when the target is a
`textarea` or `input`, except `Esc`, which blurs it. Ignore events with
Ctrl, Alt or Meta held.

| Key | Action |
|---|---|
| `j` / `↓` | next item |
| `k` / `↑` | previous item |
| `]` | first item of the next thread |
| `[` | first item of the previous thread |
| `r` | toggle reviewed on the selected item; when marking, move to the next unreviewed item |
| `u` | jump to the next unreviewed item |
| `b` | show or hide the blast panel |
| `n` | focus the note box |
| `f` | switch between Plan and Files view |
| `?` | open the help overlay |
| `Esc` | close the overlay, or leave the note box |

## Start-up sequence (`main.js`)

1. `Promise.all([getPlan(), getState()])`.
2. Fill `store`, build `order`, collapse mechanical and generated threads.
3. Apply the route.
4. `subscribe(render)`; `render()`.
5. Install the key handler and the `hashchange` listener.

If start-up fails, show the error message in `#main` as plain text.

`document.title` is `"<done>/<total> · <target title> · Blaze"`.

## Testing

There is no JavaScript test runner in the MVP. Verification is manual against
the fixture, using the checklist in milestone 8 of `09-milestones.md`. The Go
server tests already cover the API contract the UI depends on.
