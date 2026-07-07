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

func TestReadNDJSONRecordsPreservesLargeNumericKey(t *testing.T) {
	in := `{"id":9007199254740993,"name":"Ada"}` + newline
	var got Record
	if err := ReadNDJSONRecords(strings.NewReader(in), []string{"id"}, ":", func(rec Record) error {
		got = rec
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if string(got.Key) != "9007199254740993" {
		t.Fatalf("key = %q", got.Key)
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

func TestReadKcatApplyRecords(t *testing.T) {
	in := "a\t1\tA\nempty\t0\t\ndead\t-1\t\nmulti\t4\tx\ty\n\n"
	var got []ApplyRecord
	if err := ReadKcatApplyRecords(strings.NewReader(in), func(rec ApplyRecord) error {
		got = append(got, rec)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("records = %d", len(got))
	}
	if got[0].Delete || string(got[0].Key) != "a" || string(got[0].Value) != "A" {
		t.Fatalf("put = %#v", got[0])
	}
	if got[1].Delete || string(got[1].Key) != "empty" || len(got[1].Value) != 0 {
		t.Fatalf("empty put = %#v", got[1])
	}
	if !got[2].Delete || string(got[2].Key) != "dead" {
		t.Fatalf("delete = %#v", got[2])
	}
	if got[3].Delete || string(got[3].Value) != "x\ty\n" {
		t.Fatalf("multiline put = %#v", got[3])
	}
}

func TestReadKcatApplyRecordsRejectsTruncatedPayload(t *testing.T) {
	err := ReadKcatApplyRecords(strings.NewReader("a\t4\txy"), func(ApplyRecord) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "truncated payload") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadFrameApplyRecords(t *testing.T) {
	in := "P 2 3\nk\x00v\n\tD 1\nx"
	var got []ApplyRecord
	if err := ReadFrameApplyRecords(strings.NewReader(in), func(rec ApplyRecord) error {
		got = append(got, rec)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("records = %d", len(got))
	}
	if got[0].Delete || !bytes.Equal(got[0].Key, []byte{'k', 0}) || string(got[0].Value) != "v\n\t" {
		t.Fatalf("put = %#v", got[0])
	}
	if !got[1].Delete || string(got[1].Key) != "x" {
		t.Fatalf("delete = %#v", got[1])
	}
}

func TestReadFrameApplyRecordsRejectsTruncatedBody(t *testing.T) {
	err := ReadFrameApplyRecords(strings.NewReader("P 1 4\nx"), func(ApplyRecord) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "truncated body") {
		t.Fatalf("err = %v", err)
	}
}
