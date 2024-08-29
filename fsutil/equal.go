package fsutil

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
)

type EqualReason string

const (
	EqualReasonModeMismatch             EqualReason = "mode mismatch"
	EqualReasonFileContentMismatch      EqualReason = "file content mismatch"
	EqualReasonDirectoryContentMismatch EqualReason = "directory content mismatch"
)

type EqualResult []EqualReport

func (r EqualResult) Equal() bool {
	return len(r) == 0
}

// Select filters r and returns a shallowly copied instance.
// The returned result only contains reports for which filterFn returns true.
func (r EqualResult) Select(filterFn func(r EqualReport) bool) EqualResult {
	var out EqualResult
	for _, rep := range r {
		if filterFn(rep) {
			out = append(out, rep)
		}
	}
	return out
}

type EqualReport struct {
	Reason EqualReason
	// Path is a path for a mismatching file or directory.
	Path string
	// Values for Reason of Path.
	// fs.FileMode for EqualReasonModeMismatch,
	// nil for EqualReasonFileContentMismatch
	// and []string describing names of dirents for EqualReasonDirectoryContentMismatch.
	DstVal, SrcVal any
}

// Equal compares dst and src and reports result.
//
// The comparison evaluates
//   - mode bits of dirents
//   - content of directory
//   - content of regular files
//
// Equal takes also CopyFsOption. Options work as if dst was dst of CopyFs.
// That is, for example, if CopyFsWithOverridePermission is set,
// Equal compares dst's file mode against returned value of chmodIf instead of src's.
//
// Note that mode bits of the root directory is ignored since often it is not controlled.
//
// Performance:
//   - Equal takes stat of every file in l and r.
//   - Also all dirents of directories are read.
//   - Files are entirely read
func Equal(dst, src fs.FS, opts ...CopyFsOption) (EqualResult, error) {
	var result EqualResult

	opt := newCopyFsOption(opts...)

	err := fs.WalkDir(dst, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && d.Type().Type() != 0 {
			switch opt.handleNonRegularFile {
			default: // nonRegularFileHandlingError
				return fmt.Errorf("%w: only directories and regular files are supported", ErrBadInput)
			case nonRegularFileHandlingIgnore:
				return nil
				// case nonRegularFileHandlingTrySymlink:
			}
		}

		dstFile, err := dst.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = dstFile.Close() }()

		dstInfo, err := dstFile.Stat()
		if err != nil {
			return err
		}

		srcFile, err := src.Open(path)
		if err != nil {
			// number of dirents are already checked. See below.
			// ErrNotExist is possible since there could be difference.
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		defer func() { _ = srcFile.Close() }()

		srcInfo, err := srcFile.Stat()
		if err != nil {
			return err
		}

		// no mode bits comparison for root dir.
		if path != "." {
			if report, eq := sameMode(dstInfo.Mode(), srcInfo.Mode(), path, opt); !eq {
				result = append(result, report)
			}
		}

		switch {
		case dstInfo.Mode().Type() != srcInfo.Mode().Type():
			// already reported by mode bit comparison.
		case dstInfo.IsDir():
			dstDirents, err := fs.ReadDir(dst, path)
			if err != nil {
				return err
			}

			srcDirents, err := fs.ReadDir(src, path)
			if err != nil {
				return err
			}

			if !sameNames(dstDirents, srcDirents) {
				result = append(result, EqualReport{
					Reason: EqualReasonDirectoryContentMismatch,
					Path:   path,
					DstVal: direntNames(dstDirents),
					SrcVal: direntNames(srcDirents),
				})
			}
		case dstInfo.Mode().IsRegular():
			equal, err := sameFile(dstFile, srcFile)
			if err != nil {
				return err
			}
			if !equal {
				result = append(result, EqualReport{
					Reason: EqualReasonFileContentMismatch,
					Path:   path,
					DstVal: nil,
					SrcVal: nil,
				})
			}
		}

		return nil
	})

	if err != nil {
		err = fmt.Errorf("fsutil.Equal: %w", err)
	}

	return result, err
}

func sameMode(dst, src fs.FileMode, path string, opt copyFsOption) (EqualReport, bool) {
	report := EqualReport{
		Reason: EqualReasonModeMismatch,
		Path:   path,
		DstVal: dst,
		SrcVal: src,
	}
	if dst.Type() != src.Type() {
		return report, false
	}

	if opt.chmodIf != nil {
		overridden, ok := opt.chmodIf(path)
		if ok {
			if dst.Perm() != overridden.Perm() {
				report.SrcVal = src.Type() | overridden.Perm()
				return report, false
			}
			return EqualReport{}, true
		}
	}

	if !opt.noChmod && dst != src {
		return report, false
	}

	return EqualReport{}, true
}

// assumption: dst and src are already sorted.
func sameNames(dst, src []fs.DirEntry) bool {
	if len(dst) != len(src) {
		return false
	}

	for i := range dst {
		if dst[i].Name() != src[i].Name() {
			return false
		}
	}

	return true
}

func direntNames(dirents []fs.DirEntry) []string {
	names := make([]string, len(dirents))
	for i, dirent := range dirents {
		names[i] = dirent.Name()
	}
	return names
}

func sameFile(r, l fs.File) (bool, error) {
	rs, err := r.Stat()
	if err != nil {
		return false, err
	}
	ls, err := l.Stat()
	if err != nil {
		return false, err
	}

	rSize := rs.Size()
	lSize := ls.Size()

	return sameReader(l, r, lSize, rSize)
}

func sameReader(l, r io.Reader, lSize, rSize int64) (same bool, err error) {
	if rSize != lSize {
		return false, nil
	}

	if rSize == 0 {
		return true, nil
	}

	size := int(rSize)

	bufRefL, bufRefR := getBuf(), getBuf()
	defer func() {
		putBuf(bufRefL)
		putBuf(bufRefR)
	}()

	bufL, bufR := *bufRefL, *bufRefR
	for size > 0 {
		if len(bufR) > size {
			bufR = bufR[:size]
			bufL = bufL[:size]
		}
		_, err := io.ReadFull(r, bufR)
		if err != nil {
			return false, err
		}
		n, err := io.ReadFull(l, bufL)
		if err != nil {
			return false, err
		}

		if !bytes.Equal(bufR, bufL) {
			return false, nil
		}
		size -= n
	}

	return true, nil
}
