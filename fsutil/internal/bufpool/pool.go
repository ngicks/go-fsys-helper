package bufpool

import (
	"bytes"
	"sync"
)

const bufSize = 64 * 1024

var bytesPool = &sync.Pool{
	New: func() any {
		b := make([]byte, bufSize)
		return &b
	},
}

func GetBytes() *[]byte {
	return bytesPool.Get().(*[]byte)
}

func PutBytes(b *[]byte) {
	if b == nil || len(*b) != bufSize || cap(*b) != bufSize {
		// reject grown / shrunk
		return
	}
	bytesPool.Put(b)
}

var bufPool = &sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func GetBuf() *bytes.Buffer {
	return bufPool.Get().(*bytes.Buffer)
}

func PutBuf(b *bytes.Buffer) {
	if b.Cap() > 64*1024 {
		// See https://golang.org/issue/23199
		return
	}
	b.Reset()
	bufPool.Put(b)
}
