# Blaze — implementation plan

Blaze helps a **human** PR reviewer. It does not review code. It reorders a PR
into the order an experienced reviewer would read it, and shows the blast
radius of each change. Judgement stays with the person.

These documents are the complete specification for the MVP. They are written
so that an implementer can build each milestone without inventing behaviour.

## Reading order

| File | Contents |
|---|---|
| [01-product.md](01-product.md) | Problem, principles, user flow, scope, non-goals |
| [02-architecture.md](02-architecture.md) | Pipeline, package layout, dependencies, CLI, coding conventions |
| [03-git-and-diff.md](03-git-and-diff.md) | Review-target resolution, exact git/gh commands, diff parser |
| [04-dodge.md](04-dodge.md) | Dodge: the symbol graph engine (Go backend) |
| [05-plan.md](05-plan.md) | Review plan: attribution, roles, threads, ordering, blast radius, risk, JSON schema |
| [06-server.md](06-server.md) | Local HTTP server, API, review-state store, security rules |
| [07-web-ui.md](07-web-ui.md) | Browser UI: layout, modules, rendering, keyboard |
| [08-fixture.md](08-fixture.md) | Test fixture repository and the expected plan for it |
| [09-milestones.md](09-milestones.md) | Build order, one milestone per commit, acceptance criteria |
| [10-validation.md](10-validation.md) | How to decide whether the MVP works, and when to stop |
| [11-later.md](11-later.md) | Deferred work: more languages, local model, GitHub comments, caching |

Implement in the order given in `09-milestones.md`. Each milestone names the
documents it depends on.

## Decisions already made

These are settled. Do not reopen them while implementing.

| Decision | Choice | Reason |
|---|---|---|
| First language | Go only | Type-resolved references from the standard toolchain; the tool can be tested on itself |
| Surface | Local web page served by the `blaze` binary | Room for side-by-side diff, blast panel and navigation; no hosting |
| AI in the MVP | None | Ordering and blast radius are graph problems; deterministic output is explainable |
| Hosted AI APIs | Never | Any future model runs locally (see `11-later.md`) |
| Git access | Shell out to the `git` CLI | Behaviour identical to the user's terminal; no go-git |
| GitHub access | Shell out to the `gh` CLI | Reuses the user's existing authentication |
| Configuration | CLI flags only | No environment variables, no config file |
| Frontend | Plain HTML, CSS and ES modules, embedded in the binary | No Node toolchain, no build step |
| Per-PR UI | A fixed UI renders a generated **plan** (data) | Generating data is tractable; generating UI is not |

## Glossary

| Term | Meaning |
|---|---|
| Target | The thing under review: a base commit and a head commit, plus PR metadata if any |
| Base | The merge base of the base branch and the head commit. The "before" side |
| Head | The commit being reviewed. The "after" side |
| Symbol | One top-level Go declaration: a function, method, type, variable or constant |
| Ref | A reference from one symbol to another (a call, another use, or "implements") |
| Graph | All symbols and refs of the module at head |
| Dodge | The package that builds the graph |
| Item | The unit of review: one changed symbol, or one changed non-Go file |
| Slice | The part of a diff hunk that belongs to one item |
| Thread | An ordered group of items that form one logical change |
| Plan | The ordered list of threads for one target. The server serves it as JSON |
| Blast radius | The symbols that depend on a changed symbol, especially those the PR does not touch |
| Outside caller | A caller of a changed symbol that is itself not changed in the PR |

## What was verified before writing

A throwaway prototype was run against the fixture in `08-fixture.md` with
Go 1.26.0 and `golang.org/x/tools` v0.52.0. Statements in these documents
marked **(verified)** were observed in that run. Everything else is design and
must be covered by the tests named in `09-milestones.md`.
