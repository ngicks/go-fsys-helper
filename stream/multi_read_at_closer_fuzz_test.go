package stream

import (
	"bytes"
	"encoding/hex"
	"io"
	"testing"

	"github.com/ngicks/go-fsys-helper/stream/internal/testhelper"
)

type ints interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

func min[T ints](t ...T) T {
	var min T
	if len(t) == 0 {
		return min
	}
	min = t[0]
	for _, tt := range t[0:] {
		if tt < min {
			min = tt
		}
	}
	return min
}

func FuzzMultiReadAtSeekCloser_Read(f *testing.F) {
	f.Add(6602, 23109, 7697586)
	f.Fuzz(func(t *testing.T, len1, len2, len3 int) {
		if min(len1, len2, len3) <= 0 { // too small len might cause this test longer.
			t.Skip()
		}
		t.Logf("seed: %d, %d, %d", len1, len2, len3)
		r := NewMultiReadAtSeekCloser(prepareSizedReader(randomBytes, []int{len1, len2, len3}, false))
		dst, err := io.ReadAll(r)
		testhelper.AssertNilInterface(t, err)
		testhelper.AssertTrue(t, len(randomBytes) == len(dst), "src len = %d, dst len = %d", len(randomBytes), len(dst))
		testhelper.AssertTrue(t, bytes.Equal(randomBytes, dst), "bytes.Equal returned false")
	})
}

func FuzzMultiReadAtSeekCloser_ReadAt(f *testing.F) {
	f.Add(1025, 7777, 30787, 0, 255, 6666)
	f.Fuzz(func(t *testing.T, len1, len2, len3, loc1, loc2, loc3 int) {
		if min(len1, len2, len3) <= 0 ||
			min(loc1, loc2, loc3) < 0 { // ReadAt / Seek loc cannot be less than 0
			t.Skip()
		}
		t.Logf("len: %d, %d, %d", len1, len2, len3)
		t.Logf("loc: %d, %d, %d", loc1, loc2, loc3)
		lens := []int{len1, len2, len3}
		locs := [...]int{loc1, loc2, loc3}

		buf := make([]byte, 1024)
		r := NewMultiReadAtSeekCloser(prepareSizedReader(randomBytes, lens, false))
		for _, loc := range locs {
			{
				n, err := r.ReadAt(buf, int64(loc))
				testhelper.AssertNilInterface(t, err)
				split := randomBytes[:]
				if loc >= len(randomBytes) {
					split = split[len(randomBytes):]
				} else {
					split = split[loc:]
				}
				if len(split) >= 1024 {
					split = split[:1024]
				}
				testhelper.AssertTrue(t, n == len(split), "left = %d, right = %d", n, len(split))
				testhelper.AssertTrue(t, bytes.Equal(buf, split), "left = %s, right = %s", hex.EncodeToString(buf), hex.EncodeToString(split))
			}
		}
	})
}

func FuzzMultiReadAtSeekCloser_Seek(f *testing.F) {
	f.Add(1025, 7777, 30787, 0, 255, 6666)
	f.Fuzz(func(t *testing.T, len1, len2, len3, loc1, loc2, loc3 int) {
		if min(len1, len2, len3) <= 0 || // len must be greater than 0
			min(loc1, loc2, loc3) < 0 { // ReadAt / Seek loc cannot be less than 0
			t.Skip()
		}
		t.Logf("len: %d, %d, %d", len1, len2, len3)
		t.Logf("loc: %d, %d, %d", loc1, loc2, loc3)
		lens := []int{len1, len2, len3}
		locs := [...]int{loc1, loc2, loc3}

		for _, loc := range locs {
			for i := 0; i < 6; i++ {
				off := int64(loc)
				if i >= 3 {
					off *= -1
				}

				t.Logf("off = %d", off)
				var whence int
				switch i % 3 {
				case 0:
					whence = io.SeekStart
					t.Logf("whence = SeekStart")
				case 1:
					whence = io.SeekCurrent
					t.Logf("whence = SeekCurrent")
				case 2:
					whence = io.SeekEnd
					t.Logf("whence = SeekEnd")
				}

				mult := NewMultiReadAtSeekCloser(prepareSizedReader(randomBytes, lens, false))
				org := bytes.NewReader(randomBytes)

				buf1, buf2 := make([]byte, len1), make([]byte, len1)

				_, rErr := io.ReadFull(mult, buf1)
				_, orgErr := io.ReadFull(org, buf1)
				testhelper.AssertTrue(t, rErr == nil && orgErr == nil || rErr != nil && orgErr != nil, "left = %v, right = %v", rErr, orgErr)

				n, err := mult.Seek(off, whence)
				bufN, bufErr := org.Seek(off, whence)
				if err != nil {
					testhelper.AssertNonNilInterface(t, bufErr)
					continue
				}
				testhelper.AssertEq(t, n, bufN)

				_, rErr = io.ReadFull(mult, buf1)
				_, orgErr = io.ReadFull(org, buf2)
				testhelper.AssertTrue(t, rErr == nil && orgErr == nil || rErr != nil && orgErr != nil, "left = %v, right = %v", rErr, orgErr)
				if rErr == io.EOF {
					testhelper.AssertErrorsIs(t, rErr, orgErr)
				}
				if rErr == nil {
					testhelper.AssertTrue(t, bytes.Equal(buf1, buf2), "bytes.Equal returned false")
				}
			}
		}
	})
}
