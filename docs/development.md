# Development Notes

`pbl` is intentionally a small Go CLI. The current implementation favors direct
packages over a broad internal framework.

## Package Shape

```text
cmd/pbl                binary entrypoint
internal/app           Cobra command tree, IO wiring, formatting, exit codes
internal/app/commands_* command families
internal/buildinfo     version hook
internal/codec         raw, line, KV, and NDJSON edge codecs
internal/keyenc        physical Pebble key encoding and bounds
internal/store         Pebble-backed collection operations
tests/cli              functional use-case tests
tests/perf             opt-in volume and benchmark checks
scripts/install.sh     checksum-verifying release installer
.github/workflows      CI and release packaging
Makefile               local build, test, run, install shortcuts
```

`internal/keyenc` is the only package that should construct physical Pebble
keys.

## Design Principles

- Keep stdout clean; it is data.
- Keep stderr for errors and diagnostics.
- Stream stdin/stdout workflows with bounded memory.
- Batch imports and deletes.
- Ensure collection metadata before committing batched puts.
- Use one bounded Pebble iterator for prefix/range reads, in either direction.
- Count and key-only scans skip value fetches. Keep counts exact without counters.
- Drop collection data and metadata atomically using a range deletion.
- Treat slices passed to streaming callbacks as views; copy only when retaining
  them after the callback returns.
- Keep values opaque in storage.
- Add flags only when behavior is worth documenting and testing.

## Normal Checks

```sh
make test
go vet ./...
go mod tidy -diff
```

## Optional Code Navigation

When a local `.codegraph/` index is present, `codegraph explore` can navigate
symbols and call paths before raw source searches. The index is generated,
git-ignored, and optional; creating it is a developer decision.

## Local Workflow

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

The tests in `tests/cli` double as executable examples. They cover the main
workflows:

- KV import and scans with prefix/range selection.
- Compacted put/delete stream apply.
- Persistent set membership with `exists`.
- NDJSON import and `join`.
- Compound key prefix scans and joins.
- `get-many` and `del-many`.

Contract tests in `internal/app` and `internal/store` cover initialization and
ownership, exit codes, output failures, duplicate policies, partial bulk writes,
metadata, binary-safe frame restore, bounded scans in both directions, exact
counts, and collection drop/recreate.

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

The scan benchmarks compare full records, keys only, the latest ten records, and
exact counts on 25,000 keys, including CLI database open/close costs. The volume
check also verifies exact counts and bounded reverse scans on 100,000 records.

Representative tombstone-heavy apply and saturated Bloom lookup benchmarks:

```sh
go test ./tests/perf -run '^$' -bench BenchmarkApplyTombstoneDominated -benchmem -benchtime=1x
go test ./internal/app -run '^$' -bench BenchmarkDeleteBloomLookup -benchmem -benchtime=1s -cpu 1,2,4,8
```

The apply benchmark uses one million Kcat records, 0.5% puts, and unique
never-seen tombstones in a fresh database. The Bloom benchmark preloads ten
million keys before measuring absent lookups against the saturated filter.

These checks are not a full performance harness. They exist to catch obvious
streaming or batching regressions.

Focused JSON and payload workloads:

```sh
go test ./tests/perf -run '^$' -bench 'BenchmarkNDJSON|BenchmarkApplyPayload' -benchmem -benchtime=5x -count=3
```

The JSON benchmark processes 2,500 records through import, join, and keyed
export, with small objects, nested payloads, and nested lookup keys. The payload
benchmark applies 5,000 compressible 4 KiB values through Kcat and frame formats.
Inputs and read-database setup are outside the timer; measured CLI calls include
database open/close, and writes use fresh databases. Compare allocation counts
as well as elapsed time: small workloads can be dominated by filesystem costs.
These warm local benchmarks do not represent cold-disk or incompressible loads.

NDJSON records keep fields as `json.RawMessage`, decoding only key paths into
objects and strings. Wrapping and joining validate JSON without materializing
nested values or converting their numbers to floating point. Kcat readers reuse
64 KiB of key scratch space; larger keys allocate separately so a single large
key is not retained for the rest of the stream. Kcat callback slices, like other
streaming views, must be copied if retained after the callback returns.

The CLI buffers stdout once at the application edge. Pebble SSTables use
10-bit-per-key Bloom filters for point lookups; this is separate from the
in-memory `apply --bloom-filter`, which skips absent deletes.

## Release Packaging

GitHub Actions builds and publishes release artifacts:

- CI runs `make test` and `make build` on pushes to `main` and pull requests.
- CI also runs `go vet ./...` and `go mod tidy -diff`.
- CI cross-builds `pbl` for `linux/amd64`, `linux/arm64`, and `darwin/arm64`.
- Pushing a `v*` tag publishes tarball release assets named
  `pbl_<goos>_<goarch>.tar.gz`.
- Release jobs also publish `.sha256` files, and `scripts/install.sh` verifies
  them before installing.

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
