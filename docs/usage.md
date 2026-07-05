# Usage Guide

This guide shows the common `pbl` workflows. See [cli.md](cli.md) for the full
command reference.

## Quickstart

```sh
go run ./cmd/pbl init

go run ./cmd/pbl put users u1 '{"id":"u1","name":"Ada"}'
go run ./cmd/pbl get users u1

printf 'u2\tGrace\nu1\tAda\n' \
  | go run ./cmd/pbl import names --format kv

go run ./cmd/pbl scan names
```

Use a different database directory with `--db` or `PBL_DB`:

```sh
go run ./cmd/pbl --db ./local.pbl init
PBL_DB=./local.pbl go run ./cmd/pbl collections
```

Commands that write records create the database if needed. Read and lookup
commands expect the database directory to already exist.

## Persistent Set

Store one line per key:

```sh
cat blocked_ids.txt | pbl import blocked --format line --key-mode value
```

Keep only blocked IDs:

```sh
cat incoming_ids.txt | pbl exists blocked
```

Keep only IDs that are not blocked:

```sh
cat incoming_ids.txt | pbl exists blocked --invert
```

## Lookup Table

Import NDJSON keyed by `id`:

```sh
cat users.ndjson | pbl import users --format ndjson --key-field id
```

Join event records against that collection:

```sh
cat events.ndjson | pbl join users --on user_id --as user
```

Missing joins attach `null` by default. Use `--missing skip` for inner-join
style behavior. Stored values must be valid JSON for NDJSON joins.

## Ordered Index

Use repeated `--key-field` flags to build compound keys:

```sh
cat events.ndjson \
  | pbl import events --format ndjson --key-field user_id --key-field ts
```

Scan one user's events:

```sh
pbl prefix events 'u123:'
```

Scan a half-open key range:

```sh
pbl range events 'u123:2026-01-01' 'u123:2026-02-01'
```

## Batch Retrieval

Read keys from stdin and emit matching values in the same order:

```sh
cat ids.txt | pbl get-many users
```

Emit key/value records instead:

```sh
cat ids.txt | pbl get-many users --format kv
```

## Command Map

Core:

```text
pbl init
pbl put <collection> <key> <value>
pbl get <collection> <key>
pbl del <collection> <key>
```

Ordered reads:

```text
pbl scan <collection>
pbl prefix <collection> <prefix>
pbl range <collection> <start> <end>
pbl keys <collection>
pbl values <collection>
```

Streams:

```text
pbl import <collection> --format kv|line|ndjson|raw
pbl export <collection>
pbl get-many <collection>
pbl del-many <collection>
pbl exists <collection>
pbl lookup <collection>
pbl join <collection>
```

Metadata:

```text
pbl collections
pbl info
pbl stats
```
