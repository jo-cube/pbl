package store

import (
	"errors"
	"reflect"
	"strings"
	"testing"
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
	if err := s.Delete("users", []byte("a"), WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("users", []byte("a")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing get err = %v", err)
	}
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
}
