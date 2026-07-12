package app

import (
	"fmt"
	"hash/maphash"
)

const (
	deleteBloomBitsPerKey = 10
	deleteBloomProbes     = 7
)

type deleteBloom struct {
	bits     []uint64
	bitCount uint64
	seed1    maphash.Seed
	seed2    maphash.Seed
}

func newDeleteBloom(expectedKeys uint64) (*deleteBloom, error) {
	maxUint64 := ^uint64(0)
	if expectedKeys == 0 || expectedKeys > (maxUint64-63)/deleteBloomBitsPerKey {
		return nil, fmt.Errorf("invalid expected key count")
	}
	bitCount := expectedKeys * deleteBloomBitsPerKey
	words := (bitCount + 63) / 64
	if words > uint64(^uint(0)>>1)/8 {
		return nil, fmt.Errorf("expected key count is too large")
	}
	return &deleteBloom{
		bits:     make([]uint64, int(words)),
		bitCount: words * 64,
		seed1:    maphash.MakeSeed(),
		seed2:    maphash.MakeSeed(),
	}, nil
}

func (b *deleteBloom) Add(key []byte) {
	h, step := b.hashes(key)
	for i := uint64(0); i < deleteBloomProbes; i++ {
		bit := (h + i*step) % b.bitCount
		b.bits[bit>>6] |= uint64(1) << (bit & 63)
	}
}

func (b *deleteBloom) MayContain(key []byte) bool {
	h, step := b.hashes(key)
	for i := uint64(0); i < deleteBloomProbes; i++ {
		bit := (h + i*step) % b.bitCount
		if b.bits[bit>>6]&(uint64(1)<<(bit&63)) == 0 {
			return false
		}
	}
	return true
}

func (b *deleteBloom) hashes(key []byte) (uint64, uint64) {
	h := maphash.Bytes(b.seed1, key) % b.bitCount
	step := maphash.Bytes(b.seed2, key)%b.bitCount | 1
	if step >= b.bitCount {
		step = 1
	}
	return h, step
}
