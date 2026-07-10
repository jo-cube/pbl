package app

import (
	"errors"

	"github.com/jo-cube/pbl/internal/codec"
	"github.com/jo-cube/pbl/internal/store"
	"github.com/spf13/cobra"
)

func (c *cli) keysValuesCommand(mode string) *cobra.Command {
	var prefix, start, end string
	var limit int64
	cmd := &cobra.Command{
		Use:   mode + " <collection>",
		Short: keysValuesShort(mode),
		Long:  keysValuesLong(mode),
		Args:  collectionArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateLimit(limit); err != nil {
				return err
			}
			if prefix != "" && (start != "" || end != "") {
				return usagef("--prefix cannot be combined with range flags")
			}
			if (start == "") != (end == "") {
				return usagef("--range-start and --range-end must be used together")
			}
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer s.Close()
			fn := func(r store.Record) error {
				if mode == "keys" {
					return runtimeWrap(codec.WriteLine(c.stdout, r.Key))
				}
				return runtimeWrap(codec.WriteLine(c.stdout, r.Value))
			}
			opts := store.ScanOptions{Limit: limit}
			if prefix != "" {
				return storageWrap(s.Prefix(args[0], []byte(prefix), opts, fn))
			}
			if start != "" || end != "" {
				return storageWrap(s.Range(args[0], []byte(start), []byte(end), opts, fn))
			}
			return storageWrap(s.Scan(args[0], opts, fn))
		},
	}
	cmd.Flags().StringVar(&prefix, "prefix", "", "key prefix filter")
	cmd.Flags().StringVar(&start, "range-start", "", "inclusive range start")
	cmd.Flags().StringVar(&end, "range-end", "", "exclusive range end")
	cmd.Flags().Int64Var(&limit, "limit", 0, "max records; 0 means all")
	return cmd
}

func (c *cli) getManyCommand() *cobra.Command {
	var inputFormat, format, missing string
	var withKey bool
	var fields []string
	cmd := &cobra.Command{
		Use:   "get-many <collection>",
		Short: "Read keys from stdin and emit values",
		Long: `Read lookup keys from stdin and emit matching values in the same order.

Line input treats each line as a key. NDJSON input reads one or more --key-field
values from each object. Missing keys are skipped by default.`,
		Args: collectionArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOneOf("input-format", inputFormat, "line", "ndjson"); err != nil {
				return err
			}
			if err := validateOneOf("format", format, "raw", "kv", "ndjson"); err != nil {
				return err
			}
			if err := validateOneOf("missing", missing, "skip", "null", "error"); err != nil {
				return err
			}
			if err := validateNDJSONKeyFields(inputFormat, fields, ":"); err != nil {
				return err
			}
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer s.Close()
			return c.forInputRecords(inputFormat, fields, ":", func(rec codec.Record) error {
				value, err := s.Get(args[0], rec.Key)
				if errors.Is(err, store.ErrNotFound) {
					return c.handleMissing(missing, rec.Key, format, withKey)
				}
				if err != nil {
					return storageErr(err)
				}
				return c.writeRecord(rec.Key, value, format, withKey, true)
			})
		},
	}
	cmd.Flags().StringVar(&inputFormat, "input-format", "line", "line|ndjson input")
	cmd.Flags().StringVar(&format, "format", "raw", "raw|kv|ndjson output")
	cmd.Flags().StringVar(&missing, "missing", "skip", "skip|null|error for missing keys")
	cmd.Flags().BoolVar(&withKey, "with-key", false, "include key in output")
	cmd.Flags().StringArrayVar(&fields, "key-field", nil, "ndjson string key field; repeat for compound keys")
	return cmd
}

func (c *cli) delManyCommand() *cobra.Command {
	var inputFormat, batchBytesText string
	var fields []string
	var batchSize int
	var sync syncOptions
	cmd := &cobra.Command{
		Use:   "del-many <collection>",
		Short: "Read keys from stdin and delete them",
		Long: `Read keys from stdin and delete them from a collection in batches.

Line input treats each line as a key. NDJSON input reads one or more --key-field
values from each object. Success writes no stdout.`,
		Args: collectionArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOneOf("input-format", inputFormat, "line", "ndjson"); err != nil {
				return err
			}
			if err := validateSync(sync); err != nil {
				return err
			}
			if err := validateBatchSize(batchSize); err != nil {
				return err
			}
			if err := validateNDJSONKeyFields(inputFormat, fields, ":"); err != nil {
				return err
			}
			batchBytes, err := parseSize(batchBytesText)
			if err != nil {
				return usageErr(err)
			}
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer s.Close()
			b := s.NewBatch()
			defer func() { _ = b.Close() }()
			flush := func() error {
				if b.Count() == 0 {
					return nil
				}
				if err := b.Commit(store.WriteOptions{Sync: writeSync(sync, false)}); err != nil {
					return storageErr(err)
				}
				_ = b.Close()
				b = s.NewBatch()
				return nil
			}
			err = c.forInputRecords(inputFormat, fields, ":", func(rec codec.Record) error {
				if err := b.Delete(args[0], rec.Key); err != nil {
					return storageErr(err)
				}
				if b.Count() >= batchSize || b.ApproxBytes() >= batchBytes {
					return flush()
				}
				return nil
			})
			if err != nil {
				return err
			}
			return flush()
		},
	}
	cmd.Flags().StringVar(&inputFormat, "input-format", "line", "line|ndjson input")
	cmd.Flags().StringArrayVar(&fields, "key-field", nil, "ndjson string key field; repeat for compound keys")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "max records per batch")
	cmd.Flags().StringVar(&batchBytesText, "batch-bytes", "4MB", "approx bytes per batch")
	addSyncFlags(cmd, &sync)
	return cmd
}

