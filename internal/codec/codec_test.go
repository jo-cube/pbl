package codec

import (
	"bytes"
	"strings"
	"testing"
)

const tab = "\t"
const newline = "\n"

const wrappedAda = `{"_key":"u1","_value":{"name":"Ada"}}
`

func TestParseKVFirstTab(t *testing.T) {
	rec, err := ParseKV([]byte("k" + tab + "v" + tab + "still-value"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rec.Key) != "k" || string(rec.Value) != "v"+tab+"still-value" {
		t.Fatalf("record = %q %q", rec.Key, rec.Value)
	}
}

func TestLineReaderLargeLine(t *testing.T) {
	in := strings.Repeat("x", 200000) + newline
	lr := NewLineReader(strings.NewReader(in))
	line, _, err := lr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if len(line) != 200000 {
		t.Fatalf("len = %d", len(line))
	}
}

func TestNDJSONExtractKey(t *testing.T) {
	obj := map[string]any{"user": map[string]any{"id": "u1"}, "n": float64(12), "ok": true}
	key, err := ExtractKey(obj, []string{"user.id", "n", "ok"}, ":")
	if err != nil {
		t.Fatal(err)
	}
	if key != "u1:12:true" {
		t.Fatalf("key = %q", key)
	}
}

func TestWriteNDJSONValueIncludesKey(t *testing.T) {
	var b bytes.Buffer
	if err := WriteNDJSONValue(&b, []byte("u1"), []byte(`{"name":"Ada"}`), true); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != wrappedAda {
		t.Fatalf("got %q", got)
	}
}
