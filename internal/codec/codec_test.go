package codec

import (
	"bufio"
	"bytes"
	"errors"
	"strconv"
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

func TestReadUntilEnforcesContentLimit(t *testing.T) {
	br := bufio.NewReaderSize(strings.NewReader("abcd\n"), 2)
	if _, _, err := readUntil(br, '\n', 3); !errors.Is(err, ErrRecordTooLarge) {
		t.Fatalf("oversized read err = %v", err)
	}

	br = bufio.NewReaderSize(strings.NewReader("abc\r\n"), 2)
	line, _, err := readUntil(br, '\n', 4)
	if err != nil || string(TrimLine(line)) != "abc" {
		t.Fatalf("CRLF line = %q, %v", line, err)
	}
}

func TestNDJSONExtractKey(t *testing.T) {
	obj := map[string]any{"user": map[string]any{"id": "u1"}, "ts": "001"}
	key, err := ExtractKey(obj, []string{"user.id", "ts"}, ":")
	if err != nil {
		t.Fatal(err)
	}
	if key != "u1:001" {
		t.Fatalf("key = %q", key)
	}
}

func TestReadNDJSONRecordsRequiresStringKey(t *testing.T) {
	in := `{"id":9007199254740993,"name":"Ada"}` + newline
	err := ReadNDJSONRecords(strings.NewReader(in), []string{"id"}, ":", func(Record) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "key fields must be strings") {
		t.Fatalf("err = %v", err)
	}
}

func TestExtractKeyRejectsAmbiguousCompoundKey(t *testing.T) {
	obj := map[string]any{"left": "a:b", "right": "c"}
	if _, err := ExtractKey(obj, []string{"left", "right"}, ":"); err == nil || !strings.Contains(err.Error(), "contains separator") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ExtractKey(obj, []string{"left", "right"}, "::"); err == nil || !strings.Contains(err.Error(), "separator must be one byte") {
		t.Fatalf("err = %v", err)
	}
}

func TestWriteNDJSONValueIncludesKey(t *testing.T) {
	out, err := FormatNDJSONValue([]byte("u1"), []byte(`{"name":"Ada"}`), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out) + newline; got != wrappedAda {
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

func TestReadFrameApplyRecordsRejectsMalformedHeaders(t *testing.T) {
	for _, input := range []string{"P 1\n", "P 1 1 1\n", "D 1 1\n"} {
		if err := ReadFrameApplyRecords(strings.NewReader(input), func(ApplyRecord) error { return nil }); err == nil {
			t.Fatalf("ReadFrameApplyRecords(%q) succeeded", input)
		}
	}
}

func TestApplyReadersRejectOversizedRecords(t *testing.T) {
	kcat := "a\t" + strconv.FormatInt(MaxRecordBytes+1, 10) + "\t"
	if err := ReadKcatApplyRecords(strings.NewReader(kcat), func(ApplyRecord) error { return nil }); !errors.Is(err, ErrRecordTooLarge) {
		t.Fatalf("kcat err = %v", err)
	}
	frame := "P 1 " + strconv.Itoa(MaxRecordBytes) + "\n"
	if err := ReadFrameApplyRecords(strings.NewReader(frame), func(ApplyRecord) error { return nil }); !errors.Is(err, ErrRecordTooLarge) {
		t.Fatalf("frame err = %v", err)
	}
}

func TestWriteFramePutRoundTrip(t *testing.T) {
	key, value := []byte{'k', 0}, []byte("v\n\t")
	var out bytes.Buffer
	if err := WriteFramePut(&out, key, value); err != nil {
		t.Fatal(err)
	}
	if err := ReadFrameApplyRecords(&out, func(rec ApplyRecord) error {
		if rec.Delete || !bytes.Equal(rec.Key, key) || !bytes.Equal(rec.Value, value) {
			t.Fatalf("record = %#v", rec)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
