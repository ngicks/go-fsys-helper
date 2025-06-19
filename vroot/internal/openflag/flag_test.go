package openflag

import (
	"os"
	"testing"
)

// Just ensuring this is correct for all platform supported.
func TestFlag(t *testing.T) {
	type testCase struct {
		name     string
		flag     int
		readable bool
		writeble bool
	}

	for _, tc := range []testCase{
		{
			flag:     os.O_RDONLY,
			readable: true,
		},
		{
			flag:     os.O_WRONLY,
			writeble: true,
		},
		{
			flag:     os.O_RDWR,
			readable: true,
			writeble: true,
		},
		{
			flag:     os.O_APPEND | os.O_RDONLY,
			readable: true,
		},
		{
			flag:     os.O_APPEND | os.O_WRONLY,
			writeble: true,
		},
		{
			flag:     os.O_APPEND | os.O_RDWR,
			readable: true,
			writeble: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.readable != Readable(tc.flag) {
				t.Errorf("readable wrong")
			}
			if tc.writeble != Writable(tc.flag) {
				t.Errorf("writable wrong")
			}
			if (tc.readable && !tc.writeble) != ReadOnly(tc.flag) {
				t.Errorf("read-only wrong")
			}
			if (!tc.readable && tc.writeble) != WriteOnly(tc.flag) {
				t.Errorf("write-only wrong")
			}
			if (tc.readable && tc.writeble) != ReadWrite(tc.flag) {
				t.Error("read-write wrong")
			}
		})
	}
}
