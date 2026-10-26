package store

import (
	"bytes"
	"slices"
	"sync"

	"github.com/cockroachdb/pebble/v2"
	"github.com/jo-cube/pbl/internal/keyenc"
)

type Batch struct {
	batch      *pebble.Batch
	db         *pebble.DB
	count      int
	bytes      int
	collection string
	base       []byte
}

func (b *Batch) Put(collection string, key, value []byte) error {
	base, err := b.collectionBase(collection)
	if err != nil {
		return err
	}
	op := b.batch.SetDeferred(len(base)+len(key), len(value))
	n := copy(op.Key, base)
	copy(op.Key[n:], key)
	copy(op.Value, value)
	if err := op.Finish(); err != nil {
		return err
	}
	b.count++
	b.bytes += len(base) + len(key) + len(value)
	return nil
}

func (b *Batch) Delete(collection string, key []byte) error {
	base, err := b.collectionBase(collection)
	if err != nil {
		return err
	}
	op := b.batch.DeleteDeferred(len(base) + len(key))
	n := copy(op.Key, base)
	copy(op.Key[n:], key)
	if err := op.Finish(); err != nil {
		return err
	}
	b.count++
	b.bytes += len(base) + len(key)
	return nil
}

func (b *Batch) collectionBase(collection string) ([]byte, error) {
	if b.base != nil && collection == b.collection {
		return b.base, nil
	}
	if err := ValidateCollection(collection); err != nil {
		return nil, err
	}
	b.collection = collection
	b.base = keyenc.CollectionBase(collection)
	return b.base, nil
}

func (b *Batch) Commit(opts WriteOptions) error {
	if !b.shouldCoalesce() {
		return b.batch.Commit(pebbleWriteOptions(opts))
	}
	reader := b.batch.Reader()
	entries := batchEntriesPool.Get().([]batchEntry)
	if cap(entries) < b.count {
		entries = make([]batchEntry, 0, b.count)
	}
	defer func() {
		if cap(entries) <= 4096 {
			clear(entries)
			batchEntriesPool.Put(entries[:0])
		}
	}()
	for {
		kind, key, value, ok, err := reader.Next()
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		entries = append(entries, batchEntry{key: key, value: value, delete: kind == pebble.InternalKeyKindDelete})
	}
	slices.SortStableFunc(entries, func(a, b batchEntry) int { return bytes.Compare(a.key, b.key) })
	compact := b.db.NewBatch()
	keep := false
	defer func() {
		if !keep {
			_ = compact.Close()
		}
	}()
	for i, entry := range entries {
		if i+1 < len(entries) && bytes.Equal(entry.key, entries[i+1].key) {
			continue
		}
		var err error
		if entry.delete {
			err = compact.Delete(entry.key, nil)
		} else {
			err = compact.Set(entry.key, entry.value, nil)
		}
		if err != nil {
			return err
		}
	}
	original := b.batch
	b.batch = compact
	keep = true
	if err := original.Close(); err != nil {
		return err
	}
	return b.batch.Commit(pebbleWriteOptions(opts))
}

func (b *Batch) Close() error {
	return b.batch.Close()
}

func (b *Batch) Count() int {
	return b.count
}

func (b *Batch) ApproxBytes() int {
	return b.bytes
}

// Only retain small scratch buffers, and release views into the Pebble batch.
var batchEntriesPool = sync.Pool{New: func() any { return make([]batchEntry, 0, 1024) }}

type batchEntry struct {
	key, value []byte
	delete     bool
}

// Sample before sorting: most streams have too few repeated keys to repay it.
// Counts and byte limits still describe the input, preserving commit boundaries.
func (b *Batch) shouldCoalesce() bool {
	const sample = 128
	if b.count < sample {
		return false
	}
	var keys [sample][]byte
	reader := b.batch.Reader()
	for i := range keys {
		_, key, _, ok, err := reader.Next()
		if err != nil || !ok {
			return false
		}
		keys[i] = key
	}
	slices.SortFunc(keys[:], bytes.Compare)
	duplicates := 0
	for i := 1; i < len(keys); i++ {
		if bytes.Equal(keys[i-1], keys[i]) {
			duplicates++
		}
	}
	return duplicates >= sample/8
}
