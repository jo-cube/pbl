# Usage Guide

This guide shows the workflows `pbl` is meant to make easy: local lookups,
persistent sets, NDJSON joins, ordered scans, and compacted stream materializing.
See [cli.md](cli.md) for the CLI contract and [commands/](commands/) for every
command flag.

## Database Path

Use `--db` for one command or `PBL_DB` for a shell session:

```sh
export PBL_DB=./work.pbl
pbl init
pbl info
```

Write commands that create records initialize the database when needed. Read,
delete, metadata, and lookup commands require the database directory to already
exist.

## Key/Value Table

Import tab-separated rows:

```sh
printf 'u2\tGrace\nu1\tAda\n' | pbl import names --format kv
```

Scan returns records ordered by key:

```sh
pbl scan names
```

```text
u1	Ada
u2	Grace
```

Read one key:

```sh
pbl get names u1
```

Read many keys in input order. Missing keys are skipped by default:

```sh
printf 'u2\nmissing\nu1\n' | pbl get-many names
```

```text
Grace
Ada
```

Use `--missing null` when downstream tools need one output record per input key.

## Persistent Set

Store each line as both key and value:

```sh
cat blocked_ids.txt | pbl import blocked --format line --key-mode value
```

Keep only records that exist in the set:

```sh
cat incoming_ids.txt | pbl exists blocked
```

Keep only records that are not in the set:

```sh
cat incoming_ids.txt | pbl exists blocked --invert
```

## NDJSON Lookup And Join

Import JSON objects keyed by a field:

```sh
cat users.ndjson | pbl import users --format ndjson --key-field id
```

Join JSON events with stored user objects:

```sh
cat events.ndjson | pbl join users --on user_id --as user
```

Missing joins attach `null` by default. Use `--missing skip` for inner-join
behavior or `--missing error` to fail if a lookup is missing.

Stored values must be valid JSON when they are attached to NDJSON input.

## Compound Ordered Keys

Repeated `--key-field` flags build one key with `--key-sep` between parts. The
default separator is `:`.

```sh
cat events.ndjson \
  | pbl import events --format ndjson --key-field user_id --key-field ts
```

Scan one user's prefix:

```sh
pbl prefix events 'u123:'
```

Scan a half-open range:

```sh
pbl range events 'u123:2026-01-01' 'u123:2026-02-01'
```

Convenience aliases:

```sh
pbl keys events --prefix 'u123:'
pbl values events --range-start 'u123:2026-01-01' --range-end 'u123:2026-02-01'
```

`keys` and `values` require both range flags when using a range.

## Raw Values

Use `raw` for one opaque value under one key:

```sh
cat payload.bin | pbl import artifacts --format raw --key build.tar
pbl get artifacts build.tar --no-newline > build.tar
```

`raw` import and `put --stdin` read one complete value from stdin.

## Compacted Streams

Apply a compacted Kafka topic through `kcat`:

```sh
kcat -C -b "$BROKERS" -t "$TOPIC" -o beginning -e -f '%k\t%S\t%s\n' \
  | pbl apply users --format kcat --batch-size 5000 --batch-bytes 32MB
```

Payload length `-1` deletes the key. Payload length `0` stores an empty value.

For binary-safe producers, emit frame records instead:

```text
P <key-bytes> <value-bytes>\n<key><value>
D <key-bytes>\n<key>
```

## Useful Commands

```text
pbl collections
pbl info
pbl stats
pbl export <collection>
pbl del <collection> <key>
pbl del-many <collection>
```
