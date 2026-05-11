package acceptancetest

import "os"

// openFlagWrite returns the flags for opening an existing file for writing without truncation.
func openFlagWrite() int {
	return os.O_WRONLY
}

// openFlagWriteTrunc returns the flags for opening an existing file and truncating its content.
func openFlagWriteTrunc() int {
	return os.O_WRONLY | os.O_TRUNC
}

// openFlagReadWrite returns the flags for read/write access on an existing file.
func openFlagReadWrite() int {
	return os.O_RDWR
}
