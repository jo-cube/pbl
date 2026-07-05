# Development Notes

`pbl` is intentionally a small Go CLI. The current implementation favors direct
packages over a broad internal framework.

## Package Shape

```text
cmd/pbl              binary entrypoint
internal/app         Cobra command tree, IO wiring, exit-code mapping
internal/buildinfo   version hook
internal/codec       raw, line, KV, and NDJSON edge codecs
internal/keyenc      physical Pebble key encoding and bounds
internal/store       Pebble-backed collection operations
tests/cli            functional use-case tests
tests/perf           opt-in volume and benchmark checks
```

`internal/keyenc` is the only package that should construct physical Pebble
keys.

## Design Principles

- Keep stdout clean; it is data.
- Keep stderr for errors and diagnostics.
- Stream stdin/stdout workflows with bounded memory.
- Batch imports and deletes.
- Use Pebble iterators for ordered reads.
- Keep values opaque in storage.
- Add flags only when behavior is worth documenting and testing.

## Normal Checks

```sh
go test ./...
```

Quick CLI checks:

```sh
go run ./cmd/pbl --help
go run ./cmd/pbl --version
```

## Functional Tests

The tests in `tests/cli` double as executable examples. They cover:

- KV import, scan, prefix, and range.
- Persistent set membership with `exists`.
- NDJSON import and `join`.
- Compound key prefix scans.
- `get-many` and `del-many`.

Keep these tests readable; they are a reference for future docs.

The user-facing workflow guide is [usage.md](usage.md). Prefer pointing to that
guide instead of growing the root README with command examples.

## Performance And Volume Checks

Volume smoke tests are opt-in:

```sh
PBL_PERF=1 go test ./tests/perf -run TestPerfVolumeKVImportScanLookup -count=1
```

Benchmark smoke:

```sh
go test ./tests/perf -run '^$' -bench . -benchtime=1x
```

These checks are not a full performance harness. They exist to catch obvious
streaming or batching regressions.

## Boundaries

Do not add these without a concrete product decision:

- SQL or a general query language.
- Arbitrary predicates or aggregations.
- Secondary indexes.
- Server, daemon, or remote mode.
- Stored collection to stored collection joins.
- RocksDB file compatibility.
- Shell completion or progress UI.
- Read-only Pebble open mode.
- Complex Pebble tuning.

## Source Plan

The original design pack lives outside the repository at:

```text
/Users/josh/Downloads/pebble_cli_plan
```

Use it as background, not as a second source of truth. Repository docs and tests
should describe the behavior that is actually implemented here.
