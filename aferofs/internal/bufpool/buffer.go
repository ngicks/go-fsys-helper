package bufpool

import (
	"bytes"
	"sync"
)

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
