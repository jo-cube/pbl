package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cockroachdb/pebble/v2"
	"github.com/cockroachdb/pebble/v2/bloom"
	"github.com/jo-cube/pbl/internal/keyenc"
)

const FormatVersion = 1

var (
	ErrNotFound           = errors.New("not found")
	ErrAlreadyInitialized = errors.New("database is already initialized")
	ErrUninitialized      = errors.New("database is not initialized")
	ErrUnmarkedDatabase   = errors.New("non-empty Pebble database is not a pbl database")
)

type WriteOptions struct {
	Sync bool
}

type ScanOptions struct {
	Prefix, Start, End []byte // Nil Start and End leave that side unbounded.
	Limit              int64
	Reverse, KeysOnly  bool
}

func (o ScanOptions) Validate() error {
	if o.Limit < 0 {
		return fmt.Errorf("limit must be greater than or equal to 0")
	}
	if o.End != nil && bytes.Compare(o.Start, o.End) > 0 {
		return fmt.Errorf("start must be less than or equal to end")
	}
	return nil
}

// Record slices passed to scan callbacks are valid only until the callback returns.
type Record struct {
	Key   []byte
	Value []byte
}

type Info struct {
	Path                 string `json:"path"`
	StorageFormatVersion int    `json:"storage_format_version"`
	CollectionCount      int    `json:"collection_count"`
	CreatedAt            string `json:"created_at"`
}

type Stats struct {
	Path     string `json:"path"`
	DiskUsed uint64 `json:"disk_used"`
	Raw      string `json:"raw,omitempty"`
}

type Store struct {
	path string
	db   *pebble.DB
}

func Open(path string) (*Store, error) {
	return open(path, false)
}

func OpenExisting(path string) (*Store, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Name() == "CURRENT" || strings.HasPrefix(entry.Name(), "MANIFEST-") {
			return open(path, true)
		}
	}
	return nil, fmt.Errorf("%w: %s", pebble.ErrDBDoesNotExist, path)
}

func open(path string, mustExist bool) (_ *Store, err error) {
	opts := &pebble.Options{Logger: discardLogger{}, ErrorIfNotExists: mustExist}
	// L0's table filter is inherited by later levels.
	opts.Levels[0].FilterPolicy = bloom.FilterPolicy(10)
	db, err := pebble.Open(path, opts)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, db.Close())
		}
	}()
	s := &Store{path: path, db: db}
	present, err := s.checkMetadata()
	if err != nil {
		return nil, err
	}
	if !present {
		empty, err := s.empty()
		if err != nil {
			return nil, err
		}
		if !empty {
			return nil, ErrUnmarkedDatabase
		}
		if mustExist {
			return nil, ErrUninitialized
		}
	}
	return s, nil
}

type discardLogger struct{}

func (discardLogger) Infof(string, ...interface{})  {}
func (discardLogger) Errorf(string, ...interface{}) {}
func (discardLogger) Fatalf(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Init() (err error) {
	version, closer, err := s.db.Get(keyenc.MetadataKey("format-version"))
	if errors.Is(err, pebble.ErrNotFound) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		b := s.db.NewBatch()
		defer func() { err = errors.Join(err, b.Close()) }()
		if err := b.Set(keyenc.MetadataKey("format-version"), []byte(strconv.Itoa(FormatVersion)), nil); err != nil {
			return err
		}
		if err := b.Set(keyenc.MetadataKey("created-at"), []byte(now), nil); err != nil {
			return err
		}
		return b.Commit(pebble.Sync)
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closer.Close()) }()
	if string(version) != strconv.Itoa(FormatVersion) {
		return fmt.Errorf("unsupported storage format version %q", version)
	}
	return ErrAlreadyInitialized
}

