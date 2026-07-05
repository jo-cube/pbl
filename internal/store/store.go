package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cockroachdb/pebble/v2"
	"github.com/jo-cube/pbl/internal/keyenc"
)

const FormatVersion = 1

var ErrNotFound = errors.New("not found")

type WriteOptions struct {
	Sync bool
}

type ScanOptions struct {
	Limit int64
}

type Record struct {
	Key   []byte
	Value []byte
}

type Info struct {
	Path                 string
	StorageFormatVersion int
	CollectionCount      int
	CreatedAt            string
}

type Stats struct {
	Path     string
	Raw      string
	DiskUsed uint64
}

type Store struct {
	path string
	db   *pebble.DB
}

func Open(path string) (*Store, error) {
	db, err := pebble.Open(path, &pebble.Options{Logger: discardLogger{}})
	if err != nil {
		return nil, err
	}
	s := &Store{path: path, db: db}
	if err := s.checkVersionIfPresent(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

type discardLogger struct{}

func (discardLogger) Infof(string, ...interface{})  {}
func (discardLogger) Errorf(string, ...interface{}) {}
func (discardLogger) Fatalf(string, ...interface{}) {}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Init() error {
	version, closer, err := s.db.Get(keyenc.MetadataKey("format-version"))
	if errors.Is(err, pebble.ErrNotFound) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := s.db.Set(keyenc.MetadataKey("format-version"), []byte("1"), pebble.Sync); err != nil {
			return err
		}
		return s.db.Set(keyenc.MetadataKey("created-at"), []byte(now), pebble.Sync)
	}
	if err != nil {
		return err
	}
	defer closer.Close()
	if string(version) != "1" {
		return fmt.Errorf("unsupported storage format version %q", version)
	}
	return nil
}

func (s *Store) EnsureCollection(collection string) error {
	if err := ValidateCollection(collection); err != nil {
		return err
	}
	if err := s.Init(); err != nil {
		return err
	}
	key := keyenc.CollectionMetaKey(collection)
	_, closer, err := s.db.Get(key)
	if err == nil {
		return closer.Close()
	}
	if !errors.Is(err, pebble.ErrNotFound) {
		return err
	}
	meta := struct {
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
	}{collection, time.Now().UTC().Format(time.RFC3339Nano)}
	value, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return s.db.Set(key, value, pebble.Sync)
}

func (s *Store) ListCollections() ([]string, error) {
	prefix := keyenc.CollectionMetaPrefix()
	upper, _ := keyenc.NextPrefix(prefix)
	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: upper})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []string
	for valid := iter.First(); valid; valid = iter.Next() {
		name := strings.TrimPrefix(string(iter.Key()[1:]), "collection/")
		out = append(out, name)
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) Info() (Info, error) {
	if err := s.checkVersionIfPresent(); err != nil {
		return Info{}, err
	}
	collections, err := s.ListCollections()
	if err != nil {
		return Info{}, err
	}
	created := ""
	value, closer, err := s.db.Get(keyenc.MetadataKey("created-at"))
	if err == nil {
		created = string(value)
		err = closer.Close()
	}
	if err != nil && !errors.Is(err, pebble.ErrNotFound) {
		return Info{}, err
	}
	version := 0
	v, closer, err := s.db.Get(keyenc.MetadataKey("format-version"))
	if err == nil {
		version, _ = strconv.Atoi(string(v))
		err = closer.Close()
	}
	if err != nil && !errors.Is(err, pebble.ErrNotFound) {
		return Info{}, err
	}
	if version == 0 && created != "" {
		version = FormatVersion
	}
	return Info{s.path, version, len(collections), created}, nil
}

func (s *Store) Stats() (Stats, error) {
	m := s.db.Metrics()
	return Stats{Path: s.path, Raw: m.String(), DiskUsed: m.DiskSpaceUsage()}, nil
}

func (s *Store) Put(collection string, key, value []byte, opts WriteOptions) error {
	if err := s.EnsureCollection(collection); err != nil {
		return err
	}
	return s.db.Set(keyenc.DataKey(collection, key), value, pebbleWriteOptions(opts))
}

func (s *Store) Get(collection string, key []byte) ([]byte, error) {
	value, closer, err := s.db.Get(keyenc.DataKey(collection, key))
	if errors.Is(err, pebble.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	return append([]byte(nil), value...), nil
}

func (s *Store) Has(collection string, key []byte) (bool, error) {
	_, closer, err := s.db.Get(keyenc.DataKey(collection, key))
	if errors.Is(err, pebble.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, closer.Close()
}

func (s *Store) Delete(collection string, key []byte, opts WriteOptions) error {
	return s.db.Delete(keyenc.DataKey(collection, key), pebbleWriteOptions(opts))
}

func (s *Store) Scan(collection string, opts ScanOptions, fn func(Record) error) error {
	lower, upper := keyenc.CollectionBounds(collection)
	return s.scan(lower, upper, opts, fn)
}

func (s *Store) Prefix(collection string, prefix []byte, opts ScanOptions, fn func(Record) error) error {
	lower, upper := keyenc.PrefixBounds(collection, prefix)
	return s.scan(lower, upper, opts, fn)
}

func (s *Store) Range(collection string, start, end []byte, opts ScanOptions, fn func(Record) error) error {
	lower, upper := keyenc.RangeBounds(collection, start, end)
	return s.scan(lower, upper, opts, fn)
}

func (s *Store) scan(lower, upper []byte, opts ScanOptions, fn func(Record) error) error {
	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return err
	}
	defer iter.Close()
	var n int64
	for valid := iter.First(); valid; valid = iter.Next() {
		if opts.Limit > 0 && n >= opts.Limit {
			break
		}
		_, userKey, ok := keyenc.DecodeDataKey(iter.Key())
		if !ok {
			continue
		}
		value := append([]byte(nil), iter.Value()...)
		if err := fn(Record{Key: userKey, Value: value}); err != nil {
			return err
		}
		n++
	}
	return iter.Error()
}

func (s *Store) NewBatch() *Batch {
	return &Batch{store: s, batch: s.db.NewBatch()}
}

func (s *Store) checkVersionIfPresent() error {
	value, closer, err := s.db.Get(keyenc.MetadataKey("format-version"))
	if errors.Is(err, pebble.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	defer closer.Close()
	if !bytes.Equal(value, []byte("1")) {
		return fmt.Errorf("unsupported storage format version %q", value)
	}
	return nil
}

func ValidateCollection(name string) error {
	if name == "" {
		return fmt.Errorf("collection name is required")
	}
	if !utf8.ValidString(name) {
		return fmt.Errorf("collection name must be UTF-8")
	}
	for _, r := range name {
		if r == 0 || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-') {
			return fmt.Errorf("invalid collection name %q", name)
		}
	}
	return nil
}

func pebbleWriteOptions(opts WriteOptions) *pebble.WriteOptions {
	return &pebble.WriteOptions{Sync: opts.Sync}
}
