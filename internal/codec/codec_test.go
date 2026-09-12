package codec

import (
	"bufio"
	"bytes"
	"encoding/json"
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
	obj := map[string]json.RawMessage{"user": json.RawMessage(`{"id":"u1"}`), "ts": json.RawMessage(`"001"`)}
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
	obj := map[string]json.RawMessage{"left": json.RawMessage(`"a:b"`), "right": json.RawMessage(`"c"`)}
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

func TestFormatNDJSONValueCompactsToOneLine(t *testing.T) {
	out, err := FormatNDJSONValue(nil, []byte("{\n  \"name\": \"Ada\"\n}"), false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), `{"name":"Ada"}`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadKcatApplyRecords(t *testing.T) {
	in := "a\t1\tA\nempty\t0\t\ndead\t-1\t\nmulti\t4\tx\ty\n\n"
	var got []ApplyRecord
	if err := ReadKcatApplyRecords(strings.NewReader(in), func(rec ApplyRecord) error {
		rec.Key = bytes.Clone(rec.Key)
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

func TestNDJSONValidationAndNestedKeys(t *testing.T) {
	for _, input := range []string{
		`{"user":{"id":null}}`, `{"user":{"id":12}}`, `{"user":{"id":{}}}`,
		`{"user":{"id":true}}`, `{"user":{"id":[]}}`,
		`{"id":"wrong","user":{}}`, `{"user":null}`, `{"user":[]}`, `null`, `[]`,
		`{"user":{"id":"ok"}} {}`, `{"user":{"id":"ok"},"bad":[1,]}`,
	} {
		if err := ReadNDJSONRecords(strings.NewReader(input), []string{"user.id"}, ":", func(Record) error {
			t.Fatalf("accepted invalid record %s", input)
			return nil
		}); err == nil {
			t.Fatalf("accepted invalid record %s", input)
		}
	}
	input := `{"user":{"id":"first"},"user":{"id":"u\u0031"},"n":1e400}`
	if err := ReadNDJSONRecords(strings.NewReader(input), []string{"user.id"}, ":", func(rec Record) error {
		if string(rec.Key) != "u1" || string(rec.Raw) != input || string(rec.Value) != input {
			t.Fatalf("record = %#v", rec)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNDJSONOutputPreservesNumbersAndValidatesValues(t *testing.T) {
	for _, value := range []string{`9007199254740993`, `1e400`, `-0`, `1.234567890123456789`, "{\n\"n\":9007199254740993,\"s\":\"<>&\"\n}"} {
		out, err := FormatNDJSONValue([]byte("k\"\n"), []byte(value), true)
		if err != nil || !json.Valid(out) || bytes.ContainsAny(out, "\r\n") {
			t.Fatalf("FormatNDJSONValue(%q) = %q, %v", value, out, err)
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(out, &obj); err != nil {
			t.Fatal(err)
		}
		var want bytes.Buffer
		if err := json.Compact(&want, []byte(value)); err != nil {
			t.Fatal(err)
		}
		// Marshal escapes HTML characters but must leave numbers intact.
		expected := strings.NewReplacer("<", `\u003c`, ">", `\u003e`, "&", `\u0026`).Replace(want.String())
		if string(obj["_value"]) != expected {
			t.Fatalf("value = %s, want %s", obj["_value"], expected)
		}
	}
	if _, err := FormatNDJSONValue(nil, nil, true); err == nil {
		t.Fatal("accepted nil JSON")
	}
	for _, value := range []string{"", "null null", "[1,]"} {
		for _, withKey := range []bool{false, true} {
			if _, err := FormatNDJSONValue(nil, []byte(value), withKey); err == nil {
				t.Fatalf("accepted %q", value)
			}
		}
	}
}

func TestKcatKeysAcrossBufferRefills(t *testing.T) {
	var input strings.Builder
	var keys, values []string
	for i, size := range []int{1, 65535, 65536, 65537, 200000, 3} {
		key := strings.Repeat(string(rune('a'+i)), size)
		value := strings.Repeat("v\n\x00", 24000-i)
		keys = append(keys, key)
		values = append(values, value)
		input.WriteString(key + "\t" + strconv.Itoa(len(value)) + "\t" + value + "\n")
	}
	count := 0
	err := ReadKcatApplyRecords(strings.NewReader(input.String()), func(rec ApplyRecord) error {
		if count >= len(keys) || string(rec.Key) != keys[count] || string(rec.Value) != values[count] || rec.Line != int64(count+1) {
			t.Fatalf("record %d corrupted across refill", count)
		}
		count++
		if count == len(keys) && rec.Bytes != int64(input.Len()) {
			t.Fatalf("bytes = %d, want %d", rec.Bytes, input.Len())
		}
		return nil
	})
	if err != nil || count != len(keys) {
		t.Fatalf("records = %d, err = %v", count, err)
	}
}
