package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

func TestCLIRootHelpAndVersion(t *testing.T) {
	var out, err bytes.Buffer
	code := Main([]string{"--version"}, strings.NewReader(""), &out, &err)
	if code != 0 || out.String() != versionOut || err.String() != "" {
		t.Fatalf("version out=%q err=%q code=%d", out.String(), err.String(), code)
	}
	out.Reset()
	err.Reset()
	code = Main([]string{"--help"}, strings.NewReader(""), &out, &err)
	if code != 0 || strings.Count(out.String(), "Usage:") != 1 || err.String() != "" {
		t.Fatalf("help out=%q err=%q code=%d", out.String(), err.String(), code)
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

func TestCLIRejectsInvalidAndConflictingFlags(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	cases := []struct {
		name string
		args []string
	}{
		{"format", []string{"collections", "--format", "yaml"}},
		{"sync", []string{"put", "users", "a", "A", "--sync", "--no-sync"}},
		{"duplicates", []string{"import", "users", "--format", "kv", "--replace", "--ignore-duplicates"}},
		{"range", []string{"keys", "users", "--range-start", "a"}},
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
	out, err, code := run(t, db, "", "get", "users", "a")
	if out != "" || !strings.Contains(err, "database does not exist") || code != 5 {
		t.Fatalf("get missing db out=%q err=%q code=%d", out, err, code)
	}
	if _, statErr := os.Stat(db); !os.IsNotExist(statErr) {
		t.Fatalf("read created db: %v", statErr)
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
	if out, err, code := run(t, db, "", "keys", "users", "--range-start", "u1", "--range-end", "u3"); out != "u1\nu2\n" || err != "" || code != 0 {
		t.Fatalf("keys out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "values", "users", "--prefix", "u1"); out != "Ada\n" || err != "" || code != 0 {
		t.Fatalf("values out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "info"); !strings.Contains(out, "storage_format_version: 1\n") || !strings.Contains(out, "collections: 2\n") || err != "" || code != 0 {
		t.Fatalf("info out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "stats"); !strings.Contains(out, "disk_used:") || err != "" || code != 0 {
		t.Fatalf("stats out=%q err=%q code=%d", out, err, code)
	}
}

func TestCLIImportExportAndStreams(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	if out, err, code := run(t, db, usersImport, "import", "users", "--format", "kv"); out != "" || err != "" || code != 0 {
		t.Fatalf("import out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, "", "export", "users", "--format", "kv"); out != usersExport || err != "" || code != 0 {
		t.Fatalf("export out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, getManyInput, "get-many", "users"); out != getManyOutput || err != "" || code != 0 {
		t.Fatalf("get-many out=%q err=%q code=%d", out, err, code)
	}
	if out, err, code := run(t, db, existsInput, "exists", "users"); out != existsOutput || err != "" || code != 0 {
		t.Fatalf("exists out=%q err=%q code=%d", out, err, code)
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
	if out, err, code := run(t, db, "", "scan", "users"); out != kv(row("b", "")) || err != "" || code != 0 {
		t.Fatalf("scan out=%q err=%q code=%d", out, err, code)
	}
	out, errText, code = run(t, db, in, "--quiet", "apply", "users", "--format", "frame", "--stats")
	if out != "" || errText != "" || code != 0 {
		t.Fatalf("quiet stats out=%q err=%q code=%d", out, errText, code)
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
