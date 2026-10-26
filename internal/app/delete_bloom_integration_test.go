package app

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jo-cube/pbl/internal/store"
)

func TestDeleteBloomSizingAndBorrowedKeys(t *testing.T) {
	for _, tc := range []struct {
		keys  uint64
		bytes int
	}{{1, 64}, {46, 64}, {47, 128}, {100, 192}, {1000000, 1375040}} {
		b, err := newDeleteBloom(tc.keys)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(b.bits) * 8; got != tc.bytes {
			t.Fatalf("keys=%d bytes=%d want=%d", tc.keys, got, tc.bytes)
		}
	}
	for _, keys := range []uint64{0, ^uint64(0), (^uint64(0)-511)/deleteBloomBitsPerKey + 1} {
		if _, err := newDeleteBloom(keys); err == nil {
			t.Fatalf("accepted count %d", keys)
		}
	}
	for _, expected := range []uint64{1, 100, 100000} {
		b, err := newDeleteBloom(expected)
		if err != nil {
			t.Fatal(err)
		}
		var key [32]byte
		for i := uint64(0); i < 10000; i++ {
			binary.LittleEndian.PutUint64(key[:], i)
			b.Add(key[:])
			b.Add(key[:])
		}
		clear(key[:])
		for i := uint64(0); i < 10000; i++ {
			binary.LittleEndian.PutUint64(key[:], i)
			if !b.MayContain(key[:]) {
				t.Fatalf("expected=%d lost key %d", expected, i)
			}
		}
		if got := testing.AllocsPerRun(1000, func() {
			b.Add(key[:])
			if !b.MayContain(key[:]) {
				panic("false negative")
			}
		}); got != 0 {
			t.Fatalf("allocations=%v", got)
		}
	}
}

func TestDeleteBloomFalsePositiveRate(t *testing.T) {
	const keys = 100000
	b, err := newDeleteBloom(keys)
	if err != nil {
		t.Fatal(err)
	}
	var key [8]byte
	for i := uint64(0); i < keys; i++ {
		binary.LittleEndian.PutUint64(key[:], i)
		b.Add(key[:])
	}
	hits := 0
	for i := uint64(keys); i < keys+500000; i++ {
		binary.LittleEndian.PutUint64(key[:], i)
		if b.MayContain(key[:]) {
			hits++
		}
	}
	// Leave ample margin for independently seeded filters; catch a broken probe layout.
	if hits > 7500 {
		t.Fatalf("false positives=%d/500000, want below 1.5%%", hits)
	}
}

func TestApplyBloomOrderedState(t *testing.T) {
	for _, format := range []string{"frame", "kcat"} {
		for _, expected := range []string{"1", "2K"} {
			for _, batchSize := range []string{"1", "128", "1000"} {
				t.Run(format+"/"+expected+"/"+batchSize, func(t *testing.T) {
					db := filepath.Join(t.TempDir(), "db")
					want := make(map[string]string)
					var seed strings.Builder
					for i := 0; i < 64; i++ {
						key := fmt.Sprintf("key-%03d\x00", i)
						fmt.Fprintf(&seed, "P %d 4\n%sseed", len(key), key)
						want[key] = "seed"
					}
					if out, err, code := run(t, db, seed.String(), "apply", "events", "--format", "frame"); code != 0 || out != "" || err != "" {
						t.Fatalf("seed: %d %q %q", code, out, err)
					}
					if _, err, code := run(t, db, "", "put", "other", "key-000", "sentinel"); code != 0 {
						t.Fatal(code, err)
					}
					var input strings.Builder
					rng := rand.New(rand.NewPCG(42, 54))
					for i := 0; i < 10000; i++ {
						key := fmt.Sprintf("key-%03d\x00", rng.IntN(256))
						del := rng.IntN(100) < 80
						value := fmt.Sprintf("v-%d\x00\n", i)
						if i%19 == 0 {
							value = ""
						}
						if del {
							delete(want, key)
							if format == "frame" {
								fmt.Fprintf(&input, "D %d\n%s", len(key), key)
							} else {
								fmt.Fprintf(&input, "%s\t-1\t\n", key)
							}
						} else {
							want[key] = value
							if format == "frame" {
								fmt.Fprintf(&input, "P %d %d\n%s%s", len(key), len(value), key, value)
							} else {
								fmt.Fprintf(&input, "%s\t%d\t%s\n", key, len(value), value)
							}
						}
					}
					if out, err, code := run(t, db, input.String(), "apply", "events", "--format", format, "--batch-size", batchSize, "--bloom-filter", "--expected-key-count", expected); code != 0 || out != "" || err != "" {
						t.Fatalf("apply: %d %q %q", code, out, err)
					}
					keys := make([]string, 0, len(want))
					for k := range want {
						keys = append(keys, k)
					}
					slices.Sort(keys)
					var expectedOutput strings.Builder
					for _, k := range keys {
						fmt.Fprintf(&expectedOutput, "P %d %d\n%s%s", len(k), len(want[k]), k, want[k])
					}
					if out, err, code := run(t, db, "", "scan", "events", "--format", "frame"); code != 0 || out != expectedOutput.String() || err != "" {
						t.Fatalf("scan: %d %q want=%q err=%q", code, out, expectedOutput.String(), err)
					}
					if out, err, code := run(t, db, "", "get", "other", "key-000"); code != 0 || out != "sentinel\n" || err != "" {
						t.Fatalf("other: %d %q %q", code, out, err)
					}
				})
			}
		}
	}
}

