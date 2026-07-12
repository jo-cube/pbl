package app

import (
	"fmt"
	"testing"
)

func TestDeleteBloomRetainsAddedKeys(t *testing.T) {
	b, err := newDeleteBloom(1_000)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1_000; i++ {
		b.Add([]byte(fmt.Sprintf("key-%d", i)))
	}
	for i := 0; i < 1_000; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		if !b.MayContain(key) {
			t.Fatalf("filter lost %q", key)
		}
	}
}

func TestDeleteBloomSizeAndValidation(t *testing.T) {
	b, err := newDeleteBloom(100)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(b.bits)*8, 128; got != want {
		t.Fatalf("filter bytes = %d, want %d", got, want)
	}
	if _, err := newDeleteBloom(0); err == nil {
		t.Fatal("newDeleteBloom accepted zero keys")
	}
}
