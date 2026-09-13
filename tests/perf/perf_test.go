package perf_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jo-cube/pbl/internal/app"
)

const perfRecords = 100_000

const lookupSmokeKeys = `k000000
k050000
missing
k099999
`

const lookupSmokeWant = `v000000
v050000
v099999
`

func TestPerfVolumeKVImportScanLookup(t *testing.T) {
	if os.Getenv("PBL_PERF") != "1" {
		t.Skip("set PBL_PERF=1 to run volume smoke tests")
	}
	db := filepath.Join(t.TempDir(), "db")
	input := kvInput(perfRecords)
	run(t, db, input, "import", "kv", "--format", "kv")

	var out bytes.Buffer
	code := app.Main([]string{"--db", db, "scan", "kv"}, strings.NewReader(""), &out, io.Discard)
	if code != 0 {
		t.Fatalf("scan exit code %d", code)
	}
	if got := bytes.Count(out.Bytes(), []byte{newline}); got != perfRecords {
		t.Fatalf("scan records = %d, want %d", got, perfRecords)
	}

	out.Reset()
	code = app.Main([]string{"--db", db, "get-many", "kv"}, strings.NewReader(lookupSmokeKeys), &out, io.Discard)
	if code != 0 {
		t.Fatalf("get-many exit code %d", code)
	}
	if out.String() != lookupSmokeWant {
		t.Fatalf("get-many output = %q, want %q", out.String(), lookupSmokeWant)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"count", "kv"}, strconv.Itoa(perfRecords) + "\n"},
		{[]string{"scan", "kv", "--prefix", "k099", "--reverse", "--limit", "2", "--keys-only"}, "k099999\nk099998\n"},
	} {
		out.Reset()
		code := app.Main(append([]string{"--db", db}, tc.args...), strings.NewReader(""), &out, io.Discard)
		if code != 0 || out.String() != tc.want {
			t.Fatalf("%v output=%q code=%d", tc.args, out.String(), code)
		}
	}
}

func BenchmarkKVImport(b *testing.B) {
	input := kvInput(25_000)
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db := filepath.Join(b.TempDir(), fmt.Sprintf("db-%d", i))
		code := app.Main([]string{"--db", db, "import", "kv", "--format", "kv", "--batch-size", "2000"}, strings.NewReader(input), io.Discard, io.Discard)
		if code != 0 {
			b.Fatalf("import exit code %d", code)
		}
	}
}

func BenchmarkApplyDeleteHeavy(b *testing.B) {
	input := frameApplyInput(25_000)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"plain", nil},
		{"bloom", []string{"--bloom-filter", "--expected-key-count", "25K"}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				db := filepath.Join(b.TempDir(), fmt.Sprintf("db-%d", i))
				args := append([]string{"--db", db, "apply", "kv", "--format", "frame", "--batch-size", "2000"}, tc.args...)
				if code := app.Main(args, strings.NewReader(input), io.Discard, io.Discard); code != 0 {
					b.Fatalf("apply exit code %d", code)
				}
			}
		})
	}
}

func BenchmarkApplyTombstoneDominated(b *testing.B) {
	const records = 1_000_000
	input := tombstoneDominatedKcatInput(records)
	expectedKeys := strconv.Itoa(records / 200)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"plain", nil},
		{"bloom", []string{"--bloom-filter", "--expected-key-count", expectedKeys}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				db := filepath.Join(b.TempDir(), fmt.Sprintf("db-%d", i))
				args := append([]string{"--db", db, "apply", "kv", "--format", "kcat"}, tc.args...)
				if code := app.Main(args, strings.NewReader(input), io.Discard, io.Discard); code != 0 {
					b.Fatalf("apply exit code %d", code)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(records)*float64(b.N)/b.Elapsed().Seconds(), "records/s")
		})
	}
}

func BenchmarkScan(b *testing.B) {
	db := filepath.Join(b.TempDir(), "db")
	run(b, db, kvInput(25_000), "import", "kv", "--format", "kv")
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"all", []string{"scan", "kv"}},
		{"keys", []string{"scan", "kv", "--keys-only"}},
		{"latest10", []string{"scan", "kv", "--reverse", "--limit", "10"}},
		{"count", []string{"count", "kv"}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				run(b, db, "", tc.args...)
			}
		})
	}
}

