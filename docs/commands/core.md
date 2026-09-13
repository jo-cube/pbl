# Core Commands

## init

```text
pbl init [--if-not-exists]
```

Creates the Pebble directory if needed and writes pbl storage metadata. Success
writes no stdout. Running `init` on an initialized database is an error.

Flags:

- `--if-not-exists`: return success if the database is already initialized and
  compatible.

Behind the scenes: metadata records define the storage format version. pbl uses
that version check before normal reads and writes.

## put

```text
pbl put <collection> <key> <value> [--sync|--no-sync]
pbl put <collection> <key> --stdin [--sync|--no-sync]
```

Stores or replaces one value. Without `--stdin`, `<value>` is the third
argument. With `--stdin`, pbl reads all stdin bytes, including newlines, as the
value.

Flags:

- `--stdin`: read the value from stdin instead of argv.
- `--sync`: fsync before returning.
- `--no-sync`: skip fsync for faster writes with weaker crash durability.

Behind the scenes: `put` initializes the database if needed and writes collection
metadata before storing the key.

## get

```text
pbl get <collection> <key>
  [--format raw|kv|ndjson]
  [--with-key]
  [--missing error|skip|null]
  [--no-newline]
```

Reads one value. Default output is raw value bytes plus a newline.

Flags:

- `--format`: choose raw value, `key<TAB>value`, or NDJSON output.
- `--with-key`: wrap NDJSON output with `_key` and `_value`; requires NDJSON
  format. KV output always includes the key.
- `--missing`: choose whether missing keys exit 2, emit nothing, or emit null.
- `--no-newline`: suppress the added newline for raw output.

Point keys must be non-empty.

Behind the scenes: read commands require the database directory to exist. Missing
null output is shaped to the selected format.

## del

```text
pbl del <collection> <key> [--fail-missing] [--sync|--no-sync]
```

Deletes one key. Missing keys are success by default so deletes are idempotent in
scripts.

Flags:

- `--fail-missing`: exit 2 if the key is absent.
- `--sync`: fsync before returning.
- `--no-sync`: skip fsync for faster writes with weaker crash durability.

Behind the scenes: `--fail-missing` checks existence before deleting. Without it,
Pebble deletion is used directly.

## drop

```text
pbl drop <collection> [--sync|--no-sync]
```

Deletes every record and the collection metadata. Success writes no stdout.
An absent collection is success, so repeated drops are idempotent. The database
must exist. A later put, import, or apply can recreate the collection.

The operation syncs by default. `--no-sync` skips fsync.

Behind the scenes: a single Pebble batch deletes the collection key range and
its metadata atomically. No per-key scan or delete list is needed. Other
collections and database metadata are preserved. Pebble compaction reclaims disk
space later; drop does not force a compaction.