func (c *cli) existsCommand() *cobra.Command {
	var inputFormat, missing string
	var fields []string
	var invert bool
	cmd := &cobra.Command{
		Use:   "exists <collection>",
		Short: "Filter stdin by key existence",
		Long: `Filter stdin records by whether their key exists in a collection.

Matching records are emitted unchanged, preserving input order. Use --invert to
emit missing records instead. --missing error fails on the first missing key.`,
		Args: collectionArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOneOf("input-format", inputFormat, "line", "ndjson"); err != nil {
				return err
			}
			if err := validateOneOf("missing", missing, "skip", "error"); err != nil {
				return err
			}
			if err := validateNDJSONKeyFields(inputFormat, fields, ":"); err != nil {
				return err
			}
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer s.Close()
			return c.forInputRecords(inputFormat, fields, ":", func(rec codec.Record) error {
				found, err := s.Has(args[0], rec.Key)
				if err != nil {
					return storageErr(err)
				}
				if !found && missing == "error" {
					return notFoundf("not found: %s", rec.Key)
				}
				if found != invert {
					return runtimeWrap(codec.WriteLine(c.stdout, rec.Raw))
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&inputFormat, "input-format", "line", "line|ndjson input")
	cmd.Flags().StringArrayVar(&fields, "key-field", nil, "ndjson string key field; repeat for compound keys")
	cmd.Flags().BoolVar(&invert, "invert", false, "pass missing records")
	cmd.Flags().StringVar(&missing, "missing", "skip", "skip|error for missing keys")
	return cmd
}

func (c *cli) lookupCommand(join bool) *cobra.Command {
	name := "lookup"
	inputDefault := "line"
	missingDefault := "skip"
	if join {
		name = "join"
		inputDefault = "ndjson"
		missingDefault = "null"
	}
	use := name + " <collection>"
	if join {
		use = "join <collection> --on <field> --as <field>"
	}
	var inputFormat, asField, missing, keySep, on string
	var fields []string
	cmd := &cobra.Command{
		Use:   use,
		Short: lookupShort(join),
		Long:  lookupLong(join),
		Args:  collectionArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if join {
				if on == "" {
					return usagef("join requires --on")
				}
				fields = append(fields, on)
				inputFormat = "ndjson"
			}
			if err := validateOneOf("input-format", inputFormat, "line", "ndjson"); err != nil {
				return err
			}
			if err := validateOneOf("missing", missing, "null", "skip", "error"); err != nil {
				return err
			}
			if inputFormat == "ndjson" && asField == "" {
				return usagef("ndjson lookup requires --as")
			}
			if err := validateNDJSONKeyFields(inputFormat, fields, keySep); err != nil {
				return err
			}
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer s.Close()
			return c.forInputRecords(inputFormat, fields, keySep, func(rec codec.Record) error {
				value, err := s.Get(args[0], rec.Key)
				if errors.Is(err, store.ErrNotFound) {
					switch missing {
					case "skip":
						return nil
					case "error":
						return notFoundf("not found: %s", rec.Key)
					case "null":
						return c.writeLookup(rec, nil, inputFormat, asField)
					default:
						return usagef("unknown missing policy %q", missing)
					}
				}
				if err != nil {
					return storageErr(err)
				}
				return c.writeLookup(rec, value, inputFormat, asField)
			})
		},
	}
	cmd.Flags().StringVar(&inputFormat, "input-format", inputDefault, "line|ndjson input")
	cmd.Flags().StringArrayVar(&fields, "key-field", nil, "lookup string key field; repeat for compound keys")
	cmd.Flags().StringVar(&keySep, "key-sep", ":", "one-byte compound key separator")
	cmd.Flags().StringVar(&asField, "as", "", "ndjson output field for stored value")
	cmd.Flags().StringVar(&missing, "missing", missingDefault, "null|skip|error for missing keys")
	if join {
		cmd.Flags().StringVar(&on, "on", "", "ndjson input join key field")
		_ = cmd.Flags().MarkHidden("input-format")
	}
	return cmd
}
