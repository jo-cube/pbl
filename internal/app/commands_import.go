package app

import (
	"fmt"
	"io"
	"time"

	"github.com/jo-cube/pbl/internal/codec"
	"github.com/jo-cube/pbl/internal/store"
	"github.com/spf13/cobra"
)

func (c *cli) importCommand() *cobra.Command {
	var format, keyMode, key, keySep, batchBytesText string
	var fields []string
	var batchSize int
	var sync syncOptions
	var replace, ignoreDup, failDup bool
	cmd := &cobra.Command{
		Use:   "import <collection>",
		Short: "Import records from stdin",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format == "" {
				return usagef("usage: pbl import <collection> --format <format>")
			}
			if err := validateOneOf("format", format, "kv", "line", "ndjson", "raw"); err != nil {
				return err
			}
			if err := validateOneOf("key-mode", keyMode, "value", "line-number"); err != nil {
				return err
			}
			if err := validateSync(sync); err != nil {
				return err
			}
			if err := validateBatchSize(batchSize); err != nil {
				return err
			}
			if replace && (ignoreDup || failDup) {
				return usagef("--replace cannot be combined with duplicate handling flags")
			}
			if ignoreDup && failDup {
				return usagef("--ignore-duplicates and --fail-on-duplicate cannot both be set")
			}
			batchBytes, err := parseSize(batchBytesText)
			if err != nil {
				return usageErr(err)
			}
			s, err := c.open()
			if err != nil {
				return err
			}
			defer s.Close()
			collection := args[0]
			if err := s.EnsureCollection(collection); err != nil {
				return storageErr(err)
			}
			writeOpts := store.WriteOptions{Sync: writeSync(sync, false)}
			seen := map[string]struct{}{}
			shouldAdd := func(rec codec.Record) (bool, error) {
				if len(rec.Key) == 0 {
					return false, badInputf("line %d: empty key", rec.Line)
				}
				if ignoreDup || failDup {
					k := string(rec.Key)
					if _, ok := seen[k]; ok {
						if ignoreDup {
							return false, nil
						}
						return false, badInputf("line %d: duplicate key", rec.Line)
					}
					found, err := s.Has(collection, rec.Key)
					if err != nil {
						return false, storageErr(err)
					}
					if found && ignoreDup {
						return false, nil
					}
					if found && failDup {
						return false, badInputf("line %d: duplicate key", rec.Line)
					}
					seen[k] = struct{}{}
				}
				return true, nil
			}
			switch format {
			case "raw":
				if key == "" {
					return usagef("raw import requires --key")
				}
				if ignoreDup || failDup {
					found, err := s.Has(collection, []byte(key))
					if err != nil {
						return storageErr(err)
					}
					if found && ignoreDup {
						return nil
					}
					if found && failDup {
						return badInputf("duplicate key")
					}
				}
				value, err := io.ReadAll(c.stdin)
				if err != nil {
					return runtimeErr(err)
				}
				return storageWrap(s.Put(collection, []byte(key), value, writeOpts))
			case "kv", "line", "ndjson":
				return c.importRecords(s, collection, format, keyMode, fields, keySep, batchSize, batchBytes, writeOpts, shouldAdd)
			default:
				return usagef("unknown format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "kv|line|ndjson|raw")
	cmd.Flags().StringVar(&keyMode, "key-mode", "value", "value|line-number")
	cmd.Flags().StringVar(&key, "key", "", "raw key")
	cmd.Flags().StringVar(&keySep, "key-sep", ":", "compound key separator")
	cmd.Flags().StringArrayVar(&fields, "key-field", nil, "ndjson key field")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "records per batch")
	cmd.Flags().StringVar(&batchBytesText, "batch-bytes", "4MB", "bytes per batch")
	cmd.Flags().BoolVar(&replace, "replace", false, "replace existing values")
	cmd.Flags().BoolVar(&ignoreDup, "ignore-duplicates", false, "keep existing values")
	cmd.Flags().BoolVar(&failDup, "fail-on-duplicate", false, "fail on duplicate")
	addSyncFlags(cmd, &sync)
	return cmd
}

func (c *cli) importRecords(s *store.Store, collection, format, keyMode string, fields []string, keySep string, batchSize, batchBytes int, writeOpts store.WriteOptions, before func(codec.Record) (bool, error)) error {
	b := s.NewBatch()
	defer func() { _ = b.Close() }()
	flush := func() error {
		if b.Count() == 0 {
			return nil
		}
		if err := b.Commit(writeOpts); err != nil {
			return storageErr(err)
		}
		_ = b.Close()
		b = s.NewBatch()
		return nil
	}
	add := func(rec codec.Record) error {
		ok, err := before(rec)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := b.Put(collection, rec.Key, rec.Value); err != nil {
			return storageErr(err)
		}
		if b.Count() >= batchSize || b.ApproxBytes() >= batchBytes {
			return flush()
		}
		return nil
	}
	var err error
	switch format {
	case "kv":
		err = codec.ReadKVRecords(c.stdin, add)
	case "line":
		err = codec.ReadLineRecords(c.stdin, keyMode, add)
	case "ndjson":
		err = codec.ReadNDJSONRecords(c.stdin, fields, keySep, add)
	}
	if err != nil {
		return badInputErr(err)
	}
	return flush()
}

func (c *cli) applyCommand() *cobra.Command {
	var format, batchBytesText string
	var batchSize int
	var sync syncOptions
	var stats bool
	cmd := &cobra.Command{
		Use:   "apply <collection>",
		Short: "Apply put/delete records from stdin",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format == "" {
				return usagef("usage: pbl apply <collection> --format <format>")
			}
			if err := validateOneOf("format", format, "kcat", "frame"); err != nil {
				return err
			}
			if err := validateSync(sync); err != nil {
				return err
			}
			if err := validateBatchSize(batchSize); err != nil {
				return err
			}
			batchBytes, err := parseSize(batchBytesText)
			if err != nil {
				return usageErr(err)
			}
			s, err := c.open()
			if err != nil {
				return err
			}
			defer s.Close()
			collection := args[0]
			if err := s.EnsureCollection(collection); err != nil {
				return storageErr(err)
			}
			result, err := c.applyRecords(s, collection, format, batchSize, batchBytes, store.WriteOptions{Sync: writeSync(sync, false)})
			if err != nil {
				return err
			}
			if stats && !c.quiet {
				fmt.Fprintf(c.stderr, "pbl: applied records=%d puts=%d deletes=%d batches=%d bytes=%d duration=%s\n", result.records, result.puts, result.deletes, result.batches, result.bytes, result.elapsed.Round(time.Millisecond))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "kcat|frame")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "records per batch")
	cmd.Flags().StringVar(&batchBytesText, "batch-bytes", "4MB", "bytes per batch")
	cmd.Flags().BoolVar(&stats, "stats", false, "write ingest stats to stderr")
	addSyncFlags(cmd, &sync)
	return cmd
}

type applyStats struct {
	records int64
	puts    int64
	deletes int64
	batches int64
	bytes   int64
	elapsed time.Duration
}

func (c *cli) applyRecords(s *store.Store, collection, format string, batchSize, batchBytes int, writeOpts store.WriteOptions) (applyStats, error) {
	start := time.Now()
	var result applyStats
	b := s.NewBatch()
	defer func() { _ = b.Close() }()
	flush := func() error {
		if b.Count() == 0 {
			return nil
		}
		if err := b.Commit(writeOpts); err != nil {
			return storageErr(err)
		}
		result.batches++
		_ = b.Close()
		b = s.NewBatch()
		return nil
	}
	add := func(rec codec.ApplyRecord) error {
		if len(rec.Key) == 0 {
			return badInputf("record %d: empty key", rec.Line)
		}
		var err error
		if rec.Delete {
			err = b.Delete(collection, rec.Key)
			result.deletes++
		} else {
			err = b.Put(collection, rec.Key, rec.Value)
			result.puts++
		}
		if err != nil {
			return storageErr(err)
		}
		result.records++
		result.bytes = rec.Bytes
		if b.Count() >= batchSize || b.ApproxBytes() >= batchBytes {
			return flush()
		}
		return nil
	}
	var err error
	switch format {
	case "kcat":
		err = codec.ReadKcatApplyRecords(c.stdin, add)
	case "frame":
		err = codec.ReadFrameApplyRecords(c.stdin, add)
	}
	if err != nil {
		return result, badInputErr(err)
	}
	if err := flush(); err != nil {
		return result, err
	}
	result.elapsed = time.Since(start)
	return result, nil
}
