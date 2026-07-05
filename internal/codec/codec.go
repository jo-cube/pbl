package codec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Record struct {
	Key   []byte
	Value []byte
	Raw   []byte
	Line  int64
	JSON  map[string]any
}

type LineReader struct {
	r    *bufio.Reader
	line int64
}

func NewLineReader(r io.Reader) *LineReader {
	return &LineReader{r: bufio.NewReaderSize(r, 64*1024)}
}

func (r *LineReader) Next() ([]byte, int64, error) {
	var out []byte
	for {
		part, err := r.r.ReadBytes('\n')
		out = append(out, part...)
		if err == nil {
			break
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF {
			if len(out) == 0 {
				return nil, r.line, io.EOF
			}
			break
		}
		return nil, r.line, err
	}
	r.line++
	return TrimLine(out), r.line, nil
}

func TrimLine(b []byte) []byte {
	b = bytes.TrimSuffix(b, []byte{'\n'})
	b = bytes.TrimSuffix(b, []byte{'\r'})
	return b
}

func ParseKV(line []byte) (Record, error) {
	i := bytes.IndexByte(line, '\t')
	if i < 0 {
		return Record{}, fmt.Errorf("missing tab")
	}
	if i == 0 {
		return Record{}, fmt.Errorf("empty key")
	}
	return Record{Key: append([]byte(nil), line[:i]...), Value: append([]byte(nil), line[i+1:]...)}, nil
}

func ReadKVRecords(r io.Reader, fn func(Record) error) error {
	lr := NewLineReader(r)
	for {
		line, n, err := lr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		rec, err := ParseKV(line)
		if err != nil {
			return fmt.Errorf("line %d: %w", n, err)
		}
		rec.Raw = append([]byte(nil), line...)
		rec.Line = n
		if err := fn(rec); err != nil {
			return err
		}
	}
}

func ReadLineRecords(r io.Reader, keyMode string, fn func(Record) error) error {
	lr := NewLineReader(r)
	for {
		line, n, err := lr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		key := line
		if keyMode == "line-number" {
			key = []byte(strconv.FormatInt(n, 10))
		}
		if len(key) == 0 {
			return fmt.Errorf("line %d: empty key", n)
		}
		if err := fn(Record{
			Key: append([]byte(nil), key...), Value: append([]byte(nil), line...),
			Raw: append([]byte(nil), line...), Line: n,
		}); err != nil {
			return err
		}
	}
}

func ReadNDJSONRecords(r io.Reader, fields []string, sep string, fn func(Record) error) error {
	lr := NewLineReader(r)
	for {
		line, n, err := lr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			return fmt.Errorf("line %d: invalid json: %w", n, err)
		}
		key, err := ExtractKey(obj, fields, sep)
		if err != nil {
			return fmt.Errorf("line %d: %w", n, err)
		}
		if err := fn(Record{
			Key: []byte(key), Value: append([]byte(nil), line...),
			Raw: append([]byte(nil), line...), Line: n, JSON: obj,
		}); err != nil {
			return err
		}
	}
}

func ExtractKey(obj map[string]any, fields []string, sep string) (string, error) {
	if len(fields) == 0 {
		return "", fmt.Errorf("missing key field")
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		v, ok := LookupField(obj, field)
		if !ok {
			return "", fmt.Errorf("missing key field %q", field)
		}
		s, err := CanonicalKey(v)
		if err != nil {
			return "", fmt.Errorf("invalid key field %q: %w", field, err)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, sep), nil
}

func LookupField(obj map[string]any, path string) (any, bool) {
	var cur any = obj
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func CanonicalKey(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(x), nil
	case nil:
		return "null", nil
	default:
		return "", fmt.Errorf("object and array keys are not supported")
	}
}

func WriteKV(w io.Writer, key, value []byte) error {
	if _, err := w.Write(key); err != nil {
		return err
	}
	if _, err := w.Write([]byte{'\t'}); err != nil {
		return err
	}
	return WriteLine(w, value)
}

func WriteLine(w io.Writer, value []byte) error {
	_, err := w.Write(append(append([]byte(nil), value...), '\n'))
	return err
}

func WriteNDJSONValue(w io.Writer, key, value []byte, includeKey bool) error {
	if !includeKey {
		if !json.Valid(value) {
			return fmt.Errorf("value is not valid JSON")
		}
		return WriteLine(w, value)
	}
	var raw any
	if err := json.Unmarshal(value, &raw); err != nil {
		return fmt.Errorf("value is not valid JSON: %w", err)
	}
	out, err := json.Marshal(map[string]any{"_key": string(key), "_value": raw})
	if err != nil {
		return err
	}
	return WriteLine(w, out)
}
