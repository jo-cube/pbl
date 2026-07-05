package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/jo-cube/pbl/internal/buildinfo"
	"github.com/jo-cube/pbl/internal/codec"
	"github.com/jo-cube/pbl/internal/store"
	"github.com/spf13/cobra"
)

const (
	codeOK       = 0
	codeRuntime  = 1
	codeNotFound = 2
	codeUsage    = 3
	codeBadInput = 4
	codeStorage  = 5
	codePartial  = 6
)

type kind int

const (
	kindRuntime kind = iota
	kindUsage
	kindNotFound
	kindBadInput
	kindStorage
	kindPartial
)

type appError struct {
	kind kind
	msg  string
	err  error
}

func (e *appError) Error() string {
	if e.err == nil {
		return e.msg
	}
	if e.msg == "" {
		return e.err.Error()
	}
	return e.msg + ": " + e.err.Error()
}

func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	c := &cli{stdin: stdin, stdout: stdout, stderr: stderr, dbPath: ".pbl"}
	cmd := c.command()
	cmd.SetArgs(args)
	cmd.SetIn(stdin)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		if !isAppError(err) {
			err = usageErr(err)
		}
		if !c.quiet {
			fmt.Fprintf(stderr, "pbl: %v\n", err)
		}
		return exitCode(err)
	}
	return codeOK
}

type cli struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	dbPath string
	quiet  bool
}

type syncOptions struct {
	sync   bool
	noSync bool
}

type scanOptions struct {
	format     string
	limit      int64
	keysOnly   bool
	valuesOnly bool
	includeKey bool
}

func (c *cli) command() *cobra.Command {
	if env := os.Getenv("PBL_DB"); env != "" {
		c.dbPath = env
	}
	root := &cobra.Command{
		Use:           "pbl",
		Short:         "Pebble-backed key-value CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       buildinfo.Version(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.SetVersionTemplate("pbl {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().StringVarP(&c.dbPath, "db", "d", c.dbPath, "database directory")
	root.PersistentFlags().BoolVar(&c.quiet, "quiet", false, "suppress diagnostics")
	root.AddCommand(
		c.initCommand(),
		c.putCommand(),
		c.getCommand(),
		c.delCommand(),
		c.scanCommand("scan"),
		c.scanCommand("prefix"),
		c.scanCommand("range"),
		c.collectionsCommand(),
		c.infoCommand(),
		c.statsCommand(),
		c.importCommand(),
		c.scanCommand("export"),
		c.keysValuesCommand("keys"),
		c.keysValuesCommand("values"),
		c.getManyCommand(),
		c.delManyCommand(),
		c.existsCommand(),
		c.lookupCommand(false),
		c.lookupCommand(true),
	)
	return root
}

func (c *cli) initCommand() *cobra.Command {
	var ifNotExists bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a database",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := c.open()
			if err != nil {
				return err
			}
			defer s.Close()
			err = s.Init()
			if err != nil && ifNotExists && strings.Contains(err.Error(), "unsupported") {
				return storageErr(err)
			}
			return storageWrap(err)
		},
	}
	cmd.Flags().BoolVar(&ifNotExists, "if-not-exists", false, "succeed if already initialized")
	return cmd
}

func (c *cli) putCommand() *cobra.Command {
	var stdinValue bool
	var sync syncOptions
	cmd := &cobra.Command{
		Use:   "put <collection> <key> <value>",
		Short: "Store a key-value pair",
		Args: func(cmd *cobra.Command, args []string) error {
			if stdinValue {
				return wantArgs(args, 2)
			}
			return wantArgs(args, 3)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateSync(sync); err != nil {
				return err
			}
			var value []byte
			if stdinValue {
				b, err := io.ReadAll(c.stdin)
				if err != nil {
					return runtimeErr(err)
				}
				value = b
			} else {
				value = []byte(args[2])
			}
			s, err := c.open()
			if err != nil {
				return err
			}
			defer s.Close()
			return storageWrap(s.Put(args[0], []byte(args[1]), value, store.WriteOptions{Sync: writeSync(sync, true)}))
		},
	}
	cmd.Flags().BoolVar(&stdinValue, "stdin", false, "read value from stdin")
	addSyncFlags(cmd, &sync)
	return cmd
}

