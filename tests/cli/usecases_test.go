package cli_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jo-cube/pbl/internal/app"
)

type step struct {
	name    string
	args    []string
	stdin   string
	wantOut string
	wantErr string
	code    int
}

const stepFailure = `%s
pbl %s
stdout=%q
stderr=%q
code=%d`

const fieldSep = '\t'
const recordSep = '\n'
const lineSep = "\n"

var (
	eventsKV = kv(
		row("u2:1", "C"),
		row("u1:2", "B"),
		row("u1:1", "A"),
	)
	eventsScan = kv(
		row("u1:1", "A"),
		row("u1:2", "B"),
		row("u2:1", "C"),
	)
	eventsPrefix = kv(
		row("u1:1", "A"),
		row("u1:2", "B"),
	)
	eventsRange = kv(
		row("u1:2", "B"),
		row("u2:1", "C"),
	)
	usersKV = kv(
		row("u1", "Ada"),
		row("u2", "Grace"),
		row("u3", "Linus"),
	)
	remainingUserKV = kv(row("u1", "Ada"))
)

const blockedUsers = `u2
u4
`

const incomingUsers = `u1
u2
u3
u4
`

const blockedUsersOnly = `u2
u4
`

const unblockedUsersOnly = `u1
u3
`

const usersNDJSON = `{"id":"u1","name":"Ada"}
{"id":"u2","name":"Grace"}
`

const eventsNDJSON = `{"event":"login","user_id":"u1"}
{"event":"logout","user_id":"missing"}
`

const compoundEventsNDJSON = `{"user_id":"u2","ts":"002","event":"view"}
{"user_id":"u1","ts":"002","event":"click"}
{"user_id":"u1","ts":"001","event":"login"}
`

const u1CompoundKeys = `u1:001
u1:002
`

const getManyInput = `u2
missing
u1
`

const getManyOutput = `Grace
Ada
`

const deleteManyInput = `u2
u3
`

func runStep(t *testing.T, db string, s step) string {
	t.Helper()
	var out, err bytes.Buffer
	args := append([]string{"--db", db}, s.args...)
	code := app.Main(args, strings.NewReader(s.stdin), &out, &err)
	if code != s.code || out.String() != s.wantOut || err.String() != s.wantErr {
		t.Fatalf(stepFailure, s.name, strings.Join(args, " "), out.String(), err.String(), code)
	}
	return out.String()
}

func TestUseCaseKVImportScanPrefixRange(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	for _, s := range []step{
		{name: "import unordered KV", args: []string{"import", "events", "--format", "kv"}, stdin: eventsKV, code: 0},
		{name: "scan is ordered", args: []string{"scan", "events"}, wantOut: eventsScan, code: 0},
		{name: "prefix scan", args: []string{"prefix", "events", "u1:"}, wantOut: eventsPrefix, code: 0},
		{name: "half-open range", args: []string{"range", "events", "u1:2", "u3"}, wantOut: eventsRange, code: 0},
	} {
		runStep(t, db, s)
	}
}

func TestUseCasePersistentSetExistsInvert(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	for _, s := range []step{
		{name: "import set", args: []string{"import", "blocked", "--format", "line", "--key-mode", "value"}, stdin: blockedUsers, code: 0},
		{name: "keep members", args: []string{"exists", "blocked"}, stdin: incomingUsers, wantOut: blockedUsersOnly, code: 0},
		{name: "keep non-members", args: []string{"exists", "blocked", "--invert"}, stdin: incomingUsers, wantOut: unblockedUsersOnly, code: 0},
	} {
		runStep(t, db, s)
	}
}

func TestUseCaseNDJSONImportAndJoin(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	runStep(t, db, step{name: "import users", args: []string{"import", "users", "--format", "ndjson", "--key-field", "id"}, stdin: usersNDJSON, code: 0})

	var out, err bytes.Buffer
	code := app.Main([]string{"--db", db, "join", "users", "--on", "user_id", "--as", "user"}, strings.NewReader(eventsNDJSON), &out, &err)
	if code != 0 || err.String() != "" {
		t.Fatalf("join stderr=%q code=%d", err.String(), code)
	}
	want := []map[string]any{
		{"event": "login", "user_id": "u1", "user": map[string]any{"id": "u1", "name": "Ada"}},
		{"event": "logout", "user_id": "missing", "user": nil},
	}
	assertNDJSON(t, out.String(), want)
}

func TestUseCaseCompoundKeyPrefixScan(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	for _, s := range []step{
		{name: "import compound keys", args: []string{"import", "events", "--format", "ndjson", "--key-field", "user_id", "--key-field", "ts"}, stdin: compoundEventsNDJSON, code: 0},
		{name: "prefix one user", args: []string{"prefix", "events", "u1:", "--keys-only"}, wantOut: u1CompoundKeys, code: 0},
	} {
		runStep(t, db, s)
	}
}

func TestUseCaseGetManyAndDelMany(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	for _, s := range []step{
		{name: "import lookup table", args: []string{"import", "users", "--format", "kv"}, stdin: usersKV, code: 0},
		{name: "get many preserves input order", args: []string{"get-many", "users"}, stdin: getManyInput, wantOut: getManyOutput, code: 0},
		{name: "delete many", args: []string{"del-many", "users"}, stdin: deleteManyInput, code: 0},
		{name: "remaining record", args: []string{"scan", "users"}, wantOut: remainingUserKV, code: 0},
	} {
		runStep(t, db, s)
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

func assertNDJSON(t *testing.T, got string, want []map[string]any) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(got), lineSep)
	if len(lines) != len(want) {
		t.Fatalf("line count = %d, want %d: %q", len(lines), len(want), got)
	}
	for i, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %d invalid json: %v", i+1, err)
		}
		if !reflect.DeepEqual(m, want[i]) {
			t.Fatalf("line %d = %#v, want %#v", i+1, m, want[i])
		}
	}
}
