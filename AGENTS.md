# Agent Notes

This repository is `pbl`, a Go CLI backed by CockroachDB Pebble. Keep it small,
plain, and script-friendly.

## Product Shape

- `pbl` is a local ordered key-value/indexing tool, not a database shell.
- stdout is data. stderr is diagnostics.
- Stream commands must preserve input order.
- Values are opaque bytes; `kv`, `line`, `raw`, and `ndjson` are edge formats.
- One Pebble DB directory stores all logical collections.
- Storage format v1 is part of the compatibility contract.

Read these docs before changing behavior:

- [README.md](README.md) for the user-facing overview.
- [docs/usage.md](docs/usage.md) for common workflows.
- [docs/cli.md](docs/cli.md) for command behavior and exit codes.
- [docs/storage-format.md](docs/storage-format.md) for physical key encoding.
- [docs/development.md](docs/development.md) for package shape, tests, and
  current boundaries.

## Code Style

- Prefer boring Go and the standard library.
- Keep packages moderately sized and readable.
- Add dependencies only when they remove real complexity.
- Avoid speculative abstractions, empty scaffolding, and framework sprawl.
- Keep comments rare; use them for storage decisions and edge cases.
- Tests should freeze behavior, not chase coverage numbers.

## Storage Rules

- Pebble key construction belongs in `internal/keyenc`.
- No other package should hand-build physical Pebble keys.
- Collection listing must come from metadata.
- Copy values returned by Pebble before closing their closer.
- Close databases, batches, iterators, and buffered writers reliably.

## Checks

Normal check:

```sh
go test ./...
```

Optional volume smoke:

```sh
PBL_PERF=1 go test ./tests/perf -run TestPerfVolumeKVImportScanLookup -count=1
```

Optional benchmark smoke:

```sh
go test ./tests/perf -run '^$' -bench . -benchtime=1x
```

## graphify

This project may have a local knowledge graph at `graphify-out/`. The directory
is generated and git-ignored, so fresh clones will not have it until an agent or
developer builds it locally.

Rules:

- For codebase questions, first run `graphify query "<question>"` when
  `graphify-out/graph.json` exists.
- If `graphify-out/graph.json` is missing and graphify is available, run
  `graphify .` before relying on graph queries.
- Use `graphify path "<A>" "<B>"` for relationships and
  `graphify explain "<concept>"` for focused concepts.
- Dirty graph files are expected after hooks or incremental updates; they are
  not a reason to skip graphify.
- If `graphify-out/wiki/index.md` exists, use it for broad navigation instead
  of raw source browsing.
- Read `graphify-out/GRAPH_REPORT.md` only for broad architecture review or
  when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current.