func (c *cli) getCommand() *cobra.Command {
	var format, missing string
	var withKey, noNewline bool
	cmd := &cobra.Command{
		Use:   "get <collection> <key>",
		Short: "Read a value",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOneOf("format", format, "raw", "kv", "ndjson"); err != nil {
				return err
			}
			if err := validateOneOf("missing", missing, "error", "skip", "null"); err != nil {
				return err
			}
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer s.Close()
			key := []byte(args[1])
			value, err := s.Get(args[0], key)
			if errors.Is(err, store.ErrNotFound) {
				return c.handleMissing(missing, key, format, withKey)
			}
			if err != nil {
				return storageErr(err)
			}
			return c.writeRecord(key, value, format, withKey, !noNewline)
		},
	}
	cmd.Flags().StringVar(&format, "format", "raw", "raw|kv|ndjson")
	cmd.Flags().BoolVar(&withKey, "with-key", false, "include key")
	cmd.Flags().StringVar(&missing, "missing", "error", "error|skip|null")
	cmd.Flags().BoolVar(&noNewline, "no-newline", false, "do not append newline for raw")
	return cmd
}

func (c *cli) delCommand() *cobra.Command {
	var failMissing bool
	var sync syncOptions
	cmd := &cobra.Command{
		Use:   "del <collection> <key>",
		Short: "Delete a key",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateSync(sync); err != nil {
				return err
			}
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer s.Close()
			if failMissing {
				found, err := s.Has(args[0], []byte(args[1]))
				if err != nil {
					return storageErr(err)
				}
				if !found {
					return notFoundf("not found")
				}
			}
			return storageWrap(s.Delete(args[0], []byte(args[1]), store.WriteOptions{Sync: writeSync(sync, true)}))
		},
	}
	cmd.Flags().BoolVar(&failMissing, "fail-missing", false, "exit 2 if missing")
	addSyncFlags(cmd, &sync)
	return cmd
}

func (c *cli) scanCommand(mode string) *cobra.Command {
	opts := scanOptions{format: "kv"}
	use := mode + " <collection>"
	want := 1
	if mode == "prefix" {
		use = "prefix <collection> <prefix>"
		want = 2
	}
	if mode == "range" {
		use = "range <collection> <start> <end>"
		want = 3
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: scanShort(mode),
		Args:  exactArgs(want),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateScanOptions(opts); err != nil {
				return err
			}
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer s.Close()
			fn := func(r store.Record) error {
				return c.writeScanRecord(r.Key, r.Value, opts.format, opts.keysOnly, opts.valuesOnly, opts.includeKey)
			}
			scanOpts := store.ScanOptions{Limit: opts.limit}
			switch mode {
			case "scan", "export":
				return storageWrap(s.Scan(args[0], scanOpts, fn))
			case "prefix":
				return storageWrap(s.Prefix(args[0], []byte(args[1]), scanOpts, fn))
			default:
				return storageWrap(s.Range(args[0], []byte(args[1]), []byte(args[2]), scanOpts, fn))
			}
		},
	}
	addScanFlags(cmd, &opts)
	return cmd
}

func (c *cli) collectionsCommand() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "collections",
		Short: "List collections",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOneOf("format", format, "line", "ndjson"); err != nil {
				return err
			}
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer s.Close()
			names, err := s.ListCollections()
			if err != nil {
				return storageErr(err)
			}
			for _, name := range names {
				if format == "ndjson" {
					b, _ := json.Marshal(map[string]string{"name": name})
					fmt.Fprintln(c.stdout, string(b))
				} else {
					fmt.Fprintln(c.stdout, name)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "line", "line|ndjson")
	return cmd
}

func (c *cli) infoCommand() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show database information",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOneOf("format", format, "text", "ndjson"); err != nil {
				return err
			}
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer s.Close()
			info, err := s.Info()
			if err != nil {
				return storageErr(err)
			}
			if format == "ndjson" {
				return json.NewEncoder(c.stdout).Encode(info)
			}
			fmt.Fprintf(c.stdout, "path: %s\nstorage_format_version: %d\ncollections: %d\n", info.Path, info.StorageFormatVersion, info.CollectionCount)
			if info.CreatedAt != "" {
				fmt.Fprintf(c.stdout, "created_at: %s\n", info.CreatedAt)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "text|ndjson")
	return cmd
}

func (c *cli) statsCommand() *cobra.Command {
	var format string
	var raw bool
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show storage metrics",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOneOf("format", format, "text", "ndjson"); err != nil {
				return err
			}
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer s.Close()
			stats, err := s.Stats()
			if err != nil {
				return storageErr(err)
			}
			if format == "ndjson" {
				return json.NewEncoder(c.stdout).Encode(stats)
			}
			fmt.Fprintf(c.stdout, "path: %s\ndisk_used: %d\n", stats.Path, stats.DiskUsed)
			if raw {
				fmt.Fprint(c.stdout, stats.Raw)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "text|ndjson")
	cmd.Flags().BoolVar(&raw, "raw", false, "print raw Pebble metrics")
	return cmd
}

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

