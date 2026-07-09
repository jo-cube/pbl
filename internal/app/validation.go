package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func addSyncFlags(cmd *cobra.Command, opts *syncOptions) {
	cmd.Flags().BoolVar(&opts.sync, "sync", false, "fsync each commit")
	cmd.Flags().BoolVar(&opts.noSync, "no-sync", false, "skip fsync")
}

func addScanFlags(cmd *cobra.Command, opts *scanOptions) {
	cmd.Flags().StringVar(&opts.format, "format", "kv", "kv|ndjson|raw output")
	cmd.Flags().Int64Var(&opts.limit, "limit", 0, "max records; 0 means all")
	cmd.Flags().BoolVar(&opts.keysOnly, "keys-only", false, "emit keys only")
	cmd.Flags().BoolVar(&opts.valuesOnly, "values-only", false, "emit values only")
	cmd.Flags().BoolVar(&opts.includeKey, "include-key", false, "include _key in ndjson")
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

func scanLong(mode string) string {
	switch mode {
	case "prefix":
		return `Emit records whose keys start with the given prefix.

Records are ordered by raw key bytes. The prefix is matched before formatting,
and --limit stops after that many matching records.`
	case "range":
		return `Emit records in a half-open key range: start <= key < end.

Records are ordered by raw key bytes. Use ranges for compound keys and time
windows where the end bound should not be included.`
	case "export":
		return `Export records from a collection using the same ordered scan path.

Default output is key<TAB>value. Use --values-only --format raw for byte-oriented
value export.`
	default:
		return `Emit all records in a collection ordered by raw key bytes.

Default output is key<TAB>value. Use --keys-only, --values-only, --format, and
--limit to shape stdout for the next command in a pipeline.`
	}
}

func keysValuesShort(mode string) string {
	if mode == "keys" {
		return "Emit collection keys"
	}
	return "Emit collection values"
}

func keysValuesLong(mode string) string {
	if mode == "keys" {
		return `Emit collection keys, one per line.

Without filters, keys are ordered by raw key bytes. Use --prefix or a half-open
range with --range-start and --range-end to narrow the scan.`
	}
	return `Emit collection values, one per line.

Values follow raw key-byte order. Use --prefix or a half-open range with
--range-start and --range-end to narrow the scan.`
}

func lookupShort(join bool) string {
	if join {
		return "Join NDJSON stdin with stored values"
	}
	return "Lookup stdin keys in a collection"
}

func lookupLong(join bool) string {
	if join {
		return `Attach stored JSON values to NDJSON input records.

Join is the NDJSON-only form of lookup. --on names the input field used as the
join key, and --as names the field that receives the stored JSON value. Repeated
--key-field flags can add leading compound-key parts. Missing keys attach null
by default.`
	}
	return `Lookup stdin records in a collection.

Line input emits stored values. NDJSON input requires --as so pbl can attach the
stored JSON value to each input object. Stored values must be valid JSON when
attached to NDJSON.`
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