func BenchmarkGetManyMixed(b *testing.B) {
	db := filepath.Join(b.TempDir(), "db")
	run(b, db, kvInput(25_000), "import", "kv", "--format", "kv")
	keys := lookupKeys(10_000)
	b.ReportAllocs()
	b.SetBytes(int64(len(keys)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		code := app.Main([]string{"--db", db, "get-many", "kv"}, strings.NewReader(keys), io.Discard, io.Discard)
		if code != 0 {
			b.Fatalf("get-many exit code %d", code)
		}
	}
}

func run(tb testing.TB, db, stdin string, args ...string) {
	tb.Helper()
	full := append([]string{"--db", db}, args...)
	code := app.Main(full, strings.NewReader(stdin), io.Discard, io.Discard)
	if code != 0 {
		tb.Fatalf("pbl %s exit code %d", strings.Join(full, " "), code)
	}
}

func kvInput(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "k%06d", i)
		b.WriteByte(tab)
		fmt.Fprintf(&b, "v%06d", i)
		b.WriteByte(newline)
	}
	return b.String()
}

func lookupKeys(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i%10 == 0 {
			fmt.Fprintf(&b, "missing-%06d", i)
			b.WriteByte(newline)
			continue
		}
		fmt.Fprintf(&b, "k%06d", i)
		b.WriteByte(newline)
	}
	return b.String()
}

func frameApplyInput(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("k%06d", i)
		if i%10 < 7 {
			fmt.Fprintf(&b, "D %d\n%s", len(key), key)
			continue
		}
		fmt.Fprintf(&b, "P %d 1\n%sV", len(key), key)
	}
	return b.String()
}

func tombstoneDominatedKcatInput(n int) string {
	var b strings.Builder
	b.Grow(n * 16)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "k%010d\t", i)
		if i%200 == 0 {
			b.WriteString("1\tV\n")
		} else {
			b.WriteString("-1\t\n")
		}
	}
	return b.String()
}

const tab = '\t'
const newline = '\n'

func BenchmarkNDJSON(b *testing.B) {
	for _, shape := range []string{"small", "nested", "nested-key"} {
		b.Run(shape, func(b *testing.B) {
			const records = 2500
			payload := `{"active":true,"score":12.5}`
			if shape != "small" {
				payload = `[` + strings.Repeat(`{"name":"Ada","active":true,"score":12.5,"tags":["a","b"]},`, 63) + `null]`
			}
			keyField := "id"
			if shape == "nested-key" {
				keyField = "meta.id"
			}
			var input strings.Builder
			for i := 0; i < records; i++ {
				if shape == "nested-key" {
					fmt.Fprintf(&input, "{\"meta\":{\"id\":\"k%06d\",\"payload\":%s}}\n", i, payload)
					continue
				}
				fmt.Fprintf(&input, "{\"id\":\"k%06d\",\"payload\":%s}\n", i, payload)
			}
			data := input.String()
			for _, command := range []string{"import", "join", "scan"} {
				b.Run(command, func(b *testing.B) {
					db := filepath.Join(b.TempDir(), "db")
					if command != "import" {
						run(b, db, data, "import", "docs", "--format", "ndjson", "--key-field", keyField)
					}
					b.ReportAllocs()
					b.SetBytes(int64(len(data)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						switch command {
						case "import":
							run(b, filepath.Join(b.TempDir(), "db"), data, "import", "docs", "--format", "ndjson", "--key-field", keyField)
						case "join":
							run(b, db, data, "join", "docs", "--on", keyField, "--as", "doc")
						case "scan":
							run(b, db, "", "scan", "docs", "--format", "ndjson", "--with-key")
						}
					}
				})
			}
		})
	}
}

func BenchmarkApplyPayload(b *testing.B) {
	for _, format := range []string{"kcat", "frame"} {
		b.Run(format, func(b *testing.B) {
			var input strings.Builder
			value := strings.Repeat("0123456789abcdef", 256)
			for i := 0; i < 5000; i++ {
				key := fmt.Sprintf("k%06d", i)
				if format == "kcat" {
					fmt.Fprintf(&input, "%s\t%d\t%s\n", key, len(value), value)
				} else {
					fmt.Fprintf(&input, "P %d %d\n%s%s", len(key), len(value), key, value)
				}
			}
			data := input.String()
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				run(b, filepath.Join(b.TempDir(), "db"), data, "apply", "kv", "--format", format)
			}
		})
	}
}