func (s *Store) EnsureCollection(collection string) error {
	if err := ValidateCollection(collection); err != nil {
		return err
	}
	if err := s.Init(); err != nil && !errors.Is(err, ErrAlreadyInitialized) {
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

func (s *Store) ListCollections() (out []string, err error) {
	prefix := keyenc.CollectionMetaPrefix()
	upper, _ := keyenc.NextPrefix(prefix)
	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: upper})
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, iter.Close()) }()
	for valid := iter.First(); valid; valid = iter.Next() {
		name := strings.TrimPrefix(string(iter.Key()[1:]), "collection/")
		out = append(out, name)
	}
	if err = iter.Error(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) Info() (Info, error) {
	if err := s.RequireInitialized(); err != nil {
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
	return Info{Path: s.path, StorageFormatVersion: version, CollectionCount: len(collections), CreatedAt: created}, nil
}

func (s *Store) Stats(includeRaw bool) (Stats, error) {
	m := s.db.Metrics()
	stats := Stats{Path: s.path, DiskUsed: m.DiskSpaceUsage()}
	if includeRaw {
		stats.Raw = m.String()
	}
	return stats, nil
}

func (s *Store) Put(collection string, key, value []byte, opts WriteOptions) error {
	if err := s.EnsureCollection(collection); err != nil {
		return err
	}
	return s.db.Set(keyenc.DataKey(collection, key), value, pebbleWriteOptions(opts))
}

func (s *Store) Get(collection string, key []byte) (out []byte, err error) {
	if err := ValidateCollection(collection); err != nil {
		return nil, err
	}
	value, closer, err := s.db.Get(keyenc.DataKey(collection, key))
	if errors.Is(err, pebble.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, closer.Close()) }()
	return append([]byte(nil), value...), nil
}

func (s *Store) Has(collection string, key []byte) (bool, error) {
	if err := ValidateCollection(collection); err != nil {
		return false, err
	}
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
	if err := ValidateCollection(collection); err != nil {
		return err
	}
	return s.db.Delete(keyenc.DataKey(collection, key), pebbleWriteOptions(opts))
}

// Drop removes a collection's records and metadata in one atomic batch.
func (s *Store) Drop(collection string, opts WriteOptions) (err error) {
	if err := ValidateCollection(collection); err != nil {
		return err
	}
	lower, upper := keyenc.CollectionBounds(collection)
	b := s.db.NewBatch()
	defer func() { err = errors.Join(err, b.Close()) }()
	if err := b.DeleteRange(lower, upper, nil); err != nil {
		return err
	}
	if err := b.Delete(keyenc.CollectionMetaKey(collection), nil); err != nil {
		return err
	}
	return b.Commit(pebbleWriteOptions(opts))
}

func (s *Store) Scan(collection string, opts ScanOptions, fn func(Record) error) (err error) {
	if err := ValidateCollection(collection); err != nil {
		return err
	}
	if err := opts.Validate(); err != nil {
		return err
	}
	lower, upper := keyenc.ScanBounds(collection, opts.Prefix, opts.Start, opts.End)
	if bytes.Compare(lower, upper) >= 0 {
		return nil
	}
	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, iter.Close()) }()
	first, next := iter.First, iter.Next
	if opts.Reverse {
		first, next = iter.Last, iter.Prev
	}
	var n int64
	for valid := first(); valid; valid = next() {
		_, userKey, ok := keyenc.DecodeDataKeyView(iter.Key())
		if !ok {
			continue
		}
		var value []byte
		if !opts.KeysOnly {
			value, err = iter.ValueAndErr()
			if err != nil {
				return err
			}
		}
		if err := fn(Record{Key: userKey, Value: value}); err != nil {
			return err
		}
		n++
		if opts.Limit > 0 && n >= opts.Limit {
			break
		}
	}
	return iter.Error()
}

// NewBatch writes data only; callers adding puts must first call EnsureCollection.
func (s *Store) NewBatch() *Batch {
	return &Batch{batch: s.db.NewBatch()}
}

func (s *Store) RequireInitialized() error {
	present, err := s.checkMetadata()
	if err != nil {
		return err
	}
	if !present {
		return ErrUninitialized
	}
	return nil
}

func (s *Store) checkMetadata() (bool, error) {
	value, closer, err := s.db.Get(keyenc.MetadataKey("format-version"))
	if errors.Is(err, pebble.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	version := string(value)
	if err := closer.Close(); err != nil {
		return false, err
	}
	if version != strconv.Itoa(FormatVersion) {
		return false, fmt.Errorf("unsupported storage format version %q", version)
	}
	created, closer, err := s.db.Get(keyenc.MetadataKey("created-at"))
	if errors.Is(err, pebble.ErrNotFound) {
		return false, fmt.Errorf("missing required metadata %q", "created-at")
	}
	if err != nil {
		return false, err
	}
	createdAt := string(created)
	if err := closer.Close(); err != nil {
		return false, err
	}
	if _, err := time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return false, fmt.Errorf("invalid created-at metadata: %w", err)
	}
	return true, nil
}

func (s *Store) empty() (empty bool, err error) {
	iter, err := s.db.NewIter(nil)
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, iter.Close()) }()
	empty = !iter.First()
	if err = iter.Error(); err != nil {
		return false, err
	}
	return empty, nil
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
