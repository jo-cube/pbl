package app

import (
	"encoding/json"
	"fmt"

	"github.com/jo-cube/pbl/internal/store"
	"github.com/spf13/cobra"
)

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
		Long:  scanLong(mode),
		Args:  collectionArgs(want),
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
		Long: `List logical collections in the database.

Collection names come from metadata written by pbl, not from scanning physical
Pebble keys.`,
		Args: exactArgs(0),
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
					b, err := json.Marshal(map[string]string{"name": name})
					if err != nil {
						return runtimeErr(err)
					}
					if _, err := fmt.Fprintln(c.stdout, string(b)); err != nil {
						return runtimeErr(err)
					}
				} else {
					if _, err := fmt.Fprintln(c.stdout, name); err != nil {
						return runtimeErr(err)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "line", "line|ndjson output")
	return cmd
}

func (c *cli) infoCommand() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show database information",
		Long: `Show pbl metadata for the database directory.

The collection count comes from collection metadata, not by scanning data keys.`,
		Args: exactArgs(0),
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
				return runtimeWrap(json.NewEncoder(c.stdout).Encode(info))
			}
			if _, err := fmt.Fprintf(c.stdout, "path: %s\nstorage_format_version: %d\ncollections: %d\n", info.Path, info.StorageFormatVersion, info.CollectionCount); err != nil {
				return runtimeErr(err)
			}
			if info.CreatedAt != "" {
				if _, err := fmt.Fprintf(c.stdout, "created_at: %s\n", info.CreatedAt); err != nil {
					return runtimeErr(err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "text|ndjson output")
	return cmd
}

func (c *cli) statsCommand() *cobra.Command {
	var format string
	var raw bool
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show storage metrics",
		Long: `Show Pebble storage metrics for the database directory.

The default text output is stable and small. --raw appends Pebble's raw metrics
dump for debugging and may change with Pebble releases.`,
		Args: exactArgs(0),
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
				return runtimeWrap(json.NewEncoder(c.stdout).Encode(stats))
			}
			if _, err := fmt.Fprintf(c.stdout, "path: %s\ndisk_used: %d\n", stats.Path, stats.DiskUsed); err != nil {
				return runtimeErr(err)
			}
			if raw {
				if _, err := fmt.Fprint(c.stdout, stats.Raw); err != nil {
					return runtimeErr(err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "text|ndjson output")
	cmd.Flags().BoolVar(&raw, "raw", false, "append raw Pebble metrics")
	return cmd
}
