package openflag

import (
	"os"
	"syscall"
)

func WriteOp(flag int) bool {
	return flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0
}

func ReadWrite(flag int) bool {
	return Readable(flag) && Writable(flag)
}

func ReadOnly(flag int) bool {
	return flag&os.O_RDWR == 0 && flag&os.O_WRONLY == 0
}

func WriteOnly(flag int) bool {
	return flag&os.O_WRONLY != 0 && flag&os.O_RDWR == 0
}

func Readable(flag int) bool {
	return !WriteOnly(flag)
}

func Writable(flag int) bool {
	return flag&(os.O_WRONLY|os.O_RDWR) != 0
}
