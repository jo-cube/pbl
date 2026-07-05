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
scripts/install.sh   release asset installer
.github/workflows    CI and release packaging
Makefile             local build, test, run, install shortcuts
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
make test
```

Quick CLI checks:

```sh
make run ARGS='--help'
make run ARGS='--version'
```

Build into `./bin`:

```sh
make build
```

Install from source into `~/.local/bin`:

```sh
make install
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

## Release Packaging

GitHub Actions follows the same release shape as `toolbox`:

- CI runs `make test` and `make build` on pushes to `main` and pull requests.
- CI cross-builds `pbl` for `linux/amd64`, `linux/arm64`, and `darwin/arm64`.
- Pushing a `v*` tag publishes tarball release assets named
  `pbl_<goos>_<goarch>.tar.gz`.
- `scripts/install.sh` downloads those release assets from GitHub releases.

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
