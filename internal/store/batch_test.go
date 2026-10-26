package store

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/jo-cube/pbl/internal/keyenc"
)

func TestBatchCoalescingPreservesOrderedMutations(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, c := range []string{"a", "b"} {
		if err = s.EnsureCollection(c); err != nil {
			t.Fatal(err)
		}
	}
	expected := map[string]map[string][]byte{"a": {}, "b": {}}
	rng := rand.New(rand.NewPCG(1, 2))
	var key [4]byte
	var value [16]byte
	for batch := 0; batch < 6; batch++ {
		b := s.NewBatch()
		wantCoalesce := batch%2 == 0
		records := 512
		if batch == 4 {
			records = 8192
		}
		for i := 0; i < records; i++ {
			collection := []string{"a", "b"}[rng.IntN(2)]
			id := i
			if wantCoalesce {
				id = rng.IntN(8)
			}
			key = [4]byte{byte(id >> 8), byte(id), 0, 255}
			if i%3 == 0 {
				err = b.Delete(collection, key[:])
				delete(expected[collection], string(key[:]))
			} else {
				copy(value[:], fmt.Sprintf("%016d", batch*512+i))
				v := value[:]
				if i%7 == 0 {
					v = v[:0]
				}
				err = b.Put(collection, key[:], v)
				expected[collection][string(key[:])] = bytes.Clone(v)
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		if got := b.shouldCoalesce(); got != wantCoalesce {
			t.Fatalf("batch %d coalesces=%v want %v", batch, got, wantCoalesce)
		}
		count, size := b.Count(), b.ApproxBytes()
		if err = b.Commit(WriteOptions{Sync: batch%2 == 0}); err != nil {
			t.Fatal(err)
		}
		if b.Count() != count || b.ApproxBytes() != size {
			t.Fatal("coalescing changed input counters")
		}
		if err = b.Close(); err != nil {
			t.Fatal(err)
		}
		for collection, want := range expected {
			seen := 0
			err = s.Scan(collection, ScanOptions{}, func(r Record) error {
				v, ok := want[string(r.Key)]
				if !ok || !bytes.Equal(v, r.Value) {
					t.Fatalf("batch %d %s key %x value %q want %q", batch, collection, r.Key, r.Value, v)
				}
				seen++
				return nil
			})
			if err != nil || seen != len(want) {
				t.Fatalf("batch %d scan count=%d want=%d err=%v", batch, seen, len(want), err)
			}
		}
	}
}

func TestCoalescedBatchKeepsSnapshots(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Put("a", []byte("key"), []byte("old"), WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	snapshot := s.db.NewSnapshot()
	defer snapshot.Close()
	b := s.NewBatch()
	defer b.Close()
	for i := 0; i < 128; i++ {
		if err = b.Put("a", []byte("key"), []byte("new")); err != nil {
			t.Fatal(err)
		}
	}
	if err = b.Commit(WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	key := keyenc.DataKey("a", []byte("key"))
	value, closer, err := snapshot.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "old" {
		t.Fatalf("snapshot value %q", value)
	}
	if err = closer.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get("a", []byte("key")); err != nil || string(got) != "new" {
		t.Fatalf("value %q err=%v", got, err)
	}
}

func TestFlushBulkKeepsSmallTailsAndFlushesLargeTails(t *testing.T) {
	for _, large := range []bool{false, true} {
		t.Run(fmt.Sprint(large), func(t *testing.T) {
			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err = s.EnsureCollection("events"); err != nil {
				t.Fatal(err)
			}
			records, size := 10, 16
			if large {
				records, size = 1200, 8192
			}
			value := bytes.Repeat([]byte("v"), size)
			for off := 0; off < records; off += 400 {
				b := s.NewBatch()
				for i := off; i < min(off+400, records); i++ {
					if err = b.Put("events", []byte(fmt.Sprint(i)), value); err != nil {
						t.Fatal(err)
					}
				}
				if err = b.Commit(WriteOptions{}); err != nil {
					t.Fatal(err)
				}
				if err = b.Close(); err != nil {
					t.Fatal(err)
				}
			}
			before := s.db.Metrics()
			if (before.WAL.Size > 4<<20) != large {
				t.Fatalf("invalid test tail size %d", before.WAL.Size)
			}
			if err = s.FlushBulk(); err != nil {
				t.Fatal(err)
			}
			after := s.db.Metrics()
			if large && after.WAL.Size > 4<<20 {
				t.Fatalf("large tail remains: %d", after.WAL.Size)
			}
			if !large && after.Flush.Count != before.Flush.Count {
				t.Fatal("flushed small tail")
			}
			count := 0
			err = s.Scan("events", ScanOptions{}, func(r Record) error {
				if !bytes.Equal(r.Value, value) {
					t.Fatalf("wrong value for %q", r.Key)
				}
				count++
				return nil
			})
			if err != nil || count != records {
				t.Fatalf("records=%d want=%d err=%v", count, records, err)
			}
		})
	}
}
