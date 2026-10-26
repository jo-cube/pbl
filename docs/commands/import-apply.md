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
  [--ignore-duplicates|--fail-on-duplicate]
  [--sync|--no-sync]
```

Imports records from stdin. Existing values are replaced by default.

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
Keys and values are each limited to 64 MiB, independently, so a maximum-sized
raw value can be exported and restored with its key.

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
delete. Its probes stay within one 64-byte block. The filter uses about 1.375
bytes per expected key, rounded up to 64 bytes: `10M` needs about 13.75 MB and
`800M` about 1.1 GB. Size for the union of existing keys and distinct incoming
put keys, rather than the total record count. Repeated puts do not require
additional capacity. Underestimating the count reduces effectiveness but does
not change results. Deletes of previously added keys continue to pass the
filter even after those keys have been deleted from Pebble.
With `--stats`, `records` and `deletes` count accepted input operations after
filtering, before within-batch coalescing; filtered runs also report
`deletes_skipped`. The reported `duration` excludes database open, filter
construction and preload, and database close; use command wall time to measure
the complete operation. Batch limits and counts continue to describe these input
operations. Repeated keys within a batch may be reduced to their final operation.

Successful streaming bulk commands flush a remaining WAL tail larger than
4 MiB before closing, limiting replay work on the next invocation. This does not
fsync every batch or change which batches remain committed after an input error.
