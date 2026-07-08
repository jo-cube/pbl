package app

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/jo-cube/pbl/internal/codec"
)

func (c *cli) forInputKeys(inputFormat string, fields []string, sep string, fn func(codec.Record) error) error {
	return c.forInputRecords(inputFormat, fields, sep, fn)
}

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
				return runtimeErr(err)
			}
			if err := check(codec.Record{Key: append([]byte(nil), line...), Raw: append([]byte(nil), line...), Line: n}); err != nil {
				return err
			}
		}
	case "ndjson":
		return badInputErr(codec.ReadNDJSONRecords(c.stdin, fields, sep, check))
	default:
		return usagef("unknown input format %q", inputFormat)
	}
}

func (c *cli) writeLookup(rec codec.Record, value []byte, inputFormat, asField string) error {
	if inputFormat == "line" {
		if value == nil {
			return nil
		}
		return runtimeWrap(codec.WriteLine(c.stdout, value))
	}
	obj := rec.JSON
	if obj == nil {
		if err := json.Unmarshal(rec.Raw, &obj); err != nil {
			return badInputErr(err)
		}
	}
	if value == nil {
		obj[asField] = nil
	} else {
		var attached any
		if err := json.Unmarshal(value, &attached); err != nil {
			return badInputf("stored value for key %q is not valid JSON", rec.Key)
		}
		obj[asField] = attached
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return runtimeErr(err)
	}
	return runtimeWrap(codec.WriteLine(c.stdout, out))
}

func (c *cli) writeScanRecord(key, value []byte, format string, keysOnly, valuesOnly, includeKey bool) error {
	if keysOnly {
		return runtimeWrap(codec.WriteLine(c.stdout, key))
	}
	if valuesOnly {
		if format == "raw" {
			_, err := c.stdout.Write(value)
			return runtimeWrap(err)
		}
		return runtimeWrap(codec.WriteLine(c.stdout, value))
	}
	switch format {
	case "kv":
		return runtimeWrap(codec.WriteKV(c.stdout, key, value))
	case "ndjson":
		out, err := codec.FormatNDJSONValue(key, value, includeKey)
		if err != nil {
			return badInputErr(err)
		}
		return runtimeWrap(codec.WriteLine(c.stdout, out))
	case "raw":
		return usagef("raw export requires --values-only")
	default:
		return usagef("unknown format %q", format)
	}
}

func (c *cli) writeRecord(key, value []byte, format string, withKey, newline bool) error {
	switch format {
	case "raw":
		if _, err := c.stdout.Write(value); err != nil {
			return runtimeWrap(err)
		}
		if newline {
			_, err := fmt.Fprintln(c.stdout)
			return runtimeWrap(err)
		}
		return nil
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
			_, err := fmt.Fprintln(c.stdout, string(out))
			return runtimeWrap(err)
		}
		_, err := fmt.Fprintln(c.stdout, "null")
		return runtimeWrap(err)
	default:
		return usagef("unknown missing policy %q", policy)
	}
}
