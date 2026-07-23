package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cockroachdb/pebble/v2"
	"github.com/jo-cube/pbl/internal/keyenc"
)

func TestStorePutGetDeleteScan(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Put("users", []byte("b"), []byte("B"), WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("users", []byte("a"), []byte("A"), WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	value, err := s.Get("users", []byte("a"))
	if err != nil || string(value) != "A" {
		t.Fatalf("Get = %q, %v", value, err)
	}
	var got []string
	err = s.Scan("users", ScanOptions{}, func(r Record) error {
		got = append(got, string(r.Key)+"="+string(r.Value))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a=A", "b=B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("scan = %v, want %v", got, want)
	}
	got = nil
	if err := s.ScanKeys("users", func(key []byte) error {
		got = append(got, string(key))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("key scan = %v, want %v", got, want)
	}
	if err := s.Delete("users", []byte("a"), WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("users", []byte("a")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing get err = %v", err)
	}
}

func TestBatchWritesAcrossCollections(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	b := s.NewBatch()
	defer b.Close()
	for _, collection := range []string{"users", "teams", "users"} {
		if err := b.Put(collection, []byte(collection), []byte("value")); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Commit(WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, collection := range []string{"users", "teams"} {
		if value, err := s.Get(collection, []byte(collection)); err != nil || string(value) != "value" {
			t.Fatalf("Get(%q) = %q, %v", collection, value, err)
		}
	}
}

func TestInitAndRequireInitialized(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.RequireInitialized(); !errors.Is(err, ErrUninitialized) {
		t.Fatalf("RequireInitialized before init = %v", err)
	}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if err := s.Init(); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("second Init = %v", err)
	}
	if err := s.RequireInitialized(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRejectsNonEmptyUnmarkedPebbleDatabase(t *testing.T) {
	path := t.TempDir()
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Set([]byte("other-app"), []byte("value"), pebble.Sync); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); !errors.Is(err, ErrUnmarkedDatabase) {
		t.Fatalf("Open unmarked database = %v", err)
	}
}

func TestOpenExistingDoesNotCreateDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenExisting(path); !errors.Is(err, pebble.ErrDBDoesNotExist) {
		t.Fatalf("OpenExisting empty directory = %v", err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("OpenExisting created %d files", len(entries))
	}
}

func TestOpenExistingRequiresInitializedDatabase(t *testing.T) {
	path := t.TempDir()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenExisting(path); !errors.Is(err, ErrUninitialized) {
		t.Fatalf("OpenExisting uninitialized database = %v", err)
	}
}

func TestOpenRejectsIncompleteMetadata(t *testing.T) {
	for _, tc := range []struct {
		name      string
		createdAt string
		want      string
	}{
		{"missing created-at", "", "missing required metadata"},
		{"invalid created-at", "not-a-time", "invalid created-at metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir()
			db, err := pebble.Open(path, &pebble.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Set(keyenc.MetadataKey("format-version"), []byte("1"), pebble.Sync); err != nil {
				t.Fatal(err)
			}
			if tc.createdAt != "" {
				if err := db.Set(keyenc.MetadataKey("created-at"), []byte(tc.createdAt), pebble.Sync); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(path); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Open incomplete metadata = %v", err)
			}
		})
	}
}

func TestOpenRejectsUnsupportedFormatVersion(t *testing.T) {
	path := t.TempDir()
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Set(keyenc.MetadataKey("format-version"), []byte("2"), pebble.Sync); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "unsupported storage format") {
		t.Fatalf("Open unsupported database = %v", err)
	}
}

func TestDiscardLoggerFatalPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Fatalf did not panic")
		}
	}()
	discardLogger{}.Fatalf("fatal %s", "invariant")
}

func TestCollectionsFromMetadata(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, name := range []string{"bb", "a", "ccc"} {
		if err := s.EnsureCollection(name); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListCollections()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "bb", "ccc"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("collections = %v, want %v", got, want)
	}
}

func TestReadOperationsValidateCollection(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Get("bad/name", []byte("k")); err == nil || !strings.Contains(err.Error(), "invalid collection") {
		t.Fatalf("Get invalid collection err = %v", err)
	}
	if _, err := s.Has("bad/name", []byte("k")); err == nil || !strings.Contains(err.Error(), "invalid collection") {
		t.Fatalf("Has invalid collection err = %v", err)
	}
	if err := s.Scan("bad/name", ScanOptions{}, func(Record) error { return nil }); err == nil || !strings.Contains(err.Error(), "invalid collection") {
		t.Fatalf("Scan invalid collection err = %v", err)
	}
	if err := s.ScanKeys("bad/name", func([]byte) error { return nil }); err == nil || !strings.Contains(err.Error(), "invalid collection") {
		t.Fatalf("ScanKeys invalid collection err = %v", err)
	}
}
