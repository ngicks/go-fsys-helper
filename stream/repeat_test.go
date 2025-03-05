package stream

import (
	"bytes"
	"testing"
)

func TestByteRepeater(t *testing.T) {
	r := NewByteRepeater('4')

	var buf [412]byte
	n, err := r.Read(buf[:])
	if err != nil {
		t.Fatalf("should be nil: %v", err)
	}
	if n != len(buf) {
		t.Fatal("wrong len")
	}
	if !bytes.Equal(buf[:], bytes.Repeat([]byte{'4'}, 412)) {
		t.Fatal("wrong read")
	}

	var buf2 [105]byte
	n, err = r.ReadAt(buf2[:], 21378921)
	if err != nil {
		t.Fatalf("should be nil: %v", err)
	}
	if n != len(buf2) {
		t.Fatal("wrong len")
	}
	if !bytes.Equal(buf2[:], bytes.Repeat([]byte{'4'}, 105)) {
		t.Fatal("wrong read")
	}
}
