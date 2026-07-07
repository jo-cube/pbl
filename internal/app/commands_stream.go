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
		Args:  exactArgs(1),
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
					return codec.WriteLine(c.stdout, r.Key)
				}
				return codec.WriteLine(c.stdout, r.Value)
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
	cmd.Flags().StringVar(&prefix, "prefix", "", "prefix")
	cmd.Flags().StringVar(&start, "range-start", "", "range start")
	cmd.Flags().StringVar(&end, "range-end", "", "range end")
	cmd.Flags().Int64Var(&limit, "limit", 0, "max records")
	return cmd
}

func (c *cli) getManyCommand() *cobra.Command {
	var inputFormat, format, missing string
	var withKey bool
	var fields []string
	cmd := &cobra.Command{
		Use:   "get-many <collection>",
		Short: "Read keys from stdin and emit values",
		Args:  exactArgs(1),
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
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer s.Close()
			return c.forInputKeys(inputFormat, fields, ":", func(rec codec.Record) error {
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
	cmd.Flags().StringVar(&inputFormat, "input-format", "line", "line|ndjson")
	cmd.Flags().StringVar(&format, "format", "raw", "raw|kv|ndjson")
	cmd.Flags().StringVar(&missing, "missing", "skip", "skip|null|error")
	cmd.Flags().BoolVar(&withKey, "with-key", false, "include key")
	cmd.Flags().StringArrayVar(&fields, "key-field", nil, "ndjson key field")
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
		Args:  exactArgs(1),
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
			err = c.forInputKeys(inputFormat, fields, ":", func(rec codec.Record) error {
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
	cmd.Flags().StringVar(&inputFormat, "input-format", "line", "line|ndjson")
	cmd.Flags().StringArrayVar(&fields, "key-field", nil, "ndjson key field")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "records per batch")
	cmd.Flags().StringVar(&batchBytesText, "batch-bytes", "4MB", "bytes per batch")
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
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOneOf("input-format", inputFormat, "line", "ndjson"); err != nil {
				return err
			}
			if err := validateOneOf("missing", missing, "skip", "error"); err != nil {
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
					return codec.WriteLine(c.stdout, rec.Raw)
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&inputFormat, "input-format", "line", "line|ndjson")
	cmd.Flags().StringArrayVar(&fields, "key-field", nil, "ndjson key field")
	cmd.Flags().BoolVar(&invert, "invert", false, "pass missing records")
	cmd.Flags().StringVar(&missing, "missing", "skip", "skip|error")
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
	var inputFormat, asField, missing, keySep, on string
	var fields []string
	cmd := &cobra.Command{
		Use:   name + " <collection>",
		Short: lookupShort(join),
		Args:  exactArgs(1),
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
	cmd.Flags().StringVar(&inputFormat, "input-format", inputDefault, "line|ndjson")
	cmd.Flags().StringArrayVar(&fields, "key-field", nil, "lookup key field")
	cmd.Flags().StringVar(&keySep, "key-sep", ":", "compound key separator")
	cmd.Flags().StringVar(&asField, "as", "", "attachment field")
	cmd.Flags().StringVar(&missing, "missing", missingDefault, "null|skip|error")
	if join {
		cmd.Flags().StringVar(&on, "on", "", "join key field")
	}
	return cmd
}
