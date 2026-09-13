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
keys are skipped by default. With `--missing null`, missing keys emit a null
record; a present empty value in raw output emits an empty line. `--with-key`
requires NDJSON output.

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

## join

```text
pbl join <collection> --on <field> --as <field>
  [--on <field> ...] [--key-sep <sep>]
  [--missing null|skip|error]
```

Attaches stored JSON values to NDJSON input objects, preserving input order.
`--on` names an input string field; dotted paths address nested fields. Repeat
`--on` in the same order as import's `--key-field` flags to build compound keys.
The one-byte `--key-sep` defaults to `:`; compound parts may not contain it.

`--as` names the top-level output field. An existing field with that name is
replaced. Stored values must be valid JSON, including scalars and null. Numbers
retain their precision, including integers larger than 2^53. Output is one JSON
object per line; member order is not guaranteed.

Missing keys attach `null` by default. `--missing skip` omits those input records;
`--missing error` fails at the first missing key, with exit 2 or exit 6 if output
has already begun.

Behind the scenes: each input object supplies a key for a point lookup, and the
stored JSON bytes are attached without decoding their numbers to floating point.
