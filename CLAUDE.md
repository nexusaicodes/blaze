# CLAUDE.md

Blaze is a local tool that helps a human review a pull request. It reorders
the PR into the order an experienced reviewer would read it and shows the
blast radius of each change. It is a Go binary that serves a local web page.

No code exists yet. The full specification is in `docs/`. Start at
`docs/README.md` and implement the milestones in `docs/09-milestones.md` in
order.

## Rules

- Implement what the documents specify. Where they are silent, choose the
  simplest behaviour and say so in the commit message. Do not add features,
  flags or dependencies that are not listed.
- If a document turns out to be wrong, fix the document in the same commit as
  the code.
- Go only shells out to `git` and `gh` (`exec.CommandContext`, argument
  slices, never a shell). No go-git.
- Configuration is CLI flags only. No environment variables, no config file.
- No hosted AI APIs, and no language model at all in the MVP.
- The web UI is plain HTML, CSS and ES modules with no build step, and never
  uses `innerHTML`.
- The server listens on 127.0.0.1 only.
- Dependencies are limited to those in `docs/02-architecture.md`.
- Coding conventions (errors, logging, interfaces, tests) are in
  `docs/02-architecture.md`.

## Commands

Available after milestone M0:

```sh
make build     # go build -o blaze ./cmd/blaze
make test      # go test -race ./...
make lint      # go vet ./...
```
