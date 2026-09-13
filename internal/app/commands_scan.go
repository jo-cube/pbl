package app

import (
	"encoding/json"
	"fmt"

	"github.com/jo-cube/pbl/internal/store"
	"github.com/spf13/cobra"
)

func (c *cli) scanCommand() *cobra.Command {
	var selection selectionOptions
	var opts scanOptions
	cmd := &cobra.Command{
		Use:   "scan <collection>",
		Short: "Scan collection records in key order",
		Long: `Emit records ordered by raw key bytes, optionally in reverse.

--prefix, --start (inclusive), and --end (exclusive) select their intersection.
Omitted range bounds are open. --limit stops after that many matching records.
Default output is key<TAB>value; --format frame provides lossless export.`,
		Args: collectionArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateScanOptions(opts); err != nil {
				return err
			}
			scanOpts, err := selection.scanOptions(cmd)
			if err != nil {
				return err
			}
			scanOpts.Limit, scanOpts.Reverse, scanOpts.KeysOnly = opts.limit, opts.reverse, opts.keysOnly
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer c.closeStore(s)
			return storageWrap(s.Scan(args[0], scanOpts, func(r store.Record) error {
				return c.writeScanRecord(r.Key, r.Value, opts)
			}))
		},
	}
	addSelectionFlags(cmd, &selection)
	addScanFlags(cmd, &opts)
	return cmd
}

func (c *cli) countCommand() *cobra.Command {
	var selection selectionOptions
	cmd := &cobra.Command{
		Use:   "count <collection>",
		Short: "Count keys in a collection or selection",
		Long: `Print the exact number of matching keys, followed by a newline.

Uses the same --prefix, --start, and --end selection as scan. Visits matching
keys without fetching values; an empty or absent collection counts as zero.`,
		Args: collectionArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := selection.scanOptions(cmd)
			if err != nil {
				return err
			}
			opts.KeysOnly = true
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer c.closeStore(s)
			var n int64
			if err := s.Scan(args[0], opts, func(store.Record) error { n++; return nil }); err != nil {
				return storageErr(err)
			}
			_, err = fmt.Fprintln(c.stdout, n)
			return runtimeWrap(err)
		},
	}
	addSelectionFlags(cmd, &selection)
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
			defer c.closeStore(s)
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
			defer c.closeStore(s)
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

The default output is stable and small. --raw includes Pebble's raw metrics dump
for debugging and may change with Pebble releases.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOneOf("format", format, "text", "ndjson"); err != nil {
				return err
			}
			s, err := c.openExisting()
			if err != nil {
				return err
			}
			defer c.closeStore(s)
			stats, err := s.Stats(raw)
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
	cmd.Flags().BoolVar(&raw, "raw", false, "include raw Pebble metrics")
	return cmd
}
