package codec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const MaxRecordBytes = 64 << 20

var ErrRecordTooLarge = errors.New("record exceeds 64 MiB")

type Record struct {
	Key   []byte
	Value []byte
	Raw   []byte
	Line  int64
	JSON  map[string]any
}

type ApplyRecord struct {
	Delete bool
	Key    []byte
	Value  []byte
	Line   int64
	Bytes  int64
}

type LineReader struct {
	r    *bufio.Reader
	line int64
}

func NewLineReader(r io.Reader) *LineReader {
	return &LineReader{r: bufio.NewReaderSize(r, 64*1024)}
}

func (r *LineReader) Next() ([]byte, int64, error) {
	out, _, err := readUntil(r.r, '\n', MaxRecordBytes+1)
	if err == io.EOF && len(out) == 0 {
		return nil, r.line, io.EOF
	}
	if err != nil && err != io.EOF {
		return nil, r.line, err
	}
	r.line++
	out = TrimLine(out)
	if len(out) > MaxRecordBytes {
		return nil, r.line, ErrRecordTooLarge
	}
	return out, r.line, nil
}

func ReadRaw(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxRecordBytes {
		return nil, ErrRecordTooLarge
	}
	return b, nil
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
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&obj); err != nil {
			return fmt.Errorf("line %d: invalid json: %w", n, err)
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return fmt.Errorf("line %d: invalid json: multiple values", n)
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

func ReadKcatApplyRecords(r io.Reader, fn func(ApplyRecord) error) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var line, total int64
	for {
		key, n, err := readUntil(br, '\t', MaxRecordBytes)
		total += n
		if err == io.EOF && len(key) == 0 {
			return nil
		}
		line++
		if errors.Is(err, ErrRecordTooLarge) {
			return fmt.Errorf("record %d: %w", line, err)
		}
		if err != nil {
			return fmt.Errorf("record %d: missing key separator", line)
		}
		if len(key) == 0 {
			return fmt.Errorf("record %d: empty key", line)
		}
		sizeText, n, err := readUntil(br, '\t', MaxRecordBytes)
		total += n
		if errors.Is(err, ErrRecordTooLarge) {
			return fmt.Errorf("record %d: %w", line, err)
		}
		if err != nil {
			return fmt.Errorf("record %d: missing payload length separator", line)
		}
		size, err := strconv.ParseInt(string(sizeText), 10, 64)
		if err != nil || size < -1 {
			return fmt.Errorf("record %d: invalid payload length", line)
		}
		if size > MaxRecordBytes || size >= 0 && size > int64(MaxRecordBytes-len(key)) {
			return fmt.Errorf("record %d: %w", line, ErrRecordTooLarge)
		}
		rec := ApplyRecord{Delete: size == -1, Key: append([]byte(nil), key...), Line: line}
		if size >= 0 {
			rec.Value = make([]byte, size)
			nn, err := io.ReadFull(br, rec.Value)
			total += int64(nn)
			if err != nil {
				return fmt.Errorf("record %d: truncated payload", line)
			}
		}
		trailer, err := br.ReadByte()
		if err == nil {
			total++
			if trailer != '\n' {
				return fmt.Errorf("record %d: missing record newline", line)
			}
		} else if err != io.EOF {
			return err
		}
		rec.Bytes = total
		if err := fn(rec); err != nil {
			return err
		}
		if err == io.EOF {
			return nil
		}
	}
}

func ReadFrameApplyRecords(r io.Reader, fn func(ApplyRecord) error) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var line, total int64
	for {
		header, n, err := readUntil(br, '\n', MaxRecordBytes)
		if err == io.EOF && len(header) == 0 {
			return nil
		}
		line++
		total += n
		if errors.Is(err, ErrRecordTooLarge) {
			return fmt.Errorf("record %d: %w", line, err)
		}
		if err != nil {
			return fmt.Errorf("record %d: truncated header", line)
		}
		parts := strings.Split(string(header), " ")
		rec, body, err := parseFrameHeader(parts, line)
		if err != nil {
			return err
		}
		bodyN, err := io.ReadFull(br, body)
		total += int64(bodyN)
		if err != nil {
			return fmt.Errorf("record %d: truncated body", line)
		}
		rec.Bytes = total
		if err := fn(rec); err != nil {
			return err
		}
	}
}

