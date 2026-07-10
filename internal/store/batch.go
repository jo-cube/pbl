package store

import (
	"github.com/cockroachdb/pebble/v2"
	"github.com/jo-cube/pbl/internal/keyenc"
)

type Batch struct {
	batch *pebble.Batch
	count int
	bytes int
}

func (b *Batch) Put(collection string, key, value []byte) error {
	if err := ValidateCollection(collection); err != nil {
		return err
	}
	phys := keyenc.DataKey(collection, key)
	if err := b.batch.Set(phys, value, nil); err != nil {
		return err
	}
	b.count++
	b.bytes += len(phys) + len(value)
	return nil
}

func (b *Batch) Delete(collection string, key []byte) error {
	if err := ValidateCollection(collection); err != nil {
		return err
	}
	phys := keyenc.DataKey(collection, key)
	if err := b.batch.Delete(phys, nil); err != nil {
		return err
	}
	b.count++
	b.bytes += len(phys)
	return nil
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
