package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jo-cube/pbl/internal/codec"
)

const valueA = `A
`

const versionOut = `pbl dev
`

const fieldSep = '\t'
const recordSep = '\n'

var usersAB = kv(
	row("a", "A"),
	row("b", "B"),
)

var usersImport = kv(
	row("u2", "Grace"),
	row("u1", "Ada"),
)

var usersExport = kv(
	row("u1", "Ada"),
	row("u2", "Grace"),
)

const getManyInput = `u1
missing
u2
`

const getManyOutput = `Ada
Grace
`

const existsInput = `u1
missing
`

const existsOutput = `u1
`

const userAdaNDJSON = `{"id":"u1","name":"Ada"}
`

const loginEventNDJSON = `{"event":"login","user_id":"u1"}
`

var errBoom = errors.New("boom")

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errBoom
}

type writeCounter struct {
	bytes.Buffer
	writes int
}

func (w *writeCounter) Write(p []byte) (int, error) {
	w.writes++
	return w.Buffer.Write(p)
}

func run(t *testing.T, db string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	full := append([]string{"--db", db}, args...)
	code := Main(full, strings.NewReader(stdin), &out, &err)
	return out.String(), err.String(), code
}

func TestCLIPutGetMissingAndScan(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "", "init"); out != "" || err != "" || code != 0 {
		t.Fatalf("init out=%q err=%q code=%d", out, err, code)
	}
	if _, err, code := run(t, db, "", "put", "users", "b", "B"); err != "" || code != 0 {
		t.Fatalf("put b err=%q code=%d", err, code)
	}
	if _, err, code := run(t, db, "", "put", "users", "a", "A"); err != "" || code != 0 {
		t.Fatalf("put a err=%q code=%d", err, code)
	}
	if out, err, code := run(t, db, "", "get", "users", "a"); out != valueA || err != "" || code != 0 {
		t.Fatalf("get out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "get", "users", "missing"); out != "" || !strings.Contains(err, "not found") || code != 2 {
		t.Fatalf("missing out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "scan", "users"); out != usersAB || err != "" || code != 0 {
		t.Fatalf("scan out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIInitRequiresIfNotExists(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "", "init"); out != "" || err != "" || code != 0 {
		t.Fatalf("first init out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "init"); out != "" || !strings.Contains(err, "already initialized") || code != 5 {
		t.Fatalf("second init out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "init", "--if-not-exists"); out != "" || err != "" || code != 0 {
		t.Fatalf("idempotent init out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIValidatesBeforeCreatingDatabase(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"collection", []string{"put", "bad/name", "k", "v"}},
		{"put key", []string{"put", "users", "", "v"}},
		{"get key", []string{"get", "users", ""}},
		{"delete key", []string{"del", "users", ""}},
		{"raw key", []string{"import", "users", "--format", "raw"}},
		{"ndjson key field", []string{"import", "users", "--format", "ndjson"}},
		{"compound separator", []string{"import", "users", "--format", "ndjson", "--key-field", "a", "--key-field", "b", "--key-sep", "::"}},
		{"stream key field", []string{"get-many", "users", "--input-format", "ndjson"}},
		{"bloom without count", []string{"apply", "users", "--format", "frame", "--bloom-filter"}},
		{"count without bloom", []string{"apply", "users", "--format", "frame", "--expected-key-count", "1K"}},
		{"invalid bloom count", []string{"apply", "users", "--format", "frame", "--bloom-filter", "--expected-key-count", "0"}},
	}
	for _, tc := range cases {
		db := filepath.Join(t.TempDir(), "db")
		if out, err, code := run(t, db, "", tc.args...); out != "" || err == "" || code != 3 {
			t.Fatalf("%s out=%q err=%q code=%d", tc.name, out, err, code)
		}
		if _, err := os.Stat(db); !os.IsNotExist(err) {
			t.Fatalf("%s created database: %v", tc.name, err)
		}
	}
}

func TestCLIPutStdinValue(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "from stdin", "put", "users", "a", "--stdin"); out != "" || err != "" || code != 0 {
		t.Fatalf("put --stdin out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "get", "users", "a", "--no-newline"); out != "from stdin" || err != "" || code != 0 {
		t.Fatalf("get stdin value out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIMissingNullMatchesFormat(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "", "init"); out != "" || err != "" || code != 0 {
		t.Fatalf("init out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "get", "users", "missing", "--missing", "null"); out != "null\n" || err != "" || code != 0 {
		t.Fatalf("raw missing out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "get", "users", "missing", "--format", "kv", "--missing", "null"); out != "missing\tnull\n" || err != "" || code != 0 {
		t.Fatalf("kv missing out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "get", "users", "missing", "--format", "ndjson", "--missing", "null"); out != "null\n" || err != "" || code != 0 {
		t.Fatalf("ndjson missing out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "get", "users", "missing", "--format", "ndjson", "--with-key", "--missing", "null"); out != "{\"_key\":\"missing\",\"_value\":null}\n" || err != "" || code != 0 {
		t.Fatalf("ndjson missing with key out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLILookupDistinguishesEmptyValuesFromMissingKeys(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "", "put", "users", "empty", "--stdin"); out != "" || err != "" || code != 0 {
		t.Fatalf("put empty out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "empty\nmissing\n", "get-many", "users", "--missing", "null"); out != "\nnull\n" || err != "" || code != 0 {
		t.Fatalf("line lookup out=%q err=%q code=%d", out, err, code)
	}
	input := "{\"id\":\"empty\"}\n"
	if out, err, code := run(t, db, input, "join", "users", "--on", "id", "--as", "value"); out != "" || !strings.Contains(err, "not valid JSON") || code != 4 {
		t.Fatalf("ndjson lookup out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIStreamFailureAfterOutputIsPartial(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "u1\tAda\n", "import", "users", "--format", "kv"); out != "" || err != "" || code != 0 {
		t.Fatalf("import out=%q err=%q code=%d", out, err, code)
	}
	out, errText, code := run(t, db, "u1\nmissing\n", "get-many", "users", "--missing", "error")
	if out != "Ada\n" || !strings.Contains(errText, "not found") || code != 6 {
		t.Fatalf("get-many partial out=%q err=%q code=%d", out, errText, code)
	}
}

func TestCLIInvalidNDJSONScanIsBadInput(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "a\tplain text\n", "import", "users", "--format", "kv"); out != "" || err != "" || code != 0 {
		t.Fatalf("import out=%q err=%q code=%d", out, err, code)
	}
	out, errText, code := run(t, db, "", "scan", "users", "--format", "ndjson")
	if out != "" || !strings.Contains(errText, "value is not valid JSON") || code != 4 {
		t.Fatalf("scan invalid json out=%q err=%q code=%d", out, errText, code)
	}
}

func TestCLIWriteErrorIsRuntimeError(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "", "init"); out != "" || err != "" || code != 0 {
		t.Fatalf("init out=%q err=%q code=%d", out, err, code)
	}
	var stderr bytes.Buffer
	code := Main([]string{"--db", db, "get", "users", "missing", "--missing", "null"}, strings.NewReader(""), errWriter{}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("write failure stderr=%q code=%d", stderr.String(), code)
	}
	if out, err, code := run(t, db, usersAB, "import", "users", "--format", "kv"); out != "" || err != "" || code != 0 {
		t.Fatalf("import out=%q err=%q code=%d", out, err, code)
	}
	stderr.Reset()
	code = Main([]string{"--db", db, "get-many", "users", "--missing", "error"}, strings.NewReader("a\nmissing\n"), errWriter{}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("write failure with command error stderr=%q code=%d", stderr.String(), code)
	}
	var stdout bytes.Buffer
	code = Main([]string{"--db", db, "apply", "users", "--format", "frame", "--stats"}, strings.NewReader("P 1 1\ncC"), &stdout, errWriter{})
	if code != 1 || stdout.String() != "" {
		t.Fatalf("stats write failure out=%q code=%d", stdout.String(), code)
	}
}

func TestCLIStreamOutputIsBuffered(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, usersAB, "import", "users", "--format", "kv"); out != "" || err != "" || code != 0 {
		t.Fatalf("import out=%q err=%q code=%d", out, err, code)
	}
	var out writeCounter
	var stderr bytes.Buffer
	code := Main([]string{"--db", db, "scan", "users"}, strings.NewReader(""), &out, &stderr)
	if code != 0 || out.String() != usersAB || stderr.String() != "" {
		t.Fatalf("scan out=%q err=%q code=%d", out.String(), stderr.String(), code)
	}
	if out.writes != 1 {
		t.Fatalf("stdout writes = %d, want 1 buffered write", out.writes)
	}
}

func TestCLIRootHelpAndVersion(t *testing.T) {
	var out, err bytes.Buffer
	code := Main([]string{"--version"}, strings.NewReader(""), &out, &err)
	if code != 0 || out.String() != versionOut || err.String() != "" {
		t.Fatalf("version out=%q err=%q code=%d", out.String(), err.String(), code)
	}
	out.Reset()
	err.Reset()
	code = Main([]string{"--help"}, strings.NewReader(""), &out, &err)
	if code != 0 || strings.Count(out.String(), "Usage:") != 1 || !strings.Contains(out.String(), "stdout is data") || err.String() != "" {
		t.Fatalf("help out=%q err=%q code=%d", out.String(), err.String(), code)
	}
	out.Reset()
	err.Reset()
	code = Main([]string{"import", "--help"}, strings.NewReader(""), &out, &err)
	if code != 0 || !strings.Contains(out.String(), "Formats decide how input becomes keys and values") || !strings.Contains(out.String(), "--key-field") || err.String() != "" {
		t.Fatalf("import help out=%q err=%q code=%d", out.String(), err.String(), code)
	}
	out.Reset()
	err.Reset()
	code = Main([]string{"apply", "--help"}, strings.NewReader(""), &out, &err)
	if code != 0 || !strings.Contains(out.String(), "skip deletes for keys definitely absent") || !strings.Contains(out.String(), "--bloom-filter") || !strings.Contains(out.String(), "--expected-key-count") || err.String() != "" {
		t.Fatalf("apply help out=%q err=%q code=%d", out.String(), err.String(), code)
	}
}

func TestCLIImportDuplicatePoliciesSeeCurrentInput(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	dups := kv(row("a", "A"), row("a", "B"))
	if out, err, code := run(t, db, dups, "import", "users", "--format", "kv", "--fail-on-duplicate"); out != "" || !strings.Contains(err, "duplicate key") || code != 4 {
		t.Fatalf("fail duplicate out=%q err=%q code=%d", out, err, code)
	}

	db = filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, dups, "import", "users", "--format", "kv", "--ignore-duplicates"); out != "" || err != "" || code != 0 {
		t.Fatalf("ignore duplicate out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "scan", "users"); out != kv(row("a", "A")) || err != "" || code != 0 {
		t.Fatalf("scan after ignore duplicate out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIImportDuplicatePoliciesAcrossBatches(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	dups := kv(row("a", "A"), row("a", "B"))
	if out, err, code := run(t, db, dups, "import", "users", "--format", "kv", "--ignore-duplicates", "--batch-size", "1"); out != "" || err != "" || code != 0 {
		t.Fatalf("ignore duplicate out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "scan", "users"); out != kv(row("a", "A")) || err != "" || code != 0 {
		t.Fatalf("scan after duplicate out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIImportDocumentsPartialMutation(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	out, errText, code := run(t, db, "a\tA\ninvalid\n", "import", "users", "--format", "kv", "--batch-size", "1")
	if out != "" || !strings.Contains(errText, "missing tab") || code != 4 {
		t.Fatalf("import out=%q err=%q code=%d", out, errText, code)
	}
	if out, err, code := run(t, db, "", "get", "users", "a"); out != valueA || err != "" || code != 0 {
		t.Fatalf("committed record out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIApplyDocumentsPartialMutation(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	out, errText, code := run(t, db, "a\t1\tA\nb\t4\txy", "apply", "users", "--format", "kcat", "--batch-size", "1")
	if out != "" || !strings.Contains(errText, "truncated payload") || code != 4 {
		t.Fatalf("apply out=%q err=%q code=%d", out, errText, code)
	}
	if out, err, code := run(t, db, "", "get", "users", "a"); out != valueA || err != "" || code != 0 {
		t.Fatalf("committed record out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIRejectsInvalidAndConflictingFlags(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	cases := []struct {
		name string
		args []string
	}{
		{"format", []string{"collections", "--format", "yaml"}},
		{"sync", []string{"put", "users", "a", "A", "--sync", "--no-sync"}},
		{"duplicates", []string{"import", "users", "--format", "kv", "--fail-on-duplicate", "--ignore-duplicates"}},
		{"range", []string{"scan", "users", "--start", "z", "--end", "a"}},
		{"frame shaping", []string{"scan", "users", "--format", "frame", "--keys-only"}},
		{"ndjson shaping", []string{"scan", "users", "--format", "ndjson", "--values-only"}},
		{"scan key wrapper", []string{"scan", "users", "--with-key"}},
		{"get key wrapper", []string{"get", "users", "a", "--with-key"}},
		{"get-many key wrapper", []string{"get-many", "users", "--with-key"}},
		{"negative limit", []string{"scan", "users", "--limit", "-1"}},
		{"count range", []string{"count", "users", "--start", "z", "--end", "a"}},
		{"drop collection", []string{"drop", "bad/name"}},
		{"drop sync", []string{"drop", "users", "--sync", "--no-sync"}},
		{"join fields", []string{"join", "users", "--as", "user"}},
	}
	for _, tc := range cases {
		out, err, code := run(t, db, "", tc.args...)
		if out != "" || err == "" || code != 3 {
			t.Fatalf("%s out=%q err=%q code=%d", tc.name, out, err, code)
		}
	}
}

func TestCLIRejectsEmptyStreamKey(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "", "init"); out != "" || err != "" || code != 0 {
		t.Fatalf("init out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "\n", "get-many", "users"); out != "" || !strings.Contains(err, "empty key") || code != 4 {
		t.Fatalf("empty stream key out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIReadRequiresExistingDB(t *testing.T) {
	db := filepath.Join(t.TempDir(), "missing")
	for _, args := range [][]string{{"get", "users", "a"}, {"scan", "users"}, {"count", "users"}, {"drop", "users"}} {
		out, err, code := run(t, db, "", args...)
		if out != "" || !strings.Contains(err, "database does not exist") || code != 5 {
			t.Fatalf("%v out=%q err=%q code=%d", args, out, err, code)
		}
		if _, statErr := os.Stat(db); !os.IsNotExist(statErr) {
			t.Fatalf("command created db: %v", statErr)
		}
	}
}

func TestCLIMetadataRawKeysAndValues(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "blob-value", "import", "blob", "--format", "raw", "--key", "one"); out != "" || err != "" || code != 0 {
		t.Fatalf("raw import out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, usersImport, "import", "users", "--format", "kv"); out != "" || err != "" || code != 0 {
		t.Fatalf("kv import out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "get", "blob", "one", "--no-newline"); out != "blob-value" || err != "" || code != 0 {
		t.Fatalf("raw get out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "collections"); out != "blob\nusers\n" || err != "" || code != 0 {
		t.Fatalf("collections out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "scan", "users", "--keys-only", "--start", "u1", "--end", "u3"); out != "u1\nu2\n" || err != "" || code != 0 {
		t.Fatalf("keys out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "scan", "users", "--values-only", "--prefix", "u1"); out != "Ada\n" || err != "" || code != 0 {
		t.Fatalf("values out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "info"); !strings.Contains(out, "storage_format_version: 1\n") || !strings.Contains(out, "collections: 2\n") || err != "" || code != 0 {
		t.Fatalf("info out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "stats"); !strings.Contains(out, "disk_used:") || err != "" || code != 0 {
		t.Fatalf("stats out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIMetadataNDJSONUsesStableNamesAndOptionalRawStats(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "", "put", "users", "a", "A"); out != "" || err != "" || code != 0 {
		t.Fatalf("put out=%q err=%q code=%d", out, err, code)
	}
	out, errText, code := run(t, db, "", "info", "--format", "ndjson")
	if errText != "" || code != 0 {
		t.Fatalf("info err=%q code=%d", errText, code)
	}
	var info map[string]any
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatal(err)
	}
	if info["path"] != db || info["storage_format_version"] != float64(1) || info["collection_count"] != float64(1) || info["created_at"] == "" || len(info) != 4 {
		t.Fatalf("info = %#v", info)
	}

	out, errText, code = run(t, db, "", "stats", "--format", "ndjson")
	if errText != "" || code != 0 {
		t.Fatalf("stats err=%q code=%d", errText, code)
	}
	var stats map[string]any
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		t.Fatal(err)
	}
	if stats["path"] != db || stats["disk_used"] == nil || stats["raw"] != nil || len(stats) != 2 {
		t.Fatalf("stats = %#v", stats)
	}

	out, errText, code = run(t, db, "", "stats", "--format", "ndjson", "--raw")
	if errText != "" || code != 0 {
		t.Fatalf("raw stats err=%q code=%d", errText, code)
	}
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		t.Fatal(err)
	}
	if raw, ok := stats["raw"].(string); !ok || raw == "" {
		t.Fatalf("raw stats = %#v", stats)
	}
}

func TestCLIImportExportAndStreams(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, usersImport, "import", "users", "--format", "kv"); out != "" || err != "" || code != 0 {
		t.Fatalf("import out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "scan", "users", "--format", "kv"); out != usersExport || err != "" || code != 0 {
		t.Fatalf("export out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, getManyInput, "get-many", "users"); out != getManyOutput || err != "" || code != 0 {
		t.Fatalf("get-many out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, existsInput, "exists", "users"); out != existsOutput || err != "" || code != 0 {
		t.Fatalf("exists out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLINDJSONStreamCommandsHonorKeySeparator(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "left|right\tvalue\n", "import", "users", "--format", "kv"); out != "" || err != "" || code != 0 {
		t.Fatalf("import out=%q err=%q code=%d", out, err, code)
	}
	input := "{\"left\":\"left\",\"right\":\"right\"}\n"
	keyArgs := []string{"--input-format", "ndjson", "--key-field", "left", "--key-field", "right", "--key-sep", "|"}
	if out, err, code := run(t, db, input, append([]string{"get-many", "users"}, keyArgs...)...); out != "value\n" || err != "" || code != 0 {
		t.Fatalf("get-many out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, input, append([]string{"exists", "users"}, keyArgs...)...); out != input || err != "" || code != 0 {
		t.Fatalf("exists out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, input, append([]string{"del-many", "users"}, keyArgs...)...); out != "" || err != "" || code != 0 {
		t.Fatalf("del-many out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "scan", "users"); out != "" || err != "" || code != 0 {
		t.Fatalf("scan out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIApplyKcatCompactedStream(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	in := "a\t1\tA\nb\t1\tB\na\t1\tC\nb\t-1\t\nempty\t0\t\n"
	if out, err, code := run(t, db, in, "apply", "users", "--format", "kcat"); out != "" || err != "" || code != 0 {
		t.Fatalf("apply out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "scan", "users"); out != kv(row("a", "C"), row("empty", "")) || err != "" || code != 0 {
		t.Fatalf("scan out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIApplyFrameAndStats(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	in := "D 7\nmissingP 1 1\naAD 1\naP 1 0\nb"
	out, errText, code := run(t, db, in, "apply", "users", "--format", "frame", "--stats")
	if out != "" || code != 0 {
		t.Fatalf("apply out=%q err=%q code=%d", out, errText, code)
	}
	for _, want := range []string{"records=4", "puts=2", "deletes=2", "batches=1"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("stats missing %q: %q", want, errText)
		}
	}
	if strings.Contains(errText, "deletes_skipped") {
		t.Fatalf("unfiltered stats changed: %q", errText)
	}
	if out, err, code := run(t, db, "", "scan", "users"); out != kv(row("b", "")) || err != "" || code != 0 {
		t.Fatalf("scan out=%q err=%q code=%d", out, err, code)
	}
	out, errText, code = run(t, db, in, "--quiet", "apply", "users", "--format", "frame", "--stats")
	if out != "" || errText != "" || code != 0 {
		t.Fatalf("quiet stats out=%q err=%q code=%d", out, errText, code)
	}
}

func TestCLIApplyBloomFilter(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "", "put", "users", "seed", "old"); out != "" || err != "" || code != 0 {
		t.Fatalf("seed out=%q err=%q code=%d", out, err, code)
	}
	in := "D 7\nmissingD 4\nseedP 1 1\naAD 1\naD 5\nlaterP 5 1\nlaterL"
	out, errText, code := run(t, db, in, "apply", "users", "--format", "frame", "--batch-size", "1", "--bloom-filter", "--expected-key-count", "1M", "--stats")
	if out != "" || code != 0 {
		t.Fatalf("apply out=%q err=%q code=%d", out, errText, code)
	}
	for _, want := range []string{"puts=2", "deletes_skipped="} {
		if !strings.Contains(errText, want) {
			t.Fatalf("stats missing %q: %q", want, errText)
		}
	}
	if out, err, code := run(t, db, "", "scan", "users"); out != "later\tL\n" || err != "" || code != 0 {
		t.Fatalf("scan out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIFrameExportRoundTrip(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	frame := "P 2 3\nk\x00v\n\t"
	if out, err, code := run(t, db, frame, "apply", "source", "--format", "frame"); out != "" || err != "" || code != 0 {
		t.Fatalf("apply source out=%q err=%q code=%d", out, err, code)
	}
	out, errText, code := run(t, db, "", "scan", "source", "--format", "frame")
	if out != frame || errText != "" || code != 0 {
		t.Fatalf("export out=%q err=%q code=%d", out, errText, code)
	}
	if out, err, code := run(t, db, out, "apply", "copy", "--format", "frame"); out != "" || err != "" || code != 0 {
		t.Fatalf("apply copy out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "scan", "copy", "--format", "frame"); out != frame || err != "" || code != 0 {
		t.Fatalf("copy export out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLINDJSONKeysAreUnambiguousStrings(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	in := "{\"left\":\"a:b\",\"right\":\"c\"}\n"
	if out, err, code := run(t, db, in, "import", "users", "--format", "ndjson", "--key-field", "left", "--key-field", "right"); out != "" || !strings.Contains(err, "contains separator") || code != 4 {
		t.Fatalf("compound out=%q err=%q code=%d", out, err, code)
	}
	db = filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "{\"id\":1}\n", "import", "users", "--format", "ndjson", "--key-field", "id"); out != "" || !strings.Contains(err, "key fields must be strings") || code != 4 {
		t.Fatalf("numeric out=%q err=%q code=%d", out, err, code)
	}
}

func TestParseSizeRejectsOverflow(t *testing.T) {
	if _, err := parseSize("18446744073709551615M"); err == nil {
		t.Fatal("parseSize accepted overflow")
	}
}

func TestParseCount(t *testing.T) {
	for input, want := range map[string]uint64{
		"42":   42,
		"2K":   2_000,
		"800m": 800_000_000,
		"1B":   1_000_000_000,
	} {
		got, err := parseCount(input)
		if err != nil || got != want {
			t.Fatalf("parseCount(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []string{"", "0", "-1", "1.5M", "1MB", "18446744074B"} {
		if _, err := parseCount(input); err == nil {
			t.Fatalf("parseCount(%q) succeeded", input)
		}
	}
}

func TestCLIApplyBadInput(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, "a\t4\txy", "apply", "users", "--format", "kcat"); out != "" || !strings.Contains(err, "truncated payload") || code != 4 {
		t.Fatalf("apply out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIJoin(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, userAdaNDJSON, "import", "users", "--format", "ndjson", "--key-field", "id"); out != "" || err != "" || code != 0 {
		t.Fatalf("import out=%q err=%q code=%d", out, err, code)
	}
	out, errText, code := run(t, db, loginEventNDJSON, "join", "users", "--on", "user_id", "--as", "user")
	if errText != "" || code != 0 {
		t.Fatalf("join err=%q code=%d", errText, code)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"event":   "login",
		"user_id": "u1",
		"user": map[string]any{
			"id":   "u1",
			"name": "Ada",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("join = %#v", got)
	}
}

func row(key, value string) [2]string {
	return [2]string{key, value}
}

func kv(rows ...[2]string) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r[0])
		b.WriteByte(fieldSep)
		b.WriteString(r[1])
		b.WriteByte(recordSep)
	}
	return b.String()
}

func TestCLIJSONJoinPreservesNumbers(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	value := "{\n\"n\":9007199254740993,\"huge\":1e400,\"fraction\":1.234567890123456789\n}"
	if _, err, code := run(t, db, value, "put", "docs", "u1", "--stdin"); err != "" || code != 0 {
		t.Fatalf("put err=%q code=%d", err, code)
	}
	input := "{\"user\":{\"id\":\"u1\"},\"n\":9007199254740993,\"doc\":false}\n"
	out, err, code := run(t, db, input, "join", "docs", "--on", "user.id", "--as", "doc")
	if err != "" || code != 0 || strings.Count(out, "\n") != 1 {
		t.Fatalf("lookup out=%q err=%q code=%d", out, err, code)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}
	if string(obj["n"]) != "9007199254740993" || string(obj["doc"]) != `{"n":9007199254740993,"huge":1e400,"fraction":1.234567890123456789}` {
		t.Fatalf("lookup changed numbers: %s", out)
	}
	if _, err, code := run(t, db, "not json", "put", "docs", "u1", "--stdin"); err != "" || code != 0 {
		t.Fatalf("put err=%q code=%d", err, code)
	}
	if out, err, code := run(t, db, input, "join", "docs", "--on", "user.id", "--as", "doc"); out != "" || code != 4 || !strings.Contains(err, "not valid JSON") {
		t.Fatalf("invalid stored JSON out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLISelectionAndCount(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if _, err, code := run(t, db, "a:1\t1\na:2\t2\na:3\t3\nb:1\t4\n", "import", "events", "--format", "kv"); err != "" || code != 0 {
		t.Fatalf("import err=%q code=%d", err, code)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"scan", "events", "--reverse", "--limit", "2", "--values-only"}, "4\n3\n"},
		{[]string{"scan", "events", "--prefix", "a:", "--start", "a:2", "--reverse", "--keys-only"}, "a:3\na:2\n"},
		{[]string{"scan", "events", "--end", "a:2"}, "a:1\t1\n"},
		{[]string{"scan", "events", "--start", "b:"}, "b:1\t4\n"},
		{[]string{"scan", "events", "--start", "a:2", "--end", "a:2"}, ""},
		{[]string{"scan", "events", "--prefix", "b:", "--end", "a:2", "--reverse"}, ""},
		{[]string{"scan", "events", "--prefix", "a:", "--start", "b:"}, ""},
		{[]string{"scan", "events", "--end", ""}, ""},
		{[]string{"scan", "events", "--prefix", "", "--format", "raw"}, "1234"},
		{[]string{"scan", "events", "--prefix", "a:", "--end", "a:3", "--reverse", "--limit", "1", "--format", "ndjson", "--with-key"}, "{\"_key\":\"a:2\",\"_value\":2}\n"},
		{[]string{"count", "events"}, "4\n"},
		{[]string{"count", "events", "--prefix", "a:", "--start", "a:2", "--end", "b:"}, "2\n"},
		{[]string{"count", "events", "--end", ""}, "0\n"},
		{[]string{"count", "absent"}, "0\n"},
	} {
		out, err, code := run(t, db, "", tc.args...)
		if out != tc.want || err != "" || code != 0 {
			t.Fatalf("%v out=%q err=%q code=%d, want %q", tc.args, out, err, code, tc.want)
		}
	}
	if _, err, code := run(t, db, "", "put", "events", "z\nkey", ""); err != "" || code != 0 {
		t.Fatalf("put binary key err=%q code=%d", err, code)
	}
	if out, err, code := run(t, db, "", "count", "events"); out != "5\n" || err != "" || code != 0 {
		t.Fatalf("count binary key out=%q err=%q code=%d", out, err, code)
	}
	var stderr bytes.Buffer
	if code := Main([]string{"--db", db, "count", "events"}, strings.NewReader(""), errWriter{}, &stderr); code != 1 {
		t.Fatalf("count output failure code=%d err=%q", code, stderr.String())
	}
}

func TestCLIDropAndRecreateCollection(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	for _, name := range []string{"users", "userz", "users2"} {
		if _, err, code := run(t, db, "P 2 1\nk\x00vP 1 0\nx", "apply", name, "--format", "frame"); err != "" || code != 0 {
			t.Fatalf("seed %s err=%q code=%d", name, err, code)
		}
	}
	for _, name := range []string{"users", "users", "absent"} {
		if out, err, code := run(t, db, "", "drop", name); out != "" || err != "" || code != 0 {
			t.Fatalf("drop %s out=%q err=%q code=%d", name, out, err, code)
		}
	}
	if out, err, code := run(t, db, "", "collections"); out != "users2\nuserz\n" || err != "" || code != 0 {
		t.Fatalf("collections out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "count", "users"); out != "0\n" || err != "" || code != 0 {
		t.Fatalf("dropped data out=%q err=%q code=%d", out, err, code)
	}
	for _, name := range []string{"userz", "users2"} {
		if out, err, code := run(t, db, "", "count", name); out != "2\n" || err != "" || code != 0 {
			t.Fatalf("neighbor %s out=%q err=%q code=%d", name, out, err, code)
		}
	}
	if _, err, code := run(t, db, "", "put", "users", "new", "value"); err != "" || code != 0 {
		t.Fatalf("recreate err=%q code=%d", err, code)
	}
	if out, err, code := run(t, db, "", "scan", "users"); out != "new\tvalue\n" || err != "" || code != 0 {
		t.Fatalf("recreated data out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "collections"); out != "users\nusers2\nuserz\n" || err != "" || code != 0 {
		t.Fatalf("recreated metadata out=%q err=%q code=%d", out, err, code)
	}
	if _, err, code := run(t, db, "", "import", "empty", "--format", "kv"); err != "" || code != 0 {
		t.Fatalf("empty import err=%q code=%d", err, code)
	}
	if _, err, code := run(t, db, "", "drop", "empty"); err != "" || code != 0 {
		t.Fatalf("empty drop err=%q code=%d", err, code)
	}
	if out, err, code := run(t, db, "", "collections"); out != "users\nusers2\nuserz\n" || err != "" || code != 0 {
		t.Fatalf("empty drop metadata out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLICompoundJoinMissingPolicies(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if _, err, code := run(t, db, "{\"org\":\"a\",\"id\":\"b\",\"n\":9007199254740993}\n", "import", "users", "--format", "ndjson", "--key-field", "org", "--key-field", "id", "--key-sep", "|"); err != "" || code != 0 {
		t.Fatalf("import err=%q code=%d", err, code)
	}
	input := "{\"org\":\"a\",\"id\":\"b\"}\n{\"org\":\"a\",\"id\":\"missing\"}\n"
	found := "{\"id\":\"b\",\"org\":\"a\",\"user\":{\"org\":\"a\",\"id\":\"b\",\"n\":9007199254740993}}\n"
	for _, policy := range []string{"null", "skip", "error"} {
		args := []string{"join", "users", "--on", "org", "--on", "id", "--key-sep", "|", "--as", "user", "--missing", policy}
		out, err, code := run(t, db, input, args...)
		want, wantCode := found, 0
		if policy == "null" {
			want += "{\"id\":\"missing\",\"org\":\"a\",\"user\":null}\n"
		}
		if policy == "error" {
			wantCode = 6
		}
		if out != want || code != wantCode || (err != "") != (policy == "error") {
			t.Fatalf("join %s out=%q err=%q code=%d", policy, out, err, code)
		}
	}
}

func BenchmarkWriteJoin(b *testing.B) {
	for _, shape := range []string{"small", "nested"} {
		b.Run(shape, func(b *testing.B) {
			value := []byte(`{"active":true,"score":12.5}`)
			if shape == "nested" {
				value = []byte(`[` + strings.Repeat(`{"name":"Ada","active":true,"score":12.5,"tags":["a","b"]},`, 63) + `null]`)
			}
			rec := codec.Record{Key: []byte("u1"), JSON: map[string]json.RawMessage{"id": json.RawMessage(`"u1"`)}}
			c := &cli{stdout: io.Discard}
			b.ReportAllocs()
			b.SetBytes(int64(len(value)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := c.writeJoin(rec, value, "doc", false); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestJoinRejectsInvalidStoredJSON(t *testing.T) {
	for _, value := range [][]byte{nil, {}, []byte(" "), []byte("null null"), []byte("[1,]"), []byte(`{"nested":{"bad":}}`)} {
		var out bytes.Buffer
		c := &cli{stdout: &out}
		rec := codec.Record{Key: []byte("k"), JSON: map[string]json.RawMessage{"id": json.RawMessage(`"k"`)}}
		err := c.writeJoin(rec, value, "doc", false)
		if err == nil || exitCode(err) != codeBadInput || err.Error() != `stored value for key "k" is not valid JSON` || out.Len() != 0 {
			t.Fatalf("value=%q out=%q err=%v", value, out.String(), err)
		}
	}
}
