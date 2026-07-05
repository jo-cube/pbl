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
