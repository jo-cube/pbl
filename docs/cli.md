# CLI Reference

`pbl` follows a narrow rule: data goes to stdout, diagnostics go to stderr.
Commands are meant to compose with pipes and shell redirection.

```text
pbl [global flags] <command> [command flags] [args]
```

## Global Flags

```text
-d, --db <path>   database directory
--quiet           suppress diagnostics
--help            print help
--version         print version
```

Database path resolution:

1. `--db` / `-d`
2. `PBL_DB`
3. `.pbl`

Write commands that create records initialize the database if needed. Read,
delete, metadata, and stream lookup commands require the database directory to
exist.

## Exit Codes

```text
0 success
1 general runtime error
2 not found / no required match
3 bad usage
4 bad input data
5 storage/open/lock error
6 partial streaming failure
```

## Formats

`raw` is uninterpreted bytes.

`line` is one record per input line.

`kv` is one record per line, split on the first tab:

```text
key<TAB>value
```

`ndjson` is one JSON value per line. Repeated `--key-field` flags build a
compound key joined with `--key-sep`, which defaults to `:`.

## Core Commands

```text
pbl init [--if-not-exists]
```

Creates the database and writes storage metadata. Success writes no stdout.

```text
pbl put <collection> <key> <value> [--stdin] [--sync|--no-sync]
```

Stores or replaces a value. `--stdin` reads the value bytes from stdin.

```text
pbl get <collection> <key>
  [--format raw|kv|ndjson]
  [--with-key]
  [--missing error|skip|null]
  [--no-newline]
```

Default output is the raw value followed by a newline. A missing key exits `2`
unless `--missing skip` or `--missing null` is selected.

```text
pbl del <collection> <key> [--fail-missing] [--sync|--no-sync]
```

Deletes a key. Missing keys are success by default.

## Ordered Reads

```text
pbl scan <collection>
pbl prefix <collection> <prefix>
pbl range <collection> <start> <end>
```

Range scans are half-open:

```text
start <= key < end
```

Shared flags:

```text
--format kv|ndjson|raw
--limit <n>
--keys-only
--values-only
--include-key
```

Default output is ordered `key<TAB>value` records.
`--format raw` requires `--values-only`.

Convenience aliases:

```text
pbl keys <collection> [--prefix <p>] [--range-start <s>] [--range-end <e>] [--limit <n>]
pbl values <collection> [--prefix <p>] [--range-start <s>] [--range-end <e>] [--limit <n>]
```

## Import And Export

```text
pbl import <collection> --format kv|line|ndjson|raw
```

Useful flags:

```text
--key-mode value|line-number
--key <key>
--key-field <field>
--key-sep <sep>
--batch-size <n>
--batch-bytes <size>
--replace
--ignore-duplicates
--fail-on-duplicate
--sync|--no-sync
```

Examples:

```sh
printf 'u1\tAda\nu2\tGrace\n' | pbl import users --format kv
cat blocked.txt | pbl import blocked --format line --key-mode value
cat users.ndjson | pbl import users --format ndjson --key-field id
```

`raw` import stores stdin under `--key`.
Imports replace existing keys by default. `--replace` is accepted for explicit
default behavior. `--ignore-duplicates` keeps the first value already present or
seen in the current input stream. `--fail-on-duplicate` exits with bad input for
either existing keys or duplicate keys in the same stream.

```text
pbl export <collection>
```

Export uses the same scan flags.

```text
pbl apply <collection> --format kcat|frame
```

Applies an ordered stream of puts and deletes. Success writes no stdout.

Useful flags:

```text
--batch-size <n>
--batch-bytes <size>
--stats
--sync|--no-sync
```

`kcat` format is:

```text
key<TAB>payload-length<TAB>payload
```

`payload-length` greater than zero stores that many payload bytes. `0` stores an
empty value. `-1` deletes the key. Each record ends with a newline after the
payload.

Example compacted topic import:

```sh
kcat -C -b "$BROKERS" -t "$TOPIC" -o beginning -e -f '%k\t%S\t%s\n' \
  | pbl apply users --format kcat --batch-size 5000 --batch-bytes 32MB
```

`frame` format is binary-safe:

```text
P <key-bytes> <value-bytes>\n<key><value>
D <key-bytes>\n<key>
```

## Stream Commands

```text
pbl get-many <collection>
  [--input-format line|ndjson]
  [--key-field <field>]
  [--format raw|kv|ndjson]
  [--with-key]
  [--missing skip|null|error]
```

Reads lookup keys from stdin and emits results in input order.

```text
pbl del-many <collection>
  [--input-format line|ndjson]
  [--key-field <field>]
  [--batch-size <n>]
  [--batch-bytes <size>]
  [--sync|--no-sync]
```

Deletes keys from stdin in batches. Success writes no stdout.

```text
pbl exists <collection>
  [--input-format line|ndjson]
  [--key-field <field>]
  [--invert]
  [--missing skip|error]
```

Filters stdin by collection membership and emits the original input records.

```text
pbl lookup <collection>
  [--input-format line|ndjson]
  [--key-field <field>]
  [--key-sep <sep>]
  [--as <field>]
  [--missing null|skip|error]
```

Line mode emits stored values. NDJSON mode can attach the stored JSON value
under `--as`. Stored values must be valid JSON when attached to NDJSON input.

```text
pbl join <collection> --on <field> --as <field>
```

`join` is the NDJSON-friendly form of `lookup`.

## Metadata Commands

```text
pbl collections [--format line|ndjson]
pbl info [--format text|ndjson]
pbl stats [--format text|ndjson] [--raw]
```

Collection listing comes from metadata, not from data-key ordering.
