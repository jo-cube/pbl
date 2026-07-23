package app

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"testing"
)

var benchmarkBloomHits uint64

func TestDeleteBloomRetainsAddedKeys(t *testing.T) {
	b, err := newDeleteBloom(1_000)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1_000; i++ {
		b.Add([]byte(fmt.Sprintf("key-%d", i)))
	}
	for i := 0; i < 1_000; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		if !b.MayContain(key) {
			t.Fatalf("filter lost %q", key)
		}
	}
}

func TestDeleteBloomSizeAndValidation(t *testing.T) {
	b, err := newDeleteBloom(100)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(b.bits)*8, 128; got != want {
		t.Fatalf("filter bytes = %d, want %d", got, want)
	}
	if _, err := newDeleteBloom(0); err == nil {
		t.Fatal("newDeleteBloom accepted zero keys")
	}
}

func BenchmarkDeleteBloomLookup(b *testing.B) {
	const keys = 10_000_000
	filter, err := newDeleteBloom(keys)
	if err != nil {
		b.Fatal(err)
	}
	var key [8]byte
	for i := uint64(0); i < keys; i++ {
		binary.LittleEndian.PutUint64(key[:], i)
		filter.Add(key[:])
	}

	b.Run("serial", func(b *testing.B) {
		b.ReportAllocs()
		var key [8]byte
		x := uint64(1)
		var hits uint64
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			x ^= x << 13
			x ^= x >> 7
			x ^= x << 17
			binary.LittleEndian.PutUint64(key[:], x|1<<63)
			if filter.MayContain(key[:]) {
				hits++
			}
		}
		b.StopTimer()
		benchmarkBloomHits = hits
		b.ReportMetric(100*float64(hits)/float64(b.N), "false-positive-%")
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "lookups/s")
	})

	b.Run("parallel", func(b *testing.B) {
		b.ReportAllocs()
		var workerID atomic.Uint64
		var totalHits atomic.Uint64
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var key [8]byte
			x := workerID.Add(1)
			var hits uint64
			for pb.Next() {
				x ^= x << 13
				x ^= x >> 7
				x ^= x << 17
				binary.LittleEndian.PutUint64(key[:], x|1<<63)
				if filter.MayContain(key[:]) {
					hits++
				}
			}
			totalHits.Add(hits)
		})
		b.StopTimer()
		hits := totalHits.Load()
		benchmarkBloomHits = hits
		b.ReportMetric(100*float64(hits)/float64(b.N), "false-positive-%")
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "lookups/s")
	})
}
