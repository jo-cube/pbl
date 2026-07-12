# Import And Apply

## import

```text
pbl import <collection> --format kv|line|ndjson|raw
  [--key-mode value|line-number]
  [--key <key>]
  [--key-field <field>]
  [--key-sep <sep>]
  [--batch-size <n>]
  [--batch-bytes <size>]
  [--replace|--ignore-duplicates|--fail-on-duplicate]
  [--sync|--no-sync]
```

Imports records from stdin.

Flags:

- `--format`: required input format.
- `--key-mode`: for `line` input, use the line value or 1-based line number as
  the key.
- `--key`: required key for `raw` import.
- `--key-field`: NDJSON field used as the key; repeat for compound keys.
- `--key-sep`: one-byte separator between compound key parts; parts may not
  contain it.
- `--batch-size`: maximum records per write batch.
- `--batch-bytes`: approximate bytes per write batch, accepting plain numbers,
  `K`, `KB`, `M`, or `MB`.
- `--replace`: replace existing values; this is the default.
- `--ignore-duplicates`: keep the first existing or input value for each key.
- `--fail-on-duplicate`: exit 4 on existing or repeated input keys.
- `--sync`: fsync every committed batch.
- `--no-sync`: explicitly keep the bulk-write default of not syncing each batch.

Behind the scenes: import initializes the database if needed, ensures collection
metadata exists, batches writes, and rejects empty user keys. NDJSON key fields
must be strings. If input fails after a batch commit, earlier batches remain
stored.

## apply

```text
pbl apply <collection> --format kcat|frame
  [--batch-size <n>]
  [--batch-bytes <size>]
  [--bloom-filter --expected-key-count <count>]
  [--stats]
  [--sync|--no-sync]
```

Applies an ordered stream of puts and deletes. Success writes no stdout.

Flags:

- `--format`: `kcat` text records or binary-safe `frame` records.
- `--batch-size`: maximum records per write batch.
- `--batch-bytes`: approximate bytes per write batch.
- `--bloom-filter`: skip deletes for keys definitely absent from the
  collection.
- `--expected-key-count`: expected number of distinct stored or incoming put keys;
  required with `--bloom-filter`. Counts accept plain numbers or decimal `K`,
  `M`, and `B` suffixes.
- `--stats`: after success, write ingest stats to stderr.
- `--sync`: fsync every committed batch.
- `--no-sync`: explicitly keep the bulk-write default of not syncing each batch.

Behind the scenes: `apply` materializes the latest state from an ordered stream.
It batches writes and deletes, then commits each batch to Pebble. If input fails
after a batch commit, earlier batches remain stored.

The optional Bloom filter first scans the collection's live keys without
copying values, then adds every incoming put. It never removes keys. A negative
lookup safely skips a delete; a false positive only writes an unnecessary
delete. The filter uses about 1.25 bytes per expected key, so `800M` needs about
1 GB. Size for the union of existing keys and distinct incoming put keys.
Underestimating the count reduces effectiveness but does not change results.
With `--stats`, `records` and `deletes` count operations sent to Pebble, while
filtered runs also report `deletes_skipped`.
