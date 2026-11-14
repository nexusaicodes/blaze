# Blaze

Blaze helps a human review a pull request. It reorders the PR into the order
an experienced reviewer would read it and shows the blast radius of each
change. It runs locally as a single Go binary and serves a local web page; it
does not call any hosted AI service.

## Build

```sh
make build
```

## Run

```sh
./blaze review <pr>                      # review a GitHub PR
./blaze review --base main --head HEAD   # review a local range
./blaze plan --base main --head HEAD     # print the plan as JSON instead of serving it
./blaze version                          # print the version
```

See `docs/` for the full specification.
