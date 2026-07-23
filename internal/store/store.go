package store

import (
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
	Limit int64
}

// Record slices passed to scan callbacks are valid only until the callback returns.
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

func open(path string, mustExist bool) (*Store, error) {
	opts := &pebble.Options{Logger: discardLogger{}, ErrorIfNotExists: mustExist}
	// L0's table filter is inherited by later levels.
	opts.Levels[0].FilterPolicy = bloom.FilterPolicy(10)
	db, err := pebble.Open(path, opts)
	if err != nil {
		return nil, err
	}
	s := &Store{path: path, db: db}
	present, err := s.checkMetadata()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if !present {
		empty, err := s.empty()
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		if !empty {
			_ = db.Close()
			return nil, ErrUnmarkedDatabase
		}
		if mustExist {
			_ = db.Close()
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
		defer b.Close()
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

func (s *Store) Scan(collection string, opts ScanOptions, fn func(Record) error) error {
	if err := ValidateCollection(collection); err != nil {
		return err
	}
	lower, upper := keyenc.CollectionBounds(collection)
	return s.scan(lower, upper, opts, fn)
}

// ScanKeys visits keys in order. Each key is valid only until fn returns.
func (s *Store) ScanKeys(collection string, fn func([]byte) error) (err error) {
	if err := ValidateCollection(collection); err != nil {
		return err
	}
	lower, upper := keyenc.CollectionBounds(collection)
	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, iter.Close()) }()
	for valid := iter.First(); valid; valid = iter.Next() {
		_, userKey, ok := keyenc.DecodeDataKeyView(iter.Key())
		if ok {
			if err := fn(userKey); err != nil {
				return err
			}
		}
	}
	return iter.Error()
}

func (s *Store) Prefix(collection string, prefix []byte, opts ScanOptions, fn func(Record) error) error {
	if err := ValidateCollection(collection); err != nil {
		return err
	}
	lower, upper := keyenc.PrefixBounds(collection, prefix)
	return s.scan(lower, upper, opts, fn)
}

func (s *Store) Range(collection string, start, end []byte, opts ScanOptions, fn func(Record) error) error {
	if err := ValidateCollection(collection); err != nil {
		return err
	}
	lower, upper := keyenc.RangeBounds(collection, start, end)
	return s.scan(lower, upper, opts, fn)
}

func (s *Store) scan(lower, upper []byte, opts ScanOptions, fn func(Record) error) (err error) {
	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, iter.Close()) }()
	var n int64
	for valid := iter.First(); valid; valid = iter.Next() {
		if opts.Limit > 0 && n >= opts.Limit {
			break
		}
		_, userKey, ok := keyenc.DecodeDataKeyView(iter.Key())
		if !ok {
			continue
		}
		if err := fn(Record{Key: userKey, Value: iter.Value()}); err != nil {
			return err
		}
		n++
	}
	return iter.Error()
}

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
