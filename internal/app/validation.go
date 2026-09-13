package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jo-cube/pbl/internal/store"
	"github.com/spf13/cobra"
)

func addSyncFlags(cmd *cobra.Command, opts *syncOptions) {
	cmd.Flags().BoolVar(&opts.sync, "sync", false, "fsync each commit")
	cmd.Flags().BoolVar(&opts.noSync, "no-sync", false, "skip fsync")
}

func addSelectionFlags(cmd *cobra.Command, opts *selectionOptions) {
	cmd.Flags().StringVar(&opts.prefix, "prefix", "", "key prefix filter")
	cmd.Flags().StringVar(&opts.start, "start", "", "inclusive key bound; omitted means no lower bound")
	cmd.Flags().StringVar(&opts.end, "end", "", "exclusive key bound; omitted means no upper bound")
}

func (o selectionOptions) scanOptions(cmd *cobra.Command) (store.ScanOptions, error) {
	opts := store.ScanOptions{Prefix: []byte(o.prefix)}
	if cmd.Flags().Changed("start") {
		opts.Start = []byte(o.start)
	}
	if cmd.Flags().Changed("end") {
		opts.End = []byte(o.end)
	}
	if err := opts.Validate(); err != nil {
		return opts, usageErr(err)
	}
	return opts, nil
}

func addScanFlags(cmd *cobra.Command, opts *scanOptions) {
	cmd.Flags().StringVar(&opts.format, "format", "kv", "kv|ndjson|raw|frame output")
	cmd.Flags().Int64Var(&opts.limit, "limit", 0, "max records; 0 means all")
	cmd.Flags().BoolVar(&opts.reverse, "reverse", false, "scan in descending key order")
	cmd.Flags().BoolVar(&opts.keysOnly, "keys-only", false, "emit keys only, one per line")
	cmd.Flags().BoolVar(&opts.valuesOnly, "values-only", false, "emit values only, one per line")
	cmd.Flags().BoolVar(&opts.withKey, "with-key", false, "include key wrapper in ndjson output")
}

func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		return wantArgs(args, n)
	}
}

func collectionArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := wantArgs(args, n); err != nil {
			return err
		}
		if err := store.ValidateCollection(args[0]); err != nil {
			return usageErr(err)
		}
		return nil
	}
}

func collectionKeyArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := collectionArgs(n)(cmd, args); err != nil {
			return err
		}
		if args[1] == "" {
			return usagef("key is required")
		}
		return nil
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

func validateOutputFormat(format string, withKey bool, allowed ...string) error {
	if err := validateOneOf("format", format, allowed...); err != nil {
		return err
	}
	if withKey && format != "ndjson" {
		return usagef("--with-key requires ndjson format")
	}
	return nil
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
	if err := validateOutputFormat(opts.format, opts.withKey, "kv", "ndjson", "raw", "frame"); err != nil {
		return err
	}
	if err := validateLimit(opts.limit); err != nil {
		return err
	}
	if opts.keysOnly && opts.valuesOnly {
		return usagef("--keys-only and --values-only cannot both be set")
	}
	if (opts.keysOnly || opts.valuesOnly) && opts.format != "kv" {
		return usagef("--keys-only and --values-only require kv format")
	}
	return nil
}

func validateNDJSONKeyFields(format string, fields []string, sep string) error {
	if format != "ndjson" {
		return nil
	}
	if len(fields) == 0 {
		return usagef("ndjson input requires --key-field")
	}
	if len(fields) > 1 && len(sep) != 1 {
		return usagef("compound ndjson keys require a one-byte --key-sep")
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
	mult := uint64(1)
	for _, suffix := range []struct {
		s string
		m uint64
	}{{"KB", 1024}, {"K", 1024}, {"MB", 1024 * 1024}, {"M", 1024 * 1024}} {
		if strings.HasSuffix(s, suffix.s) {
			mult = suffix.m
			s = strings.TrimSuffix(s, suffix.s)
			break
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	maxInt := uint64(^uint(0) >> 1)
	if err != nil || n == 0 || n > maxInt/mult {
		return 0, fmt.Errorf("invalid size")
	}
	return int(n * mult), nil
}

func parseCount(s string) (uint64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := uint64(1)
	for _, suffix := range []struct {
		s string
		m uint64
	}{{"B", 1_000_000_000}, {"M", 1_000_000}, {"K", 1_000}} {
		if strings.HasSuffix(s, suffix.s) {
			mult = suffix.m
			s = strings.TrimSuffix(s, suffix.s)
			break
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n == 0 || n > ^uint64(0)/mult {
		return 0, fmt.Errorf("invalid count")
	}
	return n * mult, nil
}
