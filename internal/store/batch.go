package store

import (
	"github.com/cockroachdb/pebble/v2"
	"github.com/jo-cube/pbl/internal/keyenc"
)

type Batch struct {
	batch      *pebble.Batch
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
