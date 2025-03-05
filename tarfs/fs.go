package tarfs

import "archive/tar"

type Fs struct {
	headers map[string]*header
}

type header struct {
	h                      *tar.Header
	headerStart, headerEnd int
	bodyStart, bodyEnd     int
	holes                  sparseHoles
}
