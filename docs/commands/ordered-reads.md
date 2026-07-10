# Ordered Reads

## scan

```text
pbl scan <collection>
  [--format kv|ndjson|raw|frame]
  [--limit <n>]
  [--keys-only|--values-only]
  [--include-key]
```

Emits all records in raw key-byte order. Default output is `key<TAB>value`.

Flags:

- `--format`: choose `kv`, `ndjson`, raw values, or binary-safe `frame` records.
- `--limit`: maximum records to emit; `0` means no limit.
- `--keys-only`: emit only keys.
- `--values-only`: emit only values.
- `--include-key`: include `_key` beside `_value` in NDJSON output.

`frame` emits binary-safe put records containing both key and value. It cannot
be combined with output-shaping flags.

Behind the scenes: pbl scans only the selected collection keyspace inside the
shared Pebble directory.

## prefix

```text
pbl prefix <collection> <prefix> [scan flags]
```

Emits records whose keys start with `<prefix>`, in raw key-byte order.

Behind the scenes: pbl turns the prefix into a bounded Pebble iterator range.

## range

```text
pbl range <collection> <start> <end> [scan flags]
```

Emits a half-open range:

```text
start <= key < end
```

Behind the scenes: this is an ordered scan over collection data keys from start
inclusive to end exclusive.

## keys

```text
pbl keys <collection>
  [--prefix <p>]
  [--range-start <s> --range-end <e>]
  [--limit <n>]
```

Emits keys, one per line. Without filters, it scans the full collection. Range
flags must be used together.

Behind the scenes: `keys` uses the same scan, prefix, and range paths as
`scan`, then prints only keys.

## values

```text
pbl values <collection>
  [--prefix <p>]
  [--range-start <s> --range-end <e>]
  [--limit <n>]
```

Emits values, one per line, in key order. Range flags must be used together.

Behind the scenes: `values` uses the same scan, prefix, and range paths as
`scan`, then prints only values.

## export

```text
pbl export <collection> [scan flags]
pbl export <collection> --format frame
```

Exports records with the same flags and behavior as `scan`.

Behind the scenes: `export` is the `scan` path with a clearer name for backup or
pipeline use. Frame output is lossless for arbitrary key and value bytes and is
accepted by `apply --format frame`.