func (c *cli) open() (*store.Store, error) {
	s, err := store.Open(c.dbPath)
	if err != nil {
		return nil, storageErr(err)
	}
	return s, nil
}

func (c *cli) openExisting() (*store.Store, error) {
	if _, err := os.Stat(c.dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, storageErr(fmt.Errorf("database does not exist: %s", c.dbPath))
		}
		return nil, storageErr(err)
	}
	return c.open()
}

func (c *cli) forInputKeys(inputFormat string, fields []string, sep string, fn func(codec.Record) error) error {
	return c.forInputRecords(inputFormat, fields, sep, fn)
}

func (c *cli) forInputRecords(inputFormat string, fields []string, sep string, fn func(codec.Record) error) error {
	switch inputFormat {
	case "line":
		lr := codec.NewLineReader(c.stdin)
		for {
			line, n, err := lr.Next()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return runtimeErr(err)
			}
			if err := fn(codec.Record{Key: append([]byte(nil), line...), Raw: append([]byte(nil), line...), Line: n}); err != nil {
				return err
			}
		}
	case "ndjson":
		return badInputErr(codec.ReadNDJSONRecords(c.stdin, fields, sep, fn))
	default:
		return usagef("unknown input format %q", inputFormat)
	}
}

func (c *cli) writeLookup(rec codec.Record, value []byte, inputFormat, asField string) error {
	if inputFormat == "line" {
		if value == nil {
			return nil
		}
		return codec.WriteLine(c.stdout, value)
	}
	obj := rec.JSON
	if obj == nil {
		if err := json.Unmarshal(rec.Raw, &obj); err != nil {
			return badInputErr(err)
		}
	}
	if value == nil {
		obj[asField] = nil
	} else {
		var attached any
		if err := json.Unmarshal(value, &attached); err != nil {
			return badInputf("stored value for key %q is not valid JSON", rec.Key)
		}
		obj[asField] = attached
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return runtimeErr(err)
	}
	return codec.WriteLine(c.stdout, out)
}

func (c *cli) writeScanRecord(key, value []byte, format string, keysOnly, valuesOnly, includeKey bool) error {
	if keysOnly {
		return codec.WriteLine(c.stdout, key)
	}
	if valuesOnly {
		if format == "raw" {
			_, err := c.stdout.Write(value)
			return err
		}
		return codec.WriteLine(c.stdout, value)
	}
	switch format {
	case "kv":
		return codec.WriteKV(c.stdout, key, value)
	case "ndjson":
		return badInputErr(codec.WriteNDJSONValue(c.stdout, key, value, includeKey))
	case "raw":
		return usagef("raw export requires --values-only")
	default:
		return usagef("unknown format %q", format)
	}
}

func (c *cli) writeRecord(key, value []byte, format string, withKey, newline bool) error {
	switch format {
	case "raw":
		if _, err := c.stdout.Write(value); err != nil {
			return err
		}
		if newline {
			_, err := fmt.Fprintln(c.stdout)
			return err
		}
		return nil
	case "kv":
		return codec.WriteKV(c.stdout, key, value)
	case "ndjson":
		return badInputErr(codec.WriteNDJSONValue(c.stdout, key, value, withKey))
	default:
		return usagef("unknown format %q", format)
	}
}

func (c *cli) handleMissing(policy string, key []byte, format string, withKey bool) error {
	switch policy {
	case "error":
		return notFoundf("not found")
	case "skip":
		return nil
	case "null":
		if format == "ndjson" {
			out, _ := json.Marshal(map[string]any{"_key": string(key), "_value": nil})
			fmt.Fprintln(c.stdout, string(out))
			return nil
		}
		if format == "kv" && withKey {
			return codec.WriteKV(c.stdout, key, nil)
		}
		fmt.Fprintln(c.stdout, "null")
		return nil
	default:
		return usagef("unknown missing policy %q", policy)
	}
}

func addSyncFlags(cmd *cobra.Command, opts *syncOptions) {
	cmd.Flags().BoolVar(&opts.sync, "sync", false, "sync write")
	cmd.Flags().BoolVar(&opts.noSync, "no-sync", false, "do not sync write")
}

