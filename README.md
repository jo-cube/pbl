# pbl

`pbl` is a local, Pebble-backed key-value CLI for shell workflows.

It gives scripts a persistent ordered store without asking the user to run a
server. Collections are logical keyspaces. Keys are ordered bytes. Values are
opaque bytes. Text, KV, raw, and NDJSON support are import/export conventions at
the edge.

The project is small on purpose. It is meant to feel like a UNIX tool: stdout is
data, stderr is diagnostics, stream commands preserve input order, and exit
codes are stable enough for scripts.

## Why It Exists

Local data workflows often need a fast lookup table, a persistent set, a cache,
or an ordered index between pipeline steps. `pbl` aims to cover that space with a
single Go binary backed by Pebble.

It is useful for tasks like:

- importing IDs or objects once and looking them up many times;
- filtering streams by persistent set membership;
- enriching NDJSON events from a stored collection;
- scanning compound keys by prefix or half-open range;
- keeping small local cache/artifact stores by key.

## Current Shape

The first useful version supports:

- ordered key/value storage in named collections;
- `put`, `get`, `del`, `scan`, `prefix`, and `range`;
- collection metadata, `info`, and `stats`;
- `kv`, `line`, `raw`, and `ndjson` edge formats;
- batched import/export;
- ordered put/delete stream apply for compacted feeds;
- streaming `get-many`, `del-many`, `exists`, `lookup`, and `join`;
- storage format v1 over one Pebble database directory.

It is not trying to be SQL, a query language, a daemon, a remote database, a
secondary-index system, or a RocksDB compatibility layer.

## Install

Install the latest release:

```sh
./scripts/install.sh
```

Install to a custom directory:

```sh
./scripts/install.sh "$HOME/bin"
```

Install a specific release version:

```sh
VERSION=v0.1.0 ./scripts/install.sh
```

Install without cloning the repository:

```sh
curl -fsSL https://raw.githubusercontent.com/jo-cube/pbl/main/scripts/install.sh | sh
```

Release binaries are published for:

- `linux/amd64`
- `linux/arm64`
- `darwin/arm64`

Release asset names follow this pattern:

```text
pbl_linux_amd64.tar.gz
pbl_linux_arm64.tar.gz
pbl_darwin_arm64.tar.gz
```

## Documentation

- [Usage guide](docs/usage.md): quickstart and common workflows.
- [CLI reference](docs/cli.md): commands, flags, formats, and exit codes.
- [Storage format](docs/storage-format.md): physical key layout and
  compatibility rules.
- [Development notes](docs/development.md): package shape, checks, tests, and
  boundaries.
- [Agent notes](AGENTS.md): repository-specific instructions for coding agents.

## Development

Normal check:

```sh
make test
```

Quick CLI checks:

```sh
make run ARGS='--help'
make run ARGS='--version'
```

The functional tests in [tests/cli](tests/cli) are written as executable
examples. The opt-in volume checks in [tests/perf](tests/perf) exist to catch
obvious streaming and batching regressions.
