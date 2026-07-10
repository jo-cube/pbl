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
  [--stats]
  [--sync|--no-sync]
```

Applies an ordered stream of puts and deletes. Success writes no stdout.

Flags:

- `--format`: `kcat` text records or binary-safe `frame` records.
- `--batch-size`: maximum records per write batch.
- `--batch-bytes`: approximate bytes per write batch.
- `--stats`: after success, write ingest stats to stderr.
- `--sync`: fsync every committed batch.
- `--no-sync`: explicitly keep the bulk-write default of not syncing each batch.

Behind the scenes: `apply` materializes the latest state from an ordered stream.
It batches writes and deletes, then commits each batch to Pebble. If input fails
after a batch commit, earlier batches remain stored.
