package app

import (
	"encoding/binary"
	"fmt"
	"testing"
)

func BenchmarkDeleteBloomBuild(b *testing.B) {
	for _, keys := range []int{50000, 1000000, 10000000} {
		b.Run(fmt.Sprint(keys), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				filter, err := newDeleteBloom(uint64(keys))
				if err != nil {
					b.Fatal(err)
				}
				var key [24]byte
				for k := 0; k < keys; k++ {
					binary.LittleEndian.PutUint64(key[:], uint64(k))
					filter.Add(key[:])
				}
				benchmarkBloomHits = uint64(filter.bits[0])
			}
		})
	}
}

func BenchmarkDeleteBloomCardinality(b *testing.B) {
	for _, keys := range []int{50000, 1000000, 10000000} {
		b.Run(fmt.Sprint(keys), func(b *testing.B) {
			filter, err := newDeleteBloom(uint64(keys))
			if err != nil {
				b.Fatal(err)
			}
			var key [24]byte
			for k := 0; k < keys; k++ {
				binary.LittleEndian.PutUint64(key[:], uint64(k))
				filter.Add(key[:])
			}
			for _, hitPercent := range []int{0, 50, 100} {
				b.Run(fmt.Sprintf("hit=%d", hitPercent), func(b *testing.B) {
					b.ReportAllocs()
					var key [24]byte
					x := uint64(42)
					var hits, misses, falsePositives uint64
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						x ^= x << 13
						x ^= x >> 7
						x ^= x << 17
						present := i%100 < hitPercent
						id := x | 1<<63
						if present {
							id = x % uint64(keys)
						} else {
							misses++
						}
						binary.LittleEndian.PutUint64(key[:], id)
						if filter.MayContain(key[:]) {
							hits++
							if !present {
								falsePositives++
							}
						}
					}
					b.StopTimer()
					benchmarkBloomHits = hits
					b.ReportMetric(float64(len(filter.bits)*8), "filter-bytes")
					if misses > 0 {
						b.ReportMetric(100*float64(falsePositives)/float64(misses), "false-positive-%")
					}
				})
			}
		})
	}
}
