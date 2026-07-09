# pbl

`pbl` is a local ordered key-value CLI backed by Pebble. It gives shell scripts a
persistent lookup table, set, cache, or sorted index without running a server.

```sh
pbl --db ./data.pbl import users --format kv < users.tsv
pbl --db ./data.pbl get users u1
cat ids.txt | pbl --db ./data.pbl get-many users
```

Values are stored as bytes. Formats such as `kv`, `line`, `raw`, and `ndjson`
are only import/export conventions at the CLI edge.

## What It Is For

Use `pbl` when a shell workflow needs local state between steps:

- load IDs once, then filter future streams by membership;
- keep a small lookup table for enrichment;
- materialize a compacted stream into the latest value for each key;
- scan compound keys in byte order by prefix or half-open range;
- keep a local cache or artifact map by key.

It is deliberately not a database shell. There is no SQL, daemon, network
service, secondary index system, or query planner. The shape is closer to
`sort`, `join`, `grep`, and `sqlite` habits: plain commands, stable exit codes,
and stdout that can be piped to the next tool.

## Install

Install the latest release to `~/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/jo-cube/pbl/main/scripts/install.sh | sh
```

Install to a custom directory:

```sh
curl -fsSL https://raw.githubusercontent.com/jo-cube/pbl/main/scripts/install.sh | sh -s -- "$HOME/bin"
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/jo-cube/pbl/main/scripts/install.sh | VERSION=v0.1.0 sh
```

Release archives are verified with SHA-256 checksums. Published platforms:

- `linux/amd64`
- `linux/arm64`
- `darwin/arm64`

Other platforms can build from source with Go.

## Quick Use

Pick a database directory with `--db` or `PBL_DB`. If neither is set, `pbl` uses
`.pbl` in the current directory.

```sh
pbl --db ./local.pbl init
export PBL_DB=./local.pbl
pbl collections
```

Common jobs:

```sh
# Import tab-separated key/value rows.
printf 'u2\tGrace\nu1\tAda\n' | pbl import names --format kv

# Read one value.
pbl get names u1

# Read many keys in input order.
printf 'u2\nmissing\nu1\n' | pbl get-many names

# Use a stored collection as a persistent set.
cat blocked_ids.txt | pbl import blocked --format line --key-mode value
cat incoming_ids.txt | pbl exists blocked

# Join NDJSON events with stored JSON objects.
cat users.ndjson | pbl import users --format ndjson --key-field id
cat events.ndjson | pbl join users --on user_id --as user
```

## Data Model

One Pebble directory stores all collections. A collection is a named logical
keyspace. Keys are ordered bytes; values are opaque bytes.

The CLI formats are edge formats:

- `kv`: one `key<TAB>value` record per line.
- `line`: one line per record, often useful for persistent sets.
- `raw`: one stdin payload stored under `--key`.
- `ndjson`: one JSON object per line, keyed by one or more fields.

Scans are ordered by raw key bytes. `range` is half-open:

```text
start <= key < end
```

Stream commands preserve input order. Diagnostics go to stderr; stdout is data.

## Command Map

Core operations:

```text
pbl init
pbl put <collection> <key> <value>
pbl get <collection> <key>
pbl del <collection> <key>
```

Ordered reads:

```text
pbl scan <collection>
pbl prefix <collection> <prefix>
pbl range <collection> <start> <end>
pbl keys <collection>
pbl values <collection>
```

Streaming workflows:

```text
pbl import <collection> --format kv|line|ndjson|raw
pbl export <collection>
pbl get-many <collection>
pbl del-many <collection>
pbl exists <collection>
pbl lookup <collection>
pbl join <collection> --on <field> --as <field>
pbl apply <collection> --format kcat|frame
```

Metadata:

```text
pbl collections
pbl info
pbl stats
```

## Docs

- [Usage guide](docs/usage.md): practical workflows with examples.
- [CLI reference](docs/cli.md): global flags, formats, exit codes, and command
  map.
- [Command manual](docs/commands/README.md): per-command flags, parameters, and
  behind-the-scenes behavior.
- [Storage format](docs/storage-format.md): compatibility-sensitive physical key
  layout.
- [Development notes](docs/development.md): package shape, checks, and project
  boundaries.

## Development

```sh
make test
go vet ./...
go mod tidy -diff
```
