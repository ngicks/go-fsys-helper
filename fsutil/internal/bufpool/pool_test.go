package bufpool

import (
	"testing"
)

func TestBytesPool(t *testing.T) {
	// Test GetBytes and PutBytes
	buf1 := GetBytes()
	if buf1 == nil {
		t.Fatal("GetBytes returned nil")
	}
	if len(*buf1) != 64*1024 {
		t.Errorf("expected buffer size 64KB, got %d", len(*buf1))
	}

	PutBytes(buf1)

	// Get another buffer
	buf2 := GetBytes()
	if buf2 == nil {
		t.Error("GetBytes returned nil after put")
	}

	// Should not be cleared - bytesPool doesn't clear data
	// We just verify we got a buffer back

	PutBytes(buf2)
}

func TestBufPool(t *testing.T) {
	// Test GetBuf and PutBuf
	buf1 := GetBuf()
	if buf1 == nil {
		t.Error("GetBuf returned nil")
	}

	// Write to buffer
	buf1.WriteString("test content")
	if buf1.Len() == 0 {
		t.Error("buffer should contain data")
	}

	PutBuf(buf1)

	// Get another buffer
	buf2 := GetBuf()
	if buf2 == nil {
		t.Error("GetBuf returned nil after put")
	}

	// Should be reset
	if buf2.Len() != 0 {
		t.Error("buffer not reset after put/get cycle")
	}

	PutBuf(buf2)
}

func TestBufferPoolEdgeCases(t *testing.T) {
	// Test PutBytes with nil
	PutBytes(nil)

	// Test PutBytes with wrong size buffer
	wrongSizeBuf := make([]byte, 1024) // Not 64KB
	PutBytes(&wrongSizeBuf)

	// Test PutBuf with large capacity buffer
	largeBuf := GetBuf()
	// Grow the buffer beyond 64KB
	largeBuf.Grow(100 * 1024)
	largeBuf.WriteString("large content")

	// Should reject the large buffer
	PutBuf(largeBuf)

	// Verify we can still get a fresh buffer
	newBuf := GetBuf()
	if newBuf == nil {
		t.Error("GetBuf returned nil after putting large buffer")
	}
	if newBuf.Len() != 0 {
		t.Error("new buffer should be empty")
	}

	PutBuf(newBuf)
}
