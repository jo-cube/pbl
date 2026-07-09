# CLI Reference

`pbl` follows a narrow rule: data goes to stdout, diagnostics go to stderr.
Commands are meant to compose with pipes and shell redirection.

For expanded command behavior, parameters, and behind-the-scenes notes, see the
[command manual](commands/README.md).

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
6 partial failure after stdout output began
```

## Durability

Single-key writes sync by default. Bulk commands (`import`, `apply`, and
`del-many`) batch records and do not sync each batch unless `--sync` is set.
Use `--no-sync` on single-key writes when throughput matters more than crash
durability.

## Formats

`raw` is uninterpreted bytes.

`line` is one record per input line.

`kv` is one record per line, split on the first tab:

```text
key<TAB>value
```

`ndjson` input is one JSON object per line. Repeated `--key-field` flags build a
compound key joined with `--key-sep`, which defaults to `:`. String, number,
boolean, and null key fields are accepted; object and array key fields are not.
Import and stream lookup paths reject empty user keys.

## Command Map

Core commands:

```text
pbl init
pbl put <collection> <key> <value>
pbl get <collection> <key>
pbl del <collection> <key>
```

See [commands/core.md](commands/core.md).

Ordered reads:

```text
pbl scan <collection>
pbl prefix <collection> <prefix>
pbl range <collection> <start> <end>
pbl keys <collection>
pbl values <collection>
pbl export <collection>
```

See [commands/ordered-reads.md](commands/ordered-reads.md).

Import and apply:

```text
pbl import <collection> --format kv|line|ndjson|raw
pbl apply <collection> --format kcat|frame
```

See [commands/import-apply.md](commands/import-apply.md).

Stream commands:

```text
pbl get-many <collection>
pbl del-many <collection>
pbl exists <collection>
pbl lookup <collection>
pbl join <collection> --on <field> --as <field>
```

See [commands/streams.md](commands/streams.md).

Metadata:

```text
pbl collections
pbl info
pbl stats
```

See [commands/metadata.md](commands/metadata.md).
