# Command Manual

This directory holds the expanded command reference for `pbl`.

- [Core commands](core.md): `init`, `put`, `get`, `del`
- [Ordered reads](ordered-reads.md): `scan`, `prefix`, `range`, `keys`,
  `values`, `export`
- [Import and apply](import-apply.md): `import`, `apply`
- [Stream commands](streams.md): `get-many`, `del-many`, `exists`, `lookup`,
  `join`
- [Metadata commands](metadata.md): `collections`, `info`, `stats`

For common workflows, see [../usage.md](../usage.md). For the compact CLI
contract, global flags, formats, and exit codes, see [../cli.md](../cli.md).

## Shared Rules

- stdout is data. Diagnostics, errors, and optional ingest stats go to stderr.
- Values are opaque bytes. Formats only describe CLI input and output.
- Keys are ordered by raw bytes.
- Stream commands preserve input order.
- One Pebble directory stores every logical collection.
- `--db` overrides `PBL_DB`; if neither is set, `.pbl` is used.
- Single-key writes sync by default. Bulk commands (`import`, `apply`, and
  `del-many`) do not sync each batch unless `--sync` is set.
- `--limit 0` means no limit.

## Formats

`raw` is a value without a key wrapper. `get` adds a newline unless
`--no-newline` is set. `scan --format raw` requires `--values-only`.

`line` is one input record per line.

`kv` is one input or output record per line:

```text
key<TAB>value
```

`ndjson` is one JSON object per line. Repeated `--key-field` flags build a
compound key joined with `--key-sep`, which defaults to `:`.
