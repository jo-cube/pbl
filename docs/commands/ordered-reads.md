# Ordered Reads

## scan

```text
pbl scan <collection>
  [--prefix <prefix>] [--start <start>] [--end <end>]
  [--reverse] [--limit <n>]
  [--format kv|ndjson|raw|frame]
  [--keys-only|--values-only] [--with-key]
```

Emits records in raw key-byte order, ascending by default.

Selection flags compose by intersection:

- `--prefix`: include keys beginning with these bytes; an empty prefix matches all.
- `--start`: inclusive lower bound, compared against the complete key.
- `--end`: exclusive upper bound, compared against the complete key.
- Omit either bound to leave that side open. `--end ''` selects no keys.
- Equal bounds or a disjoint prefix/range produce no records. A start greater
  than the end is a usage error.

`--reverse` emits descending keys within the same selection. `--limit` caps the
number of emitted records in the chosen direction; `0` means no limit. An absent
collection emits nothing; the database must exist.

Output choices:

- Default `kv`: `key<TAB>value` followed by a newline.
- `--keys-only` or `--values-only`: one key or value per line, using `kv` format.
- `--format ndjson`: validate stored JSON and emit one JSON value per line.
- `--format ndjson --with-key`: wrap each value as `{"_key":...,"_value":...}`.
  Numeric literals retain their precision; object member order is not a contract.
- `--format raw`: concatenate value bytes without separators or added newlines.
- `--format frame`: binary-safe put records containing both key and value.

`--keys-only` and `--values-only` cannot be combined with another format.
`--with-key` requires NDJSON. Use frame format for arbitrary bytes; line and KV
output do not escape embedded tabs or newlines.

Behind the scenes: selection becomes one bounded Pebble iterator. Reverse scans
start at its high bound and limits stop iteration early. Key-only scans do not
fetch values.

## count

```text
pbl count <collection> [--prefix <prefix>] [--start <start>] [--end <end>]
```

Prints the exact number of matching live keys and a newline. Uses the same
selection rules as `scan`. An empty or absent collection prints `0`; the database
must exist. Embedded newlines in keys do not affect the count.

Behind the scenes: count visits matching keys without fetching values. It uses
bounded memory and time proportional to the matching key scan; counts are not
stored or estimated.

## Examples

```sh
# The latest ten events for one user, assuming sortable timestamps in the keys.
pbl scan events --prefix 'u123:' --reverse --limit 10 --format ndjson

# Events for one user from January onward.
pbl scan events --prefix 'u123:' --start 'u123:2026-01-01'

# Count events before February.
pbl count events --prefix 'u123:' --end 'u123:2026-02-01'

# Lossless export and restore, including binary keys and values.
pbl scan artifacts --format frame > artifacts.frame
pbl apply restored --format frame < artifacts.frame
```
