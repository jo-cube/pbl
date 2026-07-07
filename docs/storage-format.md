# Storage Format

This document describes storage format v1. Treat it as compatibility-sensitive.

`pbl` stores one Pebble database directory. Logical collections are encoded into
physical Pebble keys; values are stored unchanged.

## Keyspaces

```text
metadata: 0x00 <metadata-name>
data:     0x01 <collection-name-length-uvarint> <collection-name-bytes> 0x00 <user-key-bytes>
```

All physical key construction belongs in `internal/keyenc`.

## Metadata

Required records:

```text
0x00 "format-version" -> "1"
0x00 "created-at"     -> RFC3339Nano timestamp
```

Collection records:

```text
0x00 "collection/" <collection-name>
```

The value is JSON metadata for the collection.

Collections are listed from these metadata records. Do not infer collection
names by scanning data keys; the encoded collection length affects ordering.

## Data Keys

For collection `users` and user key `u1`, the physical key is:

```text
0x01 0x05 "users" 0x00 "u1"
```

Ordering inside a collection follows raw user-key byte order. This is what
makes `scan`, `prefix`, and half-open `range` efficient.

## Bounds

Whole collection scan:

```text
lower = collectionBase(collection)
upper = NextPrefix(collectionBase(collection))
```

Prefix scan:

```text
lower = collectionBase(collection) + userPrefix
upper = NextPrefix(collectionBase(collection) + userPrefix)
```

Range scan:

```text
lower = collectionBase(collection) + start
upper = collectionBase(collection) + end
```

Range semantics are half-open:

```text
start <= key < end
```

## Versioning

The supported format version is `1`.

Open behavior:

- Missing metadata is initialized only by commands allowed to create the DB.
- Future or older unsupported versions fail with storage exit code `5`.
- Physical key encoding must not change without a format bump and migration
  plan.

## Current Limits

Collection names must be non-empty and must use the current valid character
set enforced by the store layer.

CLI import and stream lookup paths reject empty user keys. The physical encoding
can represent them, but they are not accepted user-facing input in v1.

Collection deletion is not implemented in v0.1. When it exists, it should delete
the collection key range and its metadata together.
