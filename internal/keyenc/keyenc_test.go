package keyenc

import (
	"bytes"
	"testing"
)

func TestDataKeyRoundTrip(t *testing.T) {
	key := []byte{0x00, 'a', 0xff}
	phys := DataKey("users", key)
	gotCollection, gotKey, ok := DecodeDataKey(phys)
	if !ok || gotCollection != "users" || !bytes.Equal(gotKey, key) {
		t.Fatalf("DecodeDataKey() = %q %v %v", gotCollection, gotKey, ok)
	}
}

func TestStorageFormatV1Keys(t *testing.T) {
	if got, want := MetadataKey("format-version"), []byte{0x00, 'f', 'o', 'r', 'm', 'a', 't', '-', 'v', 'e', 'r', 's', 'i', 'o', 'n'}; !bytes.Equal(got, want) {
		t.Fatalf("MetadataKey() = %v, want %v", got, want)
	}
	if got, want := CollectionMetaKey("users"), []byte{0x00, 'c', 'o', 'l', 'l', 'e', 'c', 't', 'i', 'o', 'n', '/', 'u', 's', 'e', 'r', 's'}; !bytes.Equal(got, want) {
		t.Fatalf("CollectionMetaKey() = %v, want %v", got, want)
	}
	if got, want := DataKey("users", []byte("u1")), []byte{0x01, 0x05, 'u', 's', 'e', 'r', 's', 0x00, 'u', '1'}; !bytes.Equal(got, want) {
		t.Fatalf("DataKey() = %v, want %v", got, want)
	}
}

func TestNextPrefix(t *testing.T) {
	tests := []struct {
		in   []byte
		want []byte
		ok   bool
	}{
		{[]byte("abc"), []byte("abd"), true},
		{[]byte{'a', 'b', 0xff}, []byte{'a', 'c'}, true},
		{[]byte{0xff}, nil, false},
	}
	for _, tt := range tests {
		got, ok := NextPrefix(tt.in)
		if ok != tt.ok || !bytes.Equal(got, tt.want) {
			t.Fatalf("NextPrefix(%v) = %v %v, want %v %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestBounds(t *testing.T) {
	base := CollectionBase("users")
	lower, upper := CollectionBounds("users")
	if !bytes.Equal(lower, base) || bytes.Compare(upper, lower) <= 0 {
		t.Fatalf("bad collection bounds")
	}
	pl, pu := PrefixBounds("users", []byte("u1:"))
	if !bytes.HasPrefix(pl, base) || bytes.Compare(pu, pl) <= 0 {
		t.Fatalf("bad prefix bounds")
	}
	rl, ru := RangeBounds("users", []byte("b"), []byte("d"))
	if !bytes.Equal(rl, append(append([]byte(nil), base...), 'b')) ||
		!bytes.Equal(ru, append(append([]byte(nil), base...), 'd')) {
		t.Fatalf("bad range bounds")
	}
}
