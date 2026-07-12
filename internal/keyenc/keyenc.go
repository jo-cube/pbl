package keyenc

import "encoding/binary"

const (
	MetaPrefix byte = 0x00
	DataPrefix byte = 0x01
)

func MetadataKey(name string) []byte {
	out := make([]byte, 1, 1+len(name))
	out[0] = MetaPrefix
	return append(out, name...)
}

func CollectionMetaKey(collection string) []byte {
	return MetadataKey("collection/" + collection)
}

func CollectionMetaPrefix() []byte {
	return MetadataKey("collection/")
}

func DataKey(collection string, userKey []byte) []byte {
	base := CollectionBase(collection)
	out := make([]byte, 0, len(base)+len(userKey))
	out = append(out, base...)
	out = append(out, userKey...)
	return out
}

func CollectionBase(collection string) []byte {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], uint64(len(collection)))
	out := make([]byte, 0, 1+n+len(collection)+1)
	out = append(out, DataPrefix)
	out = append(out, buf[:n]...)
	out = append(out, collection...)
	out = append(out, 0x00)
	return out
}

func CollectionBounds(collection string) (lower, upper []byte) {
	lower = CollectionBase(collection)
	if next, ok := NextPrefix(lower); ok {
		upper = next
	}
	return lower, upper
}

func PrefixBounds(collection string, prefix []byte) (lower, upper []byte) {
	base := CollectionBase(collection)
	lower = append(append([]byte(nil), base...), prefix...)
	if next, ok := NextPrefix(lower); ok {
		return lower, next
	}
	_, upper = CollectionBounds(collection)
	return lower, upper
}

func RangeBounds(collection string, start, end []byte) (lower, upper []byte) {
	base := CollectionBase(collection)
	lower = append(append([]byte(nil), base...), start...)
	upper = append(append([]byte(nil), base...), end...)
	return lower, upper
}

func DecodeDataKey(physical []byte) (string, []byte, bool) {
	collection, userKey, ok := DecodeDataKeyView(physical)
	if !ok {
		return "", nil, false
	}
	return string(collection), append([]byte(nil), userKey...), true
}

// DecodeDataKeyView returns slices backed by physical.
func DecodeDataKeyView(physical []byte) ([]byte, []byte, bool) {
	if len(physical) < 3 || physical[0] != DataPrefix {
		return nil, nil, false
	}
	n, used := binary.Uvarint(physical[1:])
	if used <= 0 {
		return nil, nil, false
	}
	pos := 1 + used
	if pos >= len(physical) || n > uint64(len(physical)-pos-1) {
		return nil, nil, false
	}
	end := pos + int(n)
	if physical[end] != 0x00 {
		return nil, nil, false
	}
	return physical[pos:end], physical[end+1:], true
}

func NextPrefix(b []byte) ([]byte, bool) {
	out := append([]byte(nil), b...)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] != 0xff {
			out[i]++
			return out[:i+1], true
		}
	}
	return nil, false
}
