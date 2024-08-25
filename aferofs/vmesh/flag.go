package vmesh

import "os"

func flagPerm(flag int) int {
	switch flag & (os.O_RDONLY | os.O_WRONLY | os.O_RDWR) {
	case os.O_RDWR:
		return 0o6
	case os.O_WRONLY:
		return 0o2
	default: // case os.O_RDONLY:
		return 0o4
	}
}

func flagReadable(flag int) bool {
	switch flag & (os.O_RDONLY | os.O_WRONLY | os.O_RDWR) {
	case os.O_RDWR:
		return true
	case os.O_WRONLY:
		return false
	default: //	case os.O_RDONLY:
		return true
	}
}

func flagWritable(flag int) bool {
	switch flag & (os.O_RDONLY | os.O_WRONLY | os.O_RDWR) {
	case os.O_RDWR:
		return true
	case os.O_WRONLY:
		return true
	default: //	case os.O_RDONLY:
		return false
	}
}