func addScanFlags(cmd *cobra.Command, opts *scanOptions) {
	cmd.Flags().StringVar(&opts.format, "format", "kv", "kv|ndjson|raw")
	cmd.Flags().Int64Var(&opts.limit, "limit", 0, "max records")
	cmd.Flags().BoolVar(&opts.keysOnly, "keys-only", false, "emit keys only")
	cmd.Flags().BoolVar(&opts.valuesOnly, "values-only", false, "emit values only")
	cmd.Flags().BoolVar(&opts.includeKey, "include-key", false, "include key in ndjson")
}

func scanShort(mode string) string {
	switch mode {
	case "prefix":
		return "Scan keys with a prefix"
	case "range":
		return "Scan a half-open key range"
	case "export":
		return "Export collection records"
	default:
		return "Scan a collection"
	}
}

func keysValuesShort(mode string) string {
	if mode == "keys" {
		return "Emit collection keys"
	}
	return "Emit collection values"
}

func lookupShort(join bool) string {
	if join {
		return "Join NDJSON stdin with stored values"
	}
	return "Lookup stdin keys in a collection"
}

func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		return wantArgs(args, n)
	}
}

func wantArgs(args []string, n int) error {
	if len(args) != n {
		return usagef("expected %d argument(s), got %d", n, len(args))
	}
	return nil
}

func validateOneOf(name, value string, allowed ...string) error {
	for _, x := range allowed {
		if value == x {
			return nil
		}
	}
	return usagef("unknown %s %q", name, value)
}

func validateSync(opts syncOptions) error {
	if opts.sync && opts.noSync {
		return usagef("--sync and --no-sync cannot both be set")
	}
	return nil
}

func validateBatchSize(n int) error {
	if n <= 0 {
		return usagef("--batch-size must be greater than 0")
	}
	return nil
}

func validateLimit(n int64) error {
	if n < 0 {
		return usagef("--limit must be greater than or equal to 0")
	}
	return nil
}

func validateScanOptions(opts scanOptions) error {
	if err := validateOneOf("format", opts.format, "kv", "ndjson", "raw"); err != nil {
		return err
	}
	if err := validateLimit(opts.limit); err != nil {
		return err
	}
	if opts.keysOnly && opts.valuesOnly {
		return usagef("--keys-only and --values-only cannot both be set")
	}
	if opts.format == "raw" && !opts.valuesOnly {
		return usagef("raw export requires --values-only")
	}
	return nil
}

func writeSync(opts syncOptions, def bool) bool {
	if opts.sync {
		return true
	}
	if opts.noSync {
		return false
	}
	return def
}

func parseSize(s string) (int, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := 1
	for _, suffix := range []struct {
		s string
		m int
	}{{"KB", 1024}, {"K", 1024}, {"MB", 1024 * 1024}, {"M", 1024 * 1024}} {
		if strings.HasSuffix(s, suffix.s) {
			mult = suffix.m
			s = strings.TrimSuffix(s, suffix.s)
			break
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid size")
	}
	return n * mult, nil
}

func exitCode(err error) int {
	var ae *appError
	if !errors.As(err, &ae) {
		return codeRuntime
	}
	switch ae.kind {
	case kindUsage:
		return codeUsage
	case kindNotFound:
		return codeNotFound
	case kindBadInput:
		return codeBadInput
	case kindStorage:
		return codeStorage
	case kindPartial:
		return codePartial
	default:
		return codeRuntime
	}
}

func isAppError(err error) bool {
	var ae *appError
	return errors.As(err, &ae)
}

func usageErr(err error) error        { return &appError{kind: kindUsage, err: err} }
func usagef(f string, a ...any) error { return &appError{kind: kindUsage, msg: fmt.Sprintf(f, a...)} }
func runtimeErr(err error) error      { return &appError{kind: kindRuntime, err: err} }
func storageErr(err error) error      { return &appError{kind: kindStorage, err: err} }
func storageWrap(err error) error {
	if err == nil {
		return nil
	}
	return storageErr(err)
}
func badInputErr(err error) error {
	if err == nil {
		return nil
	}
	if isAppError(err) {
		return err
	}
	return &appError{kind: kindBadInput, err: err}
}
func badInputf(f string, a ...any) error {
	return &appError{kind: kindBadInput, msg: fmt.Sprintf(f, a...)}
}
func notFoundf(f string, a ...any) error {
	return &appError{kind: kindNotFound, msg: fmt.Sprintf(f, a...)}
}
