# Metadata Commands

## collections

```text
pbl collections [--format line|ndjson]
```

Lists collection names.

Behind the scenes: collection listing comes from metadata, not by scanning data
keys.

## info

```text
pbl info [--format text|ndjson]
```

Shows pbl database metadata: path, storage format version, collection count, and
creation time when present.

NDJSON fields are `path`, `storage_format_version`, `collection_count`, and
`created_at`.

Behind the scenes: this reads pbl metadata records from the Pebble directory.

## stats

```text
pbl stats [--format text|ndjson] [--raw]
```

Shows storage metrics. NDJSON uses `path` and `disk_used`. `--raw` appends
Pebble's raw metric dump to text output or includes it as the NDJSON `raw` field.

Behind the scenes: the small text fields are pbl-owned. Raw Pebble metrics are
useful for debugging but may change with Pebble releases.
