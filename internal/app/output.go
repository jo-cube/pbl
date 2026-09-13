package app

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/jo-cube/pbl/internal/codec"
)

func (c *cli) forInputRecords(inputFormat string, fields []string, sep string, fn func(codec.Record) error) error {
	check := func(rec codec.Record) error {
		if len(rec.Key) == 0 {
			return badInputf("line %d: empty key", rec.Line)
		}
		return fn(rec)
	}
	switch inputFormat {
	case "line":
		lr := codec.NewLineReader(c.stdin)
		for {
			line, n, err := lr.Next()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				if errors.Is(err, codec.ErrRecordTooLarge) {
					return badInputErr(err)
				}
				return runtimeErr(err)
			}
			if err := check(codec.Record{Key: line, Raw: line, Line: n}); err != nil {
				return err
			}
		}
	case "ndjson":
		return badInputErr(codec.ReadNDJSONRecords(c.stdin, fields, sep, check))
	default:
		return usagef("unknown input format %q", inputFormat)
	}
}

func (c *cli) writeJoin(rec codec.Record, value []byte, asField string, missing bool) error {
	obj := rec.JSON
	if missing {
		obj[asField] = nil
	} else {
		if !json.Valid(value) {
			return badInputf("stored value for key %q is not valid JSON", rec.Key)
		}
		obj[asField] = json.RawMessage(value)
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return runtimeErr(err)
	}
	return runtimeWrap(codec.WriteLine(c.stdout, out))
}

func (c *cli) writeScanRecord(key, value []byte, opts scanOptions) error {
	if opts.keysOnly {
		return runtimeWrap(codec.WriteLine(c.stdout, key))
	}
	if opts.valuesOnly {
		return runtimeWrap(codec.WriteLine(c.stdout, value))
	}
	if opts.format == "frame" {
		return runtimeWrap(codec.WriteFramePut(c.stdout, key, value))
	}
	return c.writeRecord(key, value, opts.format, opts.withKey, false)
}

func (c *cli) writeRecord(key, value []byte, format string, withKey, newline bool) error {
	switch format {
	case "raw":
		if newline {
			return runtimeWrap(codec.WriteLine(c.stdout, value))
		}
		_, err := c.stdout.Write(value)
		return runtimeWrap(err)
	case "kv":
		return runtimeWrap(codec.WriteKV(c.stdout, key, value))
	case "ndjson":
		out, err := codec.FormatNDJSONValue(key, value, withKey)
		if err != nil {
			return badInputErr(err)
		}
		return runtimeWrap(codec.WriteLine(c.stdout, out))
	default:
		return usagef("unknown format %q", format)
	}
}

func (c *cli) handleMissing(policy string, key []byte, format string, withKey bool) error {
	switch policy {
	case "error":
		return notFoundf("not found")
	case "skip":
		return nil
	case "null":
		if format == "kv" {
			return runtimeWrap(codec.WriteKV(c.stdout, key, []byte("null")))
		}
		if format == "ndjson" && withKey {
			out, _ := json.Marshal(map[string]any{"_key": string(key), "_value": nil})
			return runtimeWrap(codec.WriteLine(c.stdout, out))
		}
		return runtimeWrap(codec.WriteLine(c.stdout, []byte("null")))
	default:
		return usagef("unknown missing policy %q", policy)
	}
}
