package app

import (
	"errors"
	"io"
	"strings"

	"github.com/jo-cube/pbl/internal/store"
	"github.com/spf13/cobra"
)

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
