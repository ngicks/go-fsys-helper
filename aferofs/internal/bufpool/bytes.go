package bufpool

import "sync"

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
