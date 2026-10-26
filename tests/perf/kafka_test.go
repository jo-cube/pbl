package perf_test

import (
	"fmt"
	"io"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jo-cube/pbl/internal/app"
)

// Input generation is outside the timer. Each CLI invocation uses a fresh DB,
// including initialization, parsing, writes, normal tail flushing, and close.
func BenchmarkKafkaApply(b *testing.B) {
	for _, shape := range []string{"unique", "churn", "hot", "noisy"} {
		records := 500_000
		if shape == "noisy" {
			records = 1_000_000
		}
		data := kafkaInput(records, shape)
		variants := []string{"plain"}
		if shape == "noisy" {
			variants = append(variants, "bloom")
		}
		for _, variant := range variants {
			b.Run(shape+"/"+variant, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(data)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					args := []string{"--db", filepath.Join(b.TempDir(), "db"), "apply", "events", "--format", "kcat"}
					if variant == "bloom" {
						args = append(args, "--bloom-filter", "--expected-key-count", fmt.Sprint(records/200))
					}
					if code := app.Main(args, strings.NewReader(data), io.Discard, io.Discard); code != 0 {
						b.Fatalf("apply exit code %d", code)
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(records)*float64(b.N)/b.Elapsed().Seconds(), "records/s")
			})
		}
	}
}

func kafkaInput(records int, shape string) string {
	rng := rand.New(rand.NewPCG(42, 54))
	order := make([]int, records)
	for i := range order {
		order[i] = i
	}
	rng.Shuffle(records, func(i, j int) { order[i], order[j] = order[j], order[i] })
	domain := records
	if shape == "hot" {
		domain = 256
	}
	if shape == "churn" {
		domain = records / 10
	}
	var input strings.Builder
	if shape == "noisy" {
		input.Grow(records * 40)
	} else {
		input.Grow(records * 300)
	}
	payload := strings.Repeat("0123456789abcdef", 15)
	for i := 0; i < records; i++ {
		key := order[i]
		if shape != "unique" {
			key = rng.IntN(domain)
		}
		del := false
		if shape == "hot" || shape == "churn" {
			del = rng.IntN(10) < 3
		}
		if shape == "noisy" {
			del = i%200 != 0
			key = i
		}
		if del {
			fmt.Fprintf(&input, "tenant/%016x\t-1\t\n", key)
		} else {
			fmt.Fprintf(&input, "tenant/%016x\t256\t%016x%s\n", key, i, payload)
		}
	}
	return input.String()
}
