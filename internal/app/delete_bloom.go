package app

import (
	"fmt"
	"hash/maphash"
	"math/bits"
)

const (
	deleteBloomBitsPerKey = 11
	deleteBloomProbes     = 6
)

type deleteBloom struct {
	bits []uint64
	seed maphash.Seed
}

func newDeleteBloom(expectedKeys uint64) (*deleteBloom, error) {
	if expectedKeys == 0 || expectedKeys > (^uint64(0)-511)/deleteBloomBitsPerKey {
		return nil, fmt.Errorf("invalid expected key count")
	}
	blocks := (expectedKeys*deleteBloomBitsPerKey + 511) / 512
	if blocks > uint64(^uint(0)>>1)/64 {
		return nil, fmt.Errorf("expected key count is too large")
	}
	return &deleteBloom{bits: make([]uint64, int(blocks)*8), seed: maphash.MakeSeed()}, nil
}

func (b *deleteBloom) Add(key []byte) {
	block, h := b.block(key)
	for i := 0; i < deleteBloomProbes; i++ {
		bit := h >> 23
		block[bit>>6] |= uint64(1) << (bit & 63)
		h *= 0x9e3779b9
	}
}

func (b *deleteBloom) MayContain(key []byte) bool {
	block, h := b.block(key)
	// Starting at one keeps match in bit zero; any missing probe clears it.
	match := uint64(1)
	for i := 0; i < deleteBloomProbes; i++ {
		bit := h >> 23
		match &= block[bit>>6] >> (bit & 63)
		h *= 0x9e3779b9
	}
	return match != 0
}

// Keep all probes in a 64-byte block, using one hash and no division per probe.
func (b *deleteBloom) block(key []byte) (*[8]uint64, uint32) {
	h := maphash.Bytes(b.seed, key)
	index, _ := bits.Mul64(h, uint64(len(b.bits)/8))
	offset := index * 8
	return (*[8]uint64)(b.bits[offset : offset+8]), uint32(h)
}
