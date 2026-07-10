package app

import (
	"errors"

	"github.com/jo-cube/pbl/internal/codec"
	"github.com/jo-cube/pbl/internal/store"
	"github.com/spf13/cobra"
)

func (c *cli) initCommand() *cobra.Command {
	var ifNotExists bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a database",
		Long: `Initialize the Pebble directory and write pbl storage metadata.

Success writes no stdout. Use --if-not-exists in scripts that should tolerate an
already-initialized compatible database.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := c.open()
			if err != nil {
				return err
			}
			defer s.Close()
			err = s.Init()
			if errors.Is(err, store.ErrAlreadyInitialized) && ifNotExists {
				return nil
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
		Long: `Store or replace one key in a collection.

Without --stdin, the value is the third argument. With --stdin, pbl reads all
stdin bytes as the value, including newlines. Single-key writes sync by default;
use --no-sync only when throughput matters more than crash durability.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if stdinValue {
				return collectionArgs(2)(cmd, args)
			}
			return collectionArgs(3)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateSync(sync); err != nil {
				return err
			}
			var value []byte
			if stdinValue {
				b, err := codec.ReadRaw(c.stdin)
				if err != nil {
					if errors.Is(err, codec.ErrRecordTooLarge) {
						return badInputErr(err)
					}
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
		Long: `Read one key from a collection.

Default output is the raw value plus a newline. Use --format when downstream
tools need key/value or NDJSON output. Missing keys exit 2 by default; --missing
can skip them or emit a null-shaped record instead.`,
		Args: collectionArgs(2),
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
	cmd.Flags().StringVar(&format, "format", "raw", "raw|kv|ndjson output")
	cmd.Flags().BoolVar(&withKey, "with-key", false, "include key in output")
	cmd.Flags().StringVar(&missing, "missing", "error", "error|skip|null for missing keys")
	cmd.Flags().BoolVar(&noNewline, "no-newline", false, "suppress raw output newline")
	return cmd
}

func (c *cli) delCommand() *cobra.Command {
	var failMissing bool
	var sync syncOptions
	cmd := &cobra.Command{
		Use:   "del <collection> <key>",
		Short: "Delete a key",
		Long: `Delete one key from a collection.

Missing keys are success by default so delete is idempotent in scripts. Add
--fail-missing when absence should exit 2. Single-key deletes sync by default.`,
		Args: collectionArgs(2),
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
