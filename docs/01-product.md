# 01 — Product

## Problem

Automated AI reviewers are plentiful and their judgement is poor. A colleague's
review is still the one that is trusted. Two things slow that colleague down:

1. **The flat file list.** Review tools list changed files alphabetically.
   Nobody reviews in that order. A reviewer starts at the core of the change
   (a type, an interface, a schema), moves to the logic built on it, then to
   the callers, then to the tests, and jumps between them.
2. **Unknown blast radius.** In a codebase the reviewer does not know well,
   the diff shows what changed but not what depends on it. The risky part of a
   PR is often code that is *not* in the diff.

## What Blaze does

For one PR, Blaze produces a **review plan** and shows it in a local web page.

- Changes are split by **symbol**, not by file. A hunk that touches two
  functions becomes two items.
- Items that reference each other form a **thread**: one logical change, even
  when it spans several files and packages.
- Inside a thread, items are ordered **definitions before uses**: contracts,
  then logic, then wiring, then tests.
- Threads are ordered by **risk**, highest first. Mechanical changes
  (generated files, lockfiles, comment-only edits) are collapsed at the end.
- Every changed symbol shows its **blast radius**: who calls it, and which of
  those callers the PR did not touch.
- From any item the reviewer can jump to the changed items it uses and the
  changed items that use it.
- The reviewer marks items as reviewed. The marks survive a restart and
  survive new pushes to the PR for every item whose content did not change.

## Principles

1. **The human decides.** Blaze never says whether code is correct. It says
   where to look and why.
2. **Every ordering decision has a stated reason.** Each item carries the
   reason for its role and the factors behind its risk score, in plain text.
3. **Deterministic.** The same base and head always produce the same plan.
4. **Precise or absent.** A blast radius built from name matching is noise.
   Where type-resolved information is unavailable, Blaze shows the item
   without blast data and says so.
5. **Local.** Source code never leaves the machine. No network calls except
   the user's own `git fetch` and `gh`.

## User flow

```
$ cd ~/src/some-go-repo
$ blaze review 482            # PR number; or: blaze review --base main --head my-branch
blaze: resolving PR #482 …
blaze: analysing 214 packages …
blaze: plan ready: 4 threads, 23 items
blaze: http://127.0.0.1:53817/  (Ctrl-C to stop)
```

The browser opens. The reviewer:

1. Reads the thread list in the sidebar, highest risk first.
2. Opens the first item. Reads the diff for that symbol only.
3. Looks at the blast panel: outside callers, grouped by package.
4. Presses `r` to mark it reviewed, `j` to go to the next item.
5. Writes private notes per item where needed.
6. Leaves comments on the PR in GitHub as usual. Posting comments from Blaze
   is deferred (`11-later.md`).

## MVP scope

In scope:

- A Go module at the repository root, or in one subdirectory given by
  `--module-dir`.
- A review target given as two local refs, or as a GitHub PR number or URL.
- The plan, the web UI, the review marks and the notes.
- A `blaze plan` command that prints the plan as JSON, for tests and for the
  validation exercise.

Out of scope for the MVP (see `11-later.md`):

- Languages other than Go. Non-Go files are shown as whole-file items.
- Multi-module repositories and Go workspaces (`go.work`).
- Any language model.
- Posting comments to GitHub.
- Syntax highlighting.
- Reviewing uncommitted working-tree changes.
- Rename detection for symbols (a renamed function is one deleted item and
  one added item).
- Caching the graph between runs.

## Non-goals, permanently

- A hosted service, accounts, or teams.
- Replacing the code host's review page.
- Calling third-party model APIs.
