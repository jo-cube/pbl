package perf_test

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jo-cube/pbl/internal/app"
)

// Setup is outside the timer; apply includes generation, filter preload, and close.
func BenchmarkBloomApplyExisting(b *testing.B) {
	const records = 4000000
	for _, existing := range []int{50000, 1000000} {
		for _, hitPercent := range []int{0, 1, 50, 100} {
			for _, filter := range []bool{false, true} {
				b.Run(fmt.Sprintf("keys=%d/hit=%d/bloom=%t", existing, hitPercent, filter), func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						b.StopTimer()
						db := filepath.Join(b.TempDir(), "db")
						setup := newBloomKafkaReader(existing, 0, 1, 0)
						args := []string{"--db", db, "apply", "events", "--format", "kcat"}
						if code := app.Main(args, setup, io.Discard, io.Discard); code != 0 {
							b.Fatalf("setup exit %d", code)
						}
						input := newBloomKafkaReader(records, existing, 200, hitPercent)
						if filter {
							args = append(args, "--bloom-filter", "--expected-key-count", strconv.Itoa(existing+records/200))
						}
						b.StartTimer()
						if code := app.Main(args, input, io.Discard, io.Discard); code != 0 {
							b.Fatalf("apply exit %d", code)
						}
					}
					b.ReportMetric(float64(records)*float64(b.N)/b.Elapsed().Seconds(), "records/s")
				})
			}
		}
	}
}

type bloomKafkaReader struct {
	records, position, existing, period, hitPercent int
	random                                          uint64
	buffer, pending                                 []byte
}

func newBloomKafkaReader(records, existing, period, hitPercent int) *bloomKafkaReader {
	return &bloomKafkaReader{records: records, existing: existing, period: period, hitPercent: hitPercent, random: 42, buffer: make([]byte, 0, 128<<10)}
}

func (r *bloomKafkaReader) Read(p []byte) (int, error) {
	if len(r.pending) == 0 {
		if r.position == r.records {
			return 0, io.EOF
		}
		r.buffer = r.buffer[:0]
		for r.position < r.records && len(r.buffer) < 64<<10 {
			i := r.position
			r.position++
			r.random ^= r.random << 13
			r.random ^= r.random >> 7
			r.random ^= r.random << 17
			put := i%r.period == 0
			id := r.random | 1<<63
			if put {
				id = uint64(r.existing + i/r.period)
			} else if int(r.random%100) < r.hitPercent {
				id = (r.random >> 16) % uint64(r.existing+(i+r.period-1)/r.period)
			}
			var key [16]byte
			const hex = "0123456789abcdef"
			for k := range key {
				key[15-k] = hex[id&15]
				id >>= 4
			}
			r.buffer = append(r.buffer, "tenant/"...)
			r.buffer = append(r.buffer, key[:]...)
			if put {
				r.buffer = append(r.buffer, "\t16\t0123456789abcdef\n"...)
			} else {
				r.buffer = append(r.buffer, "\t-1\t\n"...)
			}
		}
		r.pending = r.buffer
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}
