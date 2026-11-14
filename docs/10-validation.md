# 10 — Validation

The MVP is a bet on two claims:

1. A graph-derived order is close to the order an experienced reviewer would
   choose.
2. A type-resolved blast radius surfaces things the reviewer would otherwise
   miss or spend time finding.

Neither is proven by passing tests. This document says how to check them and
when to stop.

## Material

Pick **20 merged PRs** from Go repositories:

- 10 from a codebase you know well and reviewed yourself.
- 10 from a codebase you do not know (a mid-sized open-source Go project).

Spread the sizes: about a third under 100 changed lines, a third between 100
and 500, a third above 500. Skip PRs that are only dependency bumps or
generated code.

## Procedure, per PR

Do steps 1–2 **before** looking at Blaze's output.

1. Open the PR in GitHub's normal file view. Write down, in order, the first
   five things you would read (function or type names, not files).
2. Write down any place outside the diff you would want to check.
3. Run `blaze plan <pr> > plan.json` and note the wall-clock time.
4. Run `blaze review <pr>` and review the PR with it.
5. Fill one row of the results table.

## Results table

| Column | How to fill it |
|---|---|
| PR | link |
| Changed lines | from the plan stats |
| Analysis time (s) | step 3 |
| Order overlap | how many of your five are in the first five items of the plan, ignoring order (0–5) |
| First item right | is your number 1 among the plan's first two items? (y/n) |
| Thread sense | do the threads match the logical changes you see? (good / mixed / wrong) |
| Blast hits | outside callers shown that were on your step-2 list, or that you agree you should have checked |
| Blast noise | outside callers shown that were irrelevant (rough count) |
| Missed | anything on your step-2 list that Blaze did not show |
| Warnings | plan warnings, unresolved items |
| Would use again | y / n, with one sentence |

Keep the table in `docs/validation-results.md`.

## Pass criteria

Over the 20 PRs:

| Measure | Target |
|---|---|
| Mean order overlap | ≥ 3 of 5 |
| First item right | ≥ 14 of 20 |
| Thread sense "good" | ≥ 12 of 20, and "wrong" ≤ 3 |
| PRs with at least one blast hit, among the 10 unfamiliar ones | ≥ 5 |
| Median analysis time | ≤ 30 s |
| "Would use again" | ≥ 14 of 20 |

## What to do with the result

**Passes.** Continue with `11-later.md`, starting with whichever gap the
table shows most often.

**Fails on order or threads, passes on blast.** The heuristics are wrong, the
engine is fine. Tune and re-run on the same 20 PRs:

- Threads too large → lower `HubMinUsers`; consider ignoring `ref` links to
  types that are used by more than half of the items.
- Threads too fragmented → check the orphan rule; consider grouping
  single-item threads by package.
- Wrong first item → adjust the role ranks and the sort key in step 7 of
  `05-plan.md`.
- Wrong thread order → adjust the weights in `risk.go`.

Change one thing at a time and keep the previous table for comparison. Update
`05-plan.md` and the expected values in `08-fixture.md` with every change to
a rule.

**Fails on blast** (few hits, much noise, or important misses). Look at what
was missed. If it is dynamic dispatch the graph cannot see, that is a limit of
the approach; write it down. If it is noise from type references, restrict the
caller list to `call` refs by default and put `ref` behind a toggle.

**Fails on time.** Add the graph cache from `11-later.md` before anything
else.

**Fails on "would use again" after one round of tuning.** Stop. The premise
does not hold in this form. Write down why in `docs/validation-results.md`.
Do not add languages or a model to a tool you would not use yourself.

## What not to measure

- Bugs found. Sample size is far too small, and finding bugs is the
  reviewer's job, not Blaze's.
- Total review time against an unaided baseline. You cannot review the same
  PR twice without knowing it.
