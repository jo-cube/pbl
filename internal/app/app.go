package app

import (
	"fmt"
	"io"
	"os"

	"github.com/jo-cube/pbl/internal/buildinfo"
	"github.com/jo-cube/pbl/internal/store"
	"github.com/spf13/cobra"
)

func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	out := &countingWriter{w: stdout}
	c := &cli{stdin: stdin, stdout: out, stderr: stderr, dbPath: ".pbl"}
	cmd := c.command()
	cmd.SetArgs(args)
	cmd.SetIn(stdin)
	cmd.SetOut(out)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		if !isAppError(err) {
			err = usageErr(err)
		}
		if out.n > 0 {
			err = partialErr(err)
		}
		if !c.quiet {
			fmt.Fprintf(stderr, "pbl: %v\n", err)
		}
		return exitCode(err)
	}
	return codeOK
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
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
		Use:   "pbl",
		Short: "Pebble-backed key-value CLI",
		Long: `pbl stores ordered byte keys and opaque byte values in one local Pebble
directory. Collections are logical keyspaces inside that one directory.

stdout is data; diagnostics go to stderr. Stream commands preserve input order.
Write commands that create records initialize the database when needed. Read,
delete, metadata, and lookup commands require the database directory to exist.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       buildinfo.Version(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.SetVersionTemplate("pbl {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().StringVarP(&c.dbPath, "db", "d", c.dbPath, "database directory; overrides PBL_DB")
	root.PersistentFlags().BoolVar(&c.quiet, "quiet", false, "suppress diagnostics on stderr")
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
		c.applyCommand(),
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