func TestApplyBloomRecoversWAL(t *testing.T) {
	if db := os.Getenv("PBL_BLOOM_RECOVERY_DB"); db != "" {
		s, err := store.Open(db)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.EnsureCollection("events"); err != nil {
			t.Fatal(err)
		}
		b := s.NewBatch()
		for i := 0; i < 128; i++ {
			if err = b.Put("events", []byte(strconv.Itoa(i)), []byte("value")); err != nil {
				t.Fatal(err)
			}
		}
		if err = b.Commit(store.WriteOptions{Sync: true}); err != nil {
			t.Fatal(err)
		}
		// Deliberately exit with an open DB and WAL; the next command must rebuild the filter.
		os.Exit(0)
	}
	db := filepath.Join(t.TempDir(), "db")
	cmd := exec.Command(os.Args[0], "-test.run=^TestApplyBloomRecoversWAL$")
	cmd.Env = append(os.Environ(), "PBL_BLOOM_RECOVERY_DB="+db)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper: %v %s", err, out)
	}
	var input bytes.Buffer
	for i := 0; i < 128; i++ {
		key := strconv.Itoa(i)
		fmt.Fprintf(&input, "D %d\n%s", len(key), key)
	}
	if out, err, code := run(t, db, input.String(), "apply", "events", "--format", "frame", "--bloom-filter", "--expected-key-count", "128", "--sync"); code != 0 || out != "" || err != "" {
		t.Fatalf("recovery apply: %d %q %q", code, out, err)
	}
	if out, err, code := run(t, db, "", "count", "events"); code != 0 || out != "0\n" || err != "" {
		t.Fatalf("recovered count: %d %q %q", code, out, err)
	}
}

func TestApplyBloomRebuildAfterInputFailure(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	var input strings.Builder
	for i := 0; i < 128; i++ {
		key := strconv.Itoa(i)
		fmt.Fprintf(&input, "P %d 1\n%sv", len(key), key)
	}
	input.WriteString("P 7 1\npendingvD 4\nx")
	if out, err, code := run(t, db, input.String(), "apply", "events", "--format", "frame", "--batch-size", "128", "--bloom-filter", "--expected-key-count", "128", "--sync"); code != 4 || out != "" || err == "" {
		t.Fatalf("failed apply: %d %q %q", code, out, err)
	}
	input.Reset()
	for i := 0; i < 128; i++ {
		key := strconv.Itoa(i)
		fmt.Fprintf(&input, "D %d\n%s", len(key), key)
	}
	input.WriteString("D 7\npending")
	if out, err, code := run(t, db, input.String(), "apply", "events", "--format", "frame", "--bloom-filter", "--expected-key-count", "128"); code != 0 || out != "" || err != "" {
		t.Fatalf("resume: %d %q %q", code, out, err)
	}
	if out, err, code := run(t, db, "", "count", "events"); code != 0 || out != "0\n" || err != "" {
		t.Fatalf("resumed count: %d %q %q", code, out, err)
	}
}