func parseFrameHeader(parts []string, line int64) (ApplyRecord, []byte, error) {
	if len(parts) == 0 {
		return ApplyRecord{}, nil, fmt.Errorf("record %d: empty header", line)
	}
	switch parts[0] {
	case "P":
		if len(parts) != 3 {
			return ApplyRecord{}, nil, fmt.Errorf("record %d: invalid put header", line)
		}
		keyLen, valueLen, err := parseFrameLengths(parts[1], parts[2])
		if err != nil {
			return ApplyRecord{}, nil, fmt.Errorf("record %d: %w", line, err)
		}
		if keyLen > MaxRecordBytes || valueLen > MaxRecordBytes-keyLen {
			return ApplyRecord{}, nil, fmt.Errorf("record %d: %w", line, ErrRecordTooLarge)
		}
		body := make([]byte, keyLen+valueLen)
		return ApplyRecord{Key: body[:keyLen], Value: body[keyLen:], Line: line}, body, nil
	case "D":
		if len(parts) != 2 {
			return ApplyRecord{}, nil, fmt.Errorf("record %d: invalid delete header", line)
		}
		keyLen, err := parseFrameLength(parts[1])
		if err != nil {
			return ApplyRecord{}, nil, fmt.Errorf("record %d: %w", line, err)
		}
		if keyLen > MaxRecordBytes {
			return ApplyRecord{}, nil, fmt.Errorf("record %d: %w", line, ErrRecordTooLarge)
		}
		key := make([]byte, keyLen)
		return ApplyRecord{Delete: true, Key: key, Line: line}, key, nil
	default:
		return ApplyRecord{}, nil, fmt.Errorf("record %d: unknown operation", line)
	}
}

func parseFrameLengths(keyText, valueText string) (int, int, error) {
	keyLen, err := parseFrameLength(keyText)
	if err != nil {
		return 0, 0, err
	}
	valueLen, err := parseFrameLength(valueText)
	return keyLen, valueLen, err
}

func parseFrameLength(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid length")
	}
	return n, nil
}

func readUntil(r *bufio.Reader, delim byte, maxBytes int) ([]byte, int64, error) {
	var out []byte
	var n int64
	for {
		part, err := r.ReadSlice(delim)
		n += int64(len(part))
		contentLen := len(out) + len(part)
		if err == nil {
			contentLen--
		}
		if contentLen > maxBytes {
			return nil, n, ErrRecordTooLarge
		}
		out = append(out, part...)
		if err == nil {
			return out[:len(out)-1], n, nil
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return out, n, err
	}
}

func ExtractKey(obj map[string]any, fields []string, sep string) (string, error) {
	if len(fields) == 0 {
		return "", fmt.Errorf("missing key field")
	}
	if len(fields) > 1 && len(sep) != 1 {
		return "", fmt.Errorf("compound key separator must be one byte")
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
		if len(fields) > 1 && strings.Contains(s, sep) {
			return "", fmt.Errorf("key field %q contains separator %q", field, sep)
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
	if s, ok := v.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("key fields must be strings")
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

func WriteFramePut(w io.Writer, key, value []byte) error {
	if len(key) > MaxRecordBytes || len(value) > MaxRecordBytes-len(key) {
		return ErrRecordTooLarge
	}
	if _, err := fmt.Fprintf(w, "P %d %d\n", len(key), len(value)); err != nil {
		return err
	}
	if _, err := w.Write(key); err != nil {
		return err
	}
	_, err := w.Write(value)
	return err
}

func FormatNDJSONValue(key, value []byte, includeKey bool) ([]byte, error) {
	if !includeKey {
		if !json.Valid(value) {
			return nil, fmt.Errorf("value is not valid JSON")
		}
		return append([]byte(nil), value...), nil
	}
	var raw any
	if err := json.Unmarshal(value, &raw); err != nil {
		return nil, fmt.Errorf("value is not valid JSON: %w", err)
	}
	out, err := json.Marshal(map[string]any{"_key": string(key), "_value": raw})
	if err != nil {
		return nil, err
	}
	return out, nil
}
