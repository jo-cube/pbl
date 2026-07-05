package app

import (
	"bytes"
	"encoding/json"
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
