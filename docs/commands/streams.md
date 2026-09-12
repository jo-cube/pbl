# Stream Commands

## get-many

```text
pbl get-many <collection>
  [--input-format line|ndjson]
  [--key-field <field>]
  [--key-sep <sep>]
  [--format raw|kv|ndjson]
  [--with-key]
  [--missing skip|null|error]
```

Reads lookup keys from stdin and emits matching values in the same order. Missing
keys are skipped by default.

NDJSON key fields must be strings. `get-many` joins repeated key fields with
the one-byte `--key-sep`, which defaults to `:`; compound key parts may not
contain the separator.

Behind the scenes: each input record is looked up independently. If a later
lookup fails after stdout has already been written, pbl exits with partial
failure code 6.

## del-many

```text
pbl del-many <collection>
  [--input-format line|ndjson]
  [--key-field <field>]
  [--key-sep <sep>]
  [--batch-size <n>]
  [--batch-bytes <size>]
  [--sync|--no-sync]
```

Reads keys from stdin and deletes them in batches. Success writes no stdout.

Behind the scenes: deletes are batched like import records. Missing keys are not
an error.

## exists

```text
pbl exists <collection>
  [--input-format line|ndjson]
  [--key-field <field>]
  [--key-sep <sep>]
  [--invert]
  [--missing skip|error]
```

Filters stdin by key existence and emits original input records unchanged.

Flags:

- `--invert`: emit missing records instead of existing records.
- `--missing error`: fail on the first missing key instead of filtering it out.

Behind the scenes: `exists` is a membership test against the selected collection;
it does not read stored values.

## lookup

```text
pbl lookup <collection>
  [--input-format line|ndjson]
  [--key-field <field>]
  [--key-sep <sep>]
  [--as <field>]
  [--missing null|skip|error]
```

Looks up stdin records in a collection. Line input emits stored values. NDJSON
input requires `--as` so pbl knows where to attach the stored value.

For line input, `--missing null` emits the literal line `null`, preserving one
output record per input key. A present empty value emits an empty line.

Behind the scenes: stored values must be valid JSON when attached to NDJSON
input. Missing NDJSON lookups emit null by default. Stored JSON is attached
without converting numbers to floating point, including integers larger than
2^53. Output is one JSON object per line; member order is not guaranteed.

## join

```text
pbl join <collection> --on <field> --as <field>
  [--key-field <field>]
  [--key-sep <sep>]
  [--missing null|skip|error]
```

Joins NDJSON input with stored JSON objects. `--on` names the input field used as
the lookup key, or the final part of a compound key. `--as` names the output
field receiving the stored JSON value. Use repeated `--key-field` flags for
leading compound-key parts when the stored collection uses a compound key.

Behind the scenes: `join` is the NDJSON-only convenience form of `lookup`.
